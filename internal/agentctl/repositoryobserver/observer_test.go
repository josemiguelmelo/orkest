package repositoryobserver

import (
	"reflect"
	"testing"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

func TestComparisonPreservesBaselineAndAdvancesOnlyWhenRecorded(t *testing.T) {
	state := New(nil)
	initial := observation.Snapshot{Checkout: observation.Checkout{Path: "repo", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: "main", CommitID: "one"}}}
	changes := Compare(state, []observation.Snapshot{initial})
	if len(changes) != 1 || changes[0].Branches[0].EligibleForLinking {
		t.Fatal("baseline claimed ownership")
	}
	state.Record(changes[0])
	current := initial
	current.Branches = append([]observation.Branch{}, initial.Branches...)
	current.Branches = append(current.Branches, observation.Branch{Name: "user/skill-name", CommitID: "two"})
	changes = Compare(state, []observation.Snapshot{current})
	if len(changes) != 1 || !changes[0].Branches[1].EligibleForLinking {
		t.Fatal("new branch was not eligible")
	}
	if current.Branches[1].EligibleForLinking {
		t.Fatal("comparison mutated scan result")
	}
	// A failed persistence operation does not call Record; the same change remains pending.
	if retry := Compare(state, []observation.Snapshot{current}); !reflect.DeepEqual(changes, retry) {
		t.Fatal("comparison consumed unpersisted change")
	}
	state.Record(changes[0])
	restored := New(changes)
	if repeated := Compare(restored, []observation.Snapshot{current}); len(repeated) != 0 {
		t.Fatal("restore duplicated observation")
	}
	current.Branches[1].CommitID = "three"
	changes = Compare(restored, []observation.Snapshot{current})
	if len(changes) != 1 || !changes[0].Branches[1].EligibleForLinking {
		t.Fatal("commit update lost ownership")
	}
	restored.Record(changes[0])
	current.Branches = current.Branches[:1]
	changes = Compare(restored, []observation.Snapshot{current})
	if len(changes) != 1 || len(changes[0].Branches) != 1 {
		t.Fatal("deleted branch retained")
	}
}

func TestDiagnosticsAndMissingCheckoutPreserveSuccessfulBaseline(t *testing.T) {
	initial := observation.Snapshot{Checkout: observation.Checkout{Path: "repo", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: "preexisting", CommitID: "one"}}}
	state := New([]observation.Snapshot{initial})
	missing := Compare(state, nil)
	if len(missing) != 1 || missing[0].Diagnostic != "checkout_unavailable" {
		t.Fatal("missing checkout not reported")
	}
	state.Record(missing[0])
	if repeated := Compare(state, nil); len(repeated) != 0 {
		t.Fatal("missing checkout diagnostic repeated")
	}
	changes := Compare(state, []observation.Snapshot{initial})
	if len(changes) != 1 || changes[0].Branches[0].EligibleForLinking {
		t.Fatal("recovered checkout claimed baseline branch")
	}
	state.Record(changes[0])
	diagnostic := observation.Snapshot{Checkout: initial.Checkout, Diagnostic: "git_inspection_failed"}
	state.Record(diagnostic)
	initial.Branches = append(initial.Branches, observation.Branch{Name: "created-while-unavailable", CommitID: "two"})
	changes = Compare(state, []observation.Snapshot{initial})
	if len(changes) != 1 || changes[0].Branches[0].EligibleForLinking || !changes[0].Branches[1].EligibleForLinking {
		t.Fatal("inspection failure destroyed baseline")
	}
}
