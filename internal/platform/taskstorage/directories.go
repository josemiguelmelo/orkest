package taskstorage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// Directories returns the durable work and agent state paths for a task scope.
func Directories(dataRoot, organizationID, workspaceID, taskID string) (string, string) {
	return Paths(dataRoot, Identity(organizationID, workspaceID, taskID))
}

// Identity returns the durable opaque storage key for an authorized task scope.
// Infrastructure adapters receive this value rather than interpreting domain IDs.
func Identity(organizationID, workspaceID, taskID string) string {
	scope, _ := json.Marshal([]string{organizationID, workspaceID, taskID})
	hash := sha256.Sum256(scope)
	return hex.EncodeToString(hash[:])
}

// Paths resolves an opaque storage identity within an engine's configured root.
func Paths(dataRoot, identity string) (string, string) {
	root := RootPath(dataRoot, identity)
	return filepath.Join(root, "work"), filepath.Join(root, "agentctl-state")
}

// RootPath resolves an opaque storage identity under the shared durable root.
func RootPath(dataRoot, identity string) string {
	return filepath.Join(dataRoot, "tasks", filepath.Base(identity))
}

// BindingPath resolves a caller-named relative storage path without allowing it
// to escape the durable identity root.
func BindingPath(dataRoot, identity, subpath string) (string, error) {
	if len(identity) != 64 {
		return "", errors.New("storage identity must be a SHA-256 key")
	}
	if _, err := hex.DecodeString(identity); err != nil {
		return "", errors.New("storage identity must be a SHA-256 key")
	}
	clean := path.Clean(subpath)
	if clean == "." {
		return RootPath(dataRoot, identity), nil
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("storage binding subpath must remain within its identity root")
	}
	return filepath.Join(RootPath(dataRoot, identity), filepath.FromSlash(clean)), nil
}
