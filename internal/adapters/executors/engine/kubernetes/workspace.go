package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
)

// WorkspaceResolver prepares the platform-mounted durable PVC paths used by
// Kubernetes workers. DataRoot is the host mount point; DataSubpath is applied
// only when building pod volume mounts.
type WorkspaceResolver struct {
	dataRoot string
}

func NewWorkspaceResolver(dataRoot string) (*WorkspaceResolver, error) {
	root := strings.TrimSpace(dataRoot)
	if root == "" {
		return nil, errors.New("kubernetes workspace data root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Kubernetes workspace data root: %w", err)
	}
	return &WorkspaceResolver{dataRoot: root}, nil
}

func (r *WorkspaceResolver) ResolveWorkspace(ctx context.Context, request engine.WorkspaceRequest) (engine.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return engine.Workspace{}, err
	}
	if request.Scope.OrganizationID == "" || request.Scope.WorkspaceID == "" || request.Scope.TaskID == "" {
		return engine.Workspace{}, fmt.Errorf("%w: Kubernetes task workspace scope is required", engine.ErrInvalidSpec)
	}
	capabilities := engine.Capabilities{DurableWork: true, DurableState: true, PlatformWorkspaceSetup: true}
	if err := engine.ValidateWorkspaceGuarantees(capabilities, request.Required); err != nil {
		return engine.Workspace{}, err
	}
	work, state := taskstorage.Directories(r.dataRoot, request.Scope.OrganizationID, request.Scope.WorkspaceID, request.Scope.TaskID)
	scopeBytes, _ := json.Marshal([]string{request.Scope.OrganizationID, request.Scope.WorkspaceID, request.Scope.TaskID})
	scopeHash := sha256.Sum256(scopeBytes)
	compatibilityID := hex.EncodeToString(scopeHash[:])
	workspace := engine.Workspace{Identity: compatibilityID, PreparationDirectory: work, WorkPath: work, StatePath: state, CompatibilityID: compatibilityID}
	switch request.Intent {
	case engine.WorkspaceInspect:
		paths := []string{work}
		if request.Required.DurableState {
			paths = append(paths, state)
		}
		for _, path := range paths {
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				return workspace, nil
			}
			if err != nil {
				return engine.Workspace{}, fmt.Errorf("inspect Kubernetes task workspace: %w", err)
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return engine.Workspace{}, fmt.Errorf("%w: Kubernetes task workspace path is not a regular directory", engine.ErrInvalidSpec)
			}
		}
		workspace.Exists = true
		return workspace, nil
	case engine.WorkspaceEnsureForAgent:
		for _, path := range []string{work, state} {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return engine.Workspace{}, fmt.Errorf("create durable Kubernetes workspace directory: %w", err)
			}
			if err := os.Chmod(path, 0o700); err != nil {
				return engine.Workspace{}, fmt.Errorf("restrict Kubernetes workspace directory permissions: %w", err)
			}
		}
		workspace.Exists = true
		return workspace, nil
	default:
		return engine.Workspace{}, fmt.Errorf("%w: unknown Kubernetes workspace intent", engine.ErrInvalidSpec)
	}
}
