package observability

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type TracingConfig struct {
	ServiceName, Endpoint, Sampler string
	SamplerArg                     float64
}

// NewTracerProvider configures the global provider. With an empty endpoint it
// installs an SDK provider with a never-sample policy and no exporter, so no
// collector or network connection is required.
func NewTracerProvider(ctx context.Context, cfg TracingConfig) (*sdktrace.TracerProvider, error) {
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attribute.String("service.name", cfg.ServiceName)))
	if err != nil {
		return nil, fmt.Errorf("observability: resource: %w", err)
	}
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(res), sdktrace.WithSampler(traceSampler(cfg.Sampler, cfg.SamplerArg))}
	if strings.TrimSpace(cfg.Endpoint) != "" {
		exporter, exportErr := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(cfg.Endpoint), otlptracegrpc.WithInsecure())
		if exportErr != nil {
			return nil, fmt.Errorf("observability: OTLP exporter: %w", exportErr)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	} else {
		options[1] = sdktrace.WithSampler(sdktrace.NeverSample())
	}
	tp := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func traceSampler(name string, ratio float64) sdktrace.Sampler {
	switch name {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	default:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}
}
