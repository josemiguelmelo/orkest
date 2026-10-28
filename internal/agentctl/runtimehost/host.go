// Package runtimehost implements agentctl contracts by owning subprocesses,
// provisioning, native sessions, and restoration evidence.
package runtimehost

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

// Environment contains paths and explicitly granted environment values.
type Environment struct {
	WorkDir    string
	StateDir   string
	GrantedEnv []string
}

// Host owns startup and storage compatibility for every profile and
// communication factory. The returned session owns the child and task lease.
type Host struct {
	profile     agentruntime.RuntimeProfile
	factory     agentruntime.ProtocolClientFactory
	environment Environment
}

// New binds runtime contracts to an explicitly granted execution environment.
func New(profile agentruntime.RuntimeProfile, factory agentruntime.ProtocolClientFactory, environment Environment) *Host {
	environment.GrantedEnv = slices.Clone(environment.GrantedEnv)
	return &Host{profile: profile, factory: factory, environment: environment}
}

func (h *Host) Runtime() coreruntime.Runtime { return h.profile.Descriptor().Runtime }

func (h *Host) Version() string { return h.profile.Descriptor().Version }

func (h *Host) Launch(ctx context.Context, config runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return h.start(ctx, config, false)
}

func (h *Host) Restore(ctx context.Context, config runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return h.start(ctx, config, true)
}

func (h *Host) start(ctx context.Context, config runconfig.RuntimeConfig, restoring bool) (agentruntime.RuntimeSession, error) {
	config = config.Clone()
	descriptor := h.profile.Descriptor()
	if err := h.validateStartup(ctx, config, descriptor); err != nil {
		return nil, err
	}
	store := sessionStorage{StateDir: h.environment.StateDir, WorkDir: h.environment.WorkDir, RuntimeVersion: descriptor.Version}
	owner, err := store.acquire(config)
	if err != nil {
		return nil, &agentruntime.LaunchError{Message: "task execution ownership unavailable", Cause: err}
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = owner.Close()
		}
	}()

	var nativeID string
	if restoring {
		nativeID, err = store.load(config)
	} else {
		err = store.reserveCreation(config)
	}
	if err != nil {
		return nil, &agentruntime.LaunchError{Message: "session storage requires reconciliation", Cause: err}
	}
	settings := agentruntime.ResolvedRuntimeSettings{Agent: config.Agent, Model: config.Model, MCPServers: config.MCPServers, Skills: config.Skills}
	executable, environment, err := h.prepareRuntime(ctx, config, settings, store, descriptor)
	if err != nil {
		return nil, err
	}
	session, err := h.startProcess(ctx, executable, environment, descriptor, owner)
	if err != nil {
		return nil, err
	}
	// A partial startup must terminate the child before releasing ownership.
	defer func() {
		if !transferred {
			_ = session.Close()
		}
	}()
	for _, entry := range h.environment.GrantedEnv {
		name, value, ok := strings.Cut(entry, "=")
		name = strings.ToLower(name)
		if ok && (strings.Contains(name, "key") || strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "password")) {
			session.callbacks.secrets = append(session.callbacks.secrets, value)
		}
	}
	for _, value := range config.Environment {
		if value != "" {
			session.callbacks.secrets = append(session.callbacks.secrets, value)
		}
	}
	for _, credential := range config.GitCredentials {
		if credential.Token != "" {
			session.callbacks.secrets = append(session.callbacks.secrets, credential.Token)
			session.callbacks.secrets = append(session.callbacks.secrets, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+credential.Token)))
		}
	}
	for _, server := range config.MCPServers {
		for _, value := range server.Headers {
			session.callbacks.secrets = append(session.callbacks.secrets, value)
		}
	}
	if err := h.initializeSession(ctx, session, settings, descriptor, nativeID, restoring); err != nil {
		return nil, err
	}
	if err := store.save(config, session.SessionID()); err != nil {
		return nil, &agentruntime.LaunchError{Message: "persist runtime session failed", Cause: err}
	}
	transferred = true
	go session.supervise()
	return session, nil
}

func (h *Host) validateStartup(ctx context.Context, config runconfig.RuntimeConfig, descriptor agentruntime.RuntimeDescriptor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if config.Runtime != descriptor.Runtime || descriptor.Version == "" || h.factory == nil {
		return agentruntime.ErrRuntimeHostUnavailable
	}
	if !filepath.IsAbs(h.environment.WorkDir) || !filepath.IsAbs(h.environment.StateDir) {
		return &agentruntime.LaunchError{Message: "absolute work and state directories are required"}
	}
	info, err := os.Stat(h.environment.WorkDir)
	if err != nil || !info.IsDir() {
		return &agentruntime.LaunchError{Message: "working directory must exist", Cause: err}
	}
	return nil
}

func (h *Host) prepareRuntime(ctx context.Context, config runconfig.RuntimeConfig, settings agentruntime.ResolvedRuntimeSettings, store sessionStorage, descriptor agentruntime.RuntimeDescriptor) (string, []string, error) {
	identity := config.RunID
	if config.TaskSessionID != "" {
		identity = config.TaskSessionID
	}
	paths := agentruntime.ProvisioningPaths{
		RuntimeRoot: store.Root(config), WorkDir: h.environment.WorkDir,
		ConfigSubdir: fmt.Sprintf("config-%x", sha256.Sum256([]byte(identity))),
	}
	plan, err := h.profile.BuildProvisioningPlan(settings, paths)
	if err != nil {
		return "", nil, err
	}
	granted := append(slices.Clone(h.environment.GrantedEnv), environmentEntries(config.Environment)...)
	environment, err := applyProvisioningPlan(paths.RuntimeRoot, granted, descriptor.ReservedEnvNames, descriptor.ReservedEnvPrefixes, plan)
	if err != nil {
		return "", nil, err
	}
	if err := configureGit(ctx, h.environment.WorkDir, config.GitCredentials); err != nil {
		return "", nil, err
	}
	if len(config.GitCredentials) > 0 {
		environment = withGitCredentials(environment, config.GitCredentials)
		if hasGitHubCredentials(config.GitCredentials) {
			environment, err = withGitHubCLI(paths.RuntimeRoot, environment, config.GitCredentials)
			if err != nil {
				return "", nil, err
			}
		}
	}
	executable, err := exec.LookPath(descriptor.Executable)
	if err != nil {
		return "", nil, &agentruntime.LaunchError{Message: "runtime executable unavailable", Cause: err}
	}
	if err := verifyRuntimeVersion(ctx, executable, descriptor.VersionArguments, descriptor.Version, environment); err != nil {
		return "", nil, err
	}
	return executable, environment, nil
}

func hasGitHubCredentials(credentials []runconfig.GitCredential) bool {
	for _, credential := range credentials {
		parsed, err := url.Parse(credential.RepositoryURL)
		if err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
			return true
		}
	}
	return false
}

func configureGit(ctx context.Context, workDir string, credentials []runconfig.GitCredential) error {
	for _, credential := range credentials {
		if filepath.IsAbs(credential.RepositoryPath) || filepath.Clean(credential.RepositoryPath) != credential.RepositoryPath || strings.HasPrefix(credential.RepositoryPath, "..") {
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure Git identity failed", Cause: fmt.Errorf("invalid repository path")}
		}
		path := filepath.Join(workDir, credential.RepositoryPath)
		for _, setting := range [][2]string{{"user.name", "Orkest Agent"}, {"user.email", "agent@orkest.local"}} {
			cmd := exec.CommandContext(ctx, "git", "-C", path, "config", "--local", setting[0], setting[1])
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			if err := cmd.Run(); err != nil {
				return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure Git identity failed", Cause: err}
			}
		}
	}
	return nil
}

func withGitCredentials(environment []string, credentials []runconfig.GitCredential) []string {
	values := make(map[string]string, len(environment)+1+len(credentials)*2)
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "GIT_CONFIG_COUNT" && !strings.HasPrefix(name, "GIT_CONFIG_KEY_") && !strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			values[name] = value
		}
	}
	values["GIT_CONFIG_COUNT"] = strconv.Itoa(len(credentials))
	for i, credential := range credentials {
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + credential.Token))
		values["GIT_CONFIG_KEY_"+strconv.Itoa(i)] = "http." + credential.RepositoryURL + ".extraheader"
		values["GIT_CONFIG_VALUE_"+strconv.Itoa(i)] = "AUTHORIZATION: basic " + auth
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func withGitHubCLI(runtimeRoot string, environment []string, credentials []runconfig.GitCredential) ([]string, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return nil, &agentruntime.LaunchError{Message: "GitHub CLI is unavailable", Cause: err}
	}
	binDir := filepath.Join(runtimeRoot, "github-cli")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: err}
	}
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -eu\nunset GH_TOKEN GITHUB_TOKEN GH_ENTERPRISE_TOKEN GITHUB_ENTERPRISE_TOKEN GH_HOST\n")
	script.WriteString("repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || { echo 'gh requires a registered repository checkout' >&2; exit 1; }\n")
	script.WriteString("remote=$(git -C \"$repo_root\" remote get-url origin 2>/dev/null) || { echo 'gh could not read this checkout origin' >&2; exit 1; }\n")
	script.WriteString("case \"$remote\" in\n")
	for index, credential := range credentials {
		canonical := strings.TrimSuffix(credential.RepositoryURL, ".git")
		fmt.Fprintf(&script, "  %s|%s) GH_TOKEN=\"${ORKEST_GH_TOKEN_%d:?GitHub credential unavailable}\"; export GH_TOKEN ;;\n", shellSingleQuote(canonical), shellSingleQuote(canonical+".git"), index)
	}
	script.WriteString("  *) echo 'gh has no credential for this checkout' >&2; exit 1 ;;\nesac\n")
	fmt.Fprintf(&script, "exec %s \"$@\"\n", shellSingleQuote(ghPath))
	wrapper := filepath.Join(binDir, "gh")
	if err := os.WriteFile(wrapper, []byte(script.String()), 0o700); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: err}
	}
	if err := os.Chmod(wrapper, 0o700); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: err}
	}
	values := make(map[string]string, len(environment)+1+len(credentials))
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "GH_TOKEN" || name == "GITHUB_TOKEN" || strings.HasPrefix(name, "ORKEST_GH_") {
			continue
		}
		values[name] = value
	}
	for index, credential := range credentials {
		values["ORKEST_GH_TOKEN_"+strconv.Itoa(index)] = credential.Token
	}
	values["GH_PROMPT_DISABLED"] = "1"
	runtimePath := values["PATH"]
	if strings.TrimSpace(runtimePath) == "" {
		runtimePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	values["PATH"] = binDir + string(os.PathListSeparator) + runtimePath
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result, nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func environmentEntries(values map[string]string) []string {
	entries := make([]string, 0, len(values))
	for name, value := range values {
		entries = append(entries, name+"="+value)
	}
	return entries
}

func (h *Host) startProcess(ctx context.Context, executable string, environment []string, descriptor agentruntime.RuntimeDescriptor, owner *os.File) (*Session, error) {
	cmd := exec.Command(executable, descriptor.Arguments...)
	cmd.Dir, cmd.Env, cmd.Stderr = h.environment.WorkDir, environment, io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "open runtime input failed", Cause: err}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "open runtime output failed", Cause: err}
	}
	child, err := startChildProcess(cmd, stdin, owner, stdout)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	callbacks := &sessionCallbacks{}
	connection, err := h.factory.Connect(ctx, stdin, stdout, callbacks)
	if err != nil {
		_ = child.Close()
		return nil, err
	}
	return &Session{child: child, connection: connection, callbacks: callbacks, ownership: owner}, nil
}

func (h *Host) initializeSession(ctx context.Context, session *Session, settings agentruntime.ResolvedRuntimeSettings, descriptor agentruntime.RuntimeDescriptor, nativeID string, restoring bool) error {
	capabilities, err := session.connection.Initialize(ctx)
	if err != nil {
		return err
	}
	if capabilities.ProtocolVersion != 1 {
		return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "unsupported runtime protocol version"}
	}
	if !capabilities.LoadSession {
		return &agentruntime.LaunchError{Message: "runtime is missing a required protocol capability"}
	}
	if restoring {
		session.callbacks.beginReplay(nativeID, descriptor.ReplayCoverageVerified)
		if err := session.connection.LoadSession(ctx, nativeID, h.environment.WorkDir); err != nil {
			return err
		}
		session.callbacks.finishReplay()
	} else {
		nativeID, err = session.connection.NewSession(ctx, h.environment.WorkDir)
		if err != nil {
			return err
		}
	}
	if nativeID == "" {
		return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "runtime returned an empty session identity"}
	}
	if mode := h.profile.NativeSessionMode(settings); mode != "" {
		if !capabilities.SetSessionMode {
			return &agentruntime.LaunchError{Message: "runtime cannot apply the configured session mode"}
		}
		if err := session.connection.SetSessionMode(ctx, nativeID, mode); err != nil {
			return err
		}
	}
	session.sessionID = nativeID
	return nil
}

func verifyRuntimeVersion(ctx context.Context, executable string, args []string, want string, environment []string) error {
	probe := exec.CommandContext(ctx, executable, args...)
	probe.Env = environment
	output, err := probe.Output()
	if err != nil {
		return &agentruntime.LaunchError{Message: "inspect runtime version failed", Cause: err}
	}
	if strings.TrimSpace(string(output)) != want {
		return &agentruntime.LaunchError{Message: "runtime version does not match the pinned version"}
	}
	return nil
}

var _ agentruntime.RuntimeHost = (*Host)(nil)
