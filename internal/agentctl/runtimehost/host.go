// Package runtimehost implements agentctl contracts by owning subprocesses,
// provisioning, native sessions, and restoration evidence.
package runtimehost

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/gitprovider"
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
	profile         agentruntime.RuntimeProfile
	factory         agentruntime.ProtocolClientFactory
	environment     Environment
	cliAuthAdapters []gitprovider.Adapter
}

// New binds runtime contracts to an explicitly granted execution environment.
func New(profile agentruntime.RuntimeProfile, factory agentruntime.ProtocolClientFactory, environment Environment, cliAuthAdapters ...gitprovider.Adapter) *Host {
	environment.GrantedEnv = slices.Clone(environment.GrantedEnv)
	return &Host{profile: profile, factory: factory, environment: environment, cliAuthAdapters: slices.Clone(cliAuthAdapters)}
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
	environment, err = gitprovider.PrepareRuntime(ctx, gitprovider.Preparation{
		RuntimeRoot: paths.RuntimeRoot,
		WorkDir:     h.environment.WorkDir,
		Environment: environment,
		Credentials: config.GitCredentials,
	}, h.cliAuthAdapters)
	if err != nil {
		return "", nil, err
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
