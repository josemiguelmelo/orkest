package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/scheduledjob"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type jobViewerAccess struct{}

func (jobViewerAccess) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: "viewer"}, nil
}

func TestScheduledJobAPIDeniesViewerExecutionAndApprovals(t *testing.T) {
	r := chi.NewRouter()
	scheduledJobAPI{service: scheduledjob.NewScheduledJobService(nil, jobViewerAccess{}, nil, nil)}.routes(r)
	base := "/organizations/org/workspaces/workspace/scheduled-jobs/job/runs"
	for _, suffix := range []string{"", "/occurrence/cancel", "/occurrence/permissions/respond", "/occurrence/questions/question/respond"} {
		request := httptest.NewRequest(http.MethodPost, base+suffix, strings.NewReader(`{"commandId":"key"}`))
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d %s", suffix, response.Code, response.Body.String())
		}
	}
}
