// Package repositoryobserver compares Git snapshots without owning Git or transport.
package repositoryobserver

import (
	"context"
	"reflect"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type Scanner interface {
	Scan(context.Context) ([]observation.Snapshot, error)
}

// Observer advances only after its sink durably accepts each changed snapshot.
type Observer struct {
	scanner     Scanner
	previous    map[string]observation.Snapshot
	diagnostics map[string]string
}

func New(scanner Scanner, history []observation.Snapshot) *Observer {
	o := &Observer{scanner: scanner, previous: make(map[string]observation.Snapshot), diagnostics: make(map[string]string)}
	for _, snapshot := range history {
		if snapshot.Diagnostic == "" {
			o.previous[snapshot.Checkout.Path] = snapshot
		}
		o.diagnostics[snapshot.Checkout.Path] = snapshot.Diagnostic
	}
	return o
}

func (o *Observer) Scan(ctx context.Context, commit func(observation.Snapshot) error) error {
	snapshots, err := o.scanner.Scan(ctx)
	if err != nil {
		return err
	}
	seenPaths := make(map[string]bool, len(snapshots))
	for _, snapshot := range snapshots {
		seenPaths[snapshot.Checkout.Path] = true
		if snapshot.Branches == nil {
			snapshot.Branches = []observation.Branch{}
		}
		previous, exists := o.previous[snapshot.Checkout.Path]
		// An unreadable repository must not erase the last known baseline or turn
		// its old branches into newly created branches on the next successful scan.
		if snapshot.Diagnostic != "" {
			if o.diagnostics[snapshot.Checkout.Path] == snapshot.Diagnostic {
				continue
			}
			if err := commit(snapshot); err != nil {
				return err
			}
			o.diagnostics[snapshot.Checkout.Path] = snapshot.Diagnostic
			continue
		}
		snapshot.Baseline = !exists
		branches := make(map[string]observation.Branch, len(previous.Branches))
		for _, branch := range previous.Branches {
			branches[branch.Name] = branch
		}
		for i, branch := range snapshot.Branches {
			old, seen := branches[branch.Name]
			snapshot.Branches[i].Candidate = branch.Name != snapshot.Checkout.DefaultBranch && exists && (!seen || old.Candidate)
		}
		// Baseline describes first discovery, not a recurring state change.
		comparable := previous
		comparable.Baseline = snapshot.Baseline
		if exists && o.diagnostics[snapshot.Checkout.Path] == "" && reflect.DeepEqual(comparable, snapshot) {
			continue
		}
		if err := commit(snapshot); err != nil {
			return err
		}
		o.previous[snapshot.Checkout.Path] = snapshot
		o.diagnostics[snapshot.Checkout.Path] = ""
	}
	for path, previous := range o.previous {
		if seenPaths[path] || o.diagnostics[path] == "checkout_unavailable" {
			continue
		}
		missing := observation.Snapshot{Checkout: previous.Checkout, Branches: []observation.Branch{}, Diagnostic: "checkout_unavailable"}
		if err := commit(missing); err != nil {
			return err
		}
		o.diagnostics[path] = missing.Diagnostic
	}
	return nil
}
