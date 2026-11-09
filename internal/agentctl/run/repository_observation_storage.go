package run

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
	"github.com/josemiguelmelo/orkest/internal/platform/durablefile"
)

// repositoryObservationStore retains task baselines across separate runs. Journal
// replay overlays this cache after a crash between journal and cache persistence.
type repositoryObservationStore struct {
	path      string
	snapshots map[string]observation.Snapshot
}

func loadRepositoryObservations(directory, organization, workspace, task string) (*repositoryObservationStore, error) {
	scope, _ := json.Marshal([]string{organization, workspace, task})
	store := &repositoryObservationStore{path: filepath.Join(directory, "repository-observations", fmt.Sprintf("%x.json", sha256.Sum256(scope))), snapshots: make(map[string]observation.Snapshot)}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, errors.New("repository observation state unavailable")
	}
	if json.Unmarshal(data, &store.snapshots) != nil || store.snapshots == nil {
		return nil, errors.New("repository observation state invalid")
	}
	return store, nil
}

func (s *repositoryObservationStore) history() []observation.Snapshot {
	paths := make([]string, 0, len(s.snapshots))
	for path := range s.snapshots {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	snapshots := make([]observation.Snapshot, 0, len(paths))
	for _, path := range paths {
		snapshots = append(snapshots, s.snapshots[path])
	}
	return snapshots
}

// save flushes all successful snapshots once; diagnostics cannot replace baselines.
func (s *repositoryObservationStore) save(snapshots ...observation.Snapshot) error {
	changed := false
	for _, snapshot := range snapshots {
		if snapshot.Diagnostic != "" {
			continue
		}
		s.snapshots[snapshot.Checkout.Path] = snapshot
		changed = true
	}
	if !changed {
		return nil
	}
	data, err := json.Marshal(s.snapshots)
	if err != nil {
		return err
	}
	return durablefile.Write(s.path, data)
}
