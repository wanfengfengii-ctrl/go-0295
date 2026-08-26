package arbiter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"rockwool-facade-render-handover/internal/domain"
)

// TerminalDecision is the single immutable terminal outcome.
type TerminalDecision struct {
	Kind        TerminalKind       `json:"kind"`
	Credential  string             `json:"credential,omitempty"`
	LogicalTime domain.LogicalTime `json:"logical_time"`
	Reviewer    string             `json:"reviewer"`
}

// Review is a single independent review by a reviewer.
type Review struct {
	Reviewer  string `json:"reviewer"`
	Qualified bool   `json:"qualified"`
	Opinion   string `json:"opinion"`
}

// TerminalRequest requests a terminal decision.
type TerminalRequest struct {
	Kind        TerminalKind
	Reviewer    string
	Qualified   bool
	LogicalTime domain.LogicalTime
	TaskID      string
}

// MeetsReviewQuorum reports whether two distinct qualified reviewers have
// independently approved. A single reviewer repeating their opinion does not
// satisfy the two-person requirement.
func MeetsReviewQuorum(reviews []Review) bool {
	approved := make(map[string]struct{})
	for _, r := range reviews {
		if r.Qualified && r.Opinion == "approve" {
			approved[r.Reviewer] = struct{}{}
		}
	}
	return len(approved) >= 2
}

// Decide performs the single-writer terminal competition. If a terminal
// decision already exists it reports false (the competing request loses); when
// the request wins it returns the immutable decision. Handover decisions carry
// a unique credential derived from the task id and logical time.
func Decide(existing *TerminalDecision, req TerminalRequest) (TerminalDecision, bool) {
	if existing != nil {
		return TerminalDecision{}, false
	}
	dec := TerminalDecision{
		Kind:        req.Kind,
		LogicalTime: req.LogicalTime,
		Reviewer:    req.Reviewer,
	}
	if req.Kind == TerminalHandover {
		dec.Credential = CredentialFor(req.TaskID, req.LogicalTime)
	}
	return dec, true
}

// CredentialFor derives the unique handover credential deterministically.
func CredentialFor(taskID string, at domain.LogicalTime) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", taskID, int64(at))))
	return "HW-" + hex.EncodeToString(h[:])[:16]
}
