package store

import (
	"sync"
	"testing"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
)

func TestModel_TerminalUsesOnlyCurrentTaskReviews(t *testing.T) {
	tests := []struct {
		name             string
		otherReviewers   []string
		currentReviewers []string
		kinds            []arbiter.TerminalKind
		wantFailure      domain.ErrorCode
		wantWinners      int
	}{
		{
			name:           "approvals on another facade do not authorize handover",
			otherReviewers: []string{"alice", "bob"},
			kinds:          []arbiter.TerminalKind{arbiter.TerminalHandover},
			wantFailure:    domain.CodeReviewInsufficient,
		},
		{
			name:             "repeat approval by one reviewer remains insufficient",
			currentReviewers: []string{"alice", "alice"},
			kinds:            []arbiter.TerminalKind{arbiter.TerminalHandover},
			wantFailure:      domain.CodeReviewInsufficient,
		},
		{
			name:             "two current qualified reviewers enter one terminal competition",
			currentReviewers: []string{"alice", "bob"},
			kinds: []arbiter.TerminalKind{
				arbiter.TerminalHandover,
				arbiter.TerminalQuarantine,
				arbiter.TerminalCancel,
			},
			wantWinners: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newMemEngine(t)
			other, err := e.CreateTask(CreateTaskRequest{Building: "tower", FacadeZone: "east", WallType: "concrete"})
			if err != nil {
				t.Fatalf("create other task: %v", err)
			}
			current, err := e.CreateTask(CreateTaskRequest{Building: "tower", FacadeZone: "west", WallType: "concrete"})
			if err != nil {
				t.Fatalf("create current task: %v", err)
			}

			for _, reviewer := range tt.otherReviewers {
				if _, err := e.Review(other.ID, ReviewRequest{Reviewer: reviewer, Qualified: true, Opinion: "approve"}); err != nil {
					t.Fatalf("review other task by %s: %v", reviewer, err)
				}
			}
			for _, reviewer := range tt.currentReviewers {
				if _, err := e.Review(current.ID, ReviewRequest{Reviewer: reviewer, Qualified: true, Opinion: "approve"}); err != nil {
					t.Fatalf("review current task by %s: %v", reviewer, err)
				}
			}

			start := make(chan struct{})
			decisions := make([]arbiter.TerminalDecision, len(tt.kinds))
			errs := make([]error, len(tt.kinds))
			var wg sync.WaitGroup
			for i, kind := range tt.kinds {
				wg.Add(1)
				go func(i int, kind arbiter.TerminalKind) {
					defer wg.Done()
					<-start
					decisions[i], errs[i] = e.Terminal(current.ID, TerminalRequest{
						Kind: kind, Reviewer: "bob", Qualified: true, LogicalTime: 42,
					})
				}(i, kind)
			}
			close(start)
			wg.Wait()

			winners := 0
			var winner arbiter.TerminalDecision
			for i, err := range errs {
				if err == nil {
					winners++
					winner = decisions[i]
					continue
				}
				failure, ok := err.(*domain.Failure)
				if !ok {
					t.Fatalf("terminal attempt %d returned non-domain error: %v", i, err)
				}
				wantCode := tt.wantFailure
				if wantCode == "" {
					wantCode = domain.CodeTerminalConflict
				}
				if failure.Code != wantCode {
					t.Fatalf("terminal attempt %d code = %q, want %q", i, failure.Code, wantCode)
				}
			}
			if winners != tt.wantWinners {
				t.Fatalf("terminal winners = %d, want %d", winners, tt.wantWinners)
			}

			stored, err := e.GetTerminal(current.ID)
			if err != nil {
				t.Fatalf("get terminal: %v", err)
			}
			if tt.wantWinners == 0 {
				if stored != nil {
					t.Fatalf("rejected terminal request persisted %+v", *stored)
				}
				return
			}
			if stored == nil || stored.Kind != winner.Kind {
				t.Fatalf("stored terminal = %+v, winner = %+v", stored, winner)
			}
			if winner.Kind == arbiter.TerminalHandover && winner.Credential == "" {
				t.Fatal("handover winner has no credential")
			}
			if winner.Kind != arbiter.TerminalHandover && winner.Credential != "" {
				t.Fatalf("non-handover winner has credential %q", winner.Credential)
			}
		})
	}
}
