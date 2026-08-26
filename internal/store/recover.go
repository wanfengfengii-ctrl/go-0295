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

// storedLease is the recovery checker's view of a persisted lease: only the
// fields needed to decide whether two leases for the same resource were ever
// concurrently active.
type storedLease struct {
	Holder   string
	Acquired domain.LogicalTime
	Expires  domain.LogicalTime
}

// leaseWindowOverlap reports whether two leases' active windows [Acquired,
// Expires) overlap in logical time. A lease is active only during its
// half-open window (matching ledger.Lease.ActiveAt, which excludes t==Expires),
// so an expired lease (e.g. taken at logical time 0 and long elapsed) never
// collides with a fresh lease taken later on the same resource.
func leaseWindowOverlap(a, b storedLease) bool {
	return a.Acquired < b.Expires && b.Acquired < a.Expires
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
		//
		// The invariant is that no two leases for the same resource hold
		// concurrently. A lease is "active" only during its time window
		// [Acquired, Expires); an expired lease (e.g. taken at logical time 0
		// and long since elapsed) must not collide with a fresh lease taken on
		// the same resource much later. Therefore two leases conflict only when
		// their active windows overlap, not merely because both carry a token.
		byResource := map[string][]storedLease{}
		_ = tx.ForEach(BucketLeases, func() any { return &ledger.Lease{} }, func(k string, val any) error {
			l := val.(*ledger.Lease)
			if l.Token == "" {
				return nil
			}
			resource := string(l.Kind) + "/" + l.Number
			byResource[resource] = append(byResource[resource], storedLease{
				Holder:   l.Holder,
				Acquired: l.Acquired,
				Expires:  l.Expires,
			})
			return nil
		})
		for resource, ls := range byResource {
			for i := 0; i < len(ls); i++ {
				for j := i + 1; j < len(ls); j++ {
					if leaseWindowOverlap(ls[i], ls[j]) {
						v.Violations = append(v.Violations, fmt.Sprintf(
							"duplicate active lease %s held by %s and %s",
							resource, ls[i].Holder, ls[j].Holder))
					}
				}
			}
		}
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
