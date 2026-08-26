package evidence

import "testing"

func TestStageAdvance(t *testing.T) {
	if !StageNone.CanAdvanceTo(StageBaseAccepted) {
		t.Fatal("first step must be allowed")
	}
	if StageNone.CanAdvanceTo(StageGluePrefix) {
		t.Fatal("skipping stages must be rejected")
	}
	if StageBaseAccepted.CanAdvanceTo(StageBaseAccepted) {
		t.Fatal("no-op advance must be rejected")
	}
	if StageBaseAccepted.CanAdvanceTo(StageNone) {
		t.Fatal("backward advance must be rejected")
	}
}

func TestStageNames(t *testing.T) {
	if StageInspected.String() != "inspected" {
		t.Fatalf("got %q", StageInspected.String())
	}
	if StageNone.String() != "none" {
		t.Fatalf("got %q", StageNone.String())
	}
}
