package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the shared Phase 1 environment configuration. Service-owned
// secrets are represented when documented, but Load deliberately does not
// require them: the service that uses a secret is responsible for requiring it.
type Config struct {
	Environment string
	ServiceName string
	LogLevel    string
	LogFormat   string
	HTTPAddr    string
	Version     string

	Database       DatabaseConfig
	NATS           NATSConfig
	Auth           AuthConfig
	Payment        PaymentConfig
	Gateway        GatewayConfig
	Encryption     EncryptionConfig
	RateLimit      RateLimitConfig
	Reconciliation ReconciliationConfig
	Observability  ObservabilityConfig
	WebSocket      WebSocketConfig
}

type DatabaseConfig struct {
	Driver             string
	DSN                string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetime    time.Duration
	SlowQueryThreshold time.Duration
}

type NATSConfig struct {
	URL           string
	StreamName    string
	ConsumerGroup string
	AckWait       time.Duration
	MaxDeliver    int
	MaxAckPending int
}

type AuthConfig struct {
	SessionTTL              time.Duration
	OverlayTokenTTL         time.Duration
	BcryptCost              int
	Argon2MemoryKB          int
	Argon2Time              int
	Argon2Threads           int
	LoginRateLimitPerMinute int
	LoginLockoutThreshold   int
}

type PaymentConfig struct {
	Provider            string
	PlatformFeeBPS      int
	MinDonationIDR      int64
	MaxDonationIDR      int64
	IntentTTL           time.Duration
	MidtransServerKey   string
	MidtransClientKey   string
	MidtransEnv         string
	MidtransWebhookURL  string
	XenditSecretKey     string
	XenditPublicKey     string
	XenditWebhookToken  string
	PivotMerchantID     string
	PivotMerchantSecret string
	PivotCallbackKey    string
	PivotEnv            string
	PivotBaseURL        string
	PivotRedirectURL    string
}

type GatewayConfig struct {
	Address      string
	EngineAPIKey string
	GRPCTimeout  time.Duration
}

type EncryptionConfig struct {
	AEADKeyHex     string
	RateLimiterKey string
}

type RateLimitConfig struct {
	DonationPerMinute int
	DonationPerHour   int
	SignupPerMinute   int
}

type ReconciliationConfig struct {
	Enabled           bool
	Cron              string
	DriftThresholdIDR int64
	AlertWebhook      string
}

type ObservabilityConfig struct {
	OTLPEndpoint  string
	ServiceName   string
	TracesSampler string
	SamplerArg    float64
}

type WebSocketConfig struct {
	Address                   string
	Path                      string
	HeartbeatInterval         time.Duration
	HeartbeatTimeout          time.Duration
	MaxConnectionsPerStreamer int
	ReplayWindow              time.Duration
	AuthCacheTTL              time.Duration
	ValidateTokenURL          string
	InternalAPIKey            string
}

// Load reads the documented Phase 1 environment variables, applies defaults,
// and validates types, enums, and safe ranges. serviceName supplies the service
// identity when SERVICE_NAME is unset.
func Load(serviceName string) (Config, error) {
	serviceName = value("SERVICE_NAME", serviceName)
	if strings.TrimSpace(serviceName) == "" {
		return Config{}, fmt.Errorf("SERVICE_NAME: must not be empty")
	}

	var c Config
	var err error
	c.ServiceName = serviceName
	c.Environment, err = enum("ENV", "dev", "dev", "staging", "prod")
	if err != nil {
		return Config{}, err
	}
	c.LogLevel, err = enum("LOG_LEVEL", "info", "debug", "info", "warn", "error")
	if err != nil {
		return Config{}, err
	}
	c.LogFormat, err = enum("LOG_FORMAT", "json", "json", "text")
	if err != nil {
		return Config{}, err
	}
	c.HTTPAddr = value("HTTP_ADDR", defaultHTTPAddr(serviceName))
	c.Version = value("VERSION", "dev")

	c.Database.Driver, err = enum("DB_DRIVER", "postgres", "postgres", "mysql")
	if err != nil {
		return Config{}, err
	}
	c.Database.DSN = os.Getenv("DB_DSN")
	if c.Database.MaxOpenConns, err = integer("DB_MAX_OPEN_CONNS", 50, 1, 10000); err != nil {
		return Config{}, err
	}
	if c.Database.MaxIdleConns, err = integer("DB_MAX_IDLE_CONNS", 10, 0, 10000); err != nil {
		return Config{}, err
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return Config{}, fmt.Errorf("DB_MAX_IDLE_CONNS: must be <= DB_MAX_OPEN_CONNS")
	}
	if c.Database.ConnMaxLifetime, err = seconds("DB_CONN_MAX_LIFETIME_SEC", 300, 0, 86400); err != nil {
		return Config{}, err
	}
	if c.Database.SlowQueryThreshold, err = milliseconds("DB_SLOW_QUERY_THRESHOLD_MS", 200, 0, 3600000); err != nil {
		return Config{}, err
	}

	c.NATS.URL = value("NATS_URL", "nats://localhost:4222")
	c.NATS.StreamName = value("NATS_STREAM_NAME", "inflora-events")
	c.NATS.ConsumerGroup = os.Getenv("NATS_CONSUMER_GROUP")
	if c.NATS.AckWait, err = seconds("NATS_ACK_WAIT_SEC", 30, 1, 86400); err != nil {
		return Config{}, err
	}
	if c.NATS.MaxDeliver, err = integer("NATS_MAX_DELIVER", 5, 1, 1000); err != nil {
		return Config{}, err
	}
	if c.NATS.MaxAckPending, err = integer("NATS_MAX_ACK_PENDING", 1000, 1, 10000000); err != nil {
		return Config{}, err
	}

	if c.Auth.SessionTTL, err = hours("SESSION_TTL_HOURS", 720, 1, 24*3650); err != nil {
		return Config{}, err
	}
	if c.Auth.OverlayTokenTTL, err = hours("OVERLAY_TOKEN_TTL_HOURS", 2160, 1, 24*3650); err != nil {
		return Config{}, err
	}
	if c.Auth.BcryptCost, err = integer("BCRYPT_COST", 12, 10, 31); err != nil {
		return Config{}, err
	}
	if c.Auth.Argon2MemoryKB, err = integer("ARGON2_MEMORY_KB", 65536, 8, math.MaxInt32); err != nil {
		return Config{}, err
	}
	if c.Auth.Argon2Time, err = integer("ARGON2_TIME", 3, 1, math.MaxInt32); err != nil {
		return Config{}, err
	}
	if c.Auth.Argon2Threads, err = integer("ARGON2_THREADS", 1, 1, 255); err != nil {
		return Config{}, err
	}
	if c.Auth.LoginRateLimitPerMinute, err = integer("LOGIN_RATE_LIMIT_PER_MIN", 5, 1, 1000000); err != nil {
		return Config{}, err
	}
	if c.Auth.LoginLockoutThreshold, err = integer("LOGIN_LOCKOUT_THRESHOLD", 3, 1, 1000000); err != nil {
		return Config{}, err
	}

	c.Payment.Provider, err = optionalEnum("PAYMENT_PROVIDER", "midtrans", "xendit", "pivot")
	if err != nil {
		return Config{}, err
	}
	if c.Payment.PlatformFeeBPS, err = integer("PLATFORM_FEE_BPS", 1, 0, 10000); err != nil {
		return Config{}, err
	}
	if c.Payment.MinDonationIDR, err = int64Value("MIN_DONATION_IDR", 1000, 1, math.MaxInt64); err != nil {
		return Config{}, err
	}
	if c.Payment.MaxDonationIDR, err = int64Value("MAX_DONATION_IDR", 10000000, 1, math.MaxInt64); err != nil {
		return Config{}, err
	}
	if c.Payment.MaxDonationIDR < c.Payment.MinDonationIDR {
		return Config{}, fmt.Errorf("MAX_DONATION_IDR: must be >= MIN_DONATION_IDR")
	}
	if c.Payment.IntentTTL, err = minutes("INTENT_TTL_MINUTES", 15, 1, 24*60); err != nil {
		return Config{}, err
	}
	c.Payment.MidtransServerKey = os.Getenv("MIDTRANS_SERVER_KEY")
	c.Payment.MidtransClientKey = os.Getenv("MIDTRANS_CLIENT_KEY")
	c.Payment.MidtransEnv, err = optionalEnum("MIDTRANS_ENV", "sandbox", "production")
	if err != nil {
		return Config{}, err
	}
	c.Payment.MidtransWebhookURL = os.Getenv("MIDTRANS_WEBHOOK_URL")
	c.Payment.XenditSecretKey = os.Getenv("XENDIT_SECRET_KEY")
	c.Payment.XenditPublicKey = os.Getenv("XENDIT_PUBLIC_KEY")
	c.Payment.XenditWebhookToken = os.Getenv("XENDIT_WEBHOOK_TOKEN")
	c.Payment.PivotMerchantID = os.Getenv("PIVOT_MERCHANT_ID")
	c.Payment.PivotMerchantSecret = os.Getenv("PIVOT_MERCHANT_SECRET")
	c.Payment.PivotCallbackKey = os.Getenv("PIVOT_CALLBACK_KEY")
	c.Payment.PivotEnv, err = optionalEnum("PIVOT_ENV", "sandbox", "production")
	if err != nil {
		return Config{}, err
	}
	c.Payment.PivotBaseURL = os.Getenv("PIVOT_BASE_URL")
	c.Payment.PivotRedirectURL = os.Getenv("PIVOT_REDIRECT_URL")

	c.Gateway.Address = value("SM_GATEWAY_ADDR", "localhost:7001")
	c.Gateway.EngineAPIKey = os.Getenv("SM_GATEWAY_ENGINE_API_KEY")
	if c.Gateway.GRPCTimeout, err = seconds("SM_GRPC_TIMEOUT_SEC", 5, 1, 300); err != nil {
		return Config{}, err
	}
	c.Encryption.AEADKeyHex = os.Getenv("AEAD_KEY_HEX")
	if c.Encryption.AEADKeyHex != "" && (len(c.Encryption.AEADKeyHex) != 64 || !isHex(c.Encryption.AEADKeyHex)) {
		return Config{}, fmt.Errorf("AEAD_KEY_HEX: must be 64 hexadecimal characters")
	}
	c.Encryption.RateLimiterKey = os.Getenv("RATE_LIMITER_KEY")

	if c.RateLimit.DonationPerMinute, err = integer("RATE_LIMIT_DONATION_PER_MIN", 10, 1, 10000000); err != nil {
		return Config{}, err
	}
	if c.RateLimit.DonationPerHour, err = integer("RATE_LIMIT_DONATION_PER_HOUR", 100, 1, 10000000); err != nil {
		return Config{}, err
	}
	if c.RateLimit.SignupPerMinute, err = integer("RATE_LIMIT_SIGNUP_PER_MIN", 5, 1, 10000000); err != nil {
		return Config{}, err
	}
	if c.Reconciliation.Enabled, err = boolean("RECONCILIATION_ENABLED", true); err != nil {
		return Config{}, err
	}
	c.Reconciliation.Cron = value("RECONCILIATION_CRON", "0 0 * * *")
	if c.Reconciliation.DriftThresholdIDR, err = int64Value("RECONCILIATION_DRIFT_THRESHOLD_IDR", 1000, 0, math.MaxInt64); err != nil {
		return Config{}, err
	}
	c.Reconciliation.AlertWebhook = os.Getenv("RECONCILIATION_ALERT_WEBHOOK")

	c.Observability.OTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	c.Observability.ServiceName = value("OTEL_SERVICE_NAME", serviceName)
	c.Observability.TracesSampler, err = enum("OTEL_TRACES_SAMPLER", "parentbased_traceidratio", "parentbased_traceidratio", "always_on", "always_off")
	if err != nil {
		return Config{}, err
	}
	if c.Observability.SamplerArg, err = floating("OTEL_TRACES_SAMPLER_ARG", 0.1, 0, 1); err != nil {
		return Config{}, err
	}

	c.WebSocket.Address = value("WS_ADDR", ":8081")
	c.WebSocket.Path = value("WS_PATH", "/ws")
	if !strings.HasPrefix(c.WebSocket.Path, "/") {
		return Config{}, fmt.Errorf("WS_PATH: must begin with /")
	}
	if c.WebSocket.HeartbeatInterval, err = seconds("WS_HEARTBEAT_INTERVAL_SEC", 10, 1, 3600); err != nil {
		return Config{}, err
	}
	if c.WebSocket.HeartbeatTimeout, err = seconds("WS_HEARTBEAT_TIMEOUT_SEC", 30, 1, 3600); err != nil {
		return Config{}, err
	}
	if c.WebSocket.HeartbeatTimeout < c.WebSocket.HeartbeatInterval {
		return Config{}, fmt.Errorf("WS_HEARTBEAT_TIMEOUT_SEC: must be >= WS_HEARTBEAT_INTERVAL_SEC")
	}
	if c.WebSocket.MaxConnectionsPerStreamer, err = integer("WS_MAX_CONNECTIONS_PER_STREAMER", 100, 1, 1000000); err != nil {
		return Config{}, err
	}
	if c.WebSocket.ReplayWindow, err = seconds("WS_REPLAY_WINDOW_SEC", 600, 0, 86400); err != nil {
		return Config{}, err
	}
	if c.WebSocket.AuthCacheTTL, err = seconds("WS_AUTH_CACHE_TTL_SEC", 60, 0, 86400); err != nil {
		return Config{}, err
	}
	c.WebSocket.ValidateTokenURL = value("TOLKIEN_VALIDATE_TOKEN_URL", "http://tolkien:8080/v1/internal/validate-overlay-token")
	c.WebSocket.InternalAPIKey = os.Getenv("INTERNAL_API_KEY")

	for name, raw := range map[string]string{"MIDTRANS_WEBHOOK_URL": c.Payment.MidtransWebhookURL, "RECONCILIATION_ALERT_WEBHOOK": c.Reconciliation.AlertWebhook, "TOLKIEN_VALIDATE_TOKEN_URL": c.WebSocket.ValidateTokenURL} {
		if raw != "" {
			if _, parseErr := url.ParseRequestURI(raw); parseErr != nil {
				return Config{}, fmt.Errorf("%s: invalid URL: %w", name, parseErr)
			}
		}
	}
	return c, nil
}

func defaultHTTPAddr(service string) string {
	switch service {
	case "tolkien":
		return ":8080"
	case "ingest", "ingest-api":
		return ":8081"
	case "saruman":
		return ":8082"
	case "ws-gateway", "ithildin":
		return ":8083"
	default:
		return ":8080"
	}
}
func value(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return fallback
}
func enum(name, fallback string, allowed ...string) (string, error) {
	v := value(name, fallback)
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", fmt.Errorf("%s: unsupported value %q (allowed: %s)", name, v, strings.Join(allowed, ", "))
}
func optionalEnum(name string, allowed ...string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", fmt.Errorf("%s: unsupported value %q (allowed: %s)", name, v, strings.Join(allowed, ", "))
}
func integer(name string, fallback, min, max int) (int, error) {
	v, err := int64Value(name, int64(fallback), int64(min), int64(max))
	return int(v), err
}
func int64Value(name string, fallback, min, max int64) (int64, error) {
	raw := value(name, strconv.FormatInt(fallback, 10))
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", name, raw)
	}
	if v < min || v > max {
		return 0, fmt.Errorf("%s: must be between %d and %d", name, min, max)
	}
	return v, nil
}
func floating(name string, fallback, min, max float64) (float64, error) {
	raw := value(name, strconv.FormatFloat(fallback, 'g', -1, 64))
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) {
		return 0, fmt.Errorf("%s: invalid number %q", name, raw)
	}
	if v < min || v > max {
		return 0, fmt.Errorf("%s: must be between %g and %g", name, min, max)
	}
	return v, nil
}
func boolean(name string, fallback bool) (bool, error) {
	raw := value(name, strconv.FormatBool(fallback))
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q", name, raw)
	}
	return v, nil
}
func seconds(name string, fallback, min, max int) (time.Duration, error) {
	v, err := integer(name, fallback, min, max)
	return time.Duration(v) * time.Second, err
}
func milliseconds(name string, fallback, min, max int) (time.Duration, error) {
	v, err := integer(name, fallback, min, max)
	return time.Duration(v) * time.Millisecond, err
}
func minutes(name string, fallback, min, max int) (time.Duration, error) {
	v, err := integer(name, fallback, min, max)
	return time.Duration(v) * time.Minute, err
}
func hours(name string, fallback, min, max int) (time.Duration, error) {
	v, err := integer(name, fallback, min, max)
	return time.Duration(v) * time.Hour, err
}
func isHex(s string) bool {
	_, err := strconv.ParseUint(s[:16], 16, 64)
	if err != nil {
		return false
	}
	for i := 16; i < len(s); i += 16 {
		if _, err = strconv.ParseUint(s[i:i+16], 16, 64); err != nil {
			return false
		}
	}
	return true
}
