package runtimehost

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/core/durablefile"
)

// sessionStorage owns the existing v1 layout independently of native protocols.
type sessionStorage struct{ StateDir, WorkDir, RuntimeVersion string }

type sessionManifest struct {
	Version        int    `json:"version"`
	RuntimeVersion string `json:"runtimeVersion"`
	WorkDir        string `json:"workDir"`
	Configuration  string `json:"configuration"`
	SessionID      string `json:"sessionId"`
}

func taskStorageRoot(stateDir string, config runconfig.RuntimeConfig) string {
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID})
	return filepath.Join(stateDir, fmt.Sprintf("%x", sha256.Sum256(scope)))
}

func (s sessionStorage) Root(config runconfig.RuntimeConfig) string {
	root := taskStorageRoot(s.StateDir, config)
	if config.TaskSessionID != "" {
		return filepath.Join(root, "sessions", fmt.Sprintf("%x", sha256.Sum256([]byte(config.TaskSessionID))))
	}
	return root
}

func configurationFingerprint(config runconfig.RuntimeConfig) string {
	config = fingerprintableConfiguration(config)
	data, _ := json.Marshal(config)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func fingerprintableConfiguration(config runconfig.RuntimeConfig) runconfig.RuntimeConfig {
	config.RunID = ""
	config.MCPServers = append([]runconfig.MCPServer(nil), config.MCPServers...)
	for i := range config.MCPServers {
		config.MCPServers[i].Headers = nil
	}
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	return config
}

func legacyConfigurationFingerprints(config runconfig.RuntimeConfig) []string {
	config.RunID = ""
	config.MCPServers = append([]runconfig.MCPServer(nil), config.MCPServers...)
	for i := range config.MCPServers {
		config.MCPServers[i].Headers = nil
	}
	values := []runconfig.RuntimeConfig{config, config}
	values[1].GitCredentials = nil
	values[1].GitCredentialsRequired = false
	fingerprints := make([]string, 0, len(values))
	for _, value := range values {
		data, _ := json.Marshal(value)
		fingerprints = append(fingerprints, fmt.Sprintf("%x", sha256.Sum256(data)))
	}
	return fingerprints
}

func (s sessionStorage) acquire(config runconfig.RuntimeConfig) (*os.File, error) {
	root := taskStorageRoot(s.StateDir, config)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create task storage: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open task ownership: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("task execution is still owned by another process")
	}
	return file, nil
}

func (s sessionStorage) reserveCreation(config runconfig.RuntimeConfig) error {
	root := s.Root(config)
	if _, err := os.Stat(filepath.Join(root, "session.json")); err == nil {
		return errors.New("session already exists; explicit restoration required")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	identity := config.RunID
	if config.TaskSessionID != "" {
		identity = config.TaskSessionID
	}
	path := filepath.Join(root, fmt.Sprintf("run-%x", sha256.Sum256([]byte(identity))))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot reserve session creation; reconciliation required: %w", err)
	}
	return file.Close()
}

func (s sessionStorage) load(config runconfig.RuntimeConfig) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Root(config), "session.json"))
	if err != nil {
		return "", err
	}
	var manifest sessionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	compatibleConfiguration := manifest.Configuration == configurationFingerprint(config)
	for _, fingerprint := range legacyConfigurationFingerprints(config) {
		compatibleConfiguration = compatibleConfiguration || manifest.Configuration == fingerprint
	}
	if manifest.Version != 1 || manifest.RuntimeVersion != s.RuntimeVersion || manifest.WorkDir != s.WorkDir || !compatibleConfiguration || manifest.SessionID == "" {
		return "", errors.New("session manifest is incompatible with configuration or working path")
	}
	return manifest.SessionID, nil
}

func (s sessionStorage) save(config runconfig.RuntimeConfig, id string) error {
	data, err := json.Marshal(sessionManifest{Version: 1, RuntimeVersion: s.RuntimeVersion, WorkDir: s.WorkDir, Configuration: configurationFingerprint(config), SessionID: id})
	if err != nil {
		return err
	}
	return durablefile.Write(filepath.Join(s.Root(config), "session.json"), data)
}
