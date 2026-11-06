package run

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/platform/durablefile"
)

type checkpointStore struct {
	StateDir string
}

type runCheckpoint struct {
	Version int                   `json:"version"`
	Config  configmodel.RunConfig `json:"config"`
}

func (s checkpointStore) Save(config configmodel.RunConfig) error {
	config = config.Clone()
	config.GitCredentials = nil
	data, err := json.Marshal(runCheckpoint{Version: 1, Config: config})
	if err != nil {
		return err
	}
	return durablefile.Write(filepath.Join(s.StateDir, "current-run.json"), data)
}

func (s checkpointStore) Load(expectedRunID string) (configmodel.RunConfig, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.StateDir, "current-run.json"))
	if errors.Is(err, os.ErrNotExist) {
		return configmodel.RunConfig{}, false, nil
	}
	if err != nil {
		return configmodel.RunConfig{}, false, ErrJournal
	}
	var checkpoint runCheckpoint
	if json.Unmarshal(data, &checkpoint) != nil || checkpoint.Version != 1 {
		return configmodel.RunConfig{}, false, ErrJournal
	}
	if expectedRunID != "" && checkpoint.Config.RunID != expectedRunID {
		return configmodel.RunConfig{}, false, nil
	}
	return checkpoint.Config, true, nil
}
