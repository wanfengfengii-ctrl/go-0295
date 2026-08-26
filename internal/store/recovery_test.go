package store

import (
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
)

func TestRestartRecoveryProjectionIdentical(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/rockwool.db"

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e := NewEngine(db)
	id := testLockedTask(t, e)
	if _, err := e.SubmitCommand(id, Command{
		OperationID: "mix-1", Type: CommandMix, Generation: 1, LogicalTime: 10,
		MixerNumber: "mixer-1", Holder: "me", PowderGrams: 1000, WaterGrams: 250,
	}); err != nil {
		t.Fatal(err)
	}
	before, err := e.ProjectionDigest()
	if err != nil {
		t.Fatal(err)
	}
	lb, _ := e.GetLedger(id)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and rebuild the projection.
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	e2 := NewEngine(db2)
	ver, err := e2.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !ver.OK {
		t.Fatalf("recovery verification failed: %v", ver.Violations)
	}
	after, err := e2.ProjectionDigest()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("projection changed across restart: %s vs %s", before, after)
	}
	la, _ := e2.GetLedger(id)
	if la.Mortar.Powder != lb.Mortar.Powder || la.Mortar.Water != lb.Mortar.Water {
		t.Fatalf("balance changed across restart: %+v vs %+v", la.Mortar, lb.Mortar)
	}
}

func TestMidCommandFailureLeavesNoPartialState(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// A mix command that violates the water ratio must leave no material, no
	// lease and no idempotency record.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "mix-bad", Type: CommandMix, Generation: 1, LogicalTime: 10,
		MixerNumber: "mixer-1", Holder: "me", PowderGrams: 1000, WaterGrams: 999,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeWaterRatio {
		t.Fatalf("want water ratio failure, got %v", err)
	}

	lv, _ := e.GetLedger(id)
	if lv.Mortar.Powder != 0 || lv.Mortar.Water != 0 {
		t.Fatalf("partial material withdrawal leaked: %+v", lv.Mortar)
	}
	// No mixer lease should have been created.
	var leases []ledger.Lease
	_ = e.db.View(func(tx *Tx) error {
		return tx.ForEach(BucketLeases, func() any { return &ledger.Lease{} }, func(k string, v any) error {
			leases = append(leases, *v.(*ledger.Lease))
			return nil
		})
	})
	if len(leases) != 0 {
		t.Fatalf("partial lease leaked: %+v", leases)
	}
}

var _ = domain.Generation(0)
