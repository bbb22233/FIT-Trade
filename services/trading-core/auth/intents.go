package auth

import (
	"sync"
	"time"
)

// SecurityScope is deliberately not an ownership scope.  AUTH_SECURITY
// records can carry either a pseudonymous source key or a resolved user, but
// never a trading-account identifier.
type SecurityScope struct {
	sourceKey string
	userID    string
}

func SourceSecurityScope(sourceKey string) SecurityScope { return SecurityScope{sourceKey: sourceKey} }
func UserSecurityScope(sourceKey, userID string) SecurityScope {
	return SecurityScope{sourceKey: sourceKey, userID: userID}
}
func (v SecurityScope) SourceKey() string { return v.sourceKey }
func (v SecurityScope) UserID() string    { return v.userID }

// AuditIntent is an immutable control-plane fact.  P1-004 maps these facts to
// durable AuditEvents; this in-memory development service neither persists nor
// delivers them.
type AuditIntent struct {
	kind       string
	scope      SecurityScope
	occurredAt time.Time
}

func (v AuditIntent) Kind() string          { return v.kind }
func (v AuditIntent) Scope() SecurityScope  { return v.scope }
func (v AuditIntent) OccurredAt() time.Time { return v.occurredAt }

// NotificationIntent has no destination, free-form text, or delivery
// capability.  It is an internal persistence intent only.
type NotificationIntent struct {
	kind       string
	severity   string
	scope      SecurityScope
	occurredAt time.Time
}

func (v NotificationIntent) Kind() string          { return v.kind }
func (v NotificationIntent) Severity() string      { return v.severity }
func (v NotificationIntent) Scope() SecurityScope  { return v.scope }
func (v NotificationIntent) OccurredAt() time.Time { return v.occurredAt }

// IntentSink is the minimal transaction-facing port.  It deliberately cannot
// read identities or deliver notifications outside the service boundary.
type IntentSink interface {
	Record(AuditIntent, NotificationIntent)
}

type discardIntentSink struct{}

func (discardIntentSink) Record(AuditIntent, NotificationIntent) {}

// MemoryIntentSink is test-only in-memory evidence for the development
// service.  It is not a persistence implementation.
type MemoryIntentSink struct {
	mu            sync.RWMutex
	audits        []AuditIntent
	notifications []NotificationIntent
}

func NewMemoryIntentSink() *MemoryIntentSink { return &MemoryIntentSink{} }
func (s *MemoryIntentSink) Record(a AuditIntent, n NotificationIntent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, a)
	s.notifications = append(s.notifications, n)
}
func (s *MemoryIntentSink) Audits() []AuditIntent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]AuditIntent(nil), s.audits...)
}
func (s *MemoryIntentSink) Notifications() []NotificationIntent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]NotificationIntent(nil), s.notifications...)
}
