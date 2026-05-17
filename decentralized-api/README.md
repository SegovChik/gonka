# decentralized-api (`dapi`)

The Gonka api node — HTTP/gRPC service that brokers between the chain, the operator, and the ml node. See [../CLAUDE.md](../CLAUDE.md) for the broader architecture and [../docs/observability/attributes.md](../docs/observability/attributes.md) for the canonical span attribute schema shared across api / mlnode / chain.

## Observability — distributed tracing

`dapi` ships with OpenTelemetry instrumentation. Tracing is **opt-in**: the SDK is statically linked into every build but emits no spans and starts no exporter goroutines unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

### Configuration (env vars)

| variable | required | default | meaning |
|---|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | to enable | (unset) | OTLP gRPC endpoint, e.g. `jaeger:4317` or `otelcol:4317`. **Unset or empty → no-op.** |
| `OTEL_SERVICE_NAME` | no | `decentralized-api` | Service name attached to every span (Jaeger / Tempo service dropdown). |
| `OTEL_TRACES_SAMPLER` | no | parent-based always-on (SDK default) | Sampler name, e.g. `parentbased_traceidratio` for production traffic. |
| `OTEL_TRACES_SAMPLER_ARG` | no | — | Sampler argument, e.g. `0.05` (5%) when paired with `parentbased_traceidratio`. |
| `OTEL_RESOURCE_ATTRIBUTES` | no | — | Comma-separated `key=value` resource attributes attached to every span, e.g. `gonka.network=mainnet,deployment.environment=prod`. |

### What gets emitted

Auto-instrumentation:
- One **server span** per inbound HTTP request to the public, admin, or ml Echo servers. Honors upstream W3C `traceparent` headers — a client-supplied traceparent makes the api's span a continuation of an existing trace.
- One **client span** per outbound HTTP call to the ml node (from both `mlnodeclient.NewNodeClient` and the inference proxy `s.httpClient`). Injects `traceparent` downstream so the ml node (when instrumented) chains under the api's spans.

Manual spans:
- `api.confirmation.fetch_assignments` — root span per confirmation PoC event. Attrs: `gonka.participant.address` (self), `gonka.event.trigger_height`.
- `api.confirmation.dispatch_to_mlnode` — one span per participant being validated. Attrs: `gonka.participant.address` (target), `gonka.event.trigger_height`, `gonka.model.id`. The downstream HTTP call to the ml node becomes its child.
- `api.inference.proxy_to_mlnode` — wraps the user-facing inference proxy. Attrs: `gonka.model.id`, `gonka.participant.address` (self), `gonka.task.id`. The downstream HTTP call to the ml node becomes its child.

The canonical attribute key list is in [../docs/observability/attributes.md](../docs/observability/attributes.md). New attributes go there first.

### Privacy (NFR-13)

Span attributes are restricted to structural identifiers and counts. The instrumentation **never** records:
- User prompt content, `messages[]`, request bodies, response bodies
- Authentication keys, signatures, or session tokens
- File paths or process-internal IP addresses

This is enforced by reviewer convention (no attribute is added without going through the schema doc) and verified by `TC-PII-1` in the QA suite.

### Minimal local trial

Run any OTLP receiver and point dapi at it:

```bash
# 1. Run Jaeger all-in-one (OTLP receiver on :4317, UI on :16686)
docker run -d --name jaeger \
    -e COLLECTOR_OTLP_ENABLED=true \
    -p 16686:16686 -p 4317:4317 \
    jaegertracing/all-in-one:1.62

# 2. Start dapi with the endpoint set
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 \
OTEL_SERVICE_NAME=decentralized-api \
./build/dapi

# 3. Generate traffic, then open http://localhost:16686
curl http://localhost:<public-port>/v1/status
```

### Running unwrapped (tracing off)

Just unset `OTEL_EXPORTER_OTLP_ENDPOINT`. No outbound network activity, no exporter goroutines, and the SDK installs a no-op TracerProvider — all instrumentation becomes effectively free (non-recording spans).

### Sampling for production

The SDK reads `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG` at init time. A reasonable default for production is:

```bash
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.05    # 5% of root traces, all children inherit
```

Heads-up: chain ABCI events (consumed by the `chain-trace-indexer` in PR #4) are **not** sampled — the indexer ingests every event so settlement / confirmation_weight diagnostic traces are always complete. Sampling only applies on the api / ml node side.
