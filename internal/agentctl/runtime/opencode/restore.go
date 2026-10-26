package opencode

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type sessionManifest struct {
	Version        int    `json:"version"`
	RuntimeVersion string `json:"runtimeVersion"`
	WorkDir        string `json:"workDir"`
	Configuration  string `json:"configuration"`
	SessionID      string `json:"sessionId"`
}

func (p *OpenCodeProvider) Restore(ctx context.Context, config agentctl.RunConfig) (agentctl.Process, error) {
	process, err := p.launch(ctx, config, true)
	if err != nil {
		return nil, &agentctl.LaunchError{Message: "session restoration failed; original context retained", Cause: err}
	}
	return process, nil
}

func (p *OpenCodeProvider) acquireOwnership(config agentctl.RunConfig) (*os.File, error) {
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID})
	root := filepath.Join(p.StateDir, fmt.Sprintf("%x", sha256.Sum256(scope)))
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, runtimeFailure("create task storage failed", err)
	}
	file, err := os.OpenFile(filepath.Join(root, "owner.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, runtimeFailure("open execution ownership failed", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, errors.New("task execution is still owned by another process")
	}
	return file, nil
}

func configurationIdentity(config agentctl.RunConfig) string {
	config.RunID, config.RestoreSession = "", false
	config.MCPServers = append([]agentctl.MCPServer(nil), config.MCPServers...)
	for i := range config.MCPServers {
		config.MCPServers[i].Headers = nil
	}
	data, _ := json.Marshal(config)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (p *OpenCodeProvider) readSession(config agentctl.RunConfig) (string, error) {
	data, err := os.ReadFile(filepath.Join(p.storageRoot(config), "session.json"))
	if err != nil {
		return "", runtimeFailure("read session manifest failed", err)
	}
	var manifest sessionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", runtimeFailure("decode session manifest failed", err)
	}
	if manifest.Version != 1 || manifest.RuntimeVersion != Version || manifest.WorkDir != p.WorkDir || manifest.Configuration != configurationIdentity(config) || manifest.SessionID == "" {
		return "", errors.New("session manifest is incompatible with configuration or working path")
	}
	return manifest.SessionID, nil
}

func (p *OpenCodeProvider) saveSession(config agentctl.RunConfig, sessionID string) error {
	data, err := json.Marshal(sessionManifest{Version: 1, RuntimeVersion: Version, WorkDir: p.WorkDir, Configuration: configurationIdentity(config), SessionID: sessionID})
	if err != nil {
		return runtimeFailure("encode session manifest failed", err)
	}
	return agentctl.WriteDurableFile(filepath.Join(p.storageRoot(config), "session.json"), data)
}

func (p *process) CanContinue(commandID, text string) bool {
	p.client.mu.Lock()
	defer p.client.mu.Unlock()
	return text != "" && p.client.replayUser == wirePrompt(commandID, text) && !p.client.replayTools
}

func (p *process) ConnectionLost() bool {
	select {
	case <-p.conn.Done():
		return true
	default:
		return false
	}
}
