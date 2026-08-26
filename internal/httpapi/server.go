// Package httpapi is the Go HTTP API component. It exposes the JSON command and
// query endpoints with a unified transaction boundary, deterministic error
// ordering, recovery startup and health checking.
package httpapi

import (
	"encoding/json"
	"net/http"

	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/store"
)

// Server is the Go HTTP API component. It delegates all business logic to the
// transactional engine so every request is bounded by a single database
// transaction.
type Server struct {
	engine *store.Engine
}

// NewServer constructs an HTTP API server over the supplied engine.
func NewServer(engine *store.Engine) *Server {
	return &Server{engine: engine}
}

// Handler builds the HTTP route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /v1/tasks/{id}/lock", s.handleLockTask)
	mux.HandleFunc("POST /v1/tasks/{id}/commands", s.handleCommand)
	mux.HandleFunc("POST /v1/tasks/{id}/leases/acquire", s.handleAcquireLease)
	mux.HandleFunc("POST /v1/tasks/{id}/leases/renew", s.handleRenewLease)
	mux.HandleFunc("POST /v1/tasks/{id}/retests", s.handleRetest)
	mux.HandleFunc("POST /v1/tasks/{id}/generations", s.handleGeneration)
	mux.HandleFunc("POST /v1/tasks/{id}/reviews", s.handleReview)
	mux.HandleFunc("POST /v1/tasks/{id}/terminal-decisions", s.handleTerminal)
	mux.HandleFunc("GET /v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("GET /v1/tasks/{id}/coverage", s.handleGetCoverage)
	mux.HandleFunc("GET /v1/tasks/{id}/ledger", s.handleGetLedger)
	mux.HandleFunc("GET /v1/tasks/{id}/evidence", s.handleGetEvidence)
	mux.HandleFunc("GET /v1/tasks/{id}/retests", s.handleGetRetests)
	mux.HandleFunc("GET /v1/tasks/{id}/terminal", s.handleGetTerminal)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ver, err := s.engine.Verify()
	if err != nil {
		writeFailure(w, http.StatusServiceUnavailable, domain.Failure{
			Code:      domain.CodeRecovery,
			Reasons:   []domain.Reason{{Code: domain.CodeRecovery, Detail: err.Error()}},
			Retryable: false,
		})
		return
	}
	status := http.StatusOK
	body := map[string]any{"status": "ok", "recovery_checked": ver.OK}
	if !ver.OK {
		status = http.StatusServiceUnavailable
		body["status"] = "readonly"
		body["violations"] = ver.Violations
	}
	writeJSON(w, status, body)
}

// writeJSON serializes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeFailure writes the unified rejection structure {code,reasons,retryable}.
func writeFailure(w http.ResponseWriter, status int, f domain.Failure) {
	writeJSON(w, status, f)
}

// writeDomainError maps a domain failure or generic error to the unified
// rejection structure with an appropriate HTTP status.
func writeDomainError(w http.ResponseWriter, err error) {
	if f, ok := err.(*domain.Failure); ok {
		status := http.StatusBadRequest
		switch f.Code {
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeIdempotencyConflict, domain.CodeTerminalConflict, domain.CodeVersionConflict, domain.CodeLeaseBusy:
			status = http.StatusConflict
		}
		writeFailure(w, status, *f)
		return
	}
	writeFailure(w, http.StatusInternalServerError, domain.Failure{
		Code:      domain.CodeInvalid,
		Reasons:   []domain.Reason{{Code: domain.CodeInvalid, Detail: err.Error()}},
		Retryable: false,
	})
}
