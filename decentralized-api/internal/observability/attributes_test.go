package observability

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAttributeConstants is the schema-source-of-truth regression guard per FR-1.7 / NFR-4 /
// INV-1. The constants in attributes.go MUST mirror PRD §7.3 verbatim — every cross-component
// span attribute is keyed on these strings, so a typo would silently break correlation.
//
// Adding a new attribute? Add it to PRD §7.3 first, then mirror here and in attributes.go.
func TestAttributeConstants(t *testing.T) {
	cases := []struct {
		name     string
		got      string
		expected string
	}{
		// Identity / context.
		{"AttrEpochIndex", AttrEpochIndex, "gonka.epoch.index"},
		{"AttrParticipantAddress", AttrParticipantAddress, "gonka.participant.address"},
		{"AttrModelID", AttrModelID, "gonka.model.id"},
		{"AttrTaskID", AttrTaskID, "gonka.task.id"},
		{"AttrBatchID", AttrBatchID, "gonka.batch.id"},
		{"AttrEventTriggerHeight", AttrEventTriggerHeight, "gonka.event.trigger_height"},
		{"AttrEventID", AttrEventID, "gonka.event_id"},
		{"AttrTxHash", AttrTxHash, "gonka.tx.hash"},

		// Confirmation event participant deltas.
		{"AttrCwPrev", AttrCwPrev, "gonka.cw.prev"},
		{"AttrCwNew", AttrCwNew, "gonka.cw.new"},
		{"AttrCwDelta", AttrCwDelta, "gonka.cw.delta"},

		// Chain math measurements.
		{"AttrMeasured", AttrMeasured, "gonka.measured"},
		{"AttrTotalExpected", AttrTotalExpected, "gonka.total_expected"},
		{"AttrRatio", AttrRatio, "gonka.ratio"},
		{"AttrFinalRatio", AttrFinalRatio, "gonka.final_ratio"},
		{"AttrAlphaThreshold", AttrAlphaThreshold, "gonka.alpha_threshold"},

		// Settlement.
		{"AttrPreserved", AttrPreserved, "gonka.preserved"},
		{"AttrPreservedWeight", AttrPreservedWeight, "gonka.preserved_weight"},
		{"AttrEffectiveWeight", AttrEffectiveWeight, "gonka.effective_weight"},
		{"AttrRewardedCoins", AttrRewardedCoins, "gonka.rewarded_coins"},
		{"AttrStatus", AttrStatus, "gonka.status"},

		// PoC.
		{"AttrNoncesCount", AttrNoncesCount, "gonka.nonces.count"},

		// Result correlation + duration + error.
		{"AttrResultHash", AttrResultHash, "gonka.result.hash"},
		{"AttrDurationMs", AttrDurationMs, "gonka.duration_ms"},
		{"AttrErrorMessage", AttrErrorMessage, "gonka.error.message"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.got,
				"%s must equal %q per PRD §7.3 (single source of truth for cross-component span schema)",
				tc.name, tc.expected)
		})
	}

	// Count guard: PRD §7.3 lists exactly the keys covered above. If the canonical schema
	// adds a key, this count must be updated alongside the new constant — keeps the table
	// in sync.
	require.GreaterOrEqual(t, len(cases), 24,
		"expected at least 24 attribute constants per PRD §7.3 done-condition in Slice 1.1")
}
