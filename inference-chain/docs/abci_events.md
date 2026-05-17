# ABCI events emitted by `x/inference`

This document is the **single source of truth** for the structured ABCI events that `x/inference` emits for off-chain consumption. The `chain-trace-indexer` (PR #4) subscribes to a node's Tendermint WebSocket, ingests these events, and translates each into an OpenTelemetry span via the rename / retype map in [`gonka/docs/observability/attributes.md`](../../docs/observability/attributes.md).

Attribute values follow the chain's existing convention: **all values are strings** at the ABCI layer (Cosmos SDK `EventAttribute` is `string` × `string`). Integers are serialized via `strconv.FormatInt`; decimals via `shopspring/decimal.String()`. The indexer parses known-numeric attributes back to `int64` per FR-4.12.

## Determinism rules (consensus-critical)

Every event listed below MUST be emitted deterministically:

- Identical attribute sets across all validators for the same block height.
- No `time.Now()`, no `rand`, no float math (use `shopspring/decimal`).
- No map iteration; iterate proto-slice-backed collections in their existing order.
- Emission is synchronous within the SDK message / module handler — never in a goroutine.

INV-5 (cross-PR invariant): `inference-chain/**/*.go` MUST NOT import `go.opentelemetry.io/otel`. Lint-enforced via `depguard` (`inference-chain/.golangci.yml`).

## Event types

### `gonka.confirmation_event.evaluated`

Emitted **once per confirmation PoC event processed** in `x/inference/module/confirmation_poc.go::evaluateConfirmation`. Marks that the event reached the chain's evaluator. The indexer uses this as the root span for the confirmation event in Tempo.

| attribute | type (chain side) | meaning |
|---|---|---|
| `epoch` | int (formatted string) | Epoch index. |
| `trigger_height` | int (formatted string) | Block height at which the event fired. |
| `event_id` | string | Composite stable identity: `fmt.Sprintf("%d-%d", event.EpochIndex, event.TriggerHeight)`. The indexer derives trace_id deterministically from this. |

### `gonka.confirmation_event.participant`

Emitted **once per participant whose `ConfirmationWeight` was visited** during evaluation, immediately after `foldEventReadings` returns. Iterates `epochGroupData.ValidationWeights` (proto-slice, deterministic order). The indexer turns each of these into a `chain.cw_update` child span linked to the parent `chain.confirmation_event.evaluate` via OTel `parent_span_id`.

`prev_cw` MUST be captured BEFORE `foldEventReadings` mutates the slice in place.

| attribute | type | meaning |
|---|---|---|
| `epoch` | int (formatted string) | Epoch index (same as parent event). |
| `trigger_height` | int (formatted string) | Same as parent event. |
| `event_id` | string | Same as parent event. |
| `participant` | string | Bech32 participant address. |
| `prev_cw` | int (formatted string) | `ConfirmationWeight` BEFORE this event's `foldEventReadings`. |
| `new_cw` | int (formatted string) | `ConfirmationWeight` AFTER this event's mutation. |
| `delta` | int (formatted string) | `new_cw - prev_cw` (computed by the chain — indexer maps as-is, never recomputes). |
| `measured` | int (formatted string) | Per-participant measured weight from `confirmationParticipants` aggregation. |
| `total_expected` | int (formatted string) | Sum `preserved[participant] + notPreserved[participant]` — the participant's pre-split total weight per `partitionWeightByPreservation`. |
| `preserved` | int (formatted string) | Preserved-side weight contribution from this event. |
| `ratio` | string (decimal) | Per-participant slashing ratio (the same value written to `participant.CurrentEpochStats.ConfirmationPoCRatio`). Serialize with `(shopspring/decimal).String()`. |
| `measured_model_set` | string (sorted CSV) | Comma-separated model_ids where this participant reported any measured weight (`RawWeight > 0`) for this event. Sorted alphabetically for byte-identity across validators. |
| `expected_model_set` | string (sorted CSV) | Comma-separated model_ids the participant was registered to serve this epoch (any model with at least one non-zero `PocWeight` ml-node). Sorted alphabetically. |

**Model-set-drift detector**: when `measured_model_set` is empty (or a strict subset) but `expected_model_set` is non-empty for the same participant in the same event, the slasher is about to fire against a worker that did their job for some models but not the ones the chain expected this event. Indexer/dashboards should alert on this — it's the exact symptom behind the epoch-266 incident, surfaced in real time.

### `gonka.failed_confirmation_poc.fire`

Emitted **once per participant** at the trip site that observes `calculations.ComputeStatus` returning `FailedConfirmationPoC` and flips `participant.Status` to `INACTIVE`. The exact emission site is the caller of `ComputeStatus` inside `x/inference/keeper/` (located via `grep -n FailedConfirmationPoC inference-chain/x/inference/keeper/`).

| attribute | type | meaning |
|---|---|---|
| `epoch` | int (formatted string) | Epoch in which the trip occurred. |
| `participant` | string | Bech32 participant address. |
| `final_ratio` | string (decimal) | Final ratio compared against `alpha_threshold`. |
| `alpha_threshold` | string (decimal) | Threshold value the ratio was compared against (from `pocParams`). |

### `gonka.poc.store_commit`

Emitted **once per persisted `PoCV2CommitEntry`** from `x/inference/keeper/msg_server_poc_v2_commit.go::PoCV2StoreCommit` after `persistPoCV2CommitUpdates` returns success. Closes the "did the worker actually submit?" forensic gap (today: `inferenced query inference all-poc-v2-store-commits ... --height ...` + manual group-by).

| attribute | type | meaning |
|---|---|---|
| `participant` | string | Bech32 submitter (msg.Creator). |
| `model_id` | string | Model id for this commit entry. |
| `count_delta` | int (formatted string) | New nonces added by this submission (the `countDelta` of the persisted update — accommodates incremental commits in the exchange window). |
| `root_hash` | string (hex) | Hex-encoded MMR root hash of the commit entry. |
| `poc_stage_start_height` | int (formatted string) | `msg.PocStageStartBlockHeight` — the PoC stage this commit belongs to. |
| `trigger_height` | int (formatted string) | Mirror of `poc_stage_start_height` for cross-event correlation with `confirmation_event.*`. |

### `gonka.poc.validation_vote`

Emitted **once per successfully stored `PoCValidation`** from `x/inference/keeper/msg_server_poc_validations_v2.go::SubmitPocValidationsV2` inside the per-validation loop after `SetPocValidationV2` returns success. Closes the "how did the validators vote?" forensic gap (today: `poc-v2-validations-for-stage` + manual reformat of N votes).

| attribute | type | meaning |
|---|---|---|
| `validator` | string | Bech32 validator (msg.Creator). |
| `participant` | string | Bech32 worker whose batch is being validated. |
| `model_id` | string | Model id for this vote. |
| `poc_stage_start_height` | int (formatted string) | `msg.PocStageStartBlockHeight`. |
| `trigger_height` | int (formatted string) | Mirror for correlation. |
| `validated_weight` | int (formatted string) | The validator's claimed weight contribution after sampling/check. |

### `gonka.settlement.reward_compute`

Emitted **once per participant at settlement** from `x/inference/keeper/accountsettle.go::Settle`, iterating the `amounts []*SettleResult` slice (deterministic — produced by `GetBitcoinSettleAmounts`). Emission happens in the caller, not inside `CalculateParticipantBitcoinRewards` (which has no `sdk.Context`); the function returns per-participant fields via an extended `SettleResult` struct.

| attribute | type | meaning |
|---|---|---|
| `epoch` | int (formatted string) | Settled epoch index. |
| `participant` | string | Bech32 participant address. |
| `preserved_weight` | int (formatted string) | Preserved weight at settlement. |
| `confirmation_weight` | int (formatted string) | `ConfirmationWeight` at settlement (final value used in reward math). |
| `effective_weight` | int (formatted string) | `effective_weight` after status adjustment (e.g. INACTIVE → zero). |
| `status` | string | Participant `Status` enum string at settlement time. |
| `rewarded_coins` | int (formatted string) | Coins awarded to this participant this settlement. |

## Event lifecycle and indexer mapping

```
chain (this module)                          chain-trace-indexer (PR #4)
─────────────────────────────────            ──────────────────────────────────
gonka.confirmation_event.evaluated   ───►    chain.confirmation_event.evaluate
                                             (root span, trace_id = SHA256("epoch:%d:event:%s")[:16])

gonka.confirmation_event.participant ───►    chain.cw_update
                                             (child span, parent_span_id from the .evaluated span)

gonka.failed_confirmation_poc.fire   ───►    chain.failed_confirmation_poc
                                             (standalone span, own trace_id)

gonka.settlement.reward_compute      ───►    chain.settlement.reward_compute
                                             (standalone span, own trace_id)
```

The indexer's exhaustive chain-attr → OTel-span-attr rename map (with int64 retyping) lives in PRD §4.4 FR-4.5 and is mirrored in the indexer's `internal/translate/` package when PR #4 is created.

## Adding a new event

1. Add the event type and full attribute table to this document FIRST.
2. Add a row per new attribute to [`gonka/docs/observability/attributes.md`](../../docs/observability/attributes.md) (the cross-component schema).
3. Implement the emission in the appropriate module / keeper file using the canonical pattern (`ctx.EventManager().EmitEvent(sdk.NewEvent(...))`).
4. Update the indexer's rename / retype map in PR #4 (or its branch if not yet merged).
5. Add a unit test asserting the event is emitted with the expected attribute set (mirror `x/inference/keeper/inference_status_events.go` as a pattern).

## Why ABCI events instead of OpenTelemetry SDK?

The chain is consensus-critical: every validator must execute deterministically, byte-identically. The OTel SDK introduces non-determinism (sampling, batching, network failures, time-based behavior). To get chain-side spans into the same trace tree as api / mlnode spans, the chain emits these deterministic ABCI events; an out-of-process indexer (`chain-trace-indexer`, PR #4) subscribes to them and translates each into an OTel span. This is the only safe way to surface chain-side state changes without risking consensus.
