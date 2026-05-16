package observability

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TestInitTracer_NoEnv_InstallsNoopProvider verifies FR-1.1 / NFR-1 / INV-4 / TC-NOOP-1 (init subset):
// with OTEL_EXPORTER_OTLP_ENDPOINT unset, InitTracer installs a no-op TracerProvider as the
// global, returns no error, and produces non-recording spans.
func TestInitTracer_NoEnv_InstallsNoopProvider(t *testing.T) {
	// Unset by setting empty value via t.Setenv (Go test env scoping). To be safe we
	// also explicitly unset via the OS layer indirectly by setting an empty string,
	// which the implementation MUST treat as unset per FR-1.1 / TC-A.1.8.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx)
	require.NoError(t, err, "InitTracer must not error when endpoint is unset/empty")
	require.NotNil(t, shutdown, "shutdown function must be non-nil even in no-op mode")

	t.Cleanup(func() {
		_ = shutdown(context.Background())
	})

	// Acquire global tracer; start a span and assert it is non-recording (no-op).
	tracer := otel.Tracer("test")
	_, span := tracer.Start(ctx, "test-span")
	defer span.End()

	require.False(t, span.IsRecording(), "span must be non-recording when no-op provider is installed")
}

// TestInitTracer_NoEnv_NoNetworkActivity verifies NFR-1 / INV-4 / TC-NOOP-1 (no goroutine
// leak, no panic, callable shutdown returns nil error).
func TestInitTracer_NoEnv_NoNetworkActivity(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	before := runtime.NumGoroutine()

	ctx := context.Background()
	shutdown, err := InitTracer(ctx)
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	// Give any (incorrectly) spawned goroutines a moment to register.
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	// The no-op path MUST NOT start exporter goroutines. We allow a small slack for
	// scheduler noise but reject any growth ≥ 3 goroutines as a regression signal.
	require.LessOrEqual(t, after-before, 2,
		"no-op InitTracer should not spawn exporter goroutines (before=%d, after=%d)", before, after)

	// Shutdown must be callable and return nil error in the no-op path.
	require.NoError(t, shutdown(context.Background()), "no-op shutdown must return nil error")
}

// TestInitTracer_WithEnv_InstallsRealProvider verifies FR-1.1 / FR-1.2: with the endpoint env
// set, InitTracer constructs an OTLP-exporting TracerProvider and the shutdown function is
// safely callable. We never actually connect (no collector running) — exporter background
// retry is OK; shutdown must not panic.
func TestInitTracer_WithEnv_InstallsRealProvider(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_SERVICE_NAME", "test-svc")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx)
	require.NoError(t, err, "InitTracer must succeed when endpoint is set, even if collector is unreachable")
	require.NotNil(t, shutdown, "shutdown function must be non-nil when real provider is installed")

	t.Cleanup(func() {
		// Bounded shutdown context — exporter may need to drain or fail-fast.
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Error is acceptable (collector unreachable); only assert no panic.
		_ = shutdown(shutCtx)
	})
}

// TestInitTracer_WithEnv_RegistersW3CPropagator verifies FR-1.1: when the tracer is initialized
// with an endpoint, the W3C TraceContext propagator is registered as the global propagator,
// carrying `traceparent` and `tracestate` fields (RFC: W3C Trace Context).
func TestInitTracer_WithEnv_RegistersW3CPropagator(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_SERVICE_NAME", "test-svc")

	ctx := context.Background()
	shutdown, err := InitTracer(ctx)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(shutCtx)
	})

	prop := otel.GetTextMapPropagator()
	require.NotNil(t, prop, "global TextMapPropagator must be set after InitTracer")

	fields := prop.Fields()
	require.Contains(t, fields, "traceparent", "W3C TraceContext propagator must advertise the traceparent field")
	require.Contains(t, fields, "tracestate", "W3C TraceContext propagator must advertise the tracestate field")

	// Sanity: the propagator must actually be a composite or W3C TraceContext, never a no-op.
	// We assert this indirectly by confirming the field set is non-empty.
	require.NotEmpty(t, fields, "propagator must declare at least one carrier field")

	// Defense-in-depth assertion: ensure the propagator type is not the empty (no-op) one
	// that some default OTel SDKs install. The W3C TraceContext propagator concretely
	// returns the traceparent/tracestate field names; the no-op composite returns []string{}.
	var emptyComposite propagation.TextMapPropagator = propagation.NewCompositeTextMapPropagator()
	require.NotEqual(t, emptyComposite.Fields(), fields,
		"propagator must not be the empty (no-op) composite")
}

// TestInitTracer_Idempotent verifies that calling InitTracer twice does not panic.
// The second call may either no-op or replace the global provider — either is acceptable
// per the slice spec — but it MUST NOT panic.
func TestInitTracer_Idempotent(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	ctx := context.Background()

	shutdown1, err := InitTracer(ctx)
	require.NoError(t, err, "first InitTracer call must succeed")
	require.NotNil(t, shutdown1)

	// Second invocation must not panic and must not return an error.
	shutdown2, err := InitTracer(ctx)
	require.NoError(t, err, "second InitTracer call must succeed (idempotent)")
	require.NotNil(t, shutdown2, "second InitTracer must still return a callable shutdown")

	t.Cleanup(func() {
		_ = shutdown2(context.Background())
		_ = shutdown1(context.Background())
	})
}
