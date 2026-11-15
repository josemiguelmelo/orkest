package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

// TaskCheckoutInspector inspects the existing Docker task worktree without
// preparing repositories or changing their contents.
type TaskCheckoutInspector struct {
	DataRoot string
}

func (i TaskCheckoutInspector) Inspect(ctx context.Context, organizationID, workspaceID, taskID string, selected []taskeditor.Checkout) (taskeditor.Inspection, error) {
	if strings.TrimSpace(i.DataRoot) == "" {
		return taskeditor.Inspection{}, errors.New("docker data root is required")
	}
	if err := ctx.Err(); err != nil {
		return taskeditor.Inspection{}, err
	}
	dataRoot, err := filepath.Abs(i.DataRoot)
	if err != nil {
		return taskeditor.Inspection{}, fmt.Errorf("resolve Docker data root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(dataRoot)
	if err != nil {
		return taskeditor.Inspection{}, fmt.Errorf("resolve Docker data root: %w", err)
	}
	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil {
		return taskeditor.Inspection{}, fmt.Errorf("inspect Docker data root: %w", err)
	}
	if !rootInfo.IsDir() {
		return taskeditor.Inspection{}, errors.New("docker data root is not a directory")
	}
	work, _ := taskstorage.Directories(canonicalRoot, organizationID, workspaceID, taskID)
	canonicalWork, err := existingDirectory(canonicalRoot, work)
	if errors.Is(err, os.ErrNotExist) {
		return taskeditor.Inspection{Roots: []taskeditor.Root{}, Missing: missingCheckouts(selected)}, nil
	}
	if err != nil {
		return taskeditor.Inspection{}, fmt.Errorf("inspect task work directory: %w", err)
	}
	return taskeditor.Inspection{
		Roots:   []taskeditor.Root{{Name: "Task workspace", Path: "/task", StorageRef: canonicalWork}},
		Missing: []taskeditor.MissingCheckout{},
	}, nil
}

func missingCheckouts(selected []taskeditor.Checkout) []taskeditor.MissingCheckout {
	missing := make([]taskeditor.MissingCheckout, 0, len(selected))
	for _, checkout := range selected {
		missing = append(missing, missingCheckout(checkout))
	}
	return missing
}

func missingCheckout(checkout taskeditor.Checkout) taskeditor.MissingCheckout {
	return taskeditor.MissingCheckout(checkout)
}

func existingDirectory(root, target string) (string, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil || !within(root, target) {
		return "", modelerrors.ErrInvalid
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", modelerrors.ErrInvalid
		}
	}
	return current, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
