package store

import (
	"reflect"
	"testing"

	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
)

func TestModel_IdempotencyIsScopedToTask(t *testing.T) {
	e := newMemEngine(t)
	taskIDs := []string{"building-1/east", "building-2/east"}
	for i, taskID := range taskIDs {
		building := "building-1"
		if i == 1 {
			building = "building-2"
		}
		if _, err := e.CreateTask(CreateTaskRequest{Building: building, FacadeZone: "east", WallType: "concrete"}); err != nil {
			t.Fatalf("create %s: %v", taskID, err)
		}
		if _, err := e.LockTask(taskID, LockTaskRequest{
			Snapshot: catalog.Snapshot{
				WallType: "concrete", Materials: map[string]string{"board": "batch-1"},
				FixedScale: 1000, Sampling: map[string]string{},
			},
			Thresholds: catalog.Thresholds{
				FixedScale: 1000, WaterRatioNum: 1, WaterRatioDen: 4,
				MinEdgeMM: 100, MinSpacingMM: 200, MinPullStrength: 1, MinBondStrength: 1,
			},
			Layout: coverage.Layout{Rows: 1, Cols: 1, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{}},
			Boards: []coverage.BoardPlacement{{
				ID: "first-board", Rows: 1, Cols: 1, Generation: 1, Material: "batch-1", BaseZone: "base-zone",
			}},
		}); err != nil {
			t.Fatalf("lock %s: %v", taskID, err)
		}
	}

	shared := Command{
		OperationID: "crew-operation-7", Type: CommandBase, BoardID: "first-board",
		Generation: 1, LogicalTime: 10,
	}
	var original CommandResult
	cases := []struct {
		name              string
		taskID            string
		command           Command
		wantCode          domain.ErrorCode
		wantStage         evidence.Stage
		wantOriginalReply bool
	}{
		{name: "first task executes", taskID: taskIDs[0], command: shared, wantStage: evidence.StageBaseAccepted},
		{name: "same task same content reuses response", taskID: taskIDs[0], command: shared, wantStage: evidence.StageBaseAccepted, wantOriginalReply: true},
		{name: "same task different content conflicts", taskID: taskIDs[0], command: func() Command { c := shared; c.LogicalTime = 11; return c }(), wantCode: domain.CodeIdempotencyConflict, wantStage: evidence.StageBaseAccepted},
		{name: "different task same operation executes independently", taskID: taskIDs[1], command: shared, wantStage: evidence.StageBaseAccepted},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.SubmitCommand(tc.taskID, tc.command)
			if tc.wantCode != "" {
				failure, ok := err.(*domain.Failure)
				if !ok || failure.Code != tc.wantCode {
					t.Fatalf("want error code %q, got %v", tc.wantCode, err)
				}
			} else if err != nil {
				t.Fatalf("submit command: %v", err)
			}
			if i == 0 {
				original = got
			}
			if tc.wantOriginalReply && !reflect.DeepEqual(got, original) {
				t.Fatalf("retry response = %+v, want original response %+v", got, original)
			}

			view, err := e.GetEvidence(tc.taskID)
			if err != nil {
				t.Fatalf("get evidence: %v", err)
			}
			record, ok := view.Boards[boardKey(tc.taskID, "first-board")]
			if !ok || record.Stage != tc.wantStage {
				t.Fatalf("task evidence stage = %q (present %v), want %q", record.Stage, ok, tc.wantStage)
			}
		})
	}
}
