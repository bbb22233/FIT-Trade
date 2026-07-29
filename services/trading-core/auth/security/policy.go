package security

import "context"

// TransportLimits is sealed and admits only the frozen development profile.
type TransportLimits struct {
	version                                                                                        string
	maxRequestTarget, maxHeaders, maxUnauthBody, maxAuthBody, maxCredentialBody, maxResponse       int
	maxWSData, maxWSApplicationControl, maxWSProtocolControl                                       int
	httpHeaderReadMS, httpReadMS, httpWriteMS, httpIdleMS                                          int64
	wsReadMS, wsWriteMS, wsPingMS, wsIssueBeforeExpiryMS, wsResponseMS                             int64
	maxHTTPConnections, maxWSConnections, maxInflight, maxHandlersPerConnection, maxAdmissionQueue int
	shutdownGraceMS, forceCloseMS, cleanupMS                                                       int64
}

func DevelopmentTransportLimits() TransportLimits {
	return TransportLimits{"fit.transport-limits.dev.v1", 8192, 16384, 16384, 65536, 32768, 65536, 65536, 4096, 125, 5000, 10000, 15000, 60000, 75000, 10000, 30000, 60000, 60000, 256, 128, 256, 2, 128, 10000, 10000, 15000}
}
func (v TransportLimits) Validate() error {
	if v != DevelopmentTransportLimits() {
		return ErrInvalidDTO
	}
	return nil
}
func (v TransportLimits) AdmitRequest(targetBytes, headerBytes, bodyBytes int, authenticated, credential bool) error {
	if v.Validate() != nil || targetBytes < 0 || headerBytes < 0 || bodyBytes < 0 || targetBytes > v.maxRequestTarget || headerBytes > v.maxHeaders {
		return ErrInvalidDTO
	}
	limit := v.maxUnauthBody
	if authenticated {
		limit = v.maxAuthBody
	}
	if credential {
		limit = v.maxCredentialBody
	}
	if bodyBytes > limit {
		return ErrInvalidDTO
	}
	return nil
}
func (v TransportLimits) AdmitWebSocket(dataBytes, applicationControlBytes, protocolControlBytes int) error {
	if v.Validate() != nil || dataBytes < 0 || applicationControlBytes < 0 || protocolControlBytes < 0 || dataBytes > v.maxWSData || applicationControlBytes > v.maxWSApplicationControl || protocolControlBytes > v.maxWSProtocolControl {
		return ErrInvalidDTO
	}
	return nil
}

// WSReauth is a durable server-state port. Frame delivery is never authority.
type WSReauth interface {
	IssueReauth(context.Context, WSBinding, ClockSnapshot) (WSChallenge, error)
	ConsumeReauth(context.Context, WSBinding, WSChallenge, ClockSnapshot) (WSBinding, error)
	CloseRevokedWithin(context.Context, SessionID, int64) error
}
type WSBinding struct {
	session           SessionID
	device            DeviceID
	accessExpiresAtMS int64
	issuedAtMS        int64
	revoked           bool
}

func NewWSBinding(session SessionID, device DeviceID, issuedAtMS, accessExpiresAtMS int64) (WSBinding, error) {
	if !session.value.valid() || !device.value.valid() || issuedAtMS < 0 || accessExpiresAtMS < issuedAtMS {
		return WSBinding{}, ErrInvalidDTO
	}
	return WSBinding{session: session, device: device, issuedAtMS: issuedAtMS, accessExpiresAtMS: accessExpiresAtMS}, nil
}

type WSChallenge struct {
	nonce                  Nonce96
	issuedAtMS, deadlineMS int64
}

func ReauthChallenge(binding WSBinding, snapshot ClockSnapshot, limits TransportLimits, nonce Nonce96) (WSChallenge, error) {
	if limits.Validate() != nil || binding.revoked || snapshot.utcNowMS >= binding.accessExpiresAtMS {
		return WSChallenge{}, ErrInvalidDTO
	}
	issueAt := binding.accessExpiresAtMS - limits.wsIssueBeforeExpiryMS
	if snapshot.utcNowMS < issueAt {
		return WSChallenge{}, ErrInvalidDTO
	}
	deadline := snapshot.utcNowMS + limits.wsResponseMS
	if deadline > binding.accessExpiresAtMS {
		deadline = binding.accessExpiresAtMS
	}
	return WSChallenge{nonce: nonce, issuedAtMS: snapshot.utcNowMS, deadlineMS: deadline}, nil
}
func ReauthExpired(challenge WSChallenge, snapshot ClockSnapshot) bool {
	return snapshot.utcNowMS >= challenge.deadlineMS
}

type RefreshFamily struct {
	deadlineMS int64
	revoked    bool
}
type RefreshTokenState uint8

const (
	RefreshActive RefreshTokenState = iota + 1
	RefreshRotated
	RefreshExpired
)

type RefreshDecision uint8

const (
	RefreshRotate RefreshDecision = iota + 1
	RefreshReuseRevoke
	RefreshFamilyExpired
	RefreshIndividualExpired
)

func EvaluateRefresh(family RefreshFamily, token RefreshTokenState, individualExpiresAtMS int64, snapshot ClockSnapshot) RefreshDecision {
	if snapshot.utcNowMS >= family.deadlineMS || family.revoked {
		return RefreshFamilyExpired
	}
	if token == RefreshRotated {
		return RefreshReuseRevoke
	}
	if snapshot.utcNowMS >= individualExpiresAtMS || token == RefreshExpired {
		return RefreshIndividualExpired
	}
	return RefreshRotate
}

// ActiveSessionDevice rejects a revocation at every HTTP/WS/internal ingress.
func ActiveSessionDevice(authority AuthorityScope, sessionRevoked, deviceRevoked bool) bool {
	return authority.Kind() == AuthorityOwner && authority.SessionID().value.valid() && authority.DeviceID().value.valid() && !sessionRevoked && !deviceRevoked
}
