package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestIDAndRecovery(t *testing.T) {
	h := RequestID(Recovery(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Error("missing request id")
		}
		panic("boom")
	})))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 500 || rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("code=%d headers=%v", rec.Code, rec.Header())
	}
}
func TestCORS(t *testing.T) {
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://example.test"}, AllowedMethods: []string{"GET"}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://example.test" {
		t.Fatalf("unexpected CORS response: %d %v", rec.Code, rec.Header())
	}
}
func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	h := limiter.Middleware(func(*http.Request) string { return "same" })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	for i, want := range []int{200, 429} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != want {
			t.Fatalf("request %d: got %d", i, rec.Code)
		}
	}
}
