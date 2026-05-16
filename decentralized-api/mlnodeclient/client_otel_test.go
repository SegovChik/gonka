package mlnodeclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// installSpanRecorder swaps the global TracerProvider and TextMapPropagator
// for an in-memory recorder + W3C TraceContext for the duration of the test.
// Returns the recorder so the caller can read emitted spans.
func installSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	return sr
}

// TestNewNodeClient_ClientSpanPerOutboundRequest verifies FR-1.5 / TC-A.1.2:
// every outbound HTTP call from the mlnodeclient HTTP transport produces a
// client-kind span. The slice's done-condition relies on otelhttp.NewTransport
// being installed as the Transport on the http.Client returned by NewNodeClient.
func TestNewNodeClient_ClientSpanPerOutboundRequest(t *testing.T) {
	sr := installSpanRecorder(t)

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(stub.Close)

	c := NewNodeClient(stub.URL, stub.URL)

	require.NoError(t, c.Stop(context.Background()))

	spans := sr.Ended()
	require.NotEmpty(t, spans, "outbound call must produce at least one span")

	var sawClient bool
	for _, s := range spans {
		if s.SpanKind().String() == "client" {
			sawClient = true
			break
		}
	}
	require.True(t, sawClient, "expected a client-kind span from otelhttp transport; got: %+v", spans)
}

// TestNewNodeClient_PropagatesTraceparent verifies FR-1.5 / INV-2 / TC-A.6.1
// (downstream propagation half): when an outbound call is made from inside an
// active trace context, the transport injects a W3C `traceparent` header on
// the request so the receiver (mlnode in PR #2) can continue the trace.
func TestNewNodeClient_PropagatesTraceparent(t *testing.T) {
	_ = installSpanRecorder(t)

	var seenHeader string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(stub.Close)

	tracer := otel.Tracer("client-otel-test")
	ctx, parent := tracer.Start(context.Background(), "parent")
	defer parent.End()

	c := NewNodeClient(stub.URL, stub.URL)
	require.NoError(t, c.Stop(ctx))

	require.NotEmpty(t, seenHeader, "outbound request must carry a W3C traceparent header")
	expectedTraceHex := parent.SpanContext().TraceID().String()
	require.Contains(t, seenHeader, expectedTraceHex,
		"traceparent (%q) must include the parent trace_id %q", seenHeader, expectedTraceHex)
}
