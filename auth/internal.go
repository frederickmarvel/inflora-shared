// Package auth provides internal authentication primitives shared across
// services. D-010: internal HTTP uses X-Internal-Api-Key, NOT
// Authorization: Bearer. Internal gRPC uses Authorization: Bearer.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ErrUnauthorized is returned by the auth helpers on a credential mismatch.
var ErrUnauthorized = errors.New("auth: unauthorized")

// ConstantTimeEqual compares two strings in constant time. Returns false if
// the lengths differ.
func ConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// HTTPInternalAPIKey returns an HTTP middleware that validates the
// X-Internal-Api-Key header against the configured secret. On a mismatch the
// handler responds with 401 and the standard error envelope. The header
// X-Internal-Api-Key MUST be present and match the secret (constant-time).
//
// Use this for every internal service-to-service HTTP endpoint. Never use
// Authorization: Bearer for internal HTTP (D-010).
func HTTPInternalAPIKey(secret string) func(http.Handler) http.Handler {
	if secret == "" {
		panic("auth: HTTPInternalAPIKey requires a non-empty secret")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := strings.TrimSpace(r.Header.Get("X-Internal-Api-Key"))
			if got == "" || !ConstantTimeEqual(got, secret) {
				writeUnauthorized(w, "MISSING_OR_INVALID_INTERNAL_API_KEY")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeUnauthorized(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"internal API key required"}}`))
}

// GRPCBearerAuth returns a unary gRPC server interceptor that validates the
// authorization metadata header against the configured engine-api-key. The
// header MUST be present and match (constant-time). Mismatches return
// codes.Unauthenticated with the standard error message.
//
// Use this for every internal service-to-service gRPC server.
func GRPCBearerAuth(secret string) grpc.UnaryServerInterceptor {
	if secret == "" {
		panic("auth: GRPCBearerAuth requires a non-empty secret")
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization")
		}
		got := strings.TrimPrefix(strings.TrimSpace(values[0]), "Bearer ")
		got = strings.TrimSpace(got)
		if got == "" || !ConstantTimeEqual(got, secret) {
			return nil, status.Error(codes.Unauthenticated, "invalid engine api key")
		}
		return handler(ctx, req)
	}
}

// GRPCBearerAuthClient returns a DialOption that attaches the engine-api-key
// bearer token to every outgoing unary call. Use this on every internal
// service-to-service gRPC client.
func GRPCBearerAuthClient(secret string) grpc.DialOption {
	if secret == "" {
		panic("auth: GRPCBearerAuthClient requires a non-empty secret")
	}
	return grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		md.Set("authorization", "Bearer "+secret)
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	})
}
