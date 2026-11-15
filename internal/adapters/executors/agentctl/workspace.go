// Package agentctl contains execution policy shared by the infrastructure
// engines. It prepares durable task workspaces before a workload is launched.
package agentctl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// ArgumentConfig describes agentctl's shared argument policy. Execution-mode
// details remain explicit because native workers require a run identity and
// host paths while container workers retain their session-conditioned run ID.
type ArgumentConfig struct {
	Runtime          string
	ListenAddress    string
	WorkPath         string
	StatePath        string
	RunID            string
	HasSession       bool
	Native           bool
	EnvironmentNames []string
}

// BuildArguments returns literal argv elements; endpoint addresses are already
// resolved by the selected executor and are never shell-interpolated.
func BuildArguments(config ArgumentConfig) []string {
	args := []string{"-listen", config.ListenAddress, "-runtime", config.Runtime, "-work-dir", config.WorkPath, "-state-dir", config.StatePath}
	if config.Native || config.HasSession {
		args = append(args, "-run-id", config.RunID)
	}
	seen := make(map[string]struct{}, len(config.EnvironmentNames)+1)
	for _, name := range config.EnvironmentNames {
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		args = append(args, "-env", name)
	}
	if config.Native {
		if _, granted := seen["PATH"]; !granted {
			args = append(args, "-env", "PATH")
		}
	}
	return args
}

// WorkspaceConfig contains the shared workspace preparation dependencies.
// Infrastructure-specific mounts and credential materialization belong to the
// selected engine.
type WorkspaceConfig struct {
	DataRoot     string
	Repositories checkout.Preparer
}

// PrepareWorkspace creates the historical durable task directories, preserves
// their private permissions, and prepares selected repositories in place.
func PrepareWorkspace(ctx context.Context, config WorkspaceConfig, run model.RunConfig) (engine.Workspace, error) {
	work, state, err := EnsureTaskWorkspace(config.DataRoot, run.OrganizationID, run.WorkspaceID, run.TaskID)
	if err != nil {
		return engine.Workspace{}, err
	}
	if err := config.Repositories.Prepare(ctx, work, run.WorkspaceID, run.Repositories); err != nil {
		return engine.Workspace{}, fmt.Errorf("prepare task repositories: %w", err)
	}
	return engine.Workspace{
		Identity:             filepath.Base(filepath.Dir(work)),
		Exists:               true,
		PreparationDirectory: work,
		WorkPath:             work,
		StatePath:            state,
		CompatibilityID:      filepath.Base(filepath.Dir(work)),
	}, nil
}

// EnsureTaskWorkspace creates the durable work and runtime-state directories.
func EnsureTaskWorkspace(dataRoot, organizationID, workspaceID, taskID string) (string, string, error) {
	work, state := taskstorage.Directories(dataRoot, organizationID, workspaceID, taskID)
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

// PrepareRepositories checks out the selected repositories without replacing
// preserved task branches or files.
func PrepareRepositories(ctx context.Context, preparer checkout.Preparer, workDir string, run model.RunConfig) error {
	if err := preparer.Prepare(ctx, workDir, run.WorkspaceID, run.Repositories); err != nil {
		return fmt.Errorf("prepare task repositories: %w", err)
	}
	return nil
}
