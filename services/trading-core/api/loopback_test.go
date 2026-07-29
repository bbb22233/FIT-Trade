package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"fit.trade/trading-core/auth"
)

func TestLoopbackHTTPAndWebSocketReauthAndRevocation(t *testing.T) {
	service := auth.NewService(auth.Config{Now: func() time.Time { return time.Now().UTC().Add(-14*time.Minute - time.Second) }})
	if _, err := addSyntheticUserForTest(service, "loopback@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	// Hold the replacement challenge after writer admission. This creates the
	// formerly flaky ordering deliberately: the client can receive one
	// REAUTH_REQUIRED after revocation only when its evidence proves that it
	// was admitted before the revocation fence.
	type admittedWrite struct {
		socket   *wsConnection
		evidence wsWriteEvidence
	}
	gateEntered := make(chan admittedWrite, 1)
	gateRelease := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(gateRelease) })
	var gateMu sync.Mutex
	gateEnabled := false
	server.beforeWSFrameWrite = func(socket *wsConnection, evidence wsWriteEvidence) {
		gateMu.Lock()
		enabled := gateEnabled
		gateMu.Unlock()
		if !enabled || evidence.opcode != 1 || evidence.nonce == "" {
			return
		}
		gateEntered <- admittedWrite{socket: socket, evidence: evidence}
		<-gateRelease
	}
	listener, err := server.Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config := loopbackTLSConfig(t)
	serveLoopback(t, listener, func() error { return server.ServeTLS(listener, config) })
	baseURL := "https://" + listener.Addr().String()
	client := loopbackTLSClient()

	credentials := loopbackLogin(t, client, baseURL)
	address := listener.Addr().String()
	connection, err := tls.Dial("tcp", address, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- test server certificate
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, _ = fmt.Fprintf(connection, "GET /v1/events HTTP/1.1\r\nHost: loopback.test\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: SGVybWVzLWxvb3BiYWNr\r\nSec-WebSocket-Protocol: %s\r\nAuthorization: Bearer %s\r\n\r\n", websocketProtocol, credentials.AccessToken)
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("websocket status = %d", response.StatusCode)
	}
	_, requiredRaw, err := readServerFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	var required wsReauthRequired
	if err := json.Unmarshal(requiredRaw, &required); err != nil {
		t.Fatal(err)
	}
	if required.Type != "REAUTH_REQUIRED" || required.Nonce == "" {
		t.Fatalf("unexpected challenge: %#v", required)
	}

	refreshed := loopbackRefresh(t, client, baseURL, credentials.RefreshToken)
	reauth, _ := json.Marshal(wsReauth{SchemaVersion: "fit.platform.websocket-control.v1", Type: "REAUTH", Nonce: required.Nonce, AccessToken: refreshed.AccessToken})
	gateMu.Lock()
	gateEnabled = true
	gateMu.Unlock()
	if err := writeClientFrame(connection, 1, reauth); err != nil {
		t.Fatal(err)
	}
	var admitted admittedWrite
	select {
	case admitted = <-gateEntered:
	case <-time.After(time.Second):
		t.Fatal("replacement reauth challenge was not admitted")
	}
	if err := service.RevokeSession(refreshed.Identity, refreshed.Identity.SessionID); err != nil {
		t.Fatal(err)
	}
	if admitted.evidence.sequence > admitted.socket.revocationFenceSequence() {
		t.Fatalf("replacement challenge sequence %d crossed revocation fence %d", admitted.evidence.sequence, admitted.socket.revocationFenceSequence())
	}
	releaseOnce.Do(func() { close(gateRelease) })
	// The held frame was admitted before revocation. Match both its server
	// issued-at value and nonce before consuming it; a new challenge here would
	// be a post-fence violation, not an allowed buffered control frame.
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	opcode, closeData, err := readServerFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 1 {
		t.Fatalf("buffered reauth = opcode %d data %x", opcode, closeData)
	}
	var buffered wsReauthRequired
	if err := json.Unmarshal(closeData, &buffered); err != nil {
		t.Fatal(err)
	}
	if buffered.Type != "REAUTH_REQUIRED" || buffered.Nonce != admitted.evidence.nonce || !buffered.IssuedAt.Equal(admitted.evidence.issuedAt) {
		t.Fatalf("unexpected post-fence control frame: %#v", buffered)
	}
	opcode, closeData, err = readServerFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 8 || len(closeData) < 2 || binary.BigEndian.Uint16(closeData[:2]) != 4401 {
		t.Fatalf("revocation close = opcode %d data %x", opcode, closeData)
	}
	fence := admitted.socket.revocationFenceSequence()
	if admitted.socket.writeSequenceSnapshot() != fence {
		t.Fatalf("new control or data frame admitted after revocation fence %d", fence)
	}
	// serveWebSocket closes the transport after its 4401 frame. One explicit
	// read proves no control or application frame follows that close.
	if _, _, err := readServerFrame(reader); err != io.EOF {
		t.Fatalf("frame after revocation close: %v", err)
	}
}

func TestRevocationFenceWinsOverQueuedWebSocketReauth(t *testing.T) {
	service := auth.NewService(auth.Config{Now: func() time.Time { return time.Now().UTC().Add(-14*time.Minute - time.Second) }})
	if _, err := addSyntheticUserForTest(service, "fence@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "fence@example.test", "test-password", "test-source")
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.Refresh(context.Background(), credentials.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	enteredVerify := make(chan struct{})
	releaseVerify := make(chan struct{})
	server.beforeWSReauthVerify = func() {
		close(enteredVerify)
		<-releaseVerify
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	socket := newWSConnection(serverSide)
	detach := server.ws.attach(credentials.Identity.SessionID, socket)
	defer detach()
	go server.serveWebSocket(socket, credentials.Identity, credentials.ExpiresAt)

	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	_, requiredRaw, err := readServerFrame(clientSide)
	if err != nil {
		t.Fatal(err)
	}
	var required wsReauthRequired
	if err := json.Unmarshal(requiredRaw, &required); err != nil {
		t.Fatal(err)
	}
	reauth, err := json.Marshal(wsReauth{SchemaVersion: "fit.platform.websocket-control.v1", Type: "REAUTH", Nonce: required.Nonce, AccessToken: refreshed.AccessToken})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeClientFrame(clientSide, 1, reauth); err != nil {
		t.Fatal(err)
	}
	select {
	case <-enteredVerify:
	case <-time.After(time.Second):
		t.Fatal("queued reauth was not held before verification")
	}

	revoked := make(chan error, 1)
	go func() { revoked <- service.RevokeSession(refreshed.Identity, refreshed.Identity.SessionID) }()
	select {
	case err := <-revoked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not synchronously fence the websocket")
	}
	if !socket.isInvalidated() {
		t.Fatal("successful revocation returned before the local websocket fence closed")
	}
	close(releaseVerify)

	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	opcode, closeData, err := readServerFrame(clientSide)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 8 || len(closeData) < 2 || binary.BigEndian.Uint16(closeData[:2]) != 4401 {
		t.Fatalf("fenced reauth close = opcode %d data %x", opcode, closeData)
	}
	// The fence is closed before the revocation call returns, so a queued
	// reauth cannot emit a later control or application frame.
	_ = clientSide.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := readServerFrame(clientSide); err == nil {
		t.Fatal("received a frame after revocation close")
	}
}

func TestPendingWebSocketReauthAllowsApplicationFramesUntilExpiry(t *testing.T) {
	service := auth.NewService(auth.Config{})
	if _, err := addSyntheticUserForTest(service, "pending-frame@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "pending-frame@example.test", "test-password", "source")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	socket := newWSConnection(serverSide)
	go server.serveWebSocket(socket, credentials.Identity, time.Now().UTC().Add(150*time.Millisecond))
	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	if opcode, _, err := readServerFrame(clientSide); err != nil || opcode != 1 {
		t.Fatalf("reauth required = opcode %d err %v", opcode, err)
	}
	if err := writeClientFrame(clientSide, 1, []byte(`{"event":"application-frame"}`)); err != nil {
		t.Fatal(err)
	}
	// No response means the ordinary application frame was admitted rather
	// than parsed as a control object or closed early.
	_ = clientSide.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := readServerFrame(clientSide); err == nil {
		t.Fatal("application frame unexpectedly produced a websocket close")
	}
	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	opcode, closeData, err := readServerFrame(clientSide)
	if err != nil || opcode != 8 || len(closeData) < 2 || binary.BigEndian.Uint16(closeData[:2]) != 4401 {
		t.Fatalf("expiry close = opcode %d data %x err %v", opcode, closeData, err)
	}
}

func TestPendingWebSocketReauthRejectsMalformedControl(t *testing.T) {
	service := auth.NewService(auth.Config{})
	if _, err := addSyntheticUserForTest(service, "malformed-reauth@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "malformed-reauth@example.test", "test-password", "source")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	go server.serveWebSocket(newWSConnection(serverSide), credentials.Identity, time.Now().UTC().Add(time.Second))
	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := readServerFrame(clientSide); err != nil {
		t.Fatal(err)
	}
	if err := writeClientFrame(clientSide, 1, []byte(`{"type":"REAUTH","nonce":"wrong"}`)); err != nil {
		t.Fatal(err)
	}
	opcode, closeData, err := readServerFrame(clientSide)
	if err != nil || opcode != 8 || len(closeData) < 2 || binary.BigEndian.Uint16(closeData[:2]) != 4401 {
		t.Fatalf("malformed reauth close = opcode %d data %x err %v", opcode, closeData, err)
	}
}

func TestWebSocketReauthUsesServerFrameReceiptTime(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		receiptAt  time.Duration
		wantActive bool
	}{
		{name: "expiry-minus-1ms", receiptAt: 99 * time.Millisecond, wantActive: true},
		{name: "expiry", receiptAt: 100 * time.Millisecond},
		{name: "expiry-plus-1ms", receiptAt: 101 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			var clockMu sync.RWMutex
			clock := base
			now := func() time.Time {
				clockMu.RLock()
				defer clockMu.RUnlock()
				return clock
			}
			setClock := func(value time.Time) {
				clockMu.Lock()
				clock = value
				clockMu.Unlock()
			}
			service := auth.NewService(auth.Config{Now: now})
			if _, err := addSyntheticUserForTest(service, "receipt-time-"+test.name+"@example.test", "test-password"); err != nil {
				t.Fatal(err)
			}
			credentials, err := service.Login(context.Background(), "receipt-time-"+test.name+"@example.test", "test-password", "fixture")
			if err != nil {
				t.Fatal(err)
			}
			setClock(base.Add(time.Second))
			refreshed, err := service.Refresh(context.Background(), credentials.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			setClock(base)
			server, err := NewLoopbackServer(service, bytes.Repeat([]byte{9}, 32))
			if err != nil {
				t.Fatal(err)
			}
			server.now = now
			received := make(chan time.Time, 1)
			server.afterWSFrameReceived = func(receivedAt time.Time) { received <- receivedAt }
			serverSide, clientSide := net.Pipe()
			defer clientSide.Close()
			socket := newWSConnection(serverSide)
			required := make(chan wsWriteEvidence, 1)
			release := make(chan struct{})
			socket.beforeWrite = func(_ *wsConnection, evidence wsWriteEvidence) {
				if evidence.nonce != "" {
					required <- evidence
					<-release
				}
			}
			go server.serveWebSocket(socket, credentials.Identity, base.Add(100*time.Millisecond))
			var evidence wsWriteEvidence
			select {
			case evidence = <-required:
			case <-time.After(time.Second):
				t.Fatal("REAUTH_REQUIRED was not admitted")
			}
			frame := mustJSON(wsReauth{SchemaVersion: "fit.platform.websocket-control.v1", Type: "REAUTH", Nonce: evidence.nonce, AccessToken: refreshed.AccessToken})
			setClock(base.Add(test.receiptAt))
			written := make(chan error, 1)
			go func() { written <- writeClientFrame(clientSide, 1, frame) }()
			select {
			case err := <-written:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("client frame was not received by websocket reader")
			}
			select {
			case receivedAt := <-received:
				if !receivedAt.Equal(base.Add(test.receiptAt)) {
					t.Fatalf("frame receipt time = %s, want %s", receivedAt, base.Add(test.receiptAt))
				}
			case <-time.After(time.Second):
				t.Fatal("websocket reader did not stamp receipt time")
			}
			// Queue processing deliberately happens after the old expiry. The
			// receipt timestamp above, not this consumer time, decides admission.
			setClock(base.Add(101 * time.Millisecond))
			close(release)
			_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
			if opcode, _, err := readServerFrame(clientSide); err != nil || opcode != 1 {
				t.Fatalf("REAUTH_REQUIRED = opcode %d err %v", opcode, err)
			}
			if test.wantActive {
				if err := writeClientFrame(clientSide, 9, []byte("receipt-proved")); err != nil {
					t.Fatal(err)
				}
				_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
				if opcode, data, err := readServerFrame(clientSide); err != nil || opcode != 10 || string(data) != "receipt-proved" {
					t.Fatalf("post-reauth pong = opcode %d data %q err %v", opcode, data, err)
				}
				return
			}
			_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
			opcode, data, err := readServerFrame(clientSide)
			if err != nil || opcode != 8 || len(data) < 2 || binary.BigEndian.Uint16(data[:2]) != 4401 {
				t.Fatalf("late REAUTH close = opcode %d data %x err %v", opcode, data, err)
			}
		})
	}
}

func TestWebSocketRevocationWinsQueuedPreExpiryReauth(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	var clockMu sync.RWMutex
	clock := base
	now := func() time.Time {
		clockMu.RLock()
		defer clockMu.RUnlock()
		return clock
	}
	setClock := func(value time.Time) {
		clockMu.Lock()
		clock = value
		clockMu.Unlock()
	}
	service := auth.NewService(auth.Config{Now: now})
	if _, err := addSyntheticUserForTest(service, "receipt-revocation@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "receipt-revocation@example.test", "test-password", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	setClock(base.Add(time.Second))
	refreshed, err := service.Refresh(context.Background(), credentials.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	setClock(base)
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{10}, 32))
	if err != nil {
		t.Fatal(err)
	}
	server.now = now
	received := make(chan time.Time, 1)
	server.afterWSFrameReceived = func(receivedAt time.Time) { received <- receivedAt }
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	socket := newWSConnection(serverSide)
	detach := server.ws.attach(credentials.Identity.SessionID, socket)
	defer detach()
	required := make(chan wsWriteEvidence, 1)
	releaseRequired := make(chan struct{})
	socket.beforeWrite = func(_ *wsConnection, evidence wsWriteEvidence) {
		if evidence.nonce != "" {
			required <- evidence
			<-releaseRequired
		}
	}
	enteredVerify := make(chan struct{})
	releaseVerify := make(chan struct{})
	server.beforeWSReauthVerify = func() {
		close(enteredVerify)
		<-releaseVerify
	}
	go server.serveWebSocket(socket, credentials.Identity, base.Add(100*time.Millisecond))
	var evidence wsWriteEvidence
	select {
	case evidence = <-required:
	case <-time.After(time.Second):
		t.Fatal("REAUTH_REQUIRED was not admitted")
	}
	setClock(base.Add(99 * time.Millisecond))
	frame := mustJSON(wsReauth{SchemaVersion: "fit.platform.websocket-control.v1", Type: "REAUTH", Nonce: evidence.nonce, AccessToken: refreshed.AccessToken})
	written := make(chan error, 1)
	go func() { written <- writeClientFrame(clientSide, 1, frame) }()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pre-expiry reauth was not received")
	}
	select {
	case receivedAt := <-received:
		if !receivedAt.Equal(base.Add(99 * time.Millisecond)) {
			t.Fatalf("frame receipt time = %s, want pre-expiry", receivedAt)
		}
	case <-time.After(time.Second):
		t.Fatal("websocket reader did not stamp queued reauth")
	}
	setClock(base.Add(101 * time.Millisecond))
	close(releaseRequired)
	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	if opcode, _, err := readServerFrame(clientSide); err != nil || opcode != 1 {
		t.Fatalf("REAUTH_REQUIRED = opcode %d err %v", opcode, err)
	}
	select {
	case <-enteredVerify:
	case <-time.After(time.Second):
		t.Fatal("queued pre-expiry reauth did not reach verification")
	}
	if err := service.RevokeSession(refreshed.Identity, refreshed.Identity.SessionID); err != nil {
		t.Fatal(err)
	}
	if !socket.isInvalidated() {
		t.Fatal("revocation did not synchronously fence queued reauth")
	}
	close(releaseVerify)
	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	opcode, data, err := readServerFrame(clientSide)
	if err != nil || opcode != 8 || len(data) < 2 || binary.BigEndian.Uint16(data[:2]) != 4401 {
		t.Fatalf("revocation close = opcode %d data %x err %v", opcode, data, err)
	}
}

func TestRefreshRejectsPlainHTTP(t *testing.T) {
	service := auth.NewService(auth.Config{})
	if _, err := addSyntheticUserForTest(service, "tls@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "tls@example.test", "test-password", "test-source")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := server.Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveLoopback(t, listener, func() error { return server.Serve(listener) })
	baseURL := "http://" + listener.Addr().String()
	body, err := json.Marshal(map[string]string{"refresh_token": credentials.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/auth/refresh", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("plain refresh status = %d, want %d", response.StatusCode, http.StatusUpgradeRequired)
	}
}

func TestServeAndServeTLSUseRealIPv4AndIPv6LoopbackSockets(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(address, func(t *testing.T) {
			service := auth.NewService(auth.Config{})
			server, err := NewLoopbackServer(service, bytes.Repeat([]byte{7}, 32))
			if err != nil {
				t.Fatal(err)
			}
			listener, err := server.Listen(context.Background(), address)
			if err != nil {
				if address == "[::1]:0" {
					t.Skipf("IPv6 loopback is unavailable: %v", err)
				}
				t.Fatal(err)
			}
			serveLoopback(t, listener, func() error { return server.Serve(listener) })
			response, err := http.Get("http://" + listener.Addr().String() + "/health/live")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("Serve status = %d, want %d", response.StatusCode, http.StatusOK)
			}

			tlsServer, err := NewLoopbackServer(service, bytes.Repeat([]byte{8}, 32))
			if err != nil {
				t.Fatal(err)
			}
			tlsListener, err := tlsServer.Listen(context.Background(), address)
			if err != nil {
				t.Fatal(err)
			}
			config := loopbackTLSConfig(t)
			serveLoopback(t, tlsListener, func() error { return tlsServer.ServeTLS(tlsListener, config) })
			response, err = loopbackTLSClient().Get("https://" + tlsListener.Addr().String() + "/health/live")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("ServeTLS status = %d, want %d", response.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestHandlerRejectsPublicRemotePeer(t *testing.T) {
	server, err := NewLoopbackServer(auth.NewService(auth.Config{}), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://loopback.test/health/live", nil)
	request.RemoteAddr = "203.0.113.10:4444"
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("public remote peer status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func serveLoopback(t *testing.T, listener net.Listener, serve func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- serve() }()
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
		if err := <-done; err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("serve loopback: %v", err)
		}
	})
}

func loopbackTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	fixture := httptest.NewUnstartedServer(nil)
	fixture.StartTLS()
	config := fixture.TLS.Clone()
	fixture.Close()
	return config
}

func loopbackTLSClient() *http.Client {
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} // #nosec G402 -- local test certificate
}

func loopbackLogin(t *testing.T, client *http.Client, baseURL string) auth.Credentials {
	t.Helper()
	return loopbackCredentials(t, client, baseURL, "/v1/auth/login", map[string]string{"identifier": "loopback@example.test", "password_utf8": "test-password"})
}
func loopbackRefresh(t *testing.T, client *http.Client, baseURL, token string) auth.Credentials {
	t.Helper()
	return loopbackCredentials(t, client, baseURL, "/v1/auth/refresh", map[string]string{"refresh_token": token})
}
func loopbackCredentials(t *testing.T, client *http.Client, baseURL, path string, input map[string]string) auth.Credentials {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("credential status %d: %s", response.StatusCode, raw)
	}
	var credentials auth.Credentials
	if err := json.NewDecoder(response.Body).Decode(&credentials); err != nil {
		t.Fatal(err)
	}
	return credentials
}

func readServerFrame(reader io.Reader) (byte, []byte, error) {
	var head [2]byte
	if _, err := io.ReadFull(reader, head[:]); err != nil {
		return 0, nil, err
	}
	size := uint64(head[1] & 0x7f)
	if size == 126 {
		var b [2]byte
		if _, err := io.ReadFull(reader, b[:]); err != nil {
			return 0, nil, err
		}
		size = uint64(binary.BigEndian.Uint16(b[:]))
	}
	if size > 65535 {
		return 0, nil, fmt.Errorf("frame too large")
	}
	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	return head[0] & 0x0f, data, err
}
func writeClientFrame(connection net.Conn, opcode byte, data []byte) error {
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	head := []byte{0x80 | opcode}
	switch {
	case len(data) < 126:
		head = append(head, 0x80|byte(len(data)))
	case len(data) <= 65535:
		head = append(head, 0x80|126, byte(len(data)>>8), byte(len(data)))
	default:
		return fmt.Errorf("frame too large")
	}
	masked := append([]byte(nil), data...)
	for i := range masked {
		masked[i] ^= mask[i%4]
	}
	_, err := connection.Write(append(append(head, mask...), masked...))
	return err
}
