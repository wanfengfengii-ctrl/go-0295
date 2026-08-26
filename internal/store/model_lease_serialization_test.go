package store_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
	"rockwool-facade-render-handover/internal/store"
)

func TestModel_LeaseAcquisitionIsSerializedPerResource(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(2)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })

	tests := []struct {
		name string
		run  func(t *testing.T, engine *store.Engine, taskA, taskB string)
	}{
		{
			name: "overlapping requests for one drill have one winner",
			run: func(t *testing.T, engine *store.Engine, taskA, taskB string) {
				// A populated lease bucket makes both competing conflict scans
				// substantial enough to exercise the transaction boundary, while
				// every warm-up lease names an unrelated resource.
				for i := 0; i < 256; i++ {
					if _, err := engine.AcquireLease(taskA, store.AcquireLeaseRequest{
						Kind: ledger.KindDrill, Number: fmt.Sprintf("warmup-%03d", i),
						Holder: "warmup", LogicalTime: 1,
					}); err != nil {
						t.Fatalf("warm up lease bucket: %v", err)
					}
				}

				for round := 0; round < 32; round++ {
					resource := fmt.Sprintf("drill-race-%02d", round)
					start := make(chan struct{})
					var wg sync.WaitGroup
					leases := make([]ledger.Lease, 2)
					errs := make([]error, 2)
					tasks := []string{taskA, taskB}
					holders := []string{"facade-team-a", "facade-team-b"}

					for i := range tasks {
						wg.Add(1)
						go func(i int) {
							defer wg.Done()
							<-start
							leases[i], errs[i] = engine.AcquireLease(tasks[i], store.AcquireLeaseRequest{
								Kind: ledger.KindDrill, Number: resource,
								Holder: holders[i], LogicalTime: 10,
							})
						}(i)
					}
					close(start)
					wg.Wait()

					winner := -1
					busy := 0
					for i, err := range errs {
						if err == nil {
							if leases[i].Token == "" {
								t.Fatalf("round %d: successful lease has empty token", round)
							}
							if winner != -1 {
								t.Fatalf("round %d: both holders acquired %s", round, resource)
							}
							winner = i
							continue
						}
						failure, ok := err.(*domain.Failure)
						if !ok || failure.Code != domain.CodeLeaseBusy {
							t.Fatalf("round %d: loser error = %v, want %s", round, err, domain.CodeLeaseBusy)
						}
						busy++
					}
					if winner == -1 || busy != 1 {
						t.Fatalf("round %d: winner=%d busy=%d, want one of each", round, winner, busy)
					}

					renewed, err := engine.RenewLease(tasks[winner], store.RenewLeaseRequest{
						Kind: ledger.KindDrill, Number: resource,
						Token: leases[winner].Token, LogicalTime: 11,
					})
					if err != nil {
						t.Fatalf("round %d: renew winning lease: %v", round, err)
					}
					if renewed.Token != leases[winner].Token || renewed.Expires <= leases[winner].Expires {
						t.Fatalf("round %d: renewal did not extend winning lease: before=%+v after=%+v", round, leases[winner], renewed)
					}
				}
			},
		},
		{
			name: "different drills can be acquired concurrently",
			run: func(t *testing.T, engine *store.Engine, taskA, taskB string) {
				start := make(chan struct{})
				var wg sync.WaitGroup
				errs := make([]error, 2)
				tasks := []string{taskA, taskB}
				for i := range tasks {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						<-start
						_, errs[i] = engine.AcquireLease(tasks[i], store.AcquireLeaseRequest{
							Kind: ledger.KindDrill, Number: fmt.Sprintf("independent-drill-%d", i),
							Holder: fmt.Sprintf("team-%d", i), LogicalTime: 20,
						})
					}(i)
				}
				close(start)
				wg.Wait()
				for i, err := range errs {
					if err != nil {
						t.Fatalf("resource %d acquire: %v", i, err)
					}
				}
			},
		},
		{
			name: "expired drill lease can be reused and renewed",
			run: func(t *testing.T, engine *store.Engine, taskA, taskB string) {
				first, err := engine.AcquireLease(taskA, store.AcquireLeaseRequest{
					Kind: ledger.KindDrill, Number: "reusable-drill", Holder: "team-a", LogicalTime: 30,
				})
				if err != nil {
					t.Fatalf("first acquire: %v", err)
				}
				reused, err := engine.AcquireLease(taskB, store.AcquireLeaseRequest{
					Kind: ledger.KindDrill, Number: "reusable-drill", Holder: "team-b", LogicalTime: first.Expires,
				})
				if err != nil {
					t.Fatalf("reuse at expiry: %v", err)
				}
				renewed, err := engine.RenewLease(taskB, store.RenewLeaseRequest{
					Kind: ledger.KindDrill, Number: "reusable-drill", Token: reused.Token, LogicalTime: reused.Acquired + 1,
				})
				if err != nil {
					t.Fatalf("renew reused lease: %v", err)
				}
				if renewed.Expires <= reused.Expires {
					t.Fatalf("renewal expiry = %d, want after %d", renewed.Expires, reused.Expires)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open("")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			engine := store.NewEngine(db)
			create := func(building, zone string) string {
				task, err := engine.CreateTask(store.CreateTaskRequest{
					Building: building, FacadeZone: zone, WallType: "rockwool",
				})
				if err != nil {
					t.Fatalf("create task: %v", err)
				}
				return task.ID
			}
			tc.run(t, engine, create("building-a", "north"), create("building-b", "south"))
		})
	}
}
