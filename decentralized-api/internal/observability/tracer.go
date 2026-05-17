package observability

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	envEndpoint    = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envServiceName = "OTEL_SERVICE_NAME"

	defaultServiceName = "decentralized-api"
)

// InitTracer installs the global OpenTelemetry TracerProvider and the W3C
// TraceContext + Baggage propagator. When OTEL_EXPORTER_OTLP_ENDPOINT is
// unset or empty, a no-op TracerProvider is installed — zero export, zero
// goroutines, safe to call from production binaries. Otherwise an OTLP gRPC
// exporter is configured per OTel env conventions; sampling honors
// OTEL_TRACES_SAMPLER and OTEL_TRACES_SAMPLER_ARG via the SDK defaults.
// The returned shutdown function flushes any pending spans; it is always
// safe to call exactly once and is a no-op in the disabled path.
func InitTracer(ctx context.Context) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	endpoint := os.Getenv(envEndpoint)
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	serviceName := os.Getenv(envServiceName)
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
