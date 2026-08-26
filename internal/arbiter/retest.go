package arbiter

import (
	"fmt"

	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
)

// RetestSet is a deterministic, de-duplicated anomaly impact set. A new
// generation covers the affected boards and anchors while full history is kept.
type RetestSet struct {
	SourceBoard      string            `json:"source_board"`
	SourceGeneration domain.Generation `json:"source_generation"`
	Members          []string          `json:"members"`
	Generation       domain.Generation `json:"generation"`
	Complete         bool              `json:"complete"`
}

// BuildRetestSet creates the unique retest set for an anomaly. The members are
// the deterministic union of the seed board, adjacency-reachable boards, boards
// sharing a base zone and boards of the same mortar batch. The new generation
// is one higher than the source generation so rework always supersedes it.
func BuildRetestSet(seed string, sourceGen domain.Generation, boards []coverage.BoardPlacement, adjacency []coverage.AdjEdge) RetestSet {
	members := coverage.ImpactSet(seed, boards, adjacency)
	return RetestSet{
		SourceBoard:      seed,
		SourceGeneration: sourceGen,
		Members:          members,
		Generation:       sourceGen + 1,
		Complete:         false,
	}
}

// MarkComplete marks a retest set complete once every member has been
// re-inspected in the new generation. It reports an error when any member is
// missing from the provided completed set.
func (r RetestSet) MarkComplete(completed map[string]bool) (RetestSet, error) {
	for _, m := range r.Members {
		if !completed[m] {
			return r, fmt.Errorf("arbiter: retest member %s not completed", m)
		}
	}
	r.Complete = true
	return r, nil
}

// LateReceiptIsolated reports whether a device receipt whose board generation
// is older than the current effective generation must be isolated. A stale
// receipt never changes the current projection.
func LateReceiptIsolated(receiptGen, effectiveGen domain.Generation) bool {
	return receiptGen < effectiveGen
}
