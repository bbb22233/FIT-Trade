// Package ownership keeps the small, server-authored ownership projection used
// by the development listener. It deliberately accepts no client identities.
package ownership

import (
	"errors"
	"sync"
)

var ErrDenied = errors.New("ownership: denied")

type Record struct {
	UserID           string
	TradingAccountID string
	OwnershipID      string
	Active           bool
}

// Registry is safe for concurrent use. Records are installed only by trusted
// provisioning code; lookups never reveal a record for a different user.
type Registry struct {
	mu       sync.RWMutex
	records  map[string]Record
	byUserID map[string]string
}

func NewRegistry() *Registry {
	return &Registry{records: make(map[string]Record), byUserID: make(map[string]string)}
}

func (r *Registry) Put(record Record) error {
	if record.UserID == "" || record.TradingAccountID == "" || record.OwnershipID == "" {
		return ErrDenied
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if account, ok := r.byUserID[record.UserID]; ok && account != record.TradingAccountID {
		return ErrDenied
	}
	r.records[record.TradingAccountID] = record
	r.byUserID[record.UserID] = record.TradingAccountID
	return nil
}

func (r *Registry) Authorize(userID, accountID string) (Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.records[accountID]
	if !ok || !record.Active || record.UserID != userID {
		return Record{}, ErrDenied
	}
	return record, nil
}

func (r *Registry) Revoke(accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[accountID]
	if !ok {
		return
	}
	record.Active = false
	r.records[accountID] = record
}
