package taskstorage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
)

// Directories returns the durable work and agent state paths for a task scope.
func Directories(dataRoot, organizationID, workspaceID, taskID string) (string, string) {
	scope, _ := json.Marshal([]string{organizationID, workspaceID, taskID})
	hash := sha256.Sum256(scope)
	taskDir := filepath.Join(dataRoot, "tasks", hex.EncodeToString(hash[:]))
	return filepath.Join(taskDir, "work"), filepath.Join(taskDir, "agentctl-state")
}
