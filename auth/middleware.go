package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const InternalAPIKeyHeader = "X-Internal-Api-Key"

var (
	ErrInvalidCredential = errors.New("invalid credential")
	ErrCredentialExpired = errors.New("credential expired")
	ErrForbidden         = errors.New("credential forbidden")
)

// Principal is validator-owned authenticated identity data made available to
// handlers. Metadata is intentionally open for service-specific claims.
type Principal struct {
	UserID     string
	StreamerID string
	SessionID  string
	ExpiresAt  time.Time
	Metadata   map[string]any
}

// ValidatorFunc lets services validate credentials using their own database,
// cache, or secret store. Middleware never owns persistence or business rules.
type ValidatorFunc func(context.Context, string) (*Principal, error)

type principalContextKey struct{}

func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(*Principal)
	return p, ok && p != nil
}

// RequireSession validates an sk_ bearer token and attaches its principal.
func RequireSession(validator ValidatorFunc) func(http.Handler) http.Handler {
	return requireBearer(SessionTokenPrefix, validator)
}

// RequireOverlay validates an ok_ bearer token and attaches its principal.
func RequireOverlay(validator ValidatorFunc) func(http.Handler) http.Handler {
	return requireBearer(OverlayTokenPrefix, validator)
}

// RequireInternal validates the X-Internal-Api-Key header. Keeping validation
// callback-driven supports key rotation and secret managers without coupling.
func RequireInternal(validator ValidatorFunc) func(http.Handler) http.Handler {
	return requireCredential(func(r *http.Request) (string, bool) {
		value := strings.TrimSpace(r.Header.Get(InternalAPIKeyHeader))
		return value, value != ""
	}, validator)
}

// StaticInternalValidator provides constant-time validation for the common
// single shared-key deployment while retaining the validator-driven API.
func StaticInternalValidator(expected string) ValidatorFunc {
	return func(_ context.Context, actual string) (*Principal, error) {
		if expected == "" || len(actual) != len(expected) || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
			return nil, ErrInvalidCredential
		}
		return &Principal{}, nil
	}
}

func requireBearer(prefix string, validator ValidatorFunc) func(http.Handler) http.Handler {
	return requireCredential(func(r *http.Request) (string, bool) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !strings.HasPrefix(parts[1], prefix) {
			return "", false
		}
		if (prefix == SessionTokenPrefix && !ValidSessionToken(parts[1])) || (prefix == OverlayTokenPrefix && !ValidOverlayToken(parts[1])) {
			return "", false
		}
		return parts[1], true
	}, validator)
}

func requireCredential(extract func(*http.Request) (string, bool), validator ValidatorFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			credential, ok := extract(r)
			if !ok || validator == nil {
				writeAuthError(w, r, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or missing credential")
				return
			}
			principal, err := validator(r.Context(), credential)
			if err != nil || principal == nil {
				status, code, message := http.StatusUnauthorized, "INVALID_TOKEN", "invalid credential"
				if errors.Is(err, ErrCredentialExpired) {
					code, message = "TOKEN_EXPIRED", "credential expired"
				} else if errors.Is(err, ErrForbidden) {
					status, code, message = http.StatusForbidden, "STREAMER_INACTIVE", "credential is not allowed"
				}
				writeAuthError(w, r, status, code, message)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
		})
	}
}

func writeAuthError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
		"code": code, "message": message, "request_id": requestID(r),
	}})
}

func requestID(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Request-Id")); id != "" {
		return id
	}
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}
