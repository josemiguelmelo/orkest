// Package runtimehost implements agentctl contracts by owning subprocesses,
// provisioning, native sessions, and restoration evidence.
package runtimehost

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// Host owns startup and storage compatibility for one runtime profile and
// communication factory. The returned session owns the child and task lease.
type Host struct {
	profile         agentruntime.RuntimeProfile
	factory         agentruntime.ProtocolClientFactory
	environment     Environment
	cliAuthAdapters []gitprovider.CLIAuthAdapter
}

var runtimeVersionPattern = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9._+-]* )?v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// New binds runtime contracts to an explicitly granted execution environment.
func New(profile agentruntime.RuntimeProfile, factory agentruntime.ProtocolClientFactory, environment Environment, cliAuthAdapters ...gitprovider.CLIAuthAdapter) *Host {
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
		message := "could not reserve durable session storage; check storage availability and permissions"
		switch {
		case errors.Is(err, errSessionAlreadyExists):
			message = "saved session already exists; continue that session or explicitly start a new session"
		case errors.Is(err, os.ErrExist):
			message = "session creation was already reserved by a previous attempt; reconcile it or explicitly start a new session"
		}
		return nil, &agentruntime.LaunchError{Message: message, Cause: err}
	}
	settings := agentruntime.ResolvedRuntimeSettings{Agent: config.Agent, Model: config.Model, MCPServers: config.MCPServers, Skills: config.Skills}
	paths := h.provisioningPaths(config, store)
	executable, environment, err := h.prepareRuntime(ctx, config, settings, store, descriptor, restoring)
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
			session.callbacks.AddSecretValues(value)
		}
	}
	for _, value := range config.Environment {
		if value != "" {
			session.callbacks.AddSecretValues(value)
		}
	}
	if descriptor.Runtime == coreruntime.Codex {
		authFile := filepath.Join(paths.RuntimeRoot, filepath.FromSlash(codexAuthFileRelative))
		contents, readErr := os.ReadFile(authFile)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, &agentruntime.LaunchError{Message: "read saved Codex auth state failed", Cause: readErr}
		}
		session.callbacks.AddSecretValues(codexAuthSecretValues(contents)...)
	}
	for _, credential := range config.GitCredentials {
		if credential.Token != "" {
			session.callbacks.AddSecretValues(credential.Token, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+credential.Token)))
		}
	}
	for _, server := range config.MCPServers {
		for _, value := range server.Headers {
			session.callbacks.AddSecretValues(value)
		}
	}
	options := h.profile.ProtocolSessionOptions(settings, paths)
	if err := h.initializeSession(ctx, session, settings, options, descriptor, nativeID, restoring); err != nil {
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

func (h *Host) prepareRuntime(ctx context.Context, config runconfig.RuntimeConfig, settings agentruntime.ResolvedRuntimeSettings, store sessionStorage, descriptor agentruntime.RuntimeDescriptor, restoring bool) (string, []string, error) {
	paths := h.provisioningPaths(config, store)
	plan, err := h.profile.BuildProvisioningPlan(settings, paths)
	if err != nil {
		return "", nil, err
	}
	grantedHostEnv := slices.Clone(h.environment.GrantedEnv)
	if descriptor.Runtime == coreruntime.Codex {
		var authFileContents []byte
		grantedHostEnv, authFileContents, err = prepareLocalCodexAuthFile(grantedHostEnv, paths.RuntimeRoot, restoring)
		if err != nil {
			return "", nil, err
		}
		if authFileContents != nil {
			if plan.Files == nil {
				plan.Files = make(map[string][]byte)
			}
			plan.Files["home/.codex/auth.json"] = authFileContents
		}
	}
	granted := append(grantedHostEnv, environmentEntries(config.Environment)...)
	environment, err := applyProvisioningPlan(paths.RuntimeRoot, granted, descriptor.ReservedEnvNames, descriptor.ReservedEnvPrefixes, plan)
	if err != nil {
		return "", nil, err
	}
	environment, err = gitprovider.PrepareRuntime(ctx, gitprovider.RuntimePreparation{
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
	if err := verifyRuntimeVersion(ctx, executable, descriptor.VersionArguments, descriptor.Version, descriptor.VersionOutputPrefix, descriptor.VersionOutputSuffix, environment); err != nil {
		return "", nil, err
	}
	return executable, environment, nil
}

func (h *Host) provisioningPaths(config runconfig.RuntimeConfig, store sessionStorage) agentruntime.ProvisioningPaths {
	identity := config.RunID
	if config.TaskSessionID != "" {
		identity = config.TaskSessionID
	}
	return agentruntime.ProvisioningPaths{
		RuntimeRoot: store.Root(config), WorkDir: h.environment.WorkDir,
		ConfigSubdir: fmt.Sprintf("config-%x", sha256.Sum256([]byte(identity))),
	}
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

func (h *Host) initializeSession(ctx context.Context, session *Session, settings agentruntime.ResolvedRuntimeSettings, options agentruntime.ProtocolSessionOptions, descriptor agentruntime.RuntimeDescriptor, nativeID string, restoring bool) error {
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
		if err := session.connection.LoadSession(ctx, nativeID, h.environment.WorkDir, options); err != nil {
			return err
		}
		session.callbacks.finishReplay()
	} else {
		nativeID, err = session.connection.NewSession(ctx, h.environment.WorkDir, options)
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

func verifyRuntimeVersion(ctx context.Context, executable string, args []string, want string, outputPrefix, outputSuffix string, environment []string) error {
	probe := exec.CommandContext(ctx, executable, args...)
	probe.Env = environment
	output, err := probe.Output()
	if err != nil {
		message := "runtime version command failed; verify the pinned runtime is installed and executable"
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			message = fmt.Sprintf("runtime version command exited with status %d; verify the pinned runtime is installed and executable", exitError.ExitCode())
			if detail := safeRuntimeDiagnostic(string(exitError.Stderr), environment); detail != "" {
				message = fmt.Sprintf("runtime version command exited with status %d: %s", exitError.ExitCode(), detail)
			}
		} else if errors.Is(err, context.DeadlineExceeded) {
			message = "runtime version command timed out; verify the runtime image is healthy"
		}
		return &agentruntime.LaunchError{Message: message, Cause: err}
	}
	got := strings.TrimSpace(string(output))
	if outputPrefix != "" {
		got = strings.TrimPrefix(got, outputPrefix)
	}
	if outputSuffix != "" {
		got = strings.TrimSuffix(got, outputSuffix)
	}
	if got != want {
		if !safeRuntimeVersion(strings.TrimSpace(string(output))) {
			got = "unrecognized output"
		}
		return &agentruntime.LaunchError{Message: fmt.Sprintf("runtime version mismatch: Orkest requires %s but the installed runtime reports %s", want, got)}
	}
	return nil
}

func safeRuntimeVersion(value string) bool {
	return len(value) <= 64 && runtimeVersionPattern.MatchString(value)
}

func safeRuntimeDiagnostic(value string, environment []string) string {
	for _, entry := range environment {
		_, secret, ok := strings.Cut(entry, "=")
		if ok && secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 400 {
		value = string(runes[:400]) + "…"
	}
	return value
}

var _ agentruntime.RuntimeHost = (*Host)(nil)
