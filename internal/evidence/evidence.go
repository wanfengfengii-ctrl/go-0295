// Package evidence is the glue, board, anchor and curing evidence recorder
// ("布胶铺板锚固及养护证据记录器"). It implements the per-board state machine
// for base confirmation, mixing, glue, placement, joint, curing, drilling and
// anchoring, preserving logical-time evidence chains and rejecting out-of-order
// and stale-generation writes.
package evidence

import (
	"rockwool-facade-render-handover/internal/domain"
)

// Stage is the locked per-board dependency stage.
type Stage int

const (
	StageNone Stage = iota
	StageBaseAccepted
	StageMortarValid
	StageGluePrefix
	StagePlaced
	StageJoint
	StageCured
	StageAnchored
	StageInspected
)

// String returns the stable stage name.
func (s Stage) String() string {
	switch s {
	case StageBaseAccepted:
		return "base_accepted"
	case StageMortarValid:
		return "mortar_valid"
	case StageGluePrefix:
		return "glue_prefix"
	case StagePlaced:
		return "placed"
	case StageJoint:
		return "joint"
	case StageCured:
		return "cured"
	case StageAnchored:
		return "anchored"
	case StageInspected:
		return "inspected"
	default:
		return "none"
	}
}

// CanAdvanceTo reports whether moving from s to next is a single forward step
// in the locked dependency chain. Skipping, repeating or moving backwards is
// rejected so that no evidence is produced for an out-of-order write.
func (s Stage) CanAdvanceTo(next Stage) bool {
	return next == s+1
}

// AdvanceRequest requests a single stage transition.
type AdvanceRequest struct {
	OperationID string
	BoardID     string
	Generation  domain.Generation
	From, To    Stage
	LogicalTime domain.LogicalTime
}
