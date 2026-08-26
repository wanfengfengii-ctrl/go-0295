package store

import (
	"testing"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
)

func TestModel_ReviewFirstSubmissionIsImmutable(t *testing.T) {
	cases := []struct {
		name              string
		first             ReviewRequest
		retry             ReviewRequest
		wantHandover      bool
		wantReturnedFirst bool
	}{
		{
			name:              "qualified approval survives contradictory retry",
			first:             ReviewRequest{Reviewer: "alice", Qualified: true, Opinion: "approve"},
			retry:             ReviewRequest{Reviewer: "alice", Qualified: false, Opinion: "reject"},
			wantHandover:      true,
			wantReturnedFirst: true,
		},
		{
			name:              "retry cannot upgrade an unqualified rejection",
			first:             ReviewRequest{Reviewer: "alice", Qualified: false, Opinion: "reject"},
			retry:             ReviewRequest{Reviewer: "alice", Qualified: true, Opinion: "approve"},
			wantHandover:      false,
			wantReturnedFirst: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newMemEngine(t)
			id := testLockedTask(t, e)

			first, err := e.Review(id, tc.first)
			if err != nil {
				t.Fatalf("first review: %v", err)
			}
			retried, err := e.Review(id, tc.retry)
			if err != nil {
				t.Fatalf("retry review: %v", err)
			}
			if tc.wantReturnedFirst && retried != first {
				t.Fatalf("retry returned %+v, want immutable first review %+v", retried, first)
			}

			if _, err := e.Review(id, ReviewRequest{Reviewer: "bob", Qualified: true, Opinion: "approve"}); err != nil {
				t.Fatalf("bob review: %v", err)
			}
			decision, err := e.Terminal(id, TerminalRequest{
				Kind: arbiter.TerminalHandover, Reviewer: "bob", Qualified: true, LogicalTime: 20,
			})
			if tc.wantHandover {
				if err != nil {
					t.Fatalf("handover after two immutable approvals: %v", err)
				}
				if decision.Kind != arbiter.TerminalHandover || decision.Credential == "" {
					t.Fatalf("handover decision = %+v, want credentialed handover", decision)
				}
				return
			}

			failure, ok := err.(*domain.Failure)
			if !ok || failure.Code != domain.CodeReviewInsufficient {
				t.Fatalf("terminal error = %v, want review insufficient", err)
			}
		})
	}
}
