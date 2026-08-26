package catalog

import (
	"errors"
	"testing"

	"rockwool-facade-render-handover/internal/domain"
)

func TestDigestDeterministic(t *testing.T) {
	a := Snapshot{
		WallType:   "concrete",
		Materials:  map[string]string{"board": "batch-A", "mortar": "batch-B"},
		FixedScale: 1000,
		Sampling:   map[string]string{"zone1": "point1"},
		CreatedAt:  domain.LogicalTime(1),
	}
	b := Snapshot{
		WallType:   "concrete",
		Materials:  map[string]string{"mortar": "batch-B", "board": "batch-A"},
		FixedScale: 1000,
		Sampling:   map[string]string{"zone1": "point1"},
		CreatedAt:  domain.LogicalTime(1),
	}
	if a.Digest() != b.Digest() {
		t.Fatal("equal snapshots must have equal digests")
	}
	b.FixedScale = 2000
	if a.Digest() == b.Digest() {
		t.Fatal("different snapshots must have different digests")
	}
}

func TestValidate(t *testing.T) {
	if err := (Snapshot{WallType: "", FixedScale: 1000}).Validate(); !errors.Is(err, ErrWallTypeRequired) {
		t.Fatalf("want wall type required, got %v", err)
	}
	if err := (Snapshot{WallType: "concrete", FixedScale: 0}).Validate(); !errors.Is(err, ErrBadFixedScale) {
		t.Fatalf("want bad fixed scale, got %v", err)
	}
	if err := (Snapshot{WallType: "concrete", FixedScale: 1000}).Validate(); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
}
