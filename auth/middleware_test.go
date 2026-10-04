package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireSessionValidatesAndAttachesPrincipal(t *testing.T) {
	token, _ := GenerateSessionToken()
	called := false
	validator := func(_ context.Context, got string) (*Principal, error) {
		called = true
		if got != token {
			t.Fatalf("token=%q", got)
		}
		return &Principal{UserID: "user-1", SessionID: "session-1"}, nil
	}
	h := RequireSession(validator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok || p.UserID != "user-1" {
			t.Fatalf("principal=%+v ok=%v", p, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
}

func TestRequireBearerRejectsMissingWrongTypeAndMalformed(t *testing.T) {
	session, _ := GenerateSessionToken()
	overlay, _ := GenerateOverlayToken()
	validatorCalls := 0
	validator := func(context.Context, string) (*Principal, error) { validatorCalls++; return &Principal{}, nil }
	tests := []struct {
		name, header string
		middleware   func(http.Handler) http.Handler
	}{
		{"missing", "", RequireSession(validator)}, {"wrong scheme", "Basic " + session, RequireSession(validator)},
		{"overlay as session", "Bearer " + overlay, RequireSession(validator)}, {"session as overlay", "Bearer " + session, RequireOverlay(validator)},
		{"malformed", "Bearer sk_bad", RequireSession(validator)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", tt.header)
			req.Header.Set("X-Request-Id", "req-1")
			rec := httptest.NewRecorder()
			tt.middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("called") })).ServeHTTP(rec, req)
			assertAuthError(t, rec, http.StatusUnauthorized, "INVALID_TOKEN", "req-1")
		})
	}
	if validatorCalls != 0 {
		t.Fatalf("validator calls=%d", validatorCalls)
	}
}

func TestValidatorErrorsMapToContract(t *testing.T) {
	token, _ := GenerateOverlayToken()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid", ErrInvalidCredential, 401, "INVALID_TOKEN"}, {"expired", ErrCredentialExpired, 401, "TOKEN_EXPIRED"},
		{"forbidden", ErrForbidden, 403, "STREAMER_INACTIVE"}, {"wrapped", errors.Join(errors.New("db"), ErrCredentialExpired), 401, "TOKEN_EXPIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := RequireOverlay(func(context.Context, string) (*Principal, error) { return nil, tt.err })(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("called") }))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assertAuthError(t, rec, tt.status, tt.code, "")
		})
	}
}

func TestRequireInternal(t *testing.T) {
	h := RequireInternal(StaticInternalValidator("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PrincipalFromContext(r.Context()); !ok {
			t.Fatal("missing principal")
		}
		w.WriteHeader(204)
	}))
	for _, tc := range []struct {
		key    string
		status int
	}{{"secret", 204}, {"wrong", 401}, {"", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/v1/internal/x", nil)
		req.Header.Set(InternalAPIKeyHeader, tc.key)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("key=%q status=%d", tc.key, rec.Code)
		}
	}
}

func assertAuthError(t *testing.T, rec *httptest.ResponseRecorder, status int, code, requestID string) {
	t.Helper()
	if rec.Code != status || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v", rec.Code, rec.Header())
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code || body.Error.RequestID != requestID {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
