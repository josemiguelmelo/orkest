// Package local runs agentctl as a process on the Orkest host.
package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/core/agentctllogs"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
)

const codexAuthFileEnvironment = "ORKEST_CODEX_AUTH_FILE"

type Config struct {
	DataRoot                 string
	AgentctlBinary           string
	CodexAuthFile            string
	ProviderEnv              []string
	CodingProviders          []coderepository.CodingProvider
	CodingCredentialResolver interface {
		CodingCredential(context.Context, string, string) (coderepository.Credentials, error)
	}
}

type Executor struct {
	config    Config
	providers map[string]coderepository.CodingProvider
	mu        sync.Mutex
	processes map[string]*os.Process
}

func New(config Config) (*Executor, error) {
	root := strings.TrimSpace(config.DataRoot)
	if root == "" {
		return nil, errors.New("data root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local data root: %w", err)
	}
	binary := strings.TrimSpace(config.AgentctlBinary)
	if binary == "" || !filepath.IsAbs(binary) {
		return nil, errors.New("agentctl binary path must be absolute")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return nil, fmt.Errorf("inspect agentctl binary: %w", err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return nil, errors.New("agentctl binary must be an executable file")
	}
	codexAuthFile := strings.TrimSpace(config.CodexAuthFile)
	if codexAuthFile == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			candidate := filepath.Join(home, ".codex", "auth.json")
			if fileInfo, statErr := os.Stat(candidate); statErr == nil && fileInfo.Mode().IsRegular() {
				codexAuthFile = candidate
			}
		}
	}
	if codexAuthFile != "" && !filepath.IsAbs(codexAuthFile) {
		return nil, errors.New("codex auth file path must be absolute")
	}
	providers := make(map[string]coderepository.CodingProvider, len(config.CodingProviders))
	for _, provider := range config.CodingProviders {
		if provider == nil || strings.TrimSpace(provider.ProviderName()) == "" {
			return nil, errors.New("code repository provider name is required")
		}
		providers[provider.ProviderName()] = provider
	}
	config.DataRoot, config.AgentctlBinary, config.CodexAuthFile = root, binary, codexAuthFile
	return &Executor{config: config, providers: providers, processes: make(map[string]*os.Process)}, nil
}

var (
	_ orchestration.ProcessExecutor        = (*Executor)(nil)
	_ orchestration.AgentProcessController = (*Executor)(nil)
)

func (e *Executor) StartAgentctl(ctx context.Context, config runconfig.RunConfig) (orchestration.Process, error) {
	if err := config.Validate(); err != nil {
		return orchestration.Process{}, err
	}
	// Execution profile images describe Docker toolchains and have no meaning for
	// a host process. Local deliberately ignores both profile metadata fields.
	workDir, stateDir, err := e.taskDirectories(config)
	if err != nil {
		return orchestration.Process{}, err
	}
	if err = e.prepareRepositories(ctx, workDir, config); err != nil {
		return orchestration.Process{}, fmt.Errorf("prepare task repositories: %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return orchestration.Process{}, fmt.Errorf("reserve local agentctl port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		return orchestration.Process{}, fmt.Errorf("release local agentctl port: %w", err)
	}
	args := []string{"-listen", fmt.Sprintf("127.0.0.1:%d", port), "-runtime", string(config.Runtime), "-work-dir", workDir, "-state-dir", stateDir, "-run-id", config.RunID}
	pathGranted := false
	envNames := make(map[string]bool, len(e.config.ProviderEnv)+2)
	for _, name := range e.config.ProviderEnv {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "=\x00") {
			continue
		}
		if envNames[name] {
			continue
		}
		envNames[name] = true
		if name == "PATH" {
			pathGranted = true
		}
		if _, ok := os.LookupEnv(name); ok {
			args = append(args, "-env", name)
		}
	}
	// Local runtimes and their launchers need the host PATH (for example,
	// OpenCode's node-based executable uses /usr/bin/env node).
	if !pathGranted {
		args = append(args, "-env", "PATH")
	}
	cmdEnv := localEnvironment(e.config.ProviderEnv)
	if config.Runtime == "codex" && e.config.CodexAuthFile != "" {
		args = append(args, "-env", codexAuthFileEnvironment)
		cmdEnv = setEnvironment(cmdEnv, codexAuthFileEnvironment, e.config.CodexAuthFile)
	}
	cmd := exec.Command(e.config.AgentctlBinary, args...)
	cmd.Dir = workDir
	cmd.Stdin, cmd.Stdout = nil, io.Discard
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return orchestration.Process{}, fmt.Errorf("capture local agentctl logs: %w", err)
	}
	cmd.Env = cmdEnv
	if err = cmd.Start(); err != nil {
		return orchestration.Process{}, fmt.Errorf("start local agentctl process: %w", err)
	}
	logsDone := make(chan struct{})
	go func() {
		agentctllogs.Forward(stderr, "run_id", config.RunID)
		close(logsDone)
	}()
	identity, err := processIdentity(ctx, cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		<-logsDone
		_ = cmd.Wait()
		return orchestration.Process{}, fmt.Errorf("identify started local agentctl process: %w", err)
	}
	id := strconv.Itoa(cmd.Process.Pid) + ":" + identity
	if err = e.writeRunIdentity(config.RunID, id); err != nil {
		_ = cmd.Process.Kill()
		<-logsDone
		_ = cmd.Wait()
		return orchestration.Process{ID: id}, fmt.Errorf("persist local agentctl process identity: %w", err)
	}
	e.mu.Lock()
	e.processes[id] = cmd.Process
	e.mu.Unlock()
	go func() {
		<-logsDone
		_ = cmd.Wait()
		e.mu.Lock()
		delete(e.processes, id)
		e.mu.Unlock()
	}()
	return orchestration.Process{ID: id, Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port)}, nil
}

func setEnvironment(environment []string, name, value string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if key != name {
			result = append(result, entry)
		}
	}
	return append(result, name+"="+value)
}

func localEnvironment(names []string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true, "USER": true, "LOGNAME": true, "LANG": true, "LC_ALL": true, "SYSTEMROOT": true, "WINDIR": true, "APPDATA": true, "LOCALAPPDATA": true}
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			allowed[strings.TrimSpace(name)] = true
		}
	}
	result := make([]string, 0, len(allowed))
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if allowed[name] {
			result = append(result, item)
		}
	}
	return result
}

func (e *Executor) prepareRepositories(ctx context.Context, workDir string, config runconfig.RunConfig) error {
	for _, repository := range config.Repositories {
		provider, ok := e.providers[repository.ProviderName]
		if !ok {
			return fmt.Errorf("repository provider %q is unavailable", repository.ProviderName)
		}
		if !safePathComponent(repository.ProviderName) || !safePathComponent(repository.ID) {
			return errors.New("repository identity is invalid")
		}
		path := filepath.Join(workDir, "repositories", repository.ProviderName, repository.ID)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create repository directory: %w", err)
		}
		var checkoutErr error
		if repository.IntegrationID != "" {
			if e.config.CodingCredentialResolver == nil {
				return errors.New("coding credential resolver is unavailable")
			}
			credential, err := e.config.CodingCredentialResolver.CodingCredential(ctx, config.WorkspaceID, repository.IntegrationID)
			if err != nil {
				return fmt.Errorf("resolve coding integration credential: %w", err)
			}
			metadata := coderepository.RepositoryMetadata{ID: repository.ProviderRepositoryID, Name: repository.Name, CloneURL: repository.URL, DefaultBranch: repository.DefaultBranch}
			checkoutErr = provider.PrepareAuthenticatedTaskCheckout(ctx, metadata, credential, path, "")
		} else if legacy, ok := provider.(coderepository.LegacyRepositoryCheckout); ok {
			checkoutErr = legacy.PreparePublicTaskCheckout(ctx, repository.URL, path, "")
		} else {
			return errors.New("legacy repository checkout is unavailable")
		}
		if checkoutErr != nil {
			return fmt.Errorf("prepare repository %q: %w", repository.ID, checkoutErr)
		}
	}
	return nil
}

func safePathComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsRune(value, filepath.Separator)
}

func (e *Executor) taskDirectories(config runconfig.RunConfig) (string, string, error) {
	data, err := jsonScope(config.OrganizationID, config.WorkspaceID, config.TaskID)
	if err != nil {
		return "", "", err
	}
	root := filepath.Join(e.config.DataRoot, "tasks", data)
	work, state := filepath.Join(root, "work"), filepath.Join(root, "agentctl-state")
	for _, path := range []string{work, state} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", "", fmt.Errorf("create durable task directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return "", "", fmt.Errorf("restrict durable task directory permissions: %w", err)
		}
	}
	return work, state, nil
}

func (e *Executor) AgentctlRunning(ctx context.Context, id string) (bool, error) {
	pid, identity, err := parseIdentity(id)
	if err != nil {
		return false, err
	}
	actual, err := processIdentity(ctx, pid)
	if err != nil {
		if errors.Is(err, errProcessMissing) {
			return false, nil
		}
		return false, err
	}
	return actual == identity, nil
}

func (e *Executor) AgentctlRunningByRunID(ctx context.Context, runID string) (bool, error) {
	if strings.TrimSpace(runID) == "" {
		return false, errors.New("agent run identity is required")
	}
	id, err := e.readRunIdentity(runID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return e.AgentctlRunning(ctx, id)
}

func (e *Executor) TerminateAgentctl(ctx context.Context, id string) error {
	running, err := e.AgentctlRunning(ctx, id)
	if err != nil || !running {
		return err
	}
	pid, _, _ := parseIdentity(id)
	e.mu.Lock()
	process := e.processes[id]
	e.mu.Unlock()
	if process != nil {
		err = process.Signal(os.Interrupt)
	} else {
		err = osProcessSignal(pid)
	}
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal local agentctl process: %w", err)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		running, err = e.AgentctlRunning(ctx, id)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (e *Executor) TerminateAgentctlByRunID(ctx context.Context, runID string) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("agent run identity is required")
	}
	id, err := e.readRunIdentity(runID)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return e.TerminateAgentctl(ctx, id)
}

func (e *Executor) Close() error { return nil }

func jsonScope(values ...string) (string, error) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("encode task storage scope: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (e *Executor) writeRunIdentity(runID, identity string) error {
	path := filepath.Join(e.config.DataRoot, "runs", runHash(runID)+".pid")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create local process identity directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(identity), 0o600); err != nil {
		return fmt.Errorf("write local process identity: %w", err)
	}
	return nil
}

func (e *Executor) readRunIdentity(runID string) (string, error) {
	path := filepath.Join(e.config.DataRoot, "runs", runHash(runID)+".pid")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read local process identity: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func runHash(runID string) string {
	digest := sha256.Sum256([]byte(runID))
	return hex.EncodeToString(digest[:])
}

func parseIdentity(value string) (int, string, error) {
	pidValue, token, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || token == "" {
		return 0, "", errors.New("invalid local agentctl process identity")
	}
	pid, err := strconv.Atoi(pidValue)
	if err != nil || pid < 1 {
		return 0, "", errors.New("invalid local agentctl process identity")
	}
	return pid, token, nil
}
