package domain

import (
	"errors"
	"sync"
)

var ErrIdentifierConflict = errors.New("identifier already bound to different result")
var ErrUnknownIdentifierKind = errors.New("unknown identifier kind")

type IdentifierKind string

const (
	RequestID     IdentifierKind = "request"
	AttemptID     IdentifierKind = "attempt"
	EventID       IdentifierKind = "event"
	ClientOrderID IdentifierKind = "client_order"
)

type Record struct {
	Kind   IdentifierKind
	ID     string
	Result string
}

// IdempotencyStore models the unique constraints required of durable storage.
// Snapshot and Restore make crash/restart behavior explicit without adding a
// database or network dependency to this domain module.
type IdempotencyStore struct {
	mu      sync.Mutex
	records map[IdentifierKind]map[string]string
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{records: make(map[IdentifierKind]map[string]string)}
}

func (s *IdempotencyStore) Apply(kind IdentifierKind, id, result string) (existing string, duplicate bool, err error) {
	if !knownIdentifierKind(kind) {
		return "", false, ErrUnknownIdentifierKind
	}
	if id == "" || result == "" {
		return "", false, errors.New("identifier and result are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records[kind] == nil {
		s.records[kind] = make(map[string]string)
	}
	if prior, ok := s.records[kind][id]; ok {
		if prior != result {
			return prior, true, ErrIdentifierConflict
		}
		return prior, true, nil
	}
	s.records[kind][id] = result
	return result, false, nil
}

func knownIdentifierKind(kind IdentifierKind) bool {
	switch kind {
	case RequestID, AttemptID, EventID, ClientOrderID:
		return true
	default:
		return false
	}
}

func (s *IdempotencyStore) Snapshot() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Record
	for kind, records := range s.records {
		for id, result := range records {
			out = append(out, Record{Kind: kind, ID: id, Result: result})
		}
	}
	return out
}

func RestoreIdempotencyStore(records []Record) (*IdempotencyStore, error) {
	store := NewIdempotencyStore()
	for _, record := range records {
		if _, _, err := store.Apply(record.Kind, record.ID, record.Result); err != nil {
			return nil, err
		}
	}
	return store, nil
}

type DispatchOutcome string

const (
	NotDispatched                 DispatchOutcome = "NOT_DISPATCHED"
	Acknowledged                  DispatchOutcome = "ACKNOWLEDGED"
	UnknownRequiresReconciliation DispatchOutcome = "UNKNOWN_REQUIRES_RECONCILIATION"
)

func OutcomeAfterDispatch(sent bool, responseRecorded bool) DispatchOutcome {
	if !sent {
		return NotDispatched
	}
	if !responseRecorded {
		return UnknownRequiresReconciliation
	}
	return Acknowledged
}
