package observability

// Canonical Gonka span attribute keys — single source of truth, mirrored by
// the mlnode constants (PR #2), the chain ABCI attribute names (PR #3), and
// the indexer rename map (PR #4). Changes require updating
// gonka/docs/observability/attributes.md AND every cross-component mirror.
// See PRD §7.3.
const (
	// Identity / context.
	AttrEpochIndex         = "gonka.epoch.index"
	AttrParticipantAddress = "gonka.participant.address"
	AttrModelID            = "gonka.model.id"
	AttrTaskID             = "gonka.task.id"
	AttrBatchID            = "gonka.batch.id"
	AttrEventTriggerHeight = "gonka.event.trigger_height"
	AttrEventID            = "gonka.event_id"
	AttrTxHash             = "gonka.tx.hash"

	// Confirmation event participant deltas.
	AttrCwPrev  = "gonka.cw.prev"
	AttrCwNew   = "gonka.cw.new"
	AttrCwDelta = "gonka.cw.delta"

	// Chain math measurements.
	AttrMeasured       = "gonka.measured"
	AttrTotalExpected  = "gonka.total_expected"
	AttrRatio          = "gonka.ratio"
	AttrFinalRatio     = "gonka.final_ratio"
	AttrAlphaThreshold = "gonka.alpha_threshold"

	// Settlement.
	AttrPreserved       = "gonka.preserved"
	AttrPreservedWeight = "gonka.preserved_weight"
	AttrEffectiveWeight = "gonka.effective_weight"
	AttrRewardedCoins   = "gonka.rewarded_coins"
	AttrStatus          = "gonka.status"

	// PoC.
	AttrNoncesCount = "gonka.nonces.count"

	// Result correlation + duration + error.
	AttrResultHash   = "gonka.result.hash"
	AttrDurationMs   = "gonka.duration_ms"
	AttrErrorMessage = "gonka.error.message"
)
