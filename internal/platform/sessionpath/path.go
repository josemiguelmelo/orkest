// Package sessionpath defines the durable native runtime storage layout.
package sessionpath

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
)

func TaskRoot(stateDir, organizationID, workspaceID, taskID string) string {
	scope, _ := json.Marshal([]string{organizationID, workspaceID, taskID})
	return filepath.Join(stateDir, fmt.Sprintf("%x", sha256.Sum256(scope)))
}

func Root(stateDir, organizationID, workspaceID, taskID, sessionID string) string {
	root := TaskRoot(stateDir, organizationID, workspaceID, taskID)
	if sessionID != "" {
		return filepath.Join(root, "sessions", fmt.Sprintf("%x", sha256.Sum256([]byte(sessionID))))
	}
	return root
}

func Home(stateDir, organizationID, workspaceID, taskID, sessionID string) string {
	return filepath.Join(Root(stateDir, organizationID, workspaceID, taskID, sessionID), "home")
}
