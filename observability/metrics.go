package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry *prometheus.Registry
	Requests *prometheus.CounterVec
	Duration *prometheus.HistogramVec
	InFlight prometheus.Gauge
}

// InitMetrics returns the registry and HTTP middleware form named in the Phase
// 1 contract. NewMetrics exposes the counters and handler as well.
func InitMetrics() (*prometheus.Registry, func(http.Handler) http.Handler) {
	m := NewMetrics("")
	return m.Registry, m.Middleware
}

// NewMetrics returns an isolated registry suitable for tests and for avoiding
// duplicate global registrations in multi-service processes.
func NewMetrics(serviceName string) *Metrics {
	labels := prometheus.Labels{"service": serviceName}
	m := &Metrics{Registry: prometheus.NewRegistry(), Requests: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "Total HTTP requests.", ConstLabels: labels}, []string{"method", "path", "status"}), Duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP request duration in seconds.", ConstLabels: labels, Buckets: prometheus.DefBuckets}, []string{"method", "path"}), InFlight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "http_requests_in_flight", Help: "Current in-flight HTTP requests.", ConstLabels: labels})}
	m.Registry.MustRegister(m.Requests, m.Duration, m.InFlight)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.InFlight.Inc()
		defer m.InFlight.Dec()
		started := time.Now()
		sw := &metricsWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		m.Requests.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(status)).Inc()
		m.Duration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(started).Seconds())
	})
}

type metricsWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *metricsWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
