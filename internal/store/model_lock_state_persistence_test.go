package store_test

import (
	"errors"
	"reflect"
	"testing"

	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/store"
)

func TestModel_LockTaskPreservesFirstLockedState(t *testing.T) {
	baseSnapshot := catalog.Snapshot{
		WallType:   "concrete",
		Materials:  map[string]string{"board": "proof-v1"},
		FixedScale: 1000,
		Sampling:   map[string]string{"pull": "board-a"},
	}
	thresholds := catalog.Thresholds{
		FixedScale:      1000,
		WaterRatioNum:   1,
		WaterRatioDen:   4,
		MinEdgeMM:       100,
		MinSpacingMM:    200,
		MinPullStrength: 1,
		MinBondStrength: 1,
	}
	baseLayout := coverage.Layout{
		Rows:             1,
		Cols:             2,
		Openings:         map[string]bool{},
		ForbiddenCorners: map[string]bool{},
		Adjacency:        []coverage.AdjEdge{{A: "board-a", B: "board-b"}},
	}
	baseBoards := []coverage.BoardPlacement{
		{ID: "board-a", Row: 0, Col: 0, Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "zone-1"},
		{ID: "board-b", Row: 0, Col: 1, Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "zone-1"},
	}

	cases := []struct {
		name       string
		secondLock store.LockTaskRequest
	}{
		{
			name: "different rule snapshot",
			secondLock: store.LockTaskRequest{
				Snapshot: catalog.Snapshot{
					WallType:   "concrete",
					Materials:  map[string]string{"board": "proof-v2"},
					FixedScale: 1000,
					Sampling:   map[string]string{"pull": "board-b"},
				},
				Thresholds: thresholds,
				Layout:     baseLayout,
				Boards:     baseBoards,
			},
		},
		{
			name: "different board layout",
			secondLock: store.LockTaskRequest{
				Snapshot:   baseSnapshot,
				Thresholds: thresholds,
				Layout: coverage.Layout{
					Rows:             1,
					Cols:             2,
					Openings:         map[string]bool{},
					ForbiddenCorners: map[string]bool{},
					Adjacency:        []coverage.AdjEdge{},
				},
				Boards: []coverage.BoardPlacement{
					{ID: "replacement", Row: 0, Col: 0, Rows: 1, Cols: 2, Generation: 1, Material: "batch-2", BaseZone: "zone-2"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open("")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			defer db.Close()
			engine := store.NewEngine(db)

			task, err := engine.CreateTask(store.CreateTaskRequest{
				Building: "building-1", FacadeZone: "north", WallType: "concrete",
			})
			if err != nil {
				t.Fatalf("first create failed: %v", err)
			}
			first, err := engine.LockTask(task.ID, store.LockTaskRequest{
				Snapshot: baseSnapshot, Thresholds: thresholds, Layout: baseLayout, Boards: baseBoards,
			})
			if err != nil {
				t.Fatalf("first lock failed: %v", err)
			}
			if first.SnapshotDigest != baseSnapshot.Digest() || first.CoverageDigest != coverage.CoverageDigest(baseBoards) || first.Generation != 1 {
				t.Fatalf("unexpected first lock result: %+v", first)
			}

			beforeTask, err := engine.GetTask(task.ID)
			if err != nil {
				t.Fatalf("get task before second lock: %v", err)
			}
			beforeCoverage, err := engine.GetCoverage(task.ID)
			if err != nil {
				t.Fatalf("get coverage before second lock: %v", err)
			}

			wantFailure := &domain.Failure{
				Code: domain.CodeTerminalConflict,
				Reasons: []domain.Reason{{
					Code: domain.CodeTerminalConflict, Detail: "task already locked",
				}},
			}
			for attempt := 1; attempt <= 2; attempt++ {
				_, err = engine.LockTask(task.ID, tc.secondLock)
				var failure *domain.Failure
				if !errors.As(err, &failure) {
					t.Fatalf("second lock attempt %d returned %v, want domain failure", attempt, err)
				}
				if !reflect.DeepEqual(failure, wantFailure) {
					t.Fatalf("second lock attempt %d failure = %+v, want %+v", attempt, failure, wantFailure)
				}

				afterTask, getErr := engine.GetTask(task.ID)
				if getErr != nil {
					t.Fatalf("get task after second lock attempt %d: %v", attempt, getErr)
				}
				afterCoverage, getErr := engine.GetCoverage(task.ID)
				if getErr != nil {
					t.Fatalf("get coverage after second lock attempt %d: %v", attempt, getErr)
				}
				if !reflect.DeepEqual(afterTask, beforeTask) {
					t.Fatalf("second lock attempt %d changed task: before=%+v after=%+v", attempt, beforeTask, afterTask)
				}
				if !reflect.DeepEqual(afterCoverage, beforeCoverage) {
					t.Fatalf("second lock attempt %d changed coverage: before=%+v after=%+v", attempt, beforeCoverage, afterCoverage)
				}
			}
		})
	}
}
