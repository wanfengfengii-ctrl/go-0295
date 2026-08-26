package evidence

import (
	"fmt"

	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
)

// BoardRecord is the persisted per-board stage state.
type BoardRecord struct {
	BoardID    string
	Generation domain.Generation
	Stage      Stage
}

// Advance attempts a single forward stage transition for a board. It rejects a
// stale-generation write, an out-of-order (skipped, repeated or backward)
// transition, and any write whose board generation does not match the current
// effective generation.
func Advance(rec BoardRecord, req AdvanceRequest) (BoardRecord, error) {
	if rec.Generation != req.Generation {
		return rec, fmt.Errorf("evidence: generation mismatch: record %d request %d",
			rec.Generation, req.Generation)
	}
	if req.From != rec.Stage {
		return rec, fmt.Errorf("evidence: current stage %s does not match request from %s",
			rec.Stage, req.From)
	}
	if !rec.Stage.CanAdvanceTo(req.To) {
		return rec, fmt.Errorf("evidence: cannot advance from %s to %s",
			rec.Stage, req.To)
	}
	rec.Stage = req.To
	return rec, nil
}

// Anchor is a single installed anchor with a sequence number, a hole position
// in integer millimetres and an effective anchor depth.
type Anchor struct {
	Seq     int
	Hole    coverage.Point
	DepthMM int64
}

// ValidateAnchor validates a new anchor against the already-installed prefix and
// the locked geometry rules: the sequence number must be the next consecutive
// value, the hole must be unique, inside the board bounds with the minimum edge
// margin, and at least minSpacing from every existing hole. minDepth is the
// required effective anchor depth.
func ValidateAnchor(prev []Anchor, next Anchor, rect coverage.Rect, minEdgeMM, minSpacingMM, minDepth int64) error {
	if next.Seq != len(prev)+1 {
		return fmt.Errorf("evidence: anchor sequence must be consecutive (want %d got %d)",
			len(prev)+1, next.Seq)
	}
	if next.Hole.X < rect.X+minEdgeMM || next.Hole.X > rect.X+rect.W-minEdgeMM ||
		next.Hole.Y < rect.Y+minEdgeMM || next.Hole.Y > rect.Y+rect.H-minEdgeMM {
		return fmt.Errorf("evidence: anchor hole violates board edge margin")
	}
	for _, p := range prev {
		if p.Hole == next.Hole {
			return fmt.Errorf("evidence: duplicate anchor hole")
		}
		dx := p.Hole.X - next.Hole.X
		if dx < 0 {
			dx = -dx
		}
		dy := p.Hole.Y - next.Hole.Y
		if dy < 0 {
			dy = -dy
		}
		if dx*dx+dy*dy < minSpacingMM*minSpacingMM {
			return fmt.Errorf("evidence: anchor hole interference")
		}
	}
	if next.DepthMM < minDepth {
		return fmt.Errorf("evidence: anchor depth below minimum")
	}
	return nil
}
