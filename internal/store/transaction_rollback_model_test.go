package store

import (
	"errors"
	"reflect"
	"testing"

	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/evidence"
)

var modelTransactionalBuckets = [][]byte{
	BucketMortar,
	BucketStages,
	BucketCuring,
	BucketAnchors,
	BucketIdempotency,
}

func modelStore(t *testing.T) (*DB, *Engine, string) {
	t.Helper()
	db, err := Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	engine := NewEngine(db)
	const taskID = "model-building/model-zone"
	if _, err := engine.CreateTask(CreateTaskRequest{
		Building: "model-building", FacadeZone: "model-zone", WallType: "concrete",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := engine.LockTask(taskID, LockTaskRequest{
		Snapshot: catalog.Snapshot{
			WallType: "concrete", Materials: map[string]string{"board": "batch-1"},
			FixedScale: 1000, Sampling: map[string]string{},
		},
		Thresholds: catalog.Thresholds{
			FixedScale: 1000, WaterRatioNum: 1, WaterRatioDen: 4,
			UnitAreaGlue: 0, MinCureSecs: 0, MinEdgeMM: 100,
			MinSpacingMM: 200, MinPullStrength: 1, MinBondStrength: 1,
		},
		Layout: coverage.Layout{
			Rows: 1, Cols: 1, Openings: map[string]bool{}, ForbiddenCorners: map[string]bool{},
		},
		Boards: []coverage.BoardPlacement{{
			ID: "panel", Row: 0, Col: 0, Rows: 1, Cols: 1,
			Generation: 1, Material: "batch-1", BaseZone: "zone-1",
		}},
	}); err != nil {
		t.Fatalf("lock task: %v", err)
	}
	return db, engine, taskID
}

func modelMix(t *testing.T, engine *Engine, taskID string) {
	t.Helper()
	if _, err := engine.SubmitCommand(taskID, Command{
		OperationID: "mix", Type: CommandMix, Generation: 1, LogicalTime: 1,
		MixerNumber: "mixer-1", Holder: "crew", PowderGrams: 1000, WaterGrams: 250,
	}); err != nil {
		t.Fatalf("mix mortar: %v", err)
	}
}

func modelAdvanceToMortar(t *testing.T, engine *Engine, taskID string) {
	t.Helper()
	for _, cmd := range []Command{
		{OperationID: "base", Type: CommandBase, BoardID: "panel", Generation: 1, LogicalTime: 2},
		{OperationID: "mortar", Type: CommandMortar, BoardID: "panel", Generation: 1, LogicalTime: 3},
	} {
		if _, err := engine.SubmitCommand(taskID, cmd); err != nil {
			t.Fatalf("advance %s: %v", cmd.Type, err)
		}
	}
}

func modelBucketSnapshot(t *testing.T, db *DB) map[string]map[string]string {
	t.Helper()
	out := make(map[string]map[string]string, len(modelTransactionalBuckets))
	if err := db.View(func(tx *Tx) error {
		for _, name := range modelTransactionalBuckets {
			entries := map[string]string{}
			if err := tx.bucket(name).ForEach(func(key, value []byte) error {
				entries[string(key)] = string(value)
				return nil
			}); err != nil {
				return err
			}
			out[string(name)] = entries
		}
		return nil
	}); err != nil {
		t.Fatalf("snapshot buckets: %v", err)
	}
	return out
}

func modelRequireFailureCode(t *testing.T, err error, code domain.ErrorCode) {
	t.Helper()
	failure, ok := err.(*domain.Failure)
	if !ok || failure.Code != code {
		t.Fatalf("want failure code %q, got %v", code, err)
	}
}

func TestModel_TransactionalCommandRollback(t *testing.T) {
	cases := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "DB Update rolls back an arbitrary callback error",
			run: func(t *testing.T) {
				db, _, _ := modelStore(t)
				before := modelBucketSnapshot(t, db)
				rejected := errors.New("rejected callback")
				err := db.Update(func(tx *Tx) error {
					for i, bucket := range modelTransactionalBuckets {
						if err := tx.PutJSON(bucket, "would-be-partial", map[string]int{"value": i + 1}); err != nil {
							return err
						}
					}
					return rejected
				})
				if !errors.Is(err, rejected) {
					t.Fatalf("want callback error, got %v", err)
				}
				if after := modelBucketSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("callback error committed writes: before=%v after=%v", before, after)
				}
			},
		},
		{
			name: "premature glue consumes no mortar or evidence",
			run: func(t *testing.T) {
				db, engine, taskID := modelStore(t)
				modelMix(t, engine, taskID)
				before := modelBucketSnapshot(t, db)
				_, err := engine.SubmitCommand(taskID, Command{
					OperationID: "premature-glue", Type: CommandGlue, BoardID: "panel",
					Generation: 1, LogicalTime: 2, GlueGrams: 200,
				})
				modelRequireFailureCode(t, err, domain.CodePrefixViolation)
				if after := modelBucketSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("prefix violation changed transactional buckets: before=%v after=%v", before, after)
				}
			},
		},
		{
			name: "premature curing records no interval",
			run: func(t *testing.T) {
				db, engine, taskID := modelStore(t)
				before := modelBucketSnapshot(t, db)
				_, err := engine.SubmitCommand(taskID, Command{
					OperationID: "premature-cure", Type: CommandCure, BoardID: "panel",
					Generation: 1, LogicalTime: 2, CureStart: 1, CureEnd: 2,
					CureRateNum: 1, CureRateDen: 1,
				})
				modelRequireFailureCode(t, err, domain.CodePrefixViolation)
				if after := modelBucketSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("prefix violation changed transactional buckets: before=%v after=%v", before, after)
				}
			},
		},
		{
			name: "premature anchor records no anchor",
			run: func(t *testing.T) {
				db, engine, taskID := modelStore(t)
				before := modelBucketSnapshot(t, db)
				_, err := engine.SubmitCommand(taskID, Command{
					OperationID: "premature-anchor", Type: CommandAnchor, BoardID: "panel",
					Generation: 1, LogicalTime: 2, AnchorSeq: 1,
					AnchorX: 500, AnchorY: 500, AnchorDepth: 50,
				})
				modelRequireFailureCode(t, err, domain.CodePrefixViolation)
				if after := modelBucketSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("prefix violation changed transactional buckets: before=%v after=%v", before, after)
				}
			},
		},
		{
			name: "valid glue consumes mortar and advances stage together",
			run: func(t *testing.T) {
				db, engine, taskID := modelStore(t)
				modelMix(t, engine, taskID)
				modelAdvanceToMortar(t, engine, taskID)
				result, err := engine.SubmitCommand(taskID, Command{
					OperationID: "valid-glue", Type: CommandGlue, BoardID: "panel",
					Generation: 1, LogicalTime: 4, GlueGrams: 200,
				})
				if err != nil {
					t.Fatalf("valid glue: %v", err)
				}
				if result.Stage != evidence.StageGluePrefix {
					t.Fatalf("want glue stage, got %s", result.Stage)
				}
				ledgerView, err := engine.GetLedger(taskID)
				if err != nil {
					t.Fatalf("get ledger: %v", err)
				}
				if ledgerView.Mortar.Remainder != 1050 || ledgerView.Mortar.Glue["panel"] != 200 {
					t.Fatalf("glue and remainder not committed together: %+v", ledgerView.Mortar)
				}
				snapshot := modelBucketSnapshot(t, db)
				if len(snapshot[string(BucketIdempotency)]) != 4 {
					t.Fatalf("successful commands should be idempotently recorded: %v", snapshot[string(BucketIdempotency)])
				}
			},
		},
		{
			name: "insufficient mortar leaves all command state unchanged",
			run: func(t *testing.T) {
				db, engine, taskID := modelStore(t)
				modelMix(t, engine, taskID)
				modelAdvanceToMortar(t, engine, taskID)
				before := modelBucketSnapshot(t, db)
				_, err := engine.SubmitCommand(taskID, Command{
					OperationID: "too-much-glue", Type: CommandGlue, BoardID: "panel",
					Generation: 1, LogicalTime: 4, GlueGrams: 1251,
				})
				modelRequireFailureCode(t, err, domain.CodeConservation)
				if after := modelBucketSnapshot(t, db); !reflect.DeepEqual(after, before) {
					t.Fatalf("failed glue changed transactional buckets: before=%v after=%v", before, after)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}
