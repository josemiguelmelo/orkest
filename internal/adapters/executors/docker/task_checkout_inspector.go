package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/repositorypath"
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
	for _, repository := range selected {
		if _, err := repositorypath.Relative(repository.ProviderName, repository.RegistrationID); err != nil {
			return taskeditor.Inspection{}, modelerrors.ErrInvalid
		}
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
	result := taskeditor.Inspection{Roots: []taskeditor.Root{}, Missing: []taskeditor.MissingCheckout{}}
	for _, repository := range selected {
		if err := ctx.Err(); err != nil {
			return taskeditor.Inspection{}, err
		}
		rel, _ := repositorypath.Relative(repository.ProviderName, repository.RegistrationID)
		candidate := filepath.Join(canonicalWork, rel)
		resolved, resolveErr := existingDirectory(canonicalWork, candidate)
		if errors.Is(resolveErr, os.ErrNotExist) {
			result.Missing = append(result.Missing, taskeditor.MissingCheckout(repository))
			continue
		}
		if resolveErr != nil {
			return taskeditor.Inspection{}, fmt.Errorf("resolve checkout %q: %w", repository.RegistrationID, resolveErr)
		}
		if !within(canonicalWork, resolved) {
			return taskeditor.Inspection{}, modelerrors.ErrInvalid
		}
		gitDir := filepath.Join(resolved, ".git")
		gitInfo, gitErr := os.Lstat(gitDir)
		if gitErr != nil {
			return taskeditor.Inspection{}, fmt.Errorf("checkout %q has unsupported Git metadata: %w", repository.RegistrationID, gitErr)
		}
		if gitInfo.Mode()&os.ModeSymlink != 0 || !gitInfo.IsDir() {
			return taskeditor.Inspection{}, fmt.Errorf("checkout %q has unsupported Git metadata", repository.RegistrationID)
		}
		if err := validateGitMetadata(ctx, gitDir); err != nil {
			return taskeditor.Inspection{}, fmt.Errorf("checkout %q has unsupported Git metadata: %w", repository.RegistrationID, err)
		}
		name := taskeditor.DisplayName(result.Roots, repository.Name, repository.ProviderName, repository.RegistrationID)
		result.Roots = append(result.Roots, taskeditor.Root{Name: name, Path: "/workspace/" + filepath.ToSlash(rel), Source: resolved})
	}
	return result, nil
}

func missingCheckouts(selected []taskeditor.Checkout) []taskeditor.MissingCheckout {
	missing := make([]taskeditor.MissingCheckout, 0, len(selected))
	for _, checkout := range selected {
		missing = append(missing, taskeditor.MissingCheckout(checkout))
	}
	return missing
}

func validateGitMetadata(ctx context.Context, gitDir string) error {
	if err := filepath.WalkDir(gitDir, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s contains a symlink", filepath.Base(path))
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("%s contains a non-regular file", filepath.Base(path))
		}
		return nil
	}); err != nil {
		return fmt.Errorf("inspect Git metadata tree: %w", err)
	}
	for _, relative := range []string{"commondir", filepath.Join("objects", "info", "alternates")} {
		path := filepath.Join(gitDir, relative)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not an ordinary file", relative)
		}
		return fmt.Errorf("%s is unsupported", relative)
	}
	for _, relative := range []string{"objects", "refs"} {
		info, err := os.Lstat(filepath.Join(gitDir, relative))
		if err != nil {
			return fmt.Errorf("inspect %s: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s is not an ordinary directory", relative)
		}
	}
	return nil
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
