package catalog

import (
	"fmt"

	"rockwool-facade-render-handover/internal/domain"
)

// Thresholds captures every fixed-point numeric threshold fixed at lock time.
// All values are signed integers with an explicit scale; zero or negative
// scales are rejected, and the catalogue validates sign constraints before any
// arithmetic ever uses them.
type Thresholds struct {
	// FixedScale is the shared fixed-point scale (must be > 0).
	FixedScale int64

	// Anchor geometry, in integer millimetres.
	MinEdgeMM    int64 // minimum board edge distance for an anchor
	MinSpacingMM int64 // minimum distance between adjacent anchors

	// Mortar usage.
	WaterRatioNum int64 // water : powder ratio numerator
	WaterRatioDen int64 // water : powder ratio denominator
	UnitAreaGlue  int64 // glue grams per scaled area unit

	// Curing, in logical seconds.
	MinCureSecs int64

	// Inspection strengths (scaled by FixedScale).
	MinPullStrength int64
	MinBondStrength int64
}

// Validate rejects illegal fixed-point thresholds: non-positive scale,
// non-positive denominators, negative minima or negative ratios.
func (t Thresholds) Validate() error {
	if t.FixedScale <= 0 {
		return ErrBadFixedScale
	}
	if t.WaterRatioDen <= 0 {
		return fmt.Errorf("catalog: water ratio denominator must be positive")
	}
	if t.WaterRatioNum < 0 {
		return fmt.Errorf("catalog: water ratio numerator must be non-negative")
	}
	if t.MinEdgeMM < 0 || t.MinSpacingMM < 0 || t.UnitAreaGlue < 0 ||
		t.MinCureSecs < 0 || t.MinPullStrength < 0 || t.MinBondStrength < 0 {
		return fmt.Errorf("catalog: thresholds must be non-negative")
	}
	return nil
}

// StaleDigestError builds a deterministic stale-digest failure for a summary
// mismatch between the requested summary and the current snapshot digest.
func StaleDigestError(want, got string) *domain.Failure {
	return &domain.Failure{
		Code: domain.CodeStaleDigest,
		Reasons: []domain.Reason{{
			Code:   domain.CodeStaleDigest,
			Field:  "snapshot",
			Detail: fmt.Sprintf("want %s got %s", want, got),
		}},
		Retryable: false,
	}
}
