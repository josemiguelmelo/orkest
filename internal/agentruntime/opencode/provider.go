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

type Provider struct {
	Binary, WorkDir, StateDir string
	Env                       []string
}

func (p *Provider) Runtime() string { return "opencode" }

func (p *Provider) Launch(ctx context.Context, config agentctl.Configuration) (agentctl.Process, error) {
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
	if err != nil || !info.IsDir() {
		return nil, errors.New("working directory must exist")
	}
	binary := p.Binary
	if binary == "" {
		binary = "opencode"
	}
	binary, err = exec.LookPath(binary)
	if err != nil {
		return nil, errors.New("opencode executable unavailable")
	}
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID})
	root := filepath.Join(p.StateDir, fmt.Sprintf("%x", sha256.Sum256(scope)))
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
			return nil, fmt.Errorf("create runtime storage: %w", err)
		}
		env = append(env, item.name+"="+dir)
	}
	runtimeConfig, err := json.Marshal(map[string]any{"autoupdate": false, "share": "disabled", "agent": map[string]any{"orkest": map[string]any{"mode": "primary", "prompt": config.SystemPrompt, "model": config.Model.Provider + "/" + config.Model.ID}}})
	if err != nil {
		return nil, err
	}
	env = append(env, "OPENCODE_CONFIG_CONTENT="+string(runtimeConfig), "OPENCODE_DISABLE_PROJECT_CONFIG=true")
	check := exec.CommandContext(ctx, binary, "--version")
	check.Env = env
	version, err := check.Output()
	if err != nil {
		return nil, errors.New("inspect opencode version failed")
	}
	if strings.TrimSpace(string(version)) != Version {
		return nil, fmt.Errorf("opencode version must be %s but it is %s", Version, strings.TrimSpace(string(version)))
	}
	// A retained marker fences restart attempts; absence of a heartbeat is not ownership evidence.
	marker, err := os.OpenFile(filepath.Join(root, fmt.Sprintf("run-%x", sha256.Sum256([]byte(config.RunID)))), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, errors.New("cannot reserve run; existing runs require reconciliation")
	}
	if err := marker.Close(); err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "acp")
	cmd.Dir, cmd.Env = p.WorkDir, env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	// Runtime diagnostics can contain secrets and are never forwarded to ordinary logs.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, errors.New("start opencode failed")
	}
	process := &process{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(process.done) }()
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
	initialized, err := conn.Initialize(ctx, acp.InitializeRequest{ProtocolVersion: 1})
	if err != nil {
		return nil, errors.New("initialize opencode ACP failed")
	}
	if initialized.ProtocolVersion != 1 {
		return nil, errors.New("unsupported ACP protocol version")
	}
	session, err := conn.NewSession(ctx, acp.NewSessionRequest{Cwd: p.WorkDir, McpServers: []acp.McpServer{}})
	if err != nil {
		return nil, errors.New("create opencode session failed")
	}
	if session.SessionId == "" {
		return nil, errors.New("opencode returned empty session identity")
	}
	_, err = conn.SetSessionMode(ctx, acp.SetSessionModeRequest{SessionId: session.SessionId, ModeId: "orkest"})
	if err != nil {
		return nil, errors.New("select configured opencode mode failed")
	}
	process.sessionID = string(session.SessionId)
	client.mu.Lock()
	client.sessionID = process.sessionID
	client.mu.Unlock()
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

type process struct {
	cmd       *exec.Cmd
	stdin     io.Closer
	done      chan struct{}
	sessionID string
	once      sync.Once
	conn      *acp.ClientSideConnection
	client    *runtimeClient
}

func (p *process) SessionID() string     { return p.sessionID }
func (p *process) Done() <-chan struct{} { return p.done }
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
