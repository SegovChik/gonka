# Gonka observability — span attribute schema

This document is the **single source of truth** for `gonka.*` span attribute keys used by the distributed tracing rollout. All four components reference it:

| Component | Mirrored as |
|---|---|
| `decentralized-api` (Go) | constants in `decentralized-api/internal/observability/attributes.go` |
| `mlnode` (Python) | constants in `mlnode/packages/api/src/api/observability.py` |
| `inference-chain` (Go) | raw ABCI attribute keys emitted in `x/inference/...` (chain side emits unprefixed keys; the indexer applies the prefix) |
| `chain-trace-indexer` (Go) | rename + retype map in `internal/translate/` |

**Adding or renaming an attribute requires updating this document first**, then every component mirror in the same PR. Mismatch breaks cross-component correlation silently.

## Attribute keys

### Identity / context

| key | type | meaning |
|---|---|---|
| `gonka.epoch.index` | int64 | Epoch number. |
| `gonka.participant.address` | string | Bech32 participant (host) address. |
| `gonka.model.id` | string | Model identifier (e.g. `Qwen/Qwen3-235B-A22B-Instruct-2507-FP8`). |
| `gonka.task.id` | string | Confirmation task ID. |
| `gonka.batch.id` | string | Batch identifier under confirmation. |
| `gonka.event.trigger_height` | int64 | Block height at which a chain event fired. |
| `gonka.event_id` | string | Composite chain event identity: `"<epoch>-<trigger_height>"`. Used as the deterministic seed for the indexer's trace_id derivation (`SHA256("epoch:%d:event:%s")[:16]`). |
| `gonka.tx.hash` | string | Transaction hash, when relevant (e.g. on api evidence-submission spans). |

### Confirmation event — per-participant deltas

Emitted by the chain (`gonka.confirmation_event.participant` ABCI event) and translated by the indexer into `chain.cw_update` child spans linked via `parent_span_id` to the parent `chain.confirmation_event.evaluate` span.

| key | type | meaning |
|---|---|---|
| `gonka.cw.prev` | int64 | Participant's `confirmation_weight` before the update. |
| `gonka.cw.new` | int64 | Participant's `confirmation_weight` after the update. |
| `gonka.cw.delta` | int64 | `new − prev`. Emitted directly by the chain; indexer maps as-is and does NOT recompute. |

### Chain math measurements

| key | type | meaning |
|---|---|---|
| `gonka.measured` | int64 | Measured weight observed by the chain math for this participant. |
| `gonka.total_expected` | int64 | Expected weight per the chain math. |
| `gonka.ratio` | string | `measured / total_expected` as a `shopspring/decimal` string (chain uses decimal — never float). |
| `gonka.final_ratio` | string | Final ratio compared against `alpha_threshold` at the failed-PoC trip site. |
| `gonka.alpha_threshold` | string | Threshold for participant deactivation (decimal string). |

### Settlement (bitcoin rewards)

Emitted by the chain (`gonka.settlement.reward_compute` ABCI event) from `accountsettle.go::Settle` after `CalculateParticipantBitcoinRewards` produces the per-participant amounts.

| key | type | meaning |
|---|---|---|
| `gonka.preserved` | int64 | Per-event preserved measurement contribution from `foldEventReadings`. |
| `gonka.preserved_weight` | int64 | Settlement-side preserved weight. |
| `gonka.effective_weight` | int64 | Settlement-side effective weight after status adjustment. |
| `gonka.rewarded_coins` | int64 | Coins awarded this settlement to this participant. |
| `gonka.status` | string | Participant status enum (e.g. `ACTIVE`, `INACTIVE`). Stable lowercase or enum-string form per chain convention. |

### PoC

| key | type | meaning |
|---|---|---|
| `gonka.nonces.count` | int64 | Number of nonces computed in a PoC span. |

### Result correlation, duration, error

| key | type | meaning |
|---|---|---|
| `gonka.result.hash` | string | Opaque hash of the mlnode result (e.g. SHA256 of canonicalized payload). Used for correlation — **NEVER** the result body. |
| `gonka.duration_ms` | int64 | Operation duration in milliseconds; used on mlnode handler spans where a precise number is more useful than relying on span start/end. |
| `gonka.error.message` | string | Error **class identifier** only — e.g. `"Timeout"`, `"ModelUnavailable"`, `"InvalidRequest"`. **NEVER** stack traces, raw error strings with PII, or file paths (NFR-13). |

## What MUST NOT appear as a span attribute

Per NFR-13 (no PII):
- User prompt content (`messages[].content`, `prompt`, raw request body)
- Model response content (response body, generated text)
- Per-request `prompt_tokens` / `completion_tokens` arrays carrying text
- File paths, IP addresses, raw HTTP request bodies, or anything operator-identifying beyond the bech32 participant address (which is already public chain state)

The auto-instrumentation libraries (`otelhttp`, `FastAPIInstrumentor`) default to body-capture OFF. Do not enable body capture.

## Chain ABCI attribute names

Chain code emits ABCI attributes with **unprefixed** keys (`epoch`, `prev_cw`, `participant`, …) for emission ergonomics. The indexer applies the `gonka.*` prefix and the int64 retyping per the canonical table when translating into OTel spans. The exhaustive chain attr → OTel attr map lives in [`inference-chain/docs/abci_events.md`](../../inference-chain/docs/abci_events.md) (created in PR #3, Slice 3.1) and the indexer mapping table in PR #4 Slice 4.3.
