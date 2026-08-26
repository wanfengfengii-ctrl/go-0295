package store

import (
	"sync"
	"testing"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
)

func TestRetestImpactSetDeterministic(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Seed "a": adjacency reaches b, then b reaches c; base zone z1 also reaches
	// c; material m1 reaches b and c too.
	rs, err := e.Retest(id, "a", 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if len(rs.Members) != len(want) {
		t.Fatalf("members %v want %v", rs.Members, want)
	}
	for i := range want {
		if rs.Members[i] != want[i] {
			t.Fatalf("members %v want %v", rs.Members, want)
		}
	}
	if rs.Generation != 2 {
		t.Fatalf("retest generation = %d, want 2", rs.Generation)
	}
	// Same anomaly fact and source generation yields the same unique set.
	rs2, _ := e.Retest(id, "a", 1)
	if len(rs2.Members) != len(rs.Members) {
		t.Fatal("retest set must be unique per anomaly fact")
	}
}

func TestLateReceiptDoesNotChangeProjection(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	if _, err := e.NewGeneration(id); err != nil {
		t.Fatal(err)
	}
	task, _ := e.GetTask(id)
	if task.Generation != 2 {
		t.Fatalf("generation = %d, want 2", task.Generation)
	}
	// A late receipt from generation 1 must be isolated.
	if !arbiter.LateReceiptIsolated(1, task.Generation) {
		t.Fatal("generation-1 receipt against generation-2 must be isolated")
	}
	// Attempting a generation-1 command against generation-2 task fails and does
	// not change the projection.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "late-1", Type: CommandBase, BoardID: "a", Generation: 1, LogicalTime: 10,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeVersionConflict {
		t.Fatalf("want version conflict, got %v", err)
	}
}

func TestCuringGapBlocksProgress(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Advance to joint, then add a curing interval.
	for _, step := range []struct {
		op  string
		typ CommandType
	}{
		{"base-a", CommandBase},
		{"mortar-a", CommandMortar},
		{"glue-a", CommandGlue},
		{"place-a", CommandPlace},
		{"joint-a", CommandJoint},
	} {
		if _, err := e.SubmitCommand(id, Command{
			OperationID: step.op, Type: step.typ, BoardID: "a", Generation: 1, LogicalTime: 10,
		}); err != nil {
			t.Fatalf("advance %s: %v", step.typ, err)
		}
	}
	if _, err := e.SubmitCommand(id, Command{
		OperationID: "cure-1", Type: CommandCure, BoardID: "a", Generation: 1, LogicalTime: 10,
		CureStart: 0, CureEnd: 5, CureRateNum: 1, CureRateDen: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// A gapped interval (starts at 7 instead of 5) must be rejected.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "cure-gap", Type: CommandCure, BoardID: "a", Generation: 1, LogicalTime: 10,
		CureStart: 7, CureEnd: 9, CureRateNum: 1, CureRateDen: 1,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeCuringGap {
		t.Fatalf("want curing gap, got %v", err)
	}
}

func TestReviewQuorumRequiresTwoDistinct(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Same reviewer twice does not satisfy quorum.
	if _, err := e.Review(id, ReviewRequest{Reviewer: "alice", Qualified: true, Opinion: "approve"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Review(id, ReviewRequest{Reviewer: "alice", Qualified: true, Opinion: "approve"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.Terminal(id, TerminalRequest{Kind: arbiter.TerminalHandover, Reviewer: "alice", Qualified: true, LogicalTime: 20})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeReviewInsufficient {
		t.Fatalf("want review insufficient, got %v", err)
	}

	// A second distinct qualified reviewer satisfies quorum.
	if _, err := e.Review(id, ReviewRequest{Reviewer: "bob", Qualified: true, Opinion: "approve"}); err != nil {
		t.Fatal(err)
	}
	dec, err := e.Terminal(id, TerminalRequest{Kind: arbiter.TerminalHandover, Reviewer: "bob", Qualified: true, LogicalTime: 20})
	if err != nil {
		t.Fatal(err)
	}
	if dec.Kind != arbiter.TerminalHandover || dec.Credential == "" {
		t.Fatalf("handover must carry a credential: %+v", dec)
	}
}

func TestConcurrentTerminalExactlyOneWinner(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Seed two qualified reviews so the terminal gate passes.
	for _, r := range []string{"alice", "bob"} {
		if _, err := e.Review(id, ReviewRequest{Reviewer: r, Qualified: true, Opinion: "approve"}); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	kinds := []arbiter.TerminalKind{arbiter.TerminalHandover, arbiter.TerminalQuarantine, arbiter.TerminalCancel}
	results := make([]*arbiter.TerminalDecision, len(kinds))
	errs := make([]error, len(kinds))
	for i, k := range kinds {
		wg.Add(1)
		go func(i int, k arbiter.TerminalKind) {
			defer wg.Done()
			<-start
			dec, err := e.Terminal(id, TerminalRequest{Kind: k, Reviewer: "bob", Qualified: true, LogicalTime: 30})
			results[i] = &dec
			errs[i] = err
		}(i, k)
	}
	close(start)
	wg.Wait()

	wins := 0
	var winner *arbiter.TerminalDecision
	for i := range kinds {
		if errs[i] == nil {
			wins++
			winner = results[i]
		}
	}
	if wins != 1 {
		t.Fatalf("want exactly one terminal winner, got %d", wins)
	}
	if winner.Kind == arbiter.TerminalHandover && winner.Credential == "" {
		t.Fatal("handover winner must have a credential")
	}
	if winner.Kind != arbiter.TerminalHandover && winner.Credential != "" {
		t.Fatal("non-handover terminal must not have a credential")
	}
}

var _ = evidence.StageNone
