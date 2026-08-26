package store

import (
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
	"rockwool-facade-render-handover/internal/ledger"
)

func TestPrefixViolationSkippingStages(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Attempt to place board "a" before base confirmation and glue.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "place-skip", Type: CommandPlace, BoardID: "a", Generation: 1, LogicalTime: 10,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodePrefixViolation {
		t.Fatalf("want prefix violation, got %v", err)
	}

	// Skip curing and drill straight to anchor.
	_, err = e.SubmitCommand(id, Command{
		OperationID: "anchor-skip", Type: CommandAnchor, BoardID: "a", Generation: 1, LogicalTime: 10,
		AnchorSeq: 1, AnchorX: 500, AnchorY: 500, AnchorDepth: 50,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodePrefixViolation {
		t.Fatalf("want prefix violation, got %v", err)
	}
}

func TestAnchorSequenceSkipRejected(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Advance board a through the full prefix up to Cured.
	for _, step := range []struct {
		op  string
		typ CommandType
	}{
		{"base-a", CommandBase},
		{"mortar-a", CommandMortar},
		{"glue-a", CommandGlue},
		{"place-a", CommandPlace},
		{"joint-a", CommandJoint},
		{"cure-a", CommandCure},
	} {
		if _, err := e.SubmitCommand(id, Command{
			OperationID: step.op, Type: step.typ, BoardID: "a", Generation: 1, LogicalTime: 10,
			GlueGrams: 0, CureStart: 0, CureEnd: 1,
		}); err != nil {
			t.Fatalf("advance %s: %v", step.typ, err)
		}
	}

	// Anchor with sequence number 2 instead of 1 must be rejected.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "anchor-bad", Type: CommandAnchor, BoardID: "a", Generation: 1, LogicalTime: 10,
		AnchorSeq: 2, AnchorX: 500, AnchorY: 500, AnchorDepth: 50,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodePrefixViolation {
		t.Fatalf("want prefix violation for seq skip, got %v", err)
	}
}

func TestAnchorGeometryRejected(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)
	for _, step := range []struct {
		op  string
		typ CommandType
	}{
		{"base-a", CommandBase},
		{"mortar-a", CommandMortar},
		{"glue-a", CommandGlue},
		{"place-a", CommandPlace},
		{"joint-a", CommandJoint},
		{"cure-a", CommandCure},
	} {
		if _, err := e.SubmitCommand(id, Command{
			OperationID: step.op, Type: step.typ, BoardID: "a", Generation: 1, LogicalTime: 10,
			GlueGrams: 0, CureStart: 0, CureEnd: 1,
		}); err != nil {
			t.Fatalf("advance %s: %v", step.typ, err)
		}
	}
	// First anchor is valid.
	if _, err := e.SubmitCommand(id, Command{
		OperationID: "anchor-1", Type: CommandAnchor, BoardID: "a", Generation: 1, LogicalTime: 10,
		AnchorSeq: 1, AnchorX: 500, AnchorY: 500, AnchorDepth: 50,
	}); err != nil {
		t.Fatalf("first anchor rejected: %v", err)
	}
	// Duplicate hole must be rejected.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "anchor-dup", Type: CommandAnchor, BoardID: "a", Generation: 1, LogicalTime: 10,
		AnchorSeq: 2, AnchorX: 500, AnchorY: 500, AnchorDepth: 50,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeAnchorEdge {
		t.Fatalf("want duplicate hole rejection, got %v", err)
	}
	// Out-of-bounds hole (too close to edge) rejected.
	_, err = e.SubmitCommand(id, Command{
		OperationID: "anchor-edge", Type: CommandAnchor, BoardID: "a", Generation: 1, LogicalTime: 10,
		AnchorSeq: 2, AnchorX: 50, AnchorY: 500, AnchorDepth: 50,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeAnchorEdge {
		t.Fatalf("want edge violation, got %v", err)
	}
}

func TestScriptedDeviceRetry(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Script: timeout, malformat, success.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "drill-1", Type: CommandDrill, BoardID: "a", Generation: 1, LogicalTime: 10,
		DrillScript: []evidence.DeviceOutcome{evidence.OutcomeTimeout, evidence.OutcomeMalformat, evidence.OutcomeSuccess},
		DrillValues: []int64{0, 0, 42},
	})
	if err != nil {
		t.Fatalf("scripted drill should succeed after retries: %v", err)
	}

	// A hard failure that never succeeds leaves no reading.
	_, err = e.SubmitCommand(id, Command{
		OperationID: "drill-2", Type: CommandDrill, BoardID: "b", Generation: 1, LogicalTime: 10,
		DrillScript: []evidence.DeviceOutcome{evidence.OutcomeTimeout, evidence.OutcomeTimeout, evidence.OutcomeTimeout, evidence.OutcomeTimeout},
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeDeviceError {
		t.Fatalf("want device error, got %v", err)
	}
}

func TestGlueOverflowNoPartialEvidence(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Advance to glue stage first.
	for _, step := range []struct {
		op  string
		typ CommandType
	}{
		{"base-a", CommandBase},
		{"mortar-a", CommandMortar},
	} {
		if _, err := e.SubmitCommand(id, Command{
			OperationID: step.op, Type: step.typ, BoardID: "a", Generation: 1, LogicalTime: 10,
		}); err != nil {
			t.Fatalf("advance %s: %v", step.typ, err)
		}
	}

	// A glue command with an absurd amount must not write partial evidence. Use
	// a huge glue request that would overflow area*rate if the unit rate were
	// set, but here the mortar has no powder so consumption fails conservatively.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "glue-over", Type: CommandGlue, BoardID: "a", Generation: 1, LogicalTime: 10,
		GlueGrams: 1 << 40,
	})
	if err == nil {
		t.Fatal("expected glue over-consumption to fail")
	}

	// Board stage must be unchanged (still MortarValid, not advanced to glue).
	ev, _ := e.GetEvidence(id)
	rec := ev.Boards[boardKey(id, "a")]
	if rec.Stage != evidence.StageMortarValid {
		t.Fatalf("board stage advanced despite failure: %s", rec.Stage)
	}
}

var _ = ledger.KindMixer
var _ = domain.CodeInvalid
