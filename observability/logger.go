package observability

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger builds a service-labelled zap logger using the documented level
// and json/text formats.
func NewLogger(serviceName string, options ...string) (*zap.Logger, error) {
	level, format := "info", "json"
	if len(options) > 0 && options[0] != "" {
		level = options[0]
	}
	if len(options) > 1 && options[1] != "" {
		format = options[1]
	}
	var parsed zapcore.Level
	if err := parsed.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("observability: log level: %w", err)
	}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeDuration = zapcore.StringDurationEncoder
	var encoder zapcore.Encoder
	switch format {
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	case "text":
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	default:
		return nil, fmt.Errorf("observability: unsupported log format %q", format)
	}
	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), parsed)
	return zap.New(core).With(zap.String("service", serviceName)), nil
}

// Sync flushes a logger, ignoring the harmless stdout/stderr EINVAL seen on
// some platforms and containers.
func Sync(logger *zap.Logger) {
	if logger == nil {
		return
	}
	_ = logger.Sync()
}
