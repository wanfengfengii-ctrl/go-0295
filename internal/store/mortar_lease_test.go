package store

import (
	"sync"
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
)

func TestConcurrentLastPowderOnlyOneWins(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// One mixer, but two operations race to withdraw the last powder.
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := e.SubmitCommand(id, Command{
				OperationID: "mix-" + string(rune('a'+i)),
				Type:        CommandMix,
				Generation:  1,
				LogicalTime: 10,
				MixerNumber: "mixer-1",
				Holder:      "op-" + string(rune('a'+i)),
				PowderGrams: 1000,
				WaterGrams:  250,
			})
			results[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		} else if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeLeaseBusy {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("want exactly one winner, got %d", wins)
	}

	lv, err := e.GetLedger(id)
	if err != nil {
		t.Fatal(err)
	}
	if !lv.Conservation {
		t.Fatal("ledger must conserve after race")
	}
	if lv.Mortar.Powder != 1000 || lv.Mortar.Water != 250 {
		t.Fatalf("unexpected balance: %+v", lv.Mortar)
	}
}

func TestLeaseBusyRollsBackMaterial(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Pre-acquire the mixer for a different holder.
	if _, err := e.AcquireLease(id, AcquireLeaseRequest{
		Kind: ledger.KindMixer, Number: "mixer-1", Holder: "other", LogicalTime: 5,
	}); err != nil {
		t.Fatal(err)
	}

	// Mix command tries to acquire the same mixer: it must fail and leave no
	// material withdrawal.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "mix-1", Type: CommandMix, Generation: 1, LogicalTime: 10,
		MixerNumber: "mixer-1", Holder: "me", PowderGrams: 1000, WaterGrams: 250,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeLeaseBusy {
		t.Fatalf("want lease busy, got %v", err)
	}

	lv, _ := e.GetLedger(id)
	if lv.Mortar.Powder != 0 || lv.Mortar.Water != 0 {
		t.Fatalf("material must be unchanged after rollback: %+v", lv.Mortar)
	}
}

func TestIdempotentRetrySameContent(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	cmd := Command{
		OperationID: "mix-1", Type: CommandMix, Generation: 1, LogicalTime: 10,
		MixerNumber: "mixer-1", Holder: "me", PowderGrams: 1000, WaterGrams: 250,
	}
	first, err := e.SubmitCommand(id, cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.SubmitCommand(id, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Detail != second.Detail {
		t.Fatalf("idempotent retry must reuse response: %q vs %q", first.Detail, second.Detail)
	}

	// Different content for the same operation id must conflict.
	cmd.WaterGrams = 300
	_, err = e.SubmitCommand(id, cmd)
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeIdempotencyConflict {
		t.Fatalf("want idempotency conflict, got %v", err)
	}
}

func TestWaterRatioViolation(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	// Water ratio is 1:4, so 1000g powder needs 250g water; 500g water violates.
	_, err := e.SubmitCommand(id, Command{
		OperationID: "mix-bad", Type: CommandMix, Generation: 1, LogicalTime: 10,
		MixerNumber: "mixer-1", Holder: "me", PowderGrams: 1000, WaterGrams: 500,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeWaterRatio {
		t.Fatalf("want water ratio violation, got %v", err)
	}
	lv, _ := e.GetLedger(id)
	if lv.Mortar.Powder != 0 {
		t.Fatalf("material must be unchanged: %+v", lv.Mortar)
	}
}

func TestRenewExpiredLeaseFails(t *testing.T) {
	e := newMemEngine(t)
	id := testLockedTask(t, e)

	lease, err := e.AcquireLease(id, AcquireLeaseRequest{
		Kind: ledger.KindDrill, Number: "drill-1", Holder: "me", LogicalTime: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Renew after expiry (now > lease.Expires).
	_, err = e.RenewLease(id, RenewLeaseRequest{
		Kind: ledger.KindDrill, Number: "drill-1", Token: lease.Token, LogicalTime: 10000,
	})
	if f, ok := err.(*domain.Failure); !ok || f.Code != domain.CodeLeaseExpired {
		t.Fatalf("want lease expired, got %v", err)
	}
}
