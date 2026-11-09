package run

import (
	"context"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/agentctl/repositoryobserver"
	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type observationScanner struct{ snapshot observation.Snapshot }

func (s observationScanner) Scan(context.Context) ([]observation.Snapshot, error) {
	return []observation.Snapshot{s.snapshot}, nil
}

func TestRestoreWithRepositoryEvidenceAndFollowup(t *testing.T) {
	directory := t.TempDir()
	snapshot := observation.Snapshot{Checkout: observation.Checkout{Path: "repo", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: "skill/branch", CommitID: "one", EligibleForLinking: true}}}
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
			if !event.RepositoryObservation.Branches[0].EligibleForLinking {
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

func TestObservationWriteFailurePreservesJournalEvidenceAndAcceptedState(t *testing.T) {
	directory := t.TempDir()
	process := newRecoverySession()
	conversation := newConversation(context.Background(), directory, nil)
	defer conversation.cancel()
	conversation.bind(recoveryConfig(), process)
	journal, _, err := loadJournal(directory, recoveryConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	conversation.journal = journal
	initial := observation.Snapshot{Checkout: observation.Checkout{Path: "repo", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: "main", CommitID: "one"}}}
	conversation.repositoryScanner = observationScanner{initial}
	if err := conversation.initializeRepositoryObservation(); err != nil {
		t.Fatal(err)
	}
	conversation.scanRepositories(conversation.ctx)
	if conversation.ctx.Err() != nil {
		t.Fatal("initial observation failed")
	}
	current := initial
	current.Branches = append([]observation.Branch{}, initial.Branches...)
	current.Branches = append(current.Branches, observation.Branch{Name: "skill/branch", CommitID: "two"})
	conversation.repositoryScanner = observationScanner{current}
	// Renaming a snapshot file onto an existing directory fails deterministically.
	conversation.repositoryObservationStore.path = directory
	conversation.scanRepositories(conversation.ctx)
	if conversation.ctx.Err() == nil {
		t.Fatal("failed durable write did not fence execution")
	}
	if changes := repositoryobserver.Compare(conversation.repositoryObservationState, []observation.Snapshot{current}); len(changes) != 1 || !changes[0].Branches[1].EligibleForLinking {
		t.Fatal("failed write advanced accepted state")
	}
	history, err := journal.Replay(0)
	if err != nil {
		t.Fatal(err)
	}
	var replayed []observation.Snapshot
	for _, event := range history {
		if event.RepositoryObservation != nil {
			replayed = append(replayed, *event.RepositoryObservation)
		}
	}
	store, err := loadRepositoryObservations(directory, "org", "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.save(replayed...); err != nil {
		t.Fatal(err)
	}
	restored := repositoryobserver.New(store.history())
	if changes := repositoryobserver.Compare(restored, []observation.Snapshot{current}); len(changes) != 0 {
		t.Fatal("journal recovery lost accepted branch evidence")
	}
}
