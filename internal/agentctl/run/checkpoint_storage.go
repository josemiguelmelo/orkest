package run

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/core/durablefile"
)

type checkpointStore struct {
	StateDir string
}

type runCheckpoint struct {
	Version int                 `json:"version"`
	Config  runconfig.RunConfig `json:"config"`
}

func (s checkpointStore) Save(config runconfig.RunConfig) error {
	data, err := json.Marshal(runCheckpoint{Version: 1, Config: config})
	if err != nil {
		return err
	}
	return durablefile.Write(filepath.Join(s.StateDir, "current-run.json"), data)
}

func (s checkpointStore) Load(expectedRunID string) (runconfig.RunConfig, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.StateDir, "current-run.json"))
	if errors.Is(err, os.ErrNotExist) {
		return runconfig.RunConfig{}, false, nil
	}
	if err != nil {
		return runconfig.RunConfig{}, false, ErrJournal
	}
	var checkpoint runCheckpoint
	if json.Unmarshal(data, &checkpoint) != nil || checkpoint.Version != 1 {
		return runconfig.RunConfig{}, false, ErrJournal
	}
	if expectedRunID != "" && checkpoint.Config.RunID != expectedRunID {
		return runconfig.RunConfig{}, false, nil
	}
	return checkpoint.Config, true, nil
}
