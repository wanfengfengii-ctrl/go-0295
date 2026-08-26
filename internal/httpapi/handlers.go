package httpapi

import (
	"encoding/json"
	"net/http"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/catalog"
	"rockwool-facade-render-handover/internal/coverage"
	"rockwool-facade-render-handover/internal/domain"
	"rockwool-facade-render-handover/internal/ledger"
	"rockwool-facade-render-handover/internal/store"
)

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Building   string `json:"building"`
		FacadeZone string `json:"facade_zone"`
		WallType   string `json:"wall_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	task, err := s.engine.CreateTask(store.CreateTaskRequest{
		Building: req.Building, FacadeZone: req.FacadeZone, WallType: req.WallType,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) handleLockTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WallType       string                    `json:"wall_type"`
		Materials      map[string]string         `json:"materials"`
		FixedScale     int64                     `json:"fixed_scale"`
		Sampling       map[string]string         `json:"sampling"`
		ExpectedDigest string                    `json:"expected_digest"`
		Thresholds     catalog.Thresholds        `json:"thresholds"`
		Layout         coverage.Layout           `json:"layout"`
		Boards         []coverage.BoardPlacement `json:"boards"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	result, err := s.engine.LockTask(r.PathValue("id"), store.LockTaskRequest{
		Snapshot: catalog.Snapshot{
			WallType: req.WallType, Materials: req.Materials, FixedScale: req.FixedScale, Sampling: req.Sampling,
		},
		Thresholds:     req.Thresholds,
		Layout:         req.Layout,
		Boards:         req.Boards,
		ExpectedDigest: req.ExpectedDigest,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	var cmd store.Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	result, err := s.engine.SubmitCommand(r.PathValue("id"), cmd)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAcquireLease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind        ledger.ResourceKind `json:"kind"`
		Number      string              `json:"number"`
		Holder      string              `json:"holder"`
		LogicalTime domain.LogicalTime  `json:"logical_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	lease, err := s.engine.AcquireLease(r.PathValue("id"), store.AcquireLeaseRequest{
		Kind: req.Kind, Number: req.Number, Holder: req.Holder, LogicalTime: req.LogicalTime,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

func (s *Server) handleRenewLease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind        ledger.ResourceKind `json:"kind"`
		Number      string              `json:"number"`
		Token       string              `json:"token"`
		LogicalTime domain.LogicalTime  `json:"logical_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	lease, err := s.engine.RenewLease(r.PathValue("id"), store.RenewLeaseRequest{
		Kind: req.Kind, Number: req.Number, Token: req.Token, LogicalTime: req.LogicalTime,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

func (s *Server) handleRetest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceBoard      string            `json:"source_board"`
		SourceGeneration domain.Generation `json:"source_generation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	rs, err := s.engine.Retest(r.PathValue("id"), req.SourceBoard, req.SourceGeneration)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) handleGeneration(w http.ResponseWriter, r *http.Request) {
	gen, err := s.engine.NewGeneration(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.Generation{"generation": gen})
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reviewer  string `json:"reviewer"`
		Qualified bool   `json:"qualified"`
		Opinion   string `json:"opinion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	review, err := s.engine.Review(r.PathValue("id"), store.ReviewRequest{
		Reviewer: req.Reviewer, Qualified: req.Qualified, Opinion: req.Opinion,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind        arbiter.TerminalKind `json:"kind"`
		Reviewer    string               `json:"reviewer"`
		Qualified   bool                 `json:"qualified"`
		LogicalTime domain.LogicalTime   `json:"logical_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeFailure(w, http.StatusBadRequest, domain.Failure{Code: domain.CodeInvalid,
			Reasons: []domain.Reason{{Code: domain.CodeInvalid, Detail: "invalid json"}}})
		return
	}
	dec, err := s.engine.Terminal(r.PathValue("id"), store.TerminalRequest{
		Kind: req.Kind, Reviewer: req.Reviewer, Qualified: req.Qualified, LogicalTime: req.LogicalTime,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dec)
}
