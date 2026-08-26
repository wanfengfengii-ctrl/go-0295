package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/store"
)

func jsonBody(s string) *strings.Reader { return strings.NewReader(s) }

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.Open("")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewServer(store.NewEngine(db))
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("got %v", body)
	}
	if body["recovery_checked"] != true {
		t.Fatalf("got %v", body)
	}
}

func TestCreateTaskRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks", jsonBody(`{"building":"b1","facade_zone":"z1","wall_type":"concrete"}`))
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	get := httptest.NewRecorder()
	srv.Handler().ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/v1/tasks/b1%2Fz1", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get status %d", get.Code)
	}
}

func TestWriteFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFailure(rec, http.StatusBadRequest, domain.Failure{
		Code:      domain.CodeOverlap,
		Reasons:   []domain.Reason{{Code: domain.CodeOverlap, Field: "cell", Detail: "overlap"}},
		Retryable: false,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Code      string          `json:"code"`
		Reasons   []domain.Reason `json:"reasons"`
		Retryable bool            `json:"retryable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "coverage_overlap" || body.Retryable {
		t.Fatalf("got %+v", body)
	}
	if len(body.Reasons) != 1 || body.Reasons[0].Field != "cell" {
		t.Fatalf("got %+v", body.Reasons)
	}
}
