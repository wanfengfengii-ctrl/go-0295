package store

import (
	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
)

// AcquireLeaseRequest requests a lease for a single resource.
type AcquireLeaseRequest struct {
	Kind        ledger.ResourceKind
	Number      string
	Holder      string
	LogicalTime domain.LogicalTime
}

// AcquireLease atomically acquires a lease for a resource, rejecting a busy
// resource with a deterministic conflict.
func (e *Engine) AcquireLease(id string, req AcquireLeaseRequest) (ledger.Lease, error) {
	var out ledger.Lease
	err := e.db.Update(func(tx *Tx) error {
		var task domain.FacadeTask
		if ok, _ := tx.GetJSON(BucketTasks, id, &task); !ok {
			return notFound(id)
		}
		key := ledger.LeaseKey{Kind: req.Kind, Number: req.Number}
		var existing []ledger.Lease
		_ = tx.ForEach(BucketLeases, func() any { return &ledger.Lease{} }, func(k string, v any) error {
			l := v.(*ledger.Lease)
			if l.Kind == req.Kind && l.Number == req.Number {
				existing = append(existing, *l)
			}
			return nil
		})
		if ledger.FindConflict(existing, key, req.LogicalTime) {
			return &domain.Failure{Code: domain.CodeLeaseBusy,
				Reasons: []domain.Reason{{Code: domain.CodeLeaseBusy, Detail: string(req.Kind)}}}
		}
		lease, err := ledger.AcquireLease(key, req.Holder, req.LogicalTime, ledger.LeaseDuration)
		if err != nil {
			return &domain.Failure{Code: domain.CodeInvalid, Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: err.Error()}}}
		}
		if err := tx.PutJSON(BucketLeases, leaseKey(id, lease.Kind, lease.Number), lease); err != nil {
			return err
		}
		out = lease
		return nil
	})
	return out, err
}

// RenewLeaseRequest renews a held lease.
type RenewLeaseRequest struct {
	Kind        ledger.ResourceKind
	Number      string
	Token       string
	LogicalTime domain.LogicalTime
}

// RenewLease extends a lease when the token matches and it is unexpired.
func (e *Engine) RenewLease(id string, req RenewLeaseRequest) (ledger.Lease, error) {
	var out ledger.Lease
	err := e.db.Update(func(tx *Tx) error {
		var l ledger.Lease
		ok, err := tx.GetJSON(BucketLeases, leaseKey(id, req.Kind, req.Number), &l)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(string(req.Kind))
		}
		renewed, err := ledger.RenewLease(l, req.Token, req.LogicalTime, ledger.LeaseDuration)
		if err != nil {
			return &domain.Failure{Code: domain.CodeLeaseExpired,
				Reasons: []domain.Reason{{Code: domain.CodeLeaseExpired, Detail: err.Error()}}}
		}
		if err := tx.PutJSON(BucketLeases, leaseKey(id, req.Kind, req.Number), renewed); err != nil {
			return err
		}
		out = renewed
		return nil
	})
	return out, err
}

// Retest builds and persists the unique retest set for an anomaly seed.
func (e *Engine) Retest(id string, seed string, sourceGen domain.Generation) (arbiter.RetestSet, error) {
	var out arbiter.RetestSet
	err := e.db.Update(func(tx *Tx) error {
		layout, err := loadLayout(tx, id)
		if err != nil {
			return err
		}
		var existing arbiter.RetestSet
		if ok, _ := tx.GetJSON(BucketRetests, seed, &existing); ok &&
			existing.SourceGeneration == sourceGen {
			out = existing
			return nil
		}
		out = arbiter.BuildRetestSet(seed, sourceGen, layout.Boards, layout.Layout.Adjacency)
		return tx.PutJSON(BucketRetests, seed, out)
	})
	return out, err
}

// NewGeneration advances the task generation for a rework round and marks the
// matching retest set complete.
func (e *Engine) NewGeneration(id string) (domain.Generation, error) {
	var gen domain.Generation
	err := e.db.Update(func(tx *Tx) error {
		var task domain.FacadeTask
		if ok, _ := tx.GetJSON(BucketTasks, id, &task); !ok {
			return notFound(id)
		}
		if task.Status == domain.TaskTerminal {
			return &domain.Failure{Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{Code: domain.CodeTerminalConflict, Detail: "terminal reached"}}}
		}
		task.Generation++
		gen = task.Generation
		return tx.PutJSON(BucketTasks, id, task)
	})
	return gen, err
}

// ReviewRequest submits an independent review.
type ReviewRequest struct {
	Reviewer  string
	Qualified bool
	Opinion   string
}

// Review persists a review. A reviewer may only submit one review; a repeated
// submission is a no-op that returns the stored opinion. The first recorded
// opinion is immutable, so a later retry with a different opinion cannot
// rewrite an approver into a rejecter (or vice versa).
func (e *Engine) Review(id string, req ReviewRequest) (arbiter.Review, error) {
	out := arbiter.Review{Reviewer: req.Reviewer, Qualified: req.Qualified, Opinion: req.Opinion}
	err := e.db.Update(func(tx *Tx) error {
		var existing arbiter.Review
		if ok, _ := tx.GetJSON(BucketReviews, reviewKey(id, req.Reviewer), &existing); ok {
			// First opinion wins: return the stored review unchanged.
			out = existing
			return nil
		}
		return tx.PutJSON(BucketReviews, reviewKey(id, req.Reviewer), out)
	})
	return out, err
}

func reviewKey(taskID, reviewer string) string { return keyJoin(taskID, reviewer) }

// TerminalRequest requests a terminal decision.
type TerminalRequest struct {
	Kind        arbiter.TerminalKind
	Reviewer    string
	Qualified   bool
	LogicalTime domain.LogicalTime
}

// Terminal runs the single-writer terminal competition. It first verifies the
// review quorum and retest completion, then compare-and-swaps the terminal
// record so exactly one decision is ever recorded.
func (e *Engine) Terminal(id string, req TerminalRequest) (arbiter.TerminalDecision, error) {
	var out arbiter.TerminalDecision
	err := e.db.Update(func(tx *Tx) error {
		var task domain.FacadeTask
		if ok, _ := tx.GetJSON(BucketTasks, id, &task); !ok {
			return notFound(id)
		}

		var existing arbiter.TerminalDecision
		if ok, _ := tx.GetJSON(BucketTerminal, id, &existing); ok {
			// A terminal decision already exists: every competing request loses.
			return &domain.Failure{Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{Code: domain.CodeTerminalConflict, Detail: "terminal already decided"}}}
		}

		// Review quorum and retest completion gate the terminal decision.
		var reviews []arbiter.Review
		_ = tx.ForEach(BucketReviews, func() any { return &arbiter.Review{} }, func(k string, v any) error {
			reviews = append(reviews, *v.(*arbiter.Review))
			return nil
		})
		if !arbiter.MeetsReviewQuorum(reviews) {
			return &domain.Failure{Code: domain.CodeReviewInsufficient,
				Reasons: []domain.Reason{{Code: domain.CodeReviewInsufficient, Detail: "need two qualified approvers"}}}
		}
		var retests []arbiter.RetestSet
		_ = tx.ForEach(BucketRetests, func() any { return &arbiter.RetestSet{} }, func(k string, v any) error {
			retests = append(retests, *v.(*arbiter.RetestSet))
			return nil
		})
		for _, r := range retests {
			if !r.Complete {
				return &domain.Failure{Code: domain.CodeRetestIncomplete,
					Reasons: []domain.Reason{{Code: domain.CodeRetestIncomplete, Detail: r.SourceBoard}}}
			}
		}

		dec, won := arbiter.Decide(nil, arbiter.TerminalRequest{
			Kind:        req.Kind,
			Reviewer:    req.Reviewer,
			Qualified:   req.Qualified,
			LogicalTime: req.LogicalTime,
			TaskID:      id,
		})
		if !won {
			return &domain.Failure{Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{Code: domain.CodeTerminalConflict, Detail: "terminal already decided"}}}
		}
		wrote, err := tx.compareAndSwapJSON(BucketTerminal, id, dec)
		if err != nil {
			return err
		}
		if !wrote {
			return &domain.Failure{Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{Code: domain.CodeTerminalConflict, Detail: "terminal lost competition"}}}
		}
		task.Status = domain.TaskTerminal
		if err := tx.PutJSON(BucketTasks, id, task); err != nil {
			return err
		}
		out = dec
		return tx.AppendEvent([]byte("terminal task=" + id + " kind=" + string(dec.Kind)))
	})
	return out, err
}

// MarkRetestComplete marks a retest set complete after rework. It is used when
// a new generation has re-inspected all members.
func (e *Engine) MarkRetestComplete(id string, sourceBoard string) (arbiter.RetestSet, error) {
	var out arbiter.RetestSet
	err := e.db.Update(func(tx *Tx) error {
		var rs arbiter.RetestSet
		if ok, _ := tx.GetJSON(BucketRetests, sourceBoard, &rs); !ok {
			return notFound(sourceBoard)
		}
		completed := map[string]bool{}
		for _, m := range rs.Members {
			completed[m] = true
		}
		rs, err := rs.MarkComplete(completed)
		if err != nil {
			return err
		}
		if err := tx.PutJSON(BucketRetests, sourceBoard, rs); err != nil {
			return err
		}
		out = rs
		return nil
	})
	return out, err
}
