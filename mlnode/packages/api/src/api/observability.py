"""OpenTelemetry initialization for the mlnode FastAPI service.

Mirrors gonka/decentralized-api/internal/observability/{tracer,attributes}.go.
Tracing is opt-in: unset OTEL_EXPORTER_OTLP_ENDPOINT yields a no-op
TracerProvider (no exporter, no goroutines, non-recording spans). When the
endpoint is set, an OTLP gRPC exporter is installed and the W3C
TraceContext propagator becomes global so a `traceparent` header from the
api side stitches the api server span -> api client span -> mlnode server
span into a single trace.

The Attr* constants are the canonical Gonka span attribute keys; see
gonka/docs/observability/attributes.md (the single source of truth across
api / mlnode / chain / indexer).
"""
from __future__ import annotations

import os

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.propagate import set_global_textmap
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator


_ENV_ENDPOINT = "OTEL_EXPORTER_OTLP_ENDPOINT"
_ENV_SERVICE_NAME = "OTEL_SERVICE_NAME"
_DEFAULT_SERVICE_NAME = "mlnode-api"


def init_tracer(service_version: str | None = None) -> None:
    """Install the global TracerProvider and propagator.

    No-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset or empty.
    Safe to call multiple times - subsequent calls replace the global.
    """
    # Always set the W3C TraceContext propagator so an inbound `traceparent`
    # header is honored even in the no-op path (subsequent spans become
    # non-recording but the trace_id flows through).
    set_global_textmap(TraceContextTextMapPropagator())

    endpoint = os.environ.get(_ENV_ENDPOINT, "").strip()
    if not endpoint:
        # No-op path: leave the default (proxy) TracerProvider; tracer.start_span
        # produces non-recording spans, no exporter is wired.
        return

    service_name = os.environ.get(_ENV_SERVICE_NAME, "").strip() or _DEFAULT_SERVICE_NAME

    resource_attrs = {"service.name": service_name}
    if service_version:
        resource_attrs["service.version"] = service_version

    provider = TracerProvider(resource=Resource.create(resource_attrs))
    provider.add_span_processor(
        BatchSpanProcessor(
            OTLPSpanExporter(endpoint=endpoint, insecure=True),
        ),
    )
    trace.set_tracer_provider(provider)


# Module-level tracer for handler-level instrumentation in PR #2 slices 2.4+.
tracer = trace.get_tracer("mlnode")


# Canonical Gonka span attribute keys - mirror of
# gonka/decentralized-api/internal/observability/attributes.go.
# See gonka/docs/observability/attributes.md for the canonical schema.
ATTR_EPOCH_INDEX = "gonka.epoch.index"
ATTR_PARTICIPANT_ADDRESS = "gonka.participant.address"
ATTR_MODEL_ID = "gonka.model.id"
ATTR_TASK_ID = "gonka.task.id"
ATTR_BATCH_ID = "gonka.batch.id"
ATTR_EVENT_TRIGGER_HEIGHT = "gonka.event.trigger_height"
ATTR_EVENT_ID = "gonka.event_id"
ATTR_TX_HASH = "gonka.tx.hash"

ATTR_CW_PREV = "gonka.cw.prev"
ATTR_CW_NEW = "gonka.cw.new"
ATTR_CW_DELTA = "gonka.cw.delta"

ATTR_MEASURED = "gonka.measured"
ATTR_TOTAL_EXPECTED = "gonka.total_expected"
ATTR_RATIO = "gonka.ratio"
ATTR_FINAL_RATIO = "gonka.final_ratio"
ATTR_ALPHA_THRESHOLD = "gonka.alpha_threshold"

ATTR_PRESERVED = "gonka.preserved"
ATTR_PRESERVED_WEIGHT = "gonka.preserved_weight"
ATTR_EFFECTIVE_WEIGHT = "gonka.effective_weight"
ATTR_REWARDED_COINS = "gonka.rewarded_coins"
ATTR_STATUS = "gonka.status"

ATTR_NONCES_COUNT = "gonka.nonces.count"
ATTR_RESULT_HASH = "gonka.result.hash"
ATTR_DURATION_MS = "gonka.duration_ms"
ATTR_ERROR_MESSAGE = "gonka.error.message"
