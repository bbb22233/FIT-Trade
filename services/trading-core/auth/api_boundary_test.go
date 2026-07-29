package auth_test

import (
	"reflect"
	"testing"

	"fit.trade/trading-core/auth"
	"fit.trade/trading-core/session"
)

func TestNormalBuildCannotProvisionSyntheticCredentials(t *testing.T) {
	serviceType := reflect.TypeOf(&auth.Service{})
	if _, found := serviceType.MethodByName("AddSyntheticUser"); found {
		t.Fatal("normal consumers can provision synthetic credentials")
	}
	if _, found := serviceType.MethodByName("SyntheticUser"); found {
		t.Fatal("normal consumers can provision synthetic credentials")
	}
}

func TestExternalArgon2ProfileMutationCannotWeakenNewService(t *testing.T) {
	profile := session.ProductionArgon2Profile()
	profile.MemoryKiB, profile.Iterations, profile.Parallelism = 1, 1, 1
	selected := auth.NewService(auth.Config{}).Argon2Profile()
	if selected.MemoryKiB < 64*1024 || selected.Iterations < 3 || selected.Parallelism != 1 || selected.SaltBytes != 16 || selected.KeyBytes != 32 {
		t.Fatalf("external profile copy weakened a new service: %#v", selected)
	}
}
