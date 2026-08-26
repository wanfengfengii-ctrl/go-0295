package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
	"rockwool-facade-render-handover/internal/store"
)

func TestModel_RecoveryLeaseActivityUsesOverlappingLogicalWindows(t *testing.T) {
	tests := []struct {
		name          string
		acquired      [2]domain.LogicalTime
		wantHealth    int
		wantStatus    string
		wantWritable  bool
		wantViolation string
	}{
		{
			name:         "expired historical lease does not quarantine recovery",
			acquired:     [2]domain.LogicalTime{0, 1000},
			wantHealth:   http.StatusOK,
			wantStatus:   "ok",
			wantWritable: true,
		},
		{
			name:          "overlapping active leases remain a recovery conflict",
			acquired:      [2]domain.LogicalTime{1000, 1050},
			wantHealth:    http.StatusServiceUnavailable,
			wantStatus:    "readonly",
			wantViolation: "duplicate active lease mixer/mixer-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := store.Open("")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			engine := store.NewEngine(db)

			for i, zone := range []string{"zone-old", "zone-new"} {
				task, err := engine.CreateTask(store.CreateTaskRequest{
					Building: "building-1", FacadeZone: zone, WallType: "concrete",
				})
				if err != nil {
					t.Fatalf("create lease holder task %d: %v", i, err)
				}
				if _, err := engine.AcquireLease(task.ID, store.AcquireLeaseRequest{
					Kind: ledger.KindMixer, Number: "mixer-1",
					Holder: "crew-" + zone, LogicalTime: tt.acquired[i],
				}); err != nil {
					if i == 1 && tt.wantViolation != "" {
						// The normal API correctly prevents a live collision. Persist the
						// contradictory second record to exercise recovery's detector.
						err = db.Update(func(tx *store.Tx) error {
							return tx.PutJSON(store.BucketLeases, task.ID+"/mixer/mixer-1", ledger.Lease{
								Kind: ledger.KindMixer, Number: "mixer-1", Holder: "crew-" + zone,
								Token: "persisted-conflicting-token", Acquired: tt.acquired[i],
								Expires: tt.acquired[i] + ledger.LeaseDuration,
							})
						})
						if err != nil {
							t.Fatalf("persist contradictory lease: %v", err)
						}
					} else {
						t.Fatalf("acquire lease %d at %d: %v", i, tt.acquired[i], err)
					}
				}
			}

			rec := httptest.NewRecorder()
			NewServer(engine).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if rec.Code != tt.wantHealth {
				t.Fatalf("health status = %d, want %d; body=%s", rec.Code, tt.wantHealth, rec.Body.String())
			}
			var health struct {
				Status     string   `json:"status"`
				Checked    bool     `json:"recovery_checked"`
				Violations []string `json:"violations"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
				t.Fatalf("decode health: %v", err)
			}
			if health.Status != tt.wantStatus || health.Checked != tt.wantWritable {
				t.Fatalf("health = %+v, want status=%q recovery_checked=%v", health, tt.wantStatus, tt.wantWritable)
			}
			if tt.wantViolation != "" && (len(health.Violations) == 0 || !strings.Contains(health.Violations[0], tt.wantViolation)) {
				t.Fatalf("violations = %v, want one containing %q", health.Violations, tt.wantViolation)
			}

			_, writeErr := engine.CreateTask(store.CreateTaskRequest{
				Building: "building-1", FacadeZone: "write-probe", WallType: "concrete",
			})
			if tt.wantWritable && writeErr != nil {
				t.Fatalf("healthy recovery unexpectedly quarantined writes: %v", writeErr)
			}
			if !tt.wantWritable && !errors.Is(writeErr, store.ErrReadOnly) {
				t.Fatalf("conflicting active leases did not quarantine writes: %v", writeErr)
			}
		})
	}
}
