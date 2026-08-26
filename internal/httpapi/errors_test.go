package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/store"
)

func TestUnifiedErrorStructure(t *testing.T) {
	srv := newTestServer(t)

	// Invalid JSON field.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/tasks", jsonBody("not-json")))
	assertUnifiedFailure(t, rec, http.StatusBadRequest)

	// Unknown task id -> not_found with unified structure.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/tasks/missing", nil))
	assertUnifiedFailure(t, rec, http.StatusNotFound)

	// Stale digest on lock returns a deterministic failure structure.
	if _, err := srv.engine.CreateTask(store.CreateTaskRequest{Building: "b", FacadeZone: "z", WallType: "concrete"}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/tasks/b%2Fz/lock", jsonBody(`{
		"wall_type":"concrete","fixed_scale":1000,"materials":{},"sampling":{},
		"expected_digest":"deadbeef","thresholds":{"fixed_scale":1000},
		"layout":{"rows":1,"cols":1},"boards":[]
	}`)))
	assertUnifiedFailure(t, rec, http.StatusBadRequest)
	var body domain.Failure
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != domain.CodeStaleDigest {
		t.Fatalf("want stale digest, got %q", body.Code)
	}
}

func assertUnifiedFailure(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status %d want %d body %s", rec.Code, wantStatus, rec.Body.String())
	}
	var body domain.Failure
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not unified failure JSON: %v (%s)", err, rec.Body.String())
	}
	if body.Code == "" {
		t.Fatal("failure code must be non-empty")
	}
}
