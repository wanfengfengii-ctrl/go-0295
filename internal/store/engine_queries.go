package store

import (
	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
	"rockwool-facade-render-handover/internal/ledger"
)

// GetTask returns the persisted task or a not-found failure.
func (e *Engine) GetTask(id string) (domain.FacadeTask, error) {
	var task domain.FacadeTask
	err := e.db.View(func(tx *Tx) error {
		ok, err := tx.GetJSON(BucketTasks, id, &task)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(id)
		}
		return nil
	})
	return task, err
}

// CoverageView is the deterministic coverage projection for a task.
type CoverageView struct {
	Layout coverage.Layout           `json:"layout"`
	Boards []coverage.BoardPlacement `json:"boards"`
	Digest string                    `json:"digest"`
}

// GetCoverage returns the locked layout, boards and deterministic digest.
func (e *Engine) GetCoverage(id string) (CoverageView, error) {
	var out CoverageView
	err := e.db.View(func(tx *Tx) error {
		tl, err := loadLayout(tx, id)
		if err != nil {
			return err
		}
		out.Layout = tl.Layout
		out.Boards = tl.Boards
		out.Digest = coverage.CoverageDigest(tl.Boards)
		return nil
	})
	return out, err
}

// LedgerView is the mortar conservation projection.
type LedgerView struct {
	Mortar       *ledger.MortarState `json:"mortar"`
	Conservation bool                `json:"conservation"`
	GlueTotal    int64               `json:"glue_total"`
}

// GetLedger returns the mortar state and conservation status.
func (e *Engine) GetLedger(id string) (LedgerView, error) {
	var out LedgerView
	err := e.db.View(func(tx *Tx) error {
		m, err := loadMortar(tx, id)
		if err != nil {
			return err
		}
		out.Mortar = m
		out.Conservation = m.CheckConservation()
		out.GlueTotal = m.GlueTotal()
		return nil
	})
	return out, err
}

// EvidenceView is the per-board evidence projection.
type EvidenceView struct {
	Boards  map[string]evidence.BoardRecord      `json:"boards"`
	Anchors map[string][]evidence.Anchor         `json:"anchors"`
	Curing  map[string][]evidence.CuringInterval `json:"curing"`
}

// GetEvidence returns the board stages, anchors and curing intervals. Each
// rework generation persists its own isolated evidence chain under a
// generation-scoped key; here they are projected back onto the stable board
// key, with the highest surviving generation's record winning so the current
// effective state is visible while earlier generations stay on disk as
// immutable history.
func (e *Engine) GetEvidence(id string) (EvidenceView, error) {
	out := EvidenceView{Boards: map[string]evidence.BoardRecord{}, Anchors: map[string][]evidence.Anchor{}, Curing: map[string][]evidence.CuringInterval{}}
	err := e.db.View(func(tx *Tx) error {
		anchorGen := map[string]domain.Generation{}
		curingGen := map[string]domain.Generation{}
		_ = tx.ForEach(BucketStages, func() any { return &evidence.BoardRecord{} }, func(k string, v any) error {
			base, _, ok := splitGenKey(k)
			if !ok {
				return nil
			}
			rec := *v.(*evidence.BoardRecord)
			if cur, exists := out.Boards[base]; !exists || rec.Generation > cur.Generation {
				out.Boards[base] = rec
			}
			return nil
		})
		_ = tx.ForEach(BucketAnchors, func() any { return &[]evidence.Anchor{} }, func(k string, v any) error {
			base, gen, ok := splitGenKey(k)
			if !ok {
				return nil
			}
			anchors := *v.(*[]evidence.Anchor)
			if prev, exists := anchorGen[base]; !exists || gen > prev {
				anchorGen[base] = gen
				out.Anchors[base] = anchors
			}
			return nil
		})
		_ = tx.ForEach(BucketCuring, func() any { return &[]evidence.CuringInterval{} }, func(k string, v any) error {
			base, gen, ok := splitGenKey(k)
			if !ok {
				return nil
			}
			intervals := *v.(*[]evidence.CuringInterval)
			if prev, exists := curingGen[base]; !exists || gen > prev {
				curingGen[base] = gen
				out.Curing[base] = intervals
			}
			return nil
		})
		return nil
	})
	return out, err
}

// GetRetests returns all persisted retest sets deterministically ordered.
func (e *Engine) GetRetests(id string) ([]arbiter.RetestSet, error) {
	var out []arbiter.RetestSet
	err := e.db.View(func(tx *Tx) error {
		return tx.ForEach(BucketRetests, func() any { return &arbiter.RetestSet{} }, func(k string, v any) error {
			out = append(out, *v.(*arbiter.RetestSet))
			return nil
		})
	})
	return out, err
}

// GetTerminal returns the terminal decision if one exists.
func (e *Engine) GetTerminal(id string) (*arbiter.TerminalDecision, error) {
	var out *arbiter.TerminalDecision
	err := e.db.View(func(tx *Tx) error {
		var dec arbiter.TerminalDecision
		if ok, _ := tx.GetJSON(BucketTerminal, id, &dec); ok {
			out = &dec
		}
		return nil
	})
	return out, err
}
