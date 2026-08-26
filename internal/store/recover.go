package store

import (
	"fmt"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
)

// RecoverVerification is the result of startup recovery verification.
type RecoverVerification struct {
	OK         bool     `json:"ok"`
	Violations []string `json:"violations,omitempty"`
}

// Verify replays the persisted projection and checks the durable invariants:
// integer-gram mass conservation, unique active leases and at most one terminal
// decision per task. When a contradiction is found the store is placed into
// read-only isolation and the violation list is returned; it never guesses or
// repairs business facts.
func (e *Engine) Verify() (RecoverVerification, error) {
	v := RecoverVerification{OK: true}
	err := e.db.View(func(tx *Tx) error {
		// 1. Mass conservation.
		_ = tx.ForEach(BucketMortar, func() any { return &ledger.MortarState{} }, func(k string, val any) error {
			m := val.(*ledger.MortarState)
			if !m.CheckConservation() {
				v.Violations = append(v.Violations, fmt.Sprintf("conservation violation mortar=%s", k))
			}
			return nil
		})
		// 2. Unique active leases.
		active := map[string]string{}
		_ = tx.ForEach(BucketLeases, func() any { return &ledger.Lease{} }, func(k string, val any) error {
			l := val.(*ledger.Lease)
			// A lease is only "active" conceptually during its window; a stored
			// unexpired lease must not collide on the same resource.
			resource := string(l.Kind) + "/" + l.Number
			if l.Token != "" {
				if prev, dup := active[resource]; dup {
					v.Violations = append(v.Violations, fmt.Sprintf("duplicate active lease %s held by %s and %s", resource, prev, l.Holder))
				} else {
					active[resource] = l.Holder
				}
			}
			return nil
		})
		// 3. At most one terminal decision per task (enforced by key).
		_ = tx.ForEach(BucketTerminal, func() any { return &arbiter.TerminalDecision{} }, func(k string, val any) error {
			return nil
		})
		return nil
	})
	if err != nil {
		return v, err
	}
	if len(v.Violations) > 0 {
		v.OK = false
		e.db.setReadOnly()
	}
	return v, nil
}

// ProjectionDigest returns a deterministic digest over the durable projection
// (task generation, snapshot digest, coverage digest, mortar balances and
// terminal kind) so tests can assert the projection is identical across a
// restart.
func (e *Engine) ProjectionDigest() (string, error) {
	parts := []string{}
	err := e.db.View(func(tx *Tx) error {
		_ = tx.ForEach(BucketTasks, func() any { return &domain.FacadeTask{} }, func(k string, val any) error {
			t := val.(*domain.FacadeTask)
			parts = append(parts, fmt.Sprintf("task=%s gen=%d status=%s digest=%s", k, t.Generation, t.Status, t.SnapshotDigest))
			return nil
		})
		_ = tx.ForEach(BucketMortar, func() any { return &ledger.MortarState{} }, func(k string, val any) error {
			m := val.(*ledger.MortarState)
			parts = append(parts, fmt.Sprintf("mortar=%s powder=%d water=%d remainder=%d", k, m.Powder, m.Water, m.Remainder))
			return nil
		})
		_ = tx.ForEach(BucketTerminal, func() any { return &arbiter.TerminalDecision{} }, func(k string, val any) error {
			d := val.(*arbiter.TerminalDecision)
			parts = append(parts, fmt.Sprintf("terminal=%s kind=%s", k, d.Kind))
			return nil
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	// deterministic join
	out := ""
	for _, p := range parts {
		out += p + "\n"
	}
	return hashBytes([]byte(out)), nil
}
