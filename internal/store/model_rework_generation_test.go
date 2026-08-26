package store_test

import (
	"testing"

	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
	"rockwool-facade-render-handover/internal/store"
)

func TestModel_ReworkGenerationKeepsIndependentEvidence(t *testing.T) {
	db, err := store.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	engine := store.NewEngine(db)

	task, err := engine.CreateTask(store.CreateTaskRequest{
		Building: "model-building", FacadeZone: "model-zone", WallType: "concrete",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	boards := []coverage.BoardPlacement{{
		ID: "panel-a", Row: 0, Col: 0, Rows: 1, Cols: 1,
		Generation: 1, Material: "batch-a", BaseZone: "base-a",
	}}
	_, err = engine.LockTask(task.ID, store.LockTaskRequest{
		Snapshot: catalog.Snapshot{
			WallType: "concrete", Materials: map[string]string{"board": "batch-a"},
			FixedScale: 1000, Sampling: map[string]string{},
		},
		Thresholds: catalog.Thresholds{
			FixedScale: 1000, WaterRatioNum: 1, WaterRatioDen: 4,
			UnitAreaGlue: 0, MinCureSecs: 0, MinEdgeMM: 100,
			MinSpacingMM: 200, MinPullStrength: 1, MinBondStrength: 1,
		},
		Layout: coverage.Layout{
			Rows: 1, Cols: 1, Openings: map[string]bool{},
			ForbiddenCorners: map[string]bool{},
		},
		Boards: boards,
	})
	if err != nil {
		t.Fatalf("lock task: %v", err)
	}

	stages := []struct {
		typ  store.CommandType
		want evidence.Stage
	}{
		{store.CommandBase, evidence.StageBaseAccepted},
		{store.CommandMortar, evidence.StageMortarValid},
		{store.CommandGlue, evidence.StageGluePrefix},
		{store.CommandPlace, evidence.StagePlaced},
		{store.CommandJoint, evidence.StageJoint},
		{store.CommandCure, evidence.StageCured},
		{store.CommandAnchor, evidence.StageAnchored},
		{store.CommandInspect, evidence.StageInspected},
	}
	submitPrefix := func(gen domain.Generation, operationPrefix string) {
		for i, step := range stages {
			cmd := store.Command{
				OperationID: operationPrefix + "-" + step.want.String(),
				Type:        step.typ, BoardID: "panel-a", Generation: gen,
				LogicalTime: domain.LogicalTime(100*int64(gen) + int64(i)),
				CureStart:   0, CureEnd: 1, CureRateNum: 1, CureRateDen: 1,
				AnchorSeq: 1, AnchorX: 500, AnchorY: 500, AnchorDepth: 50,
			}
			got, err := engine.SubmitCommand(task.ID, cmd)
			if err != nil {
				t.Fatalf("generation %d advance to %s: %v", gen, step.want, err)
			}
			if got.Stage != step.want {
				t.Fatalf("generation %d stage = %s, want %s", gen, got.Stage, step.want)
			}
		}
	}

	var retestGeneration domain.Generation
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "prior generation has a complete evidence chain",
			run: func(t *testing.T) {
				submitPrefix(1, "original")
			},
		},
		{
			name: "pull anomaly opens a new rework generation",
			run: func(t *testing.T) {
				retest, err := engine.Retest(task.ID, "panel-a", 1)
				if err != nil {
					t.Fatalf("create retest scope: %v", err)
				}
				got, err := engine.NewGeneration(task.ID)
				if err != nil {
					t.Fatalf("open rework generation: %v", err)
				}
				if got != retest.Generation || got != 2 {
					t.Fatalf("generation = %d, retest generation = %d, want both 2", got, retest.Generation)
				}
				retestGeneration = got
			},
		},
		{
			name: "new generation restarts at base and follows the full prefix",
			run: func(t *testing.T) {
				submitPrefix(retestGeneration, "rework")
				view, err := engine.GetEvidence(task.ID)
				if err != nil {
					t.Fatalf("query current evidence: %v", err)
				}
				record := view.Boards[task.ID+"/panel-a"]
				if record.Generation != 2 || record.Stage != evidence.StageInspected {
					t.Fatalf("current evidence = generation %d stage %s, want generation 2 stage %s", record.Generation, record.Stage, evidence.StageInspected)
				}
			},
		},
		{
			name: "stale generation is rejected without replacing current evidence",
			run: func(t *testing.T) {
				_, err := engine.SubmitCommand(task.ID, store.Command{
					OperationID: "late-original-command", Type: store.CommandBase,
					BoardID: "panel-a", Generation: 1, LogicalTime: 999,
				})
				failure, ok := err.(*domain.Failure)
				if !ok || failure.Code != domain.CodeVersionConflict {
					t.Fatalf("stale command error = %v, want version_conflict", err)
				}
				view, queryErr := engine.GetEvidence(task.ID)
				if queryErr != nil {
					t.Fatalf("query evidence after stale command: %v", queryErr)
				}
				record := view.Boards[task.ID+"/panel-a"]
				if record.Generation != 2 || record.Stage != evidence.StageInspected {
					t.Fatalf("stale command replaced current evidence: %+v", record)
				}
			},
		},
		{
			name: "retest history remains queryable",
			run: func(t *testing.T) {
				retests, err := engine.GetRetests(task.ID)
				if err != nil {
					t.Fatalf("query retest history: %v", err)
				}
				if len(retests) != 1 || retests[0].SourceGeneration != 1 || retests[0].Generation != 2 || retests[0].SourceBoard != "panel-a" {
					t.Fatalf("retest history = %+v, want preserved generation 1 to 2 record", retests)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}
