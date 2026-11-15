package repositorypath

import (
	"errors"
	"path/filepath"
)

// Relative validates repository identity components and returns their task-worktree path.
func Relative(provider, repositoryID string) (string, error) {
	if !safeComponent(provider) || !safeComponent(repositoryID) {
		return "", errors.New("repository identity is invalid")
	}
	return filepath.Join("repositories", provider, repositoryID), nil
}

func safeComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && filepath.IsLocal(value)
}
