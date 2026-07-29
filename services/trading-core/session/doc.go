// Package session contains transport-neutral session constants shared by the
// bounded development API. Persistent production session storage is outside
// this Phase 1 in-memory implementation.
package session

import "time"

const (
	AccessTTL        = 15 * time.Minute
	RefreshTTL       = 7 * 24 * time.Hour
	FamilyMaximumTTL = 30 * 24 * time.Hour
	RevocationBound  = 5 * time.Second
)

// Argon2Profile is an explicit, versioned password-verifier profile. The
// production profile is deliberately the minimum accepted by the frozen
// contract; callers receive a copy and cannot alter the profile selected by
// an auth.Service constructor.
type Argon2Profile struct {
	Version     string
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltBytes   uint32
	KeyBytes    uint32
}

// ProductionArgon2Profile returns the frozen production verifier profile.
// Its literal result deliberately has no mutable package-level backing state.
func ProductionArgon2Profile() Argon2Profile {
	return Argon2Profile{
		Version: "argon2id-v1", MemoryKiB: 64 * 1024, Iterations: 3,
		Parallelism: 1, SaltBytes: 16, KeyBytes: 32,
	}
}
