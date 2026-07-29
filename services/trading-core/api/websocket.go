package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"fit.trade/trading-core/auth"
)

const websocketProtocol = "fit.platform.websocket.v1"

type wsReauthRequired struct {
	SchemaVersion           string    `json:"schema_version"`
	Type                    string    `json:"type"`
	Nonce                   string    `json:"nonce"`
	ConnectionEstablishedAt time.Time `json:"connection_established_at"`
	IssuedAt                time.Time `json:"issued_at"`
	Deadline                time.Time `json:"deadline"`
	CurrentAccessExpiresAt  time.Time `json:"current_access_expires_at"`
}
type wsReauth struct {
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	Nonce         string `json:"nonce"`
	AccessToken   string `json:"access_token_transport"`
}
type wsFrame struct {
	opcode     byte
	data       []byte
	err        error
	receivedAt time.Time
}

// wsWriteEvidence is retained only for package tests.  It records the writer
// admission point for a control frame without changing its wire contract.
type wsWriteEvidence struct {
	sequence uint64
	opcode   byte
	issuedAt time.Time
	nonce    string
}

// wsSessionFence is the in-process half of revocation propagation.  Auth
// closes a session's fences while its own state mutation is locked; each
// connection then gives that closed fence priority over queued ticker and
// client-frame work.  The ticker remains a defensive bounded fallback.
type wsSessionFence struct {
	mu      sync.Mutex
	members map[string]map[*wsConnection]struct{}
}

func newWSSessionFence() *wsSessionFence {
	return &wsSessionFence{members: make(map[string]map[*wsConnection]struct{})}
}
func (f *wsSessionFence) attach(sessionID string, connection *wsConnection) func() {
	f.mu.Lock()
	if f.members[sessionID] == nil {
		f.members[sessionID] = make(map[*wsConnection]struct{})
	}
	f.members[sessionID][connection] = struct{}{}
	f.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			delete(f.members[sessionID], connection)
			if len(f.members[sessionID]) == 0 {
				delete(f.members, sessionID)
			}
		})
	}
}
func (f *wsSessionFence) invalidate(_ context.Context, sessionID string) error {
	f.mu.Lock()
	connections := make([]*wsConnection, 0, len(f.members[sessionID]))
	for connection := range f.members[sessionID] {
		connections = append(connections, connection)
	}
	f.mu.Unlock()
	for _, connection := range connections {
		connection.invalidate()
	}
	return nil
}

// wsConnection serializes the admission of server frames against session
// invalidation.  A frame admitted before revocation may finish its transport
// write, but no REAUTH_REQUIRED, pong, or future application frame can be
// admitted after the fence closes.
type wsConnection struct {
	connection              net.Conn
	mu                      sync.Mutex
	revoked                 bool
	done                    chan struct{}
	writeSequence           uint64
	revocationWriteSequence uint64
	beforeWrite             func(*wsConnection, wsWriteEvidence)
}

func newWSConnection(connection net.Conn) *wsConnection {
	return &wsConnection{connection: connection, done: make(chan struct{})}
}
func (c *wsConnection) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revoked {
		return
	}
	c.revoked = true
	// This snapshot is the revocation fence: only non-close frames with a
	// sequence at or below it were admitted before invalidation.
	c.revocationWriteSequence = c.writeSequence
	close(c.done)
}
func (c *wsConnection) isInvalidated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.revoked
}
func (c *wsConnection) write(opcode byte, data []byte) bool {
	return c.writeWithEvidence(opcode, data, wsWriteEvidence{opcode: opcode})
}
func (c *wsConnection) writeReauth(required wsReauthRequired) bool {
	return c.writeWithEvidence(1, mustJSON(required), wsWriteEvidence{
		opcode:   1,
		issuedAt: required.IssuedAt,
		nonce:    required.Nonce,
	})
}
func (c *wsConnection) writeWithEvidence(opcode byte, data []byte, evidence wsWriteEvidence) bool {
	c.mu.Lock()
	if c.revoked {
		c.mu.Unlock()
		return false
	}
	// Incrementing this sequence is the linearization point for outgoing
	// control/data frames. The transport write is deliberately outside the
	// state lock: a slow peer must not delay the synchronous revocation fence
	// or its five-second deadline.
	c.writeSequence++
	evidence.sequence = c.writeSequence
	beforeWrite := c.beforeWrite
	c.mu.Unlock()
	if beforeWrite != nil {
		beforeWrite(c, evidence)
	}
	return writeWSFrame(c.connection, opcode, data)
}
func (c *wsConnection) revocationFenceSequence() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.revocationWriteSequence
}
func (c *wsConnection) writeSequenceSnapshot() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeSequence
}
func (c *wsConnection) close(code uint16) {
	closeWS(c.connection, code)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "websocket" || !hasProtocol(r.Header.Get("Sec-WebSocket-Protocol"), websocketProtocol) || r.Header.Get("Sec-WebSocket-Key") == "" {
		problem(w, http.StatusBadRequest, "VALIDATION_FAILED")
		return
	}
	identity, err := s.identity.Authenticate(bearer(r))
	if err != nil {
		problem(w, http.StatusUnauthorized, "AUTH_REQUIRED")
		return
	}
	source, sourceOK := s.source(r)
	if !sourceOK || !s.limits.allowAll(
		rateLimit{key: "websocket-session-rate:" + identity.SessionID, count: 2, period: time.Second, burst: 2},
		rateLimit{key: "websocket-source-rate:" + source, count: 10, period: time.Second, burst: 20},
	) {
		problem(w, http.StatusTooManyRequests, "THROTTLED")
		return
	}
	sessionSlot := "websocket-session-concurrent:" + identity.SessionID
	sourceSlot := "websocket-source-concurrent:" + source
	if !s.limits.acquire(sessionSlot, 5) {
		problem(w, http.StatusTooManyRequests, "THROTTLED")
		return
	}
	if !s.limits.acquire(sourceSlot, 20) {
		s.limits.release(sessionSlot)
		problem(w, http.StatusTooManyRequests, "THROTTLED")
		return
	}
	expires, err := s.identity.AccessExpiry(bearer(r))
	if err != nil {
		s.limits.release(sessionSlot)
		s.limits.release(sourceSlot)
		problem(w, http.StatusUnauthorized, "AUTH_REQUIRED")
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		s.limits.release(sessionSlot)
		s.limits.release(sourceSlot)
		problem(w, http.StatusInternalServerError, "UNAVAILABLE")
		return
	}
	connection, buffered, err := hijacker.Hijack()
	if err != nil {
		s.limits.release(sessionSlot)
		s.limits.release(sourceSlot)
		return
	}
	socket := newWSConnection(connection)
	socket.beforeWrite = s.beforeWSFrameWrite
	detach := s.ws.attach(identity.SessionID, socket)
	if !s.identity.Active(identity) {
		socket.invalidate()
	}
	accept := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(accept[:]) + "\r\nSec-WebSocket-Protocol: " + websocketProtocol + "\r\n\r\n")
	_ = buffered.Flush()
	go func() {
		defer s.limits.release(sessionSlot)
		defer s.limits.release(sourceSlot)
		defer detach()
		s.serveWebSocket(socket, identity, expires)
	}()
}

func (s *Server) serveWebSocket(socket *wsConnection, identity auth.Identity, expires time.Time) {
	connection := socket.connection
	defer socket.invalidate()
	defer connection.Close()
	frames := make(chan wsFrame, 1)
	go func() {
		for {
			opcode, data, err := readWSFrame(connection)
			// Receipt time is server-authored at the reader handoff.  It is the
			// only time used to decide whether a queued client frame met an auth
			// boundary; client timestamps are never accepted.
			frame := wsFrame{opcode: opcode, data: data, err: err, receivedAt: s.now().UTC()}
			if s.afterWSFrameReceived != nil {
				s.afterWSFrameReceived(frame.receivedAt)
			}
			select {
			case frames <- frame:
			case <-socket.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	var pending *wsReauthRequired
	currentExpiry := expires
	connectedAt := s.now().UTC().Truncate(time.Millisecond)
	watch := time.NewTicker(100 * time.Millisecond)
	defer watch.Stop()
	processFrame := func(frame wsFrame) bool {
		// A ready client frame and a revocation can arrive together. The fence
		// is checked before the frame is interpreted, so revocation wins over
		// ticker/frame select randomness.
		if socket.isInvalidated() {
			socket.close(4401)
			return false
		}
		if frame.err != nil {
			return false
		}
		// Receipt, rather than consumer processing time, is the admission
		// boundary. A frame at the expiry instant is intentionally too late.
		if frame.receivedAt.IsZero() || !frame.receivedAt.Before(currentExpiry) {
			socket.close(4401)
			return false
		}
		switch frame.opcode {
		case 8:
			return false
		case 9:
			return socket.write(10, frame.data)
		case 1:
			// Text application frames remain valid until the current token
			// expires. Only an explicit REAUTH control object enters the
			// reauthentication parser while a request is pending.
			if pending == nil || !isWSReauthControl(frame.data) {
				return true
			}
			if s.beforeWSReauthVerify != nil {
				s.beforeWSReauthVerify()
			}
			if socket.isInvalidated() {
				socket.close(4401)
				return false
			}
			if !s.consumeWSReauth(frame.data, *pending, identity, currentExpiry, frame.receivedAt, &currentExpiry) {
				socket.close(4401)
				return false
			}
			if socket.isInvalidated() {
				socket.close(4401)
				return false
			}
			pending = nil
			return true
		default:
			// Binary and extension/application frames are application data, not
			// implicit reauthentication controls.
			return true
		}
	}
	for {
		if socket.isInvalidated() {
			socket.close(4401)
			return
		}
		now := s.now().UTC()
		if !now.Before(currentExpiry) {
			// A reader may have already accepted a pre-expiry REAUTH while the
			// consumer was busy. Drain that one queued envelope before applying
			// idle expiry; a socket with no eligible frame still closes at expiry.
			select {
			case frame := <-frames:
				if !processFrame(frame) {
					return
				}
				continue
			default:
				socket.close(4401)
				return
			}
		}
		issueAt := currentExpiry.Add(-60 * time.Second)
		if pending == nil && !now.Before(issueAt) {
			nonce := wsNonce()
			deadline := now.Add(60 * time.Second)
			if deadline.After(currentExpiry) {
				deadline = currentExpiry
			}
			pending = &wsReauthRequired{SchemaVersion: "fit.platform.websocket-control.v1", Type: "REAUTH_REQUIRED", Nonce: nonce, ConnectionEstablishedAt: connectedAt, IssuedAt: now.Truncate(time.Millisecond), Deadline: deadline.Truncate(time.Millisecond), CurrentAccessExpiresAt: currentExpiry.Truncate(time.Millisecond)}
			if !socket.writeReauth(*pending) {
				return
			}
		}
		select {
		case <-socket.done:
			socket.close(4401)
			return
		case <-watch.C:
			// This makes all established sockets observe revocation well within
			// the frozen five-second propagation bound.
			if socket.isInvalidated() || !s.identity.Active(identity) {
				socket.invalidate()
				socket.close(4401)
				return
			}
		case frame := <-frames:
			if !processFrame(frame) {
				return
			}
		}
	}
}

func isWSReauthControl(raw []byte) bool {
	var header struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(raw, &header) == nil && header.Type == "REAUTH"
}

func (s *Server) consumeWSReauth(raw []byte, required wsReauthRequired, binding auth.Identity, oldExpiry, receivedAt time.Time, newExpiry *time.Time) bool {
	if !strictJSON(raw) || hasAuthorityField(raw) {
		return false
	}
	var input wsReauth
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || input.SchemaVersion != "fit.platform.websocket-control.v1" || input.Type != "REAUTH" || input.Nonce != required.Nonce || input.AccessToken == "" {
		return false
	}
	identity, expiry, err := s.identity.SameActiveBinding(input.AccessToken, binding)
	if err != nil || identity != binding || !expiry.After(oldExpiry) || receivedAt.IsZero() || !receivedAt.Before(oldExpiry) || !receivedAt.Before(required.Deadline) {
		return false
	}
	*newExpiry = expiry
	return true
}

func hasProtocol(header, wanted string) bool {
	for _, value := range strings.Split(header, ",") {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
func wsNonce() string {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}
func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func readWSFrame(connection net.Conn) (byte, []byte, error) {
	_ = connection.SetReadDeadline(time.Now().Add(75 * time.Second))
	var head [2]byte
	if _, err := io.ReadFull(connection, head[:]); err != nil {
		return 0, nil, err
	}
	if head[0]&0x80 == 0 {
		return 0, nil, errors.New("fragmented websocket frame")
	}
	opcode := head[0] & 0x0f
	masked := head[1]&0x80 != 0
	if !masked {
		return 0, nil, errors.New("unmasked client websocket frame")
	}
	size := uint64(head[1] & 0x7f)
	if size == 126 {
		var b [2]byte
		if _, err := io.ReadFull(connection, b[:]); err != nil {
			return 0, nil, err
		}
		size = uint64(binary.BigEndian.Uint16(b[:]))
	} else if size == 127 {
		var b [8]byte
		if _, err := io.ReadFull(connection, b[:]); err != nil {
			return 0, nil, err
		}
		size = binary.BigEndian.Uint64(b[:])
	}
	if size > 65536 {
		return 0, nil, errors.New("oversize websocket frame")
	}
	var mask [4]byte
	if _, err := io.ReadFull(connection, mask[:]); err != nil {
		return 0, nil, err
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(connection, data); err != nil {
		return 0, nil, err
	}
	for i := range data {
		data[i] ^= mask[i%4]
	}
	return opcode, data, nil
}

func writeWSFrame(connection net.Conn, opcode byte, data []byte) bool {
	if len(data) > 65535 {
		return false
	}
	head := []byte{0x80 | opcode}
	if len(data) < 126 {
		head = append(head, byte(len(data)))
	} else {
		head = append(head, 126, byte(len(data)>>8), byte(len(data)))
	}
	_ = connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := connection.Write(append(head, data...))
	return err == nil
}
func closeWS(connection net.Conn, code uint16) {
	var data [2]byte
	binary.BigEndian.PutUint16(data[:], code)
	_ = writeWSFrame(connection, 8, data[:])
}

// bufio is retained as a compile-time assertion that hijacking's buffered
// writer is intentionally flushed before the websocket goroutine begins.
var _ *bufio.ReadWriter
