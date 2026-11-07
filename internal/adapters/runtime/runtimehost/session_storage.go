package runtimehost

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/platform/durablefile"
)

// sessionStorage owns the stable task-scoped layout independently of native protocols.
type sessionStorage struct {
	StateDir, WorkDir, RuntimeVersion string
	QuestionToolVersion               int
}

var errSessionAlreadyExists = errors.New("session already exists; explicit restoration required")

type sessionManifest struct {
	Version                  int               `json:"version"`
	RuntimeVersion           string            `json:"runtimeVersion"`
	WorkDir                  string            `json:"workDir"`
	Configuration            string            `json:"configuration"`
	ConfigurationFingerprint map[string]string `json:"configurationFingerprint,omitempty"`
	SessionID                string            `json:"sessionId"`
	QuestionToolVersion      int               `json:"questionToolVersion,omitempty"`
}

type sessionManifestCompatibilityError struct {
	Mismatches []string
	Fields     []string
	Legacy     bool
}

func (e *sessionManifestCompatibilityError) Error() string {
	message := "session manifest compatibility check failed: " + strings.Join(e.Mismatches, ",")
	if slices.Contains(e.Mismatches, "configuration_mismatch") {
		if e.Legacy {
			return message + "; field details unavailable for legacy session manifest"
		}
		return message + "; differing configuration fields: " + strings.Join(e.Fields, ",")
	}
	return message
}

func taskStorageRoot(stateDir string, config configmodel.RuntimeConfig) string {
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID})
	return filepath.Join(stateDir, fmt.Sprintf("%x", sha256.Sum256(scope)))
}

func (s sessionStorage) Root(config configmodel.RuntimeConfig) string {
	root := taskStorageRoot(s.StateDir, config)
	if config.TaskSessionID != "" {
		return filepath.Join(root, "sessions", fmt.Sprintf("%x", sha256.Sum256([]byte(config.TaskSessionID))))
	}
	return root
}

func configurationFingerprint(config configmodel.RuntimeConfig) string {
	config = fingerprintableConfiguration(config)
	data, _ := json.Marshal(config)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func sessionConfigurationFingerprint(config configmodel.RuntimeConfig) string {
	config = config.Clone()
	config = fingerprintableConfiguration(config)
	config.Environment = nil
	data, _ := json.Marshal(config)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func configurationFieldFingerprints(config configmodel.RuntimeConfig) map[string]string {
	config = config.Clone()
	config = fingerprintableConfiguration(config)
	config.Environment = nil
	fields := map[string]any{
		"organization_id": config.OrganizationID,
		"workspace_id":    config.WorkspaceID,
		"task_id":         config.TaskID,
		"task_session_id": config.TaskSessionID,
		"runtime":         config.Runtime,
		"agent":           config.Agent,
		"model":           config.Model,
		"mcp_servers":     config.MCPServers,
		"skills":          config.Skills,
	}
	fingerprints := make(map[string]string, len(fields))
	for name, value := range fields {
		data, _ := json.Marshal(value)
		fingerprints[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return fingerprints
}

func configurationMismatchFields(saved map[string]string, config configmodel.RuntimeConfig) []string {
	current := configurationFieldFingerprints(config)
	fields := make([]string, 0)
	for name, fingerprint := range current {
		if saved[name] != fingerprint {
			fields = append(fields, name)
		}
	}
	for name := range saved {
		if _, exists := current[name]; !exists {
			fields = append(fields, name)
		}
	}
	slices.Sort(fields)
	return fields
}

func fingerprintableConfiguration(config configmodel.RuntimeConfig) configmodel.RuntimeConfig {
	config.RunID = ""
	config.MCPServers = append([]configmodel.MCPServer(nil), config.MCPServers...)
	for i := range config.MCPServers {
		config.MCPServers[i].Headers = nil
	}
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	return config
}

func legacyConfigurationFingerprints(config configmodel.RuntimeConfig) []string {
	config.RunID = ""
	config.MCPServers = append([]configmodel.MCPServer(nil), config.MCPServers...)
	for i := range config.MCPServers {
		config.MCPServers[i].Headers = nil
	}
	values := []configmodel.RuntimeConfig{config, config}
	values[1].GitCredentials = nil
	values[1].GitCredentialsRequired = false
	fingerprints := make([]string, 0, len(values))
	for _, value := range values {
		data, _ := json.Marshal(value)
		fingerprints = append(fingerprints, fmt.Sprintf("%x", sha256.Sum256(data)))
	}
	return fingerprints
}

func (s sessionStorage) acquire(config configmodel.RuntimeConfig) (*os.File, error) {
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

func (s sessionStorage) reserveCreation(config configmodel.RuntimeConfig) error {
	root := s.Root(config)
	if _, err := os.Stat(filepath.Join(root, "session.json")); err == nil {
		return errSessionAlreadyExists
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

func (s sessionStorage) load(config configmodel.RuntimeConfig) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.Root(config), "session.json"))
	if err != nil {
		return "", err
	}
	var manifest sessionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	compatibleConfiguration := false
	if manifest.Version >= 3 {
		compatibleConfiguration = manifest.Configuration == sessionConfigurationFingerprint(config)
	} else {
		compatibleConfiguration = manifest.Configuration == configurationFingerprint(config)
		for _, fingerprint := range legacyConfigurationFingerprints(config) {
			compatibleConfiguration = compatibleConfiguration || manifest.Configuration == fingerprint
		}
	}
	var mismatches []string
	var configurationFields []string
	legacyConfiguration := manifest.Version < 2
	if !compatibleConfiguration && manifest.Version == 2 {
		configurationFields = configurationMismatchFields(manifest.ConfigurationFingerprint, config)
		if slices.Equal(configurationFields, []string{"environment"}) {
			compatibleConfiguration = true
			slog.Default().Info("restoring session with refreshed workspace environment",
				"organization_id", config.OrganizationID,
				"workspace_id", config.WorkspaceID,
				"task_id", config.TaskID,
				"run_id", config.RunID,
				"task_session_id", config.TaskSessionID,
			)
		}
	}
	if manifest.Version != 1 && manifest.Version != 2 && manifest.Version != 3 && manifest.Version != 4 {
		mismatches = append(mismatches, "manifest_version_mismatch")
	}
	if manifest.Version == 4 && (manifest.QuestionToolVersion != 1 || s.QuestionToolVersion != 1) {
		mismatches = append(mismatches, "question_tool_version_mismatch")
	}
	if manifest.RuntimeVersion != s.RuntimeVersion {
		mismatches = append(mismatches, "runtime_version_mismatch")
	}
	if manifest.WorkDir != s.WorkDir {
		mismatches = append(mismatches, "working_directory_mismatch")
	}
	if !compatibleConfiguration {
		mismatches = append(mismatches, "configuration_mismatch")
		if !legacyConfiguration && len(configurationFields) == 0 {
			configurationFields = configurationMismatchFields(manifest.ConfigurationFingerprint, config)
		}
	}
	if manifest.SessionID == "" {
		mismatches = append(mismatches, "native_session_id_missing")
	}
	if len(mismatches) != 0 {
		return "", &sessionManifestCompatibilityError{Mismatches: mismatches, Fields: configurationFields, Legacy: legacyConfiguration}
	}
	return manifest.SessionID, nil
}

func (s sessionStorage) save(config configmodel.RuntimeConfig, id string) error {
	version := 3
	if s.QuestionToolVersion != 0 {
		version = 4
	}
	data, err := json.Marshal(sessionManifest{
		Version:                  version,
		QuestionToolVersion:      s.QuestionToolVersion,
		RuntimeVersion:           s.RuntimeVersion,
		WorkDir:                  s.WorkDir,
		Configuration:            sessionConfigurationFingerprint(config),
		ConfigurationFingerprint: configurationFieldFingerprints(config),
		SessionID:                id,
	})
	if err != nil {
		return err
	}
	return durablefile.Write(filepath.Join(s.Root(config), "session.json"), data)
}
