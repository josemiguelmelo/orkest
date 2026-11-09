package repositoryobserver

import (
	"context"
	"errors"
	"testing"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type scannerFake struct{ snapshots []observation.Snapshot }

func (s *scannerFake) Scan(context.Context) ([]observation.Snapshot, error) {
	result := make([]observation.Snapshot, len(s.snapshots))
	copy(result, s.snapshots)
	for i := range result {
		result[i].Branches = append([]observation.Branch(nil), result[i].Branches...)
	}
	return result, nil
}

func TestObservationBaselineRestoreAndFailedCommit(t *testing.T) {
	scanner := &scannerFake{snapshots: []observation.Snapshot{{Checkout: observation.Checkout{Path: "repo", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: "main", CommitID: "one"}}}}}
	observer := New(scanner, nil)
	var committed []observation.Snapshot
	sink := func(s observation.Snapshot) error { committed = append(committed, s); return nil }
	if err := observer.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if !committed[0].Baseline || committed[0].Branches[0].Candidate {
		t.Fatal("baseline claimed ownership")
	}
	scanner.snapshots[0].Branches = append(scanner.snapshots[0].Branches, observation.Branch{Name: "user/skill-name", CommitID: "two"})
	failure := errors.New("disk unavailable")
	if err := observer.Scan(context.Background(), func(observation.Snapshot) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := observer.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if !committed[1].Branches[1].Candidate {
		t.Fatal("new branch not a candidate after retry")
	}
	restored := New(scanner, committed)
	if err := restored.Scan(context.Background(), func(observation.Snapshot) error { t.Fatal("restore duplicated observation"); return nil }); err != nil {
		t.Fatal(err)
	}
	scanner.snapshots[0].Branches[1].CommitID = "three"
	if err := restored.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if !committed[2].Branches[1].Candidate {
		t.Fatal("restore lost candidate")
	}
	scanner.snapshots[0].Branches = scanner.snapshots[0].Branches[:1]
	if err := restored.Scan(context.Background(), sink); err != nil {
		t.Fatal(err)
	}
	if len(committed[3].Branches) != 1 {
		t.Fatal("deleted branch retained")
	}
}
