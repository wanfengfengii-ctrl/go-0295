package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"

	"rockwool-facade-render-handover/internal/httpapi"
	"rockwool-facade-render-handover/internal/ledger"
	"rockwool-facade-render-handover/internal/store"
)

func TestModel_RecoveryRejectsUnreplayableMortarRecord(t *testing.T) {
	validRecord, err := json.Marshal(ledger.MortarState{
		Batch: "batch-1", Powder: 1000, Water: 250, Remainder: 1250,
		Glue: map[string]int64{},
	})
	if err != nil {
		t.Fatalf("marshal valid mortar record: %v", err)
	}

	cases := []struct {
		name              string
		record            []byte
		wantRecoveryOK    bool
		wantHealthStatus  int
		wantHealthState   string
		wantViolationText string
		wantReadOnly      bool
	}{
		{
			name:             "valid persisted ledger remains healthy",
			record:           validRecord,
			wantRecoveryOK:   true,
			wantHealthStatus: http.StatusOK,
			wantHealthState:  "ok",
		},
		{
			name:              "malformed persisted ledger is isolated",
			record:            []byte(`{"batch":"batch-1","powder":1000`),
			wantRecoveryOK:    false,
			wantHealthStatus:  http.StatusServiceUnavailable,
			wantHealthState:   "readonly",
			wantViolationText: "unrecoverable mortar ledger record",
			wantReadOnly:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rockwool.db")
			initialized, err := store.Open(path)
			if err != nil {
				t.Fatalf("initialize store: %v", err)
			}
			if err := initialized.Close(); err != nil {
				t.Fatalf("close initialized store: %v", err)
			}

			rawDB, err := bolt.Open(path, 0o600, nil)
			if err != nil {
				t.Fatalf("open persisted database: %v", err)
			}
			if err := rawDB.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(store.BucketMortar).Put([]byte("task-1/1"), tc.record)
			}); err != nil {
				rawDB.Close()
				t.Fatalf("seed persisted mortar record: %v", err)
			}
			if err := rawDB.Close(); err != nil {
				t.Fatalf("close seeded database: %v", err)
			}

			db, err := store.Open(path)
			if err != nil {
				t.Fatalf("reopen store: %v", err)
			}
			engine := store.NewEngine(db)
			verification, err := engine.Verify()
			if err != nil {
				db.Close()
				t.Fatalf("startup recovery returned transport error: %v", err)
			}
			if verification.OK != tc.wantRecoveryOK {
				db.Close()
				t.Fatalf("startup recovery OK = %v, want %v; violations=%v", verification.OK, tc.wantRecoveryOK, verification.Violations)
			}
			joinedViolations := strings.Join(verification.Violations, "\n")
			if tc.wantViolationText == "" && len(verification.Violations) != 0 {
				db.Close()
				t.Fatalf("valid recovery reported violations: %v", verification.Violations)
			}
			if tc.wantViolationText != "" && !strings.Contains(joinedViolations, tc.wantViolationText) {
				db.Close()
				t.Fatalf("startup recovery violations %q do not identify the unreadable record", joinedViolations)
			}

			handler := httpapi.NewServer(engine).Handler()
			var firstBody []byte
			for requestNumber := 0; requestNumber < 2; requestNumber++ {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
				if recorder.Code != tc.wantHealthStatus {
					db.Close()
					t.Fatalf("health attempt %d status = %d, want %d; body=%s", requestNumber+1, recorder.Code, tc.wantHealthStatus, recorder.Body.String())
				}
				var body struct {
					Status          string   `json:"status"`
					RecoveryChecked bool     `json:"recovery_checked"`
					Violations      []string `json:"violations"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					db.Close()
					t.Fatalf("decode health response: %v", err)
				}
				if body.Status != tc.wantHealthState || body.RecoveryChecked != tc.wantRecoveryOK {
					db.Close()
					t.Fatalf("health attempt %d body = %+v", requestNumber+1, body)
				}
				if strings.Join(body.Violations, "\n") != joinedViolations {
					db.Close()
					t.Fatalf("health violations %v differ from startup recovery %v", body.Violations, verification.Violations)
				}
				if requestNumber == 0 {
					firstBody = append([]byte(nil), recorder.Body.Bytes()...)
				} else if !bytes.Equal(firstBody, recorder.Body.Bytes()) {
					db.Close()
					t.Fatalf("health response changed between checks: %q then %q", firstBody, recorder.Body.Bytes())
				}
			}

			writeErr := db.Update(func(*store.Tx) error { return nil })
			if tc.wantReadOnly && !errors.Is(writeErr, store.ErrReadOnly) {
				db.Close()
				t.Fatalf("write after failed recovery = %v, want read-only isolation", writeErr)
			}
			if !tc.wantReadOnly && writeErr != nil {
				db.Close()
				t.Fatalf("valid recovery unexpectedly blocked writes: %v", writeErr)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close recovered store: %v", err)
			}

			checkDB, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true})
			if err != nil {
				t.Fatalf("reopen database for record check: %v", err)
			}
			var persisted []byte
			if err := checkDB.View(func(tx *bolt.Tx) error {
				persisted = append([]byte(nil), tx.Bucket(store.BucketMortar).Get([]byte("task-1/1"))...)
				return nil
			}); err != nil {
				checkDB.Close()
				t.Fatalf("read persisted mortar record: %v", err)
			}
			if err := checkDB.Close(); err != nil {
				t.Fatalf("close record check database: %v", err)
			}
			if !bytes.Equal(persisted, tc.record) {
				t.Fatalf("recovery rewrote persisted business data: got %q, want %q", persisted, tc.record)
			}
		})
	}
}
