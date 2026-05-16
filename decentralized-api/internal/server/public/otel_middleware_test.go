package public

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	// Blank-import otelecho so the contrib module stays pinned in go.mod / go.sum
	// from Slice 1.3 onward. The production import will be added by the
	// implementer when `applyInboundTracingMiddleware` is introduced in
	// server.go.
	_ "go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// installRecorder swaps in a fresh in-memory span recorder and W3C TraceContext
// propagator as the OTel globals, returning the recorder. Cleanup restores prior
// globals so tests are deterministic per t.Cleanup hooks.
//
// Used by Slice 1.3 (TC-A.1.1, TC-A.6.1) to assert that the production public
// server registers `otelecho.Middleware("decentralized-api")` on its Echo
// instance, producing one server span per inbound request and honoring upstream
// `traceparent` headers per W3C TraceContext.
func installRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	t.Cleanup(func() {
		_ = tp.Shutdown(t.Context())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	return sr
}

// newServerEchoForTracingTest constructs a minimal Echo instance and applies the
// production tracing-middleware registration as it will be wired by the
// public.NewServer constructor in Slice 1.3 implementation. The implementer
// MUST add a package-private helper:
//
//	func applyInboundTracingMiddleware(e *echo.Echo, serviceName string) {
//	    e.Use(otelecho.Middleware(serviceName))
//	}
//
// and call it from `NewServer` immediately after `e := echo.New()`. Until that
// helper exists, this test file will fail to compile, producing the TDD red
// signal at the package level. Once the helper is added, these tests assert
// the runtime behavior matches TC-A.1.1 and TC-A.6.1.
func newServerEchoForTracingTest(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	applyInboundTracingMiddleware(e, "decentralized-api")
	return e
}

// TestPublicServer_EmitsServerSpanPerRequest verifies TC-A.1.1 / FR-1.3 / INV-2:
// the production public server's middleware emits exactly one server-kind span
// per inbound HTTP request and the span's trace_id is valid (non-zero).
func TestPublicServer_EmitsServerSpanPerRequest(t *testing.T) {
	sr := installRecorder(t)

	e := newServerEchoForTracingTest(t)
	e.GET("/v1/test-endpoint", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/test-endpoint", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code, "handler must run and return 204")

	spans := sr.Ended()
	require.Len(t, spans, 1, "exactly one server span must be emitted per inbound request (TC-A.1.1)")

	span := spans[0]
	require.Equal(t, trace.SpanKindServer, span.SpanKind(),
		"span kind must be server-side for otelecho ingress middleware")
	require.True(t, span.SpanContext().TraceID().IsValid(),
		"server span must have a valid trace_id")
	require.True(t, span.SpanContext().SpanID().IsValid(),
		"server span must have a valid span_id")
}

// TestPublicServer_HonorsUpstreamTraceparent verifies TC-A.6.1 / TC-A.1.2 /
// FR-1.3 / NFR-6 / INV-2: when an upstream caller sends a W3C `traceparent`
// header, the public server's middleware adopts it — the server span shares
// the upstream trace_id and reports the upstream span_id as its parent.
func TestPublicServer_HonorsUpstreamTraceparent(t *testing.T) {
	sr := installRecorder(t)

	e := newServerEchoForTracingTest(t)
	e.GET("/v1/test-endpoint", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	// Canonical W3C traceparent value: version-traceid-spanid-flags (sampled).
	const (
		traceparentHdr = "00-00112233445566778899aabbccddeeff-0011223344556677-01"
		wantTraceIDHex = "00112233445566778899aabbccddeeff"
		wantParentHex  = "0011223344556677"
	)

	wantTraceID, err := trace.TraceIDFromHex(wantTraceIDHex)
	require.NoError(t, err, "test fixture: trace_id hex must parse")
	wantParentSpanID, err := trace.SpanIDFromHex(wantParentHex)
	require.NoError(t, err, "test fixture: parent span_id hex must parse")

	req := httptest.NewRequest(http.MethodGet, "/v1/test-endpoint", nil)
	req.Header.Set("traceparent", traceparentHdr)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	spans := sr.Ended()
	require.Len(t, spans, 1, "exactly one server span per request (TC-A.6.1)")
	span := spans[0]

	require.Equal(t, wantTraceID, span.SpanContext().TraceID(),
		"server span trace_id must match upstream traceparent (TC-A.6.1)")
	require.Equal(t, wantParentSpanID, span.Parent().SpanID(),
		"server span Parent().SpanID() must match upstream traceparent span_id (TC-A.6.1)")
	require.True(t, span.Parent().IsRemote(),
		"parent must be marked remote — it was extracted from inbound headers")
}

// TestPublicServer_NewTraceWhenNoTraceparent verifies TC-A.1.1 boundary:
// when no `traceparent` header is supplied, the middleware starts a fresh
// trace — the emitted server span has a valid non-zero trace_id and span_id.
func TestPublicServer_NewTraceWhenNoTraceparent(t *testing.T) {
	sr := installRecorder(t)

	e := newServerEchoForTracingTest(t)
	e.GET("/v1/test-endpoint", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/test-endpoint", nil)
	// Intentionally do NOT set traceparent.
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	spans := sr.Ended()
	require.Len(t, spans, 1, "exactly one server span per request")

	span := spans[0]
	require.True(t, span.SpanContext().TraceID().IsValid(),
		"server span must have a freshly-generated valid trace_id when no upstream traceparent is present")
	require.NotEqual(t, trace.TraceID{}, span.SpanContext().TraceID(),
		"trace_id must be non-zero")
	require.False(t, span.Parent().SpanID().IsValid(),
		"parent span_id must be zero/invalid: this is a root server span (no upstream)")
}
