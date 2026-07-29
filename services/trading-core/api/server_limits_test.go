package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fit.trade/trading-core/auth"
)

func TestHTTPServerConfiguresTimeouts(t *testing.T) {
	server, err := NewLoopbackServer(auth.NewService(auth.Config{}), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	httpServer := server.httpServer()
	if httpServer.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout = %s, want 5s", httpServer.ReadHeaderTimeout)
	}
	if httpServer.ReadTimeout != 10*time.Second {
		t.Fatalf("ReadTimeout = %s, want 10s", httpServer.ReadTimeout)
	}
	if httpServer.WriteTimeout != 15*time.Second {
		t.Fatalf("WriteTimeout = %s, want 15s", httpServer.WriteTimeout)
	}
	if httpServer.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout = %s, want 60s", httpServer.IdleTimeout)
	}
}

func TestAuthenticatedSourceThrottleReturnsThrottled(t *testing.T) {
	service := auth.NewService(auth.Config{})
	if _, err := addSyntheticUserForTest(service, "limits@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	accessTokens := make([]string, 3)
	for i := range accessTokens {
		credentials, err := service.Login(context.Background(), "limits@example.test", "test-password", "test-source")
		if err != nil {
			t.Fatal(err)
		}
		accessTokens[i] = credentials.AccessToken
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server.limits = newRateBookWithClock(func() time.Time { return now })

	for i := range 100 {
		request := authenticatedMutationRequest(t, accessTokens[i/40], "127.0.0.1:43210")
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("request %d status = %d, want %d", i+1, response.Code, http.StatusAccepted)
		}
	}

	request := authenticatedMutationRequest(t, accessTokens[2], "127.0.0.1:43210")
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	var problemResponse map[string]string
	if err := json.NewDecoder(response.Body).Decode(&problemResponse); err != nil {
		t.Fatal(err)
	}
	if problemResponse["code"] != "THROTTLED" {
		t.Fatalf("problem code = %q, want THROTTLED", problemResponse["code"])
	}
	if err := server.RotateSourceKey("source-key-v2", bytes.Repeat([]byte{4}, 32)); err != nil {
		t.Fatal(err)
	}
	rotatedRequest := authenticatedMutationRequest(t, accessTokens[2], "127.0.0.1:43210")
	rotatedResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(rotatedResponse, rotatedRequest)
	if rotatedResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("same source after key rotation status = %d, want %d", rotatedResponse.Code, http.StatusTooManyRequests)
	}

	otherSource := authenticatedMutationRequest(t, accessTokens[2], "127.0.0.2:43210")
	otherResponse := httptest.NewRecorder()
	server.handler.ServeHTTP(otherResponse, otherSource)
	if otherResponse.Code != http.StatusAccepted {
		t.Fatalf("separate source status = %d, want %d", otherResponse.Code, http.StatusAccepted)
	}
}

func TestSourceThrottleDoesNotDrainAuthenticatedSession(t *testing.T) {
	service := auth.NewService(auth.Config{})
	if profile := service.Argon2Profile(); profile.MemoryKiB < 64*1024 || profile.Iterations < 3 || profile.Parallelism != 1 || profile.SaltBytes != 16 || profile.KeyBytes != 32 {
		t.Fatalf("cross-package service selected a weak Argon2id profile: %#v", profile)
	}
	if _, err := addSyntheticUserForTest(service, "atomic-limits@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	accessTokens := make([]string, 4)
	for i := range accessTokens {
		credentials, err := service.Login(context.Background(), "atomic-limits@example.test", "test-password", "test-source")
		if err != nil {
			t.Fatal(err)
		}
		accessTokens[i] = credentials.AccessToken
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	server.limits = newRateBookWithClock(func() time.Time { return now })

	for i := range 100 {
		request := authenticatedMutationRequest(t, accessTokens[1+i%3], "127.0.0.1:43210")
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("source fill request %d status = %d, want %d", i+1, response.Code, http.StatusAccepted)
		}
	}
	for range 5 {
		request := authenticatedMutationRequest(t, accessTokens[0], "127.0.0.1:43210")
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("source-throttled status = %d, want %d", response.Code, http.StatusTooManyRequests)
		}
	}
	for i := range 40 {
		request := authenticatedMutationRequest(t, accessTokens[0], "127.0.0.2:43210")
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("fresh-source session request %d status = %d, want %d", i+1, response.Code, http.StatusAccepted)
		}
	}
}

func TestMutationRejectsEveryFrozenServerAuthoredFieldRecursively(t *testing.T) {
	service := auth.NewService(auth.Config{})
	if _, err := addSyntheticUserForTest(service, "authority-fields@example.test", "test-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := service.Login(context.Background(), "authority-fields@example.test", "test-password", "fixture-source")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"user_id", "trading_account_id", "session_id", "device_id", "source_key", "source_key_digest", "server_received_at", "server_time", "token_digest", "family_id", "family_created_at", "family_deadline", "refresh_family_id", "rotated_to_digest", "canonical_request_digest", "device_verification", "identity_origin", "signature_verified",
	} {
		t.Run(field, func(t *testing.T) {
			server, err := NewLoopbackServer(service, bytes.Repeat([]byte{9}, 32))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "http://loopback.test/v1/auth/login", bytes.NewBufferString(`{"identifier":"authority-fields@example.test","password_utf8":"test-password","payload":{"nested":{"`+field+`":"client"}}}`))
			request.RemoteAddr = "127.0.0.1:43210"
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("field %q status = %d, want %d", field, response.Code, http.StatusBadRequest)
			}
		})
	}
	server, err := NewLoopbackServer(service, bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	request := authenticatedMutationRequest(t, credentials.AccessToken, "127.0.0.2:43211")
	request.Body = io.NopCloser(bytes.NewBufferString(`{"payload":{"family":"client-supplied-near-miss","metadata":{"note":"allowed"}}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("near-miss client fields status = %d, want %d", response.Code, http.StatusAccepted)
	}
}

func TestFrozenCompletionControlInputNamesRejectAliases(t *testing.T) {
	t.Run("enrollment completion", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://loopback.test/ignored", bytes.NewBufferString(`{"subject_handle":"handle","candidate_public_key":"ed25519-public:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signature":"ed25519-signature:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		var input enrollmentCompleteInput
		if !decodeMutation(response, request, &input) || input.PublicKey == "" {
			t.Fatal("candidate_public_key was not accepted")
		}
		request = httptest.NewRequest(http.MethodPost, "http://loopback.test/ignored", bytes.NewBufferString(`{"subject_handle":"handle","public_key":"ed25519-public:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signature":"x"}`))
		request.Header.Set("Content-Type", "application/json")
		if decodeMutation(httptest.NewRecorder(), request, &enrollmentCompleteInput{}) {
			t.Fatal("legacy public_key alias was accepted")
		}
	})
	t.Run("device action proof", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://loopback.test/ignored", bytes.NewBufferString(`{"challenge_nonce":"handle","signature":"ed25519-signature:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
		request.Header.Set("Content-Type", "application/json")
		var input actionCompleteInput
		if !decodeMutation(httptest.NewRecorder(), request, &input) || input.Nonce != "handle" {
			t.Fatal("challenge_nonce was not accepted")
		}
		request = httptest.NewRequest(http.MethodPost, "http://loopback.test/ignored", bytes.NewBufferString(`{"nonce":"handle","signature":"x"}`))
		request.Header.Set("Content-Type", "application/json")
		if decodeMutation(httptest.NewRecorder(), request, &actionCompleteInput{}) {
			t.Fatal("legacy nonce alias was accepted")
		}
	})
}

func authenticatedMutationRequest(t *testing.T, token, remoteAddr string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://loopback.test/v1/owner-mutations", bytes.NewBufferString(`{"payload":{}}`))
	request.RemoteAddr = remoteAddr
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	return request
}
