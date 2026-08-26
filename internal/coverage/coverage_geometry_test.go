package coverage

import (
	"testing"

	"rockwool-facade-render-handover/internal/domain"
)

func twoByTwoLayout() Layout {
	return Layout{
		Rows: 2, Cols: 2,
		Openings:         map[string]bool{},
		ForbiddenCorners: map[string]bool{},
		Adjacency:        []AdjEdge{},
	}
}

func TestLockFullCoverage(t *testing.T) {
	layout := Layout{Rows: 1, Cols: 4, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{}}
	boards := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "z1"},
		{ID: "b", Row: 0, Col: 1, Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "z1"},
		{ID: "c", Row: 0, Col: 2, Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "z1"},
		{ID: "d", Row: 0, Col: 3, Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "z1"},
	}
	summary, err := LockLayout(layout, boards)
	if err != nil {
		t.Fatalf("valid full coverage rejected: %v", err)
	}
	if summary.Digest == "" {
		t.Fatal("coverage digest must be non-empty")
	}
	if summary.Generation != 1 {
		t.Fatalf("generation = %d, want 1", summary.Generation)
	}
	// Digest is deterministic across ordering permutations.
	perm := []BoardPlacement{boards[3], boards[0], boards[2], boards[1]}
	summary2, _ := LockLayout(layout, perm)
	if summary.Digest != summary2.Digest {
		t.Fatalf("digest not order-independent: %s vs %s", summary.Digest, summary2.Digest)
	}
}

func TestLockOverlapMissingDegenerate(t *testing.T) {
	overlap := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 2, Generation: 1},
		{ID: "b", Row: 0, Col: 0, Rows: 2, Cols: 1, Generation: 1},
	}
	_, err := LockLayout(twoByTwoLayout(), overlap)
	f, ok := err.(*domain.Failure)
	if !ok || f.Code != domain.CodeInvalid {
		t.Fatalf("want invalid failure, got %v", err)
	}
	if !hasCode(f.Reasons, domain.CodeOverlap) {
		t.Fatalf("missing overlap reason: %+v", f.Reasons)
	}

	missing := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 1, Generation: 1},
	}
	_, err = LockLayout(twoByTwoLayout(), missing)
	f, _ = err.(*domain.Failure)
	if !hasCode(f.Reasons, domain.CodeMissingCell) {
		t.Fatalf("missing missing-cell reason: %+v", f.Reasons)
	}

	degenerate := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 0, Cols: 1, Generation: 1},
	}
	_, err = LockLayout(twoByTwoLayout(), degenerate)
	f, _ = err.(*domain.Failure)
	if !hasCode(f.Reasons, domain.CodeDegenerate) {
		t.Fatalf("missing degenerate reason: %+v", f.Reasons)
	}
}

func TestVerticalJointAndCornerForbidden(t *testing.T) {
	// Vertical through joint: interior seam at col 2 shared by both courses.
	vertical := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 2, Generation: 1},
		{ID: "b", Row: 0, Col: 2, Rows: 1, Cols: 2, Generation: 1},
		{ID: "c", Row: 1, Col: 0, Rows: 1, Cols: 2, Generation: 1},
		{ID: "d", Row: 1, Col: 2, Rows: 1, Cols: 2, Generation: 1},
	}
	_, err := LockLayout(Layout{Rows: 2, Cols: 4, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{}}, vertical)
	f, _ := err.(*domain.Failure)
	if !hasCode(f.Reasons, domain.CodeVerticalJoint) {
		t.Fatalf("missing vertical-joint reason: %+v", f.Reasons)
	}

	// Legal running bond passes: course seams are offset (row0 seam at 2, row1
	// seams at 1 and 3, so none align).
	stagger := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 2, Generation: 1},
		{ID: "b", Row: 0, Col: 2, Rows: 1, Cols: 2, Generation: 1},
		{ID: "c", Row: 1, Col: 0, Rows: 1, Cols: 1, Generation: 1},
		{ID: "d", Row: 1, Col: 1, Rows: 1, Cols: 2, Generation: 1},
		{ID: "e", Row: 1, Col: 3, Rows: 1, Cols: 1, Generation: 1},
	}
	if _, err := LockLayout(Layout{Rows: 2, Cols: 4, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{}}, stagger); err != nil {
		t.Fatalf("legal stagger rejected: %v", err)
	}

	// Corner forbidden seam: a board corner lands on a forbidden point.
	forbidden := []BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 2, Generation: 1},
		{ID: "b", Row: 0, Col: 2, Rows: 1, Cols: 2, Generation: 1},
		{ID: "c", Row: 1, Col: 0, Rows: 1, Cols: 2, Generation: 1},
		{ID: "d", Row: 1, Col: 2, Rows: 1, Cols: 2, Generation: 1},
	}
	layout := Layout{Rows: 2, Cols: 4, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{CornerKey(Corner{Row: 1, Col: 2}): true}}
	_, err = LockLayout(layout, forbidden)
	f, _ = err.(*domain.Failure)
	if !hasCode(f.Reasons, domain.CodeCornerOpening) {
		t.Fatalf("missing corner-opening reason: %+v", f.Reasons)
	}
}

func TestImpactSetDeterministic(t *testing.T) {
	boards := []BoardPlacement{
		{ID: "a", Material: "m1", BaseZone: "z1"},
		{ID: "b", Material: "m1", BaseZone: "z2"},
		{ID: "c", Material: "m2", BaseZone: "z1"},
		{ID: "d", Material: "m2", BaseZone: "z3"},
	}
	adj := []AdjEdge{{A: "a", B: "b"}}
	got := ImpactSet("a", boards, adj)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func hasCode(rs []domain.Reason, code domain.ErrorCode) bool {
	for _, r := range rs {
		if r.Code == code {
			return true
		}
	}
	return false
}
