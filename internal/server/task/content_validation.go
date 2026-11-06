package task

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/textutil"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

func NormalizeTaskContent(input *taskmodel.TaskInput) error {
	if input.StageVersion < 0 || (input.StartWork && input.CompleteStage) || len(input.Status) > 100 {
		return modelerrors.ErrInvalid
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" || len(input.Title) > 500 || len(input.Description) > 100000 {
		return modelerrors.ErrInvalid
	}
	input.AssigneeAgentID = strings.TrimSpace(input.AssigneeAgentID)
	input.Labels = textutil.Unique(input.Labels)
	input.RepositoryIDs = textutil.Unique(input.RepositoryIDs)
	return nil
}
