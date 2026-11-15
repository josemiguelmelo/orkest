package docker

import "github.com/josemiguelmelo/orkest/internal/platform/taskstorage"

type taskStorageScope struct {
	OrganizationID string
	WorkspaceID    string
	TaskID         string
}

func taskStorageDirectories(dataRoot string, scope taskStorageScope) (string, string, error) {
	work, state := taskstorage.Directories(dataRoot, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	return work, state, nil
}
