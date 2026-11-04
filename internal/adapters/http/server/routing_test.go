package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type routeWorkspaceRepository struct{ workspace.WorkspaceRepository }

func (routeWorkspaceRepository) Workspace(_ context.Context, orgID, id, _ string) (workspace.Workspace, error) {
	return workspace.Workspace{ID: id, OrganizationID: orgID, Name: "Workspace", Role: "viewer"}, nil
}

func (routeWorkspaceRepository) WorkspaceMembers(context.Context, string, string) ([]workspace.Membership, error) {
	return []workspace.Membership{}, nil
}

func TestTaskRoutesPreserveWorkspaceRoutes(t *testing.T) {
	router := chi.NewRouter()
	organizationAPI{workspaces: workspace.NewWorkspaceService(routeWorkspaceRepository{}, nil)}.routes(router)
	taskAPI{}.routes(router)
	for _, path := range []string{"/organizations/org/workspaces/ws", "/organizations/org/workspaces/ws/members"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "404") {
			t.Fatalf("workspace route %s shadowed: %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
}
