package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewLoggerValidation(t *testing.T) {
	if _, err := NewLogger("test", "invalid", "json"); err == nil {
		t.Fatal("expected invalid level")
	}
	if _, err := NewLogger("test", "info", "invalid"); err == nil {
		t.Fatal("expected invalid format")
	}
}
func TestNoExporterTracerProvider(t *testing.T) {
	tp, err := NewTracerProvider(context.Background(), TracingConfig{ServiceName: "test", Sampler: "always_on", SamplerArg: 1})
	if err != nil {
		t.Fatal(err)
	}
	if tp == nil {
		t.Fatal("nil provider")
	}
	if err = tp.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestMetricsMiddleware(t *testing.T) {
	m := NewMetrics("test")
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/items", nil))
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), `http_requests_total{method="POST",path="/items",service="test",status="201"} 1`) {
		t.Fatalf("metric missing:\n%s", rec.Body.String())
	}
}
