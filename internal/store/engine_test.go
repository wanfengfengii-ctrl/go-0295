package store

import (
	"testing"

	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
)

// testLockedTask creates and locks a 2x2 single-board-per-cell task, returning
// the task id.
func testLockedTask(t *testing.T, e *Engine) string {
	t.Helper()
	id := "b1/z1"
	if _, err := e.CreateTask(CreateTaskRequest{Building: "b1", FacadeZone: "z1", WallType: "concrete"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	boards := []coverage.BoardPlacement{
		{ID: "a", Row: 0, Col: 0, Rows: 1, Cols: 1, Generation: 1, Material: "m1", BaseZone: "z1"},
		{ID: "b", Row: 0, Col: 1, Rows: 1, Cols: 1, Generation: 1, Material: "m1", BaseZone: "z2"},
		{ID: "c", Row: 0, Col: 2, Rows: 1, Cols: 1, Generation: 1, Material: "m1", BaseZone: "z1"},
		{ID: "d", Row: 0, Col: 3, Rows: 1, Cols: 1, Generation: 1, Material: "m2", BaseZone: "z3"},
	}
	snap := catalog.Snapshot{WallType: "concrete", Materials: map[string]string{"board": "m1"}, FixedScale: 1000, Sampling: map[string]string{}}
	th := catalog.Thresholds{
		FixedScale: 1000, WaterRatioNum: 1, WaterRatioDen: 4,
		UnitAreaGlue: 0, MinCureSecs: 0, MinEdgeMM: 100, MinSpacingMM: 200,
		MinPullStrength: 1, MinBondStrength: 1,
	}
	if _, err := e.LockTask(id, LockTaskRequest{
		Snapshot: snap, Thresholds: th,
		Layout: coverage.Layout{Rows: 1, Cols: 4, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{}, Adjacency: []coverage.AdjEdge{{A: "a", B: "b"}, {A: "b", B: "c"}}},
		Boards: boards,
	}); err != nil {
		t.Fatalf("lock: %v", err)
	}
	return id
}

func newMemEngine(t *testing.T) *Engine {
	t.Helper()
	db, err := Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewEngine(db)
}

var _ = domain.Generation(0)
