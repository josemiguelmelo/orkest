// Package opencode implements the agentctl provider contract using ACP stdio.
package opencode

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

const Version = "opencode v2.1.4"

type OpenCodeProvider struct {
	Binary, WorkDir, StateDir string
	Env                       []string
}

func (p *OpenCodeProvider) Runtime() string { return "opencode" }

func (p *OpenCodeProvider) Launch(ctx context.Context, config agentctl.RunConfig) (agentctl.Process, error) {
	process, err := p.launch(ctx, config)
	if err != nil {
		var validation *agentctl.ValidationError
		if errors.As(err, &validation) {
			return nil, err
		}
		return nil, &agentctl.LaunchError{Message: err.Error(), Cause: err}
	}
	return process, nil
}

func (p *OpenCodeProvider) launch(ctx context.Context, config agentctl.RunConfig) (agentctl.Process, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Runtime != p.Runtime() {
		return nil, errors.New("runtime does not match opencode")
	}
	if !filepath.IsAbs(p.WorkDir) || !filepath.IsAbs(p.StateDir) {
		return nil, errors.New("absolute work and state directories are required")
	}
	info, err := os.Stat(p.WorkDir)
	if err != nil {
		return nil, runtimeFailure("inspect working directory failed", err)
	}
	if !info.IsDir() {
		return nil, errors.New("working directory must exist")
	}
	binary := p.Binary
	if binary == "" {
		binary = "opencode"
	}
	binary, err = exec.LookPath(binary)
	if err != nil {
		return nil, runtimeFailure("opencode executable unavailable", err)
	}
	root := p.storageRoot(config)
	env, err := p.prepareEnvironment(root, config)
	if err != nil {
		return nil, err
	}
	if err := verifyVersion(ctx, binary, env); err != nil {
		return nil, err
	}
	if err := reserveRun(root, config.RunID); err != nil {
		return nil, err
	}
	return p.startProcess(ctx, binary, env)
}

func (p *OpenCodeProvider) storageRoot(config agentctl.RunConfig) string {
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID})
	return filepath.Join(p.StateDir, fmt.Sprintf("%x", sha256.Sum256(scope)))
}

type runtimeConfiguration struct {
	Autoupdate bool                          `json:"autoupdate"`
	Share      string                        `json:"share"`
	Agent      map[string]agentConfiguration `json:"agent"`
}

type agentConfiguration struct {
	Mode   string `json:"mode"`
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func (p *OpenCodeProvider) prepareEnvironment(root string, config agentctl.RunConfig) ([]string, error) {
	env := append([]string{}, p.Env...)
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "HOME" || strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "OPENCODE_") {
			return nil, errors.New("reserved or invalid provider environment variable")
		}
	}
	for _, item := range []struct{ name, dir string }{{"HOME", "home"}, {"XDG_DATA_HOME", "data"}, {"XDG_CONFIG_HOME", "config"}, {"XDG_STATE_HOME", "state"}, {"XDG_CACHE_HOME", "cache"}} {
		dir := filepath.Join(root, item.dir)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, runtimeFailure("create runtime storage failed", err)
		}
		env = append(env, item.name+"="+dir)
	}
	runtimeConfig, err := json.Marshal(runtimeConfiguration{Share: "disabled", Agent: map[string]agentConfiguration{"orkest": {Mode: "primary", Prompt: config.SystemPrompt, Model: config.Model.Provider + "/" + config.Model.ID}}})
	if err != nil {
		return nil, runtimeFailure("encode runtime configuration failed", err)
	}
	env = append(env, "OPENCODE_CONFIG_CONTENT="+string(runtimeConfig), "OPENCODE_DISABLE_PROJECT_CONFIG=true")
	return env, nil
}

func verifyVersion(ctx context.Context, binary string, env []string) error {
	check := exec.CommandContext(ctx, binary, "--version")
	check.Env = env
	version, err := check.Output()
	if err != nil {
		return runtimeFailure("inspect opencode version failed", err)
	}
	if strings.TrimSpace(string(version)) != Version {
		return fmt.Errorf("opencode version must be %s", Version)
	}
	return nil
}

// A retained marker fences restart attempts; absence of a heartbeat is not ownership evidence.
func reserveRun(root, runID string) error {
	marker, err := os.OpenFile(filepath.Join(root, fmt.Sprintf("run-%x", sha256.Sum256([]byte(runID)))), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return runtimeFailure("cannot reserve run; existing runs require reconciliation", err)
	}
	if err := marker.Close(); err != nil {
		return runtimeFailure("close run reservation failed", err)
	}
	return nil
}

func (p *OpenCodeProvider) startProcess(ctx context.Context, binary string, env []string) (agentctl.Process, error) {
	cmd := exec.Command(binary, "acp")
	cmd.Dir, cmd.Env = p.WorkDir, env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, runtimeFailure("open opencode input failed", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, runtimeFailure("open opencode output failed", err)
	}
	// Runtime diagnostics can contain secrets and are never forwarded to ordinary logs.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, runtimeFailure("start opencode failed", err)
	}
	process := &process{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() { process.waitErr = cmd.Wait(); close(process.done) }()
	success := false
	defer func() {
		if !success {
			_ = process.Close()
		}
	}()
	client := &runtimeClient{}
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	process.conn, process.client = conn, client
	conn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := process.initializeSession(ctx, p.WorkDir); err != nil {
		_ = process.Close()
		return nil, errors.Join(err, process.Err())
	}
	// A lost ACP connection invalidates this process; never launch a replacement automatically.
	go func() {
		select {
		case <-conn.Done():
			_ = process.Close()
		case <-process.done:
		}
	}()
	success = true
	return process, nil
}

func (p *process) initializeSession(ctx context.Context, workDir string) error {
	conn := p.conn
	initialized, err := conn.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: 1})
	if err != nil {
		return runtimeFailure("initialize opencode ACP failed", err)
	}
	if initialized.ProtocolVersion != 1 {
		return errors.New("unsupported ACP protocol version")
	}
	session, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: workDir, McpServers: []acp.McpServer{}})
	if err != nil {
		return runtimeFailure("create opencode session failed", err)
	}
	if session.SessionId == "" {
		return errors.New("opencode returned empty session identity")
	}
	_, err = conn.SetSessionMode(ctx, acp.SetSessionModeRequest{SessionId: session.SessionId, ModeId: "orkest"})
	if err != nil {
		return runtimeFailure("select configured opencode mode failed", err)
	}
	p.sessionID = string(session.SessionId)
	p.client.mu.Lock()
	p.client.sessionID = p.sessionID
	p.client.mu.Unlock()
	return nil
}

type process struct {
	cmd   *exec.Cmd
	stdin io.Closer
	done  chan struct{}
	// waitErr is written before done closes; readers must observe done first.
	waitErr   error
	sessionID string
	once      sync.Once
	conn      *acp.ClientSideConnection
	client    *runtimeClient
}

func (p *process) SessionID() string     { return p.sessionID }
func (p *process) Done() <-chan struct{} { return p.done }
func (p *process) Err() error {
	select {
	case <-p.done:
		if p.waitErr != nil {
			return runtimeFailure("opencode process exited unsuccessfully", p.waitErr)
		}
	default:
	}
	return nil
}

func (p *process) Close() error {
	p.once.Do(func() {
		_ = p.stdin.Close()
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-p.done:
		case <-timer.C:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	return nil
}
