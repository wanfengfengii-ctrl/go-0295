package store

import (
	"bytes"
	"reflect"
	"testing"

	"rockwool-facade-render-handover/internal/arbiter"
	"rockwool-facade-render-handover/internal/domain"
)

func TestModel_TerminalIsIrreversible(t *testing.T) {
	tests := []struct {
		name   string
		winner arbiter.TerminalKind
		action string
	}{
		{name: "handover_commits_terminal_state_and_event", winner: arbiter.TerminalHandover, action: "observe"},
		{name: "handover_rejects_commands", winner: arbiter.TerminalHandover, action: "command"},
		{name: "handover_rejects_generations", winner: arbiter.TerminalHandover, action: "generation"},
		{name: "handover_rejects_competing_terminal", winner: arbiter.TerminalHandover, action: "terminal"},
		{name: "quarantine_rejects_commands", winner: arbiter.TerminalQuarantine, action: "command"},
		{name: "quarantine_rejects_generations", winner: arbiter.TerminalQuarantine, action: "generation"},
		{name: "quarantine_rejects_competing_terminal", winner: arbiter.TerminalQuarantine, action: "terminal"},
		{name: "cancel_rejects_commands", winner: arbiter.TerminalCancel, action: "command"},
		{name: "cancel_rejects_generations", winner: arbiter.TerminalCancel, action: "generation"},
		{name: "cancel_rejects_competing_terminal", winner: arbiter.TerminalCancel, action: "terminal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newMemEngine(t)
			id := testLockedTask(t, e)
			for _, reviewer := range []string{"alice", "bob"} {
				if _, err := e.Review(id, ReviewRequest{Reviewer: reviewer, Qualified: true, Opinion: "approve"}); err != nil {
					t.Fatalf("review %s: %v", reviewer, err)
				}
			}

			won, err := e.Terminal(id, TerminalRequest{
				Kind: tt.winner, Reviewer: "bob", Qualified: true, LogicalTime: 20,
			})
			if err != nil {
				t.Fatalf("winning terminal: %v", err)
			}
			if tt.winner == arbiter.TerminalHandover && won.Credential == "" {
				t.Fatal("handover terminal has no credential")
			}

			beforeTask, err := e.GetTask(id)
			if err != nil {
				t.Fatal(err)
			}
			beforeEvidence, err := e.GetEvidence(id)
			if err != nil {
				t.Fatal(err)
			}
			beforeTerminal, err := e.GetTerminal(id)
			if err != nil {
				t.Fatal(err)
			}

			terminalEvents := 0
			if err := e.db.View(func(tx *Tx) error {
				return tx.bucket(BucketEvents).ForEach(func(_, value []byte) error {
					if bytes.HasPrefix(value, []byte("terminal task="+id+" kind=")) {
						terminalEvents++
					}
					return nil
				})
			}); err != nil {
				t.Fatalf("read events: %v", err)
			}
			if beforeTask.Status != domain.TaskTerminal || terminalEvents != 1 || beforeTerminal == nil || !reflect.DeepEqual(*beforeTerminal, won) {
				t.Fatalf("terminal record, task state, and event not committed together: task=%+v events=%d stored=%+v won=%+v", beforeTask, terminalEvents, beforeTerminal, won)
			}

			for attempt := 1; attempt <= 2 && tt.action != "observe"; attempt++ {
				switch tt.action {
				case "command":
					_, err = e.SubmitCommand(id, Command{
						OperationID: "after-terminal", Type: CommandBase, BoardID: "a",
						Generation: beforeTask.Generation, LogicalTime: 21,
					})
				case "generation":
					_, err = e.NewGeneration(id)
				case "terminal":
					competitor := arbiter.TerminalCancel
					if tt.winner == arbiter.TerminalCancel {
						competitor = arbiter.TerminalHandover
					}
					_, err = e.Terminal(id, TerminalRequest{
						Kind: competitor, Reviewer: "alice", Qualified: true, LogicalTime: 21,
					})
				}
				failure, ok := err.(*domain.Failure)
				if !ok || failure.Code != domain.CodeTerminalConflict {
					t.Fatalf("attempt %d: want terminal conflict, got %v", attempt, err)
				}
			}

			afterTask, _ := e.GetTask(id)
			afterEvidence, _ := e.GetEvidence(id)
			afterTerminal, _ := e.GetTerminal(id)
			if !reflect.DeepEqual(afterTask, beforeTask) {
				t.Fatalf("task changed after terminal: before=%+v after=%+v", beforeTask, afterTask)
			}
			if !reflect.DeepEqual(afterEvidence, beforeEvidence) {
				t.Fatalf("evidence changed after terminal: before=%+v after=%+v", beforeEvidence, afterEvidence)
			}
			if !reflect.DeepEqual(afterTerminal, beforeTerminal) {
				t.Fatalf("terminal credential changed: before=%+v after=%+v", beforeTerminal, afterTerminal)
			}
		})
	}
}
