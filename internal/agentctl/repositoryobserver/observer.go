// Package repositoryobserver compares Git snapshots without owning Git or persistence.
package repositoryobserver

import (
	"context"
	"reflect"
	"sort"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type Scanner interface {
	Scan(context.Context) ([]observation.Snapshot, error)
}

type repositoryState struct {
	snapshot   *observation.Snapshot
	diagnostic string
}

// State contains only observations accepted by durable storage.
type State struct{ repositories map[string]repositoryState }

func New(history []observation.Snapshot) *State {
	state := &State{repositories: make(map[string]repositoryState)}
	for _, snapshot := range history {
		state.Record(snapshot)
	}
	return state
}

// Record advances state after persistence. Diagnostics preserve the last good baseline.
func (s *State) Record(snapshot observation.Snapshot) {
	previous := s.repositories[snapshot.Checkout.Path]
	previous.diagnostic = snapshot.Diagnostic
	if snapshot.Diagnostic == "" {
		snapshot.Branches = append([]observation.Branch{}, snapshot.Branches...)
		previous.snapshot = &snapshot
	}
	s.repositories[snapshot.Checkout.Path] = previous
}

// Compare returns changes without modifying either the accepted state or scan results.
func Compare(previous *State, current []observation.Snapshot) []observation.Snapshot {
	var changes []observation.Snapshot
	seen := make(map[string]bool, len(current))
	for _, snapshot := range current {
		path := snapshot.Checkout.Path
		seen[path] = true
		prior := previous.repositories[path]
		if snapshot.Diagnostic != "" {
			if snapshot.Diagnostic != prior.diagnostic {
				changes = append(changes, snapshot)
			}
			continue
		}
		snapshot.Branches = append([]observation.Branch{}, snapshot.Branches...)
		branches := make(map[string]observation.Branch)
		if prior.snapshot != nil {
			for _, branch := range prior.snapshot.Branches {
				branches[branch.Name] = branch
			}
		}
		for index, branch := range snapshot.Branches {
			old, exists := branches[branch.Name]
			snapshot.Branches[index].EligibleForLinking = prior.snapshot != nil && branch.Name != snapshot.Checkout.DefaultBranch && (!exists || old.EligibleForLinking)
		}
		if prior.diagnostic != "" || prior.snapshot == nil || !reflect.DeepEqual(*prior.snapshot, snapshot) {
			changes = append(changes, snapshot)
		}
	}
	for path, prior := range previous.repositories {
		if !seen[path] && prior.snapshot != nil && prior.diagnostic != "checkout_unavailable" {
			changes = append(changes, observation.Snapshot{Checkout: prior.snapshot.Checkout, Branches: []observation.Branch{}, Diagnostic: "checkout_unavailable"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Checkout.Path < changes[j].Checkout.Path })
	return changes
}
