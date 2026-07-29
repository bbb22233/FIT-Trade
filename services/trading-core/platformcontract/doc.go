// Package platformcontract mechanically consumes FIT Trade's frozen Phase 1
// platform contracts.  It is deliberately transport-only: it opens no
// listener and owns no credential, database, broker, wallet, or signer.
package platformcontract

//go:generate go run ./cmd/freeze
//go:generate go run ./cmd/gentypes
//go:generate go run ./cmd/genunicode
