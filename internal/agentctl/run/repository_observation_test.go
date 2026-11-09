package run

import (
	"context"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type observationScanner struct{ snapshot observation.Snapshot }

func (s observationScanner) Scan(context.Context) ([]observation.Snapshot, error) {
	return []observation.Snapshot{s.snapshot}, nil
}

func TestRestoreWithRepositoryEvidenceAndFollowup(t *testing.T) {
	directory := t.TempDir()
	snapshot := observation.Snapshot{Checkout: observation.Checkout{Path: "repo", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: "skill/branch", CommitID: "one", Candidate: true}}}
	recordRecoveryHistory(t, directory, interaction.Event{Type: interaction.EventRepositoryObserved, RepositoryObservation: &snapshot})
	process := newRecoverySession()
	controller := NewRunController(context.Background(), recoveryRuntimeHost{process}, directory, nil, observationScanner{snapshot})
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.RestoreSession(); err != nil {
		t.Fatal(err)
	}
	_, events, stop, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := controller.Prompt("followup", "continue after restoring"); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-process.calls:
		if text != "continue after restoring" {
			t.Fatal(text)
		}
	case <-time.After(time.Second):
		t.Fatal("follow-up did not reach restored runtime")
	}
	process.release <- struct{}{}
	waitRecoveryEvent(t, events, interaction.EventRunCompleted)
	history, _, unsubscribe, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	observations := 0
	for _, event := range history {
		if event.RepositoryObservation != nil {
			observations++
			if !event.RepositoryObservation.Branches[0].Candidate {
				t.Fatal("restored ownership lost")
			}
		}
	}
	if observations != 1 {
		t.Fatalf("restore duplicated evidence: %d", observations)
	}
	store, err := loadRepositoryObservations(directory, "org", "workspace", "task")
	if err != nil || len(store.history()) != 1 {
		t.Fatalf("task baseline not retained across runs: %+v %v", store, err)
	}
}
