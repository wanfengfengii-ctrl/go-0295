package httpapi

import (
	"net/http"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
)

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.engine.GetTask(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleGetCoverage(w http.ResponseWriter, r *http.Request) {
	view, err := s.engine.GetCoverage(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetLedger(w http.ResponseWriter, r *http.Request) {
	view, err := s.engine.GetLedger(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	view, err := s.engine.GetEvidence(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetRetests(w http.ResponseWriter, r *http.Request) {
	retests, err := s.engine.GetRetests(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if retests == nil {
		retests = []arbiter.RetestSet{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"retests": retests})
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	dec, err := s.engine.GetTerminal(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if dec == nil {
		writeJSON(w, http.StatusOK, map[string]any{"terminal": nil})
		return
	}
	writeJSON(w, http.StatusOK, dec)
}

// ensure domain is referenced so the package keeps its error-mapping import
// when the handler set changes.
var _ = domain.CodeInvalid
