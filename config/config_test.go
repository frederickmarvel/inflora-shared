package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	clearKnownEnv(t)
	c, err := Load("saruman")
	if err != nil {
		t.Fatal(err)
	}
	if c.ServiceName != "saruman" || c.HTTPAddr != ":8082" || c.Database.MaxOpenConns != 50 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.Auth.SessionTTL != 720*time.Hour || c.Observability.ServiceName != "saruman" {
		t.Fatalf("unexpected duration/otel defaults: %+v", c)
	}
}

func TestLoadOverridesAndDoesNotRequireSecrets(t *testing.T) {
	clearKnownEnv(t)
	t.Setenv("ENV", "prod")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("DB_MAX_OPEN_CONNS", "20")
	t.Setenv("DB_MAX_IDLE_CONNS", "4")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")
	c, err := Load("tolkien")
	if err != nil {
		t.Fatal(err)
	}
	if c.Environment != "prod" || c.Database.MaxIdleConns != 4 || c.Observability.SamplerArg != .25 {
		t.Fatalf("overrides not loaded: %+v", c)
	}
}

func TestLoadValidation(t *testing.T) {
	tests := []struct{ name, key, val, contains string }{
		{"enum", "LOG_LEVEL", "verbose", "LOG_LEVEL"},
		{"range", "OTEL_TRACES_SAMPLER_ARG", "1.1", "between"},
		{"relation", "DB_MAX_IDLE_CONNS", "51", "DB_MAX_IDLE_CONNS"},
		{"aead", "AEAD_KEY_HEX", "not-a-key", "64 hexadecimal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearKnownEnv(t)
			t.Setenv(tt.key, tt.val)
			_, err := Load("saruman")
			if err == nil || !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("got %v, want error containing %q", err, tt.contains)
			}
		})
	}
}

func clearKnownEnv(t *testing.T) {
	keys := []string{"SERVICE_NAME", "ENV", "LOG_LEVEL", "LOG_FORMAT", "HTTP_ADDR", "VERSION", "DB_DRIVER", "DB_DSN", "DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME_SEC", "DB_SLOW_QUERY_THRESHOLD_MS", "NATS_URL", "NATS_STREAM_NAME", "NATS_CONSUMER_GROUP", "NATS_ACK_WAIT_SEC", "NATS_MAX_DELIVER", "NATS_MAX_ACK_PENDING", "SESSION_TTL_HOURS", "OVERLAY_TOKEN_TTL_HOURS", "BCRYPT_COST", "ARGON2_MEMORY_KB", "ARGON2_TIME", "ARGON2_THREADS", "LOGIN_RATE_LIMIT_PER_MIN", "LOGIN_LOCKOUT_THRESHOLD", "PAYMENT_PROVIDER", "PLATFORM_FEE_BPS", "MIN_DONATION_IDR", "MAX_DONATION_IDR", "INTENT_TTL_MINUTES", "MIDTRANS_SERVER_KEY", "MIDTRANS_CLIENT_KEY", "MIDTRANS_ENV", "MIDTRANS_WEBHOOK_URL", "XENDIT_SECRET_KEY", "XENDIT_PUBLIC_KEY", "XENDIT_WEBHOOK_TOKEN", "SM_GATEWAY_ADDR", "SM_GATEWAY_ENGINE_API_KEY", "SM_GRPC_TIMEOUT_SEC", "AEAD_KEY_HEX", "RATE_LIMITER_KEY", "RATE_LIMIT_DONATION_PER_MIN", "RATE_LIMIT_DONATION_PER_HOUR", "RATE_LIMIT_SIGNUP_PER_MIN", "RECONCILIATION_ENABLED", "RECONCILIATION_CRON", "RECONCILIATION_DRIFT_THRESHOLD_IDR", "RECONCILIATION_ALERT_WEBHOOK", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "OTEL_TRACES_SAMPLER", "OTEL_TRACES_SAMPLER_ARG", "WS_ADDR", "WS_PATH", "WS_HEARTBEAT_INTERVAL_SEC", "WS_HEARTBEAT_TIMEOUT_SEC", "WS_MAX_CONNECTIONS_PER_STREAMER", "WS_REPLAY_WINDOW_SEC", "WS_AUTH_CACHE_TTL_SEC", "TOLKIEN_VALIDATE_TOKEN_URL", "INTERNAL_API_KEY"}
	type prior struct {
		value string
		set   bool
	}
	old := make(map[string]prior, len(keys))
	for _, key := range keys {
		v, ok := os.LookupEnv(key)
		old[key] = prior{v, ok}
		_ = os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if p := old[key]; p.set {
				_ = os.Setenv(key, p.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}
