package task

import (
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

// PreserveSourceContent applies edits without replacing source-managed fields.
// Callers provide the task read under the same lock as the eventual update.
func PreserveSourceContent(current taskmodel.Task, input taskmodel.TaskInput) taskmodel.TaskInput {
	if current.SourceProvider != "manual" {
		input.Title, input.Description, input.Labels = current.Title, current.Description, current.Labels
	}
	return input
}
