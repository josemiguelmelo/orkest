package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type questionTaskRepository struct{ task.TaskRepository }

func (questionTaskRepository) Task(_ context.Context, orgID, workspaceID, taskID string) (taskmodel.Task, error) {
	if orgID != "org" || workspaceID != "workspace" || taskID != "task" {
		return taskmodel.Task{}, modelerrors.ErrNotFound
	}
	return taskmodel.Task{ID: taskID}, nil
}

func TestQuestionRoutesAuthorizeBeforeAnswerDelivery(t *testing.T) {
	tasks := task.NewTaskService(questionTaskRepository{}, nil, nil, nil, profileRouteAccess{role: "viewer"})
	router := chi.NewRouter()
	taskRunAPI{tasks: tasks}.routes(router)
	directChatAPI{workspaces: workspace.NewWorkspaceService(routeWorkspaceRepository{}, nil)}.routes(router)
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/organizations/org/workspaces/workspace/tasks/task/runs/run/questions/question/respond", http.StatusForbidden},
		{"/organizations/org/workspaces/another/tasks/task/runs/run/questions/question/respond", http.StatusNotFound},
		{"/organizations/org/workspaces/workspace/agent-conversations/conversation/runs/run/questions/question/respond", http.StatusForbidden},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("%s: got %d, want %d", test.path, response.Code, test.status)
		}
	}
}
