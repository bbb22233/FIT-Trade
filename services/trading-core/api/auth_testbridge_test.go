package api

import (
	"fit.trade/trading-core/auth"
	_ "unsafe"
)

// addSyntheticUserForTest reaches the auth package's unexported fixture only
// from test code. No normal-build exported API can provision credentials.
//
//go:linkname addSyntheticUserForTest fit.trade/trading-core/auth.(*Service).addSyntheticUser
func addSyntheticUserForTest(*auth.Service, string, string) (auth.Identity, error)
