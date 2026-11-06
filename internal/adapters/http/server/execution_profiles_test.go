package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
	executionprofilemodel "github.com/josemiguelmelo/orkest/internal/server/executionprofile/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type profileRouteAccess struct{ role string }

func (a profileRouteAccess) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: a.role}, nil
}

type profileRouteRepository struct {
	executionprofile.ExecutionProfileRepository
	saved int
}

func (r *profileRouteRepository) Save(_ context.Context, p executionprofilemodel.Profile, b executionprofilemodel.Build, _ bool) (executionprofilemodel.Profile, error) {
	r.saved++
	p.Status = "queued"
	p.Generation = 1
	return p, nil
}

func TestProfileCreationEnqueuesAndEnforcesManagementRole(t *testing.T) {
	for _, role := range []string{"admin", "member", "viewer"} {
		t.Run(role, func(t *testing.T) {
			repo := &profileRouteRepository{}
			service := executionprofile.NewExecutionProfileService(repo, profileRouteAccess{role}, "example.com/images", nil)
			router := chi.NewRouter()
			executionProfileAPI{service: service}.routes(router)
			request := httptest.NewRequest(http.MethodPost, "/organizations/o/workspaces/w/execution-profiles/", strings.NewReader(`{"name":"Java","description":"Backend","runtime":"opencode","toolchainImage":"example.com/java:21"}`))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			expected := http.StatusForbidden
			if role == "admin" {
				expected = http.StatusAccepted
			}
			if response.Code != expected {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if role == "admin" {
				if repo.saved != 1 || !strings.Contains(response.Body.String(), `"status":"queued"`) {
					t.Fatal("profile not queued", response.Body.String())
				}
			} else if repo.saved != 0 {
				t.Fatal("unauthorized mutation")
			}
		})
	}
}

func TestProfileCreationReportsMissingRegistryOnClientRoute(t *testing.T) {
	service := executionprofile.NewExecutionProfileService(nil, profileRouteAccess{"admin"}, "", nil)
	router := chi.NewRouter()
	executionProfileAPI{service: service}.routes(router)
	for _, suffix := range []string{"", "/"} {
		request := httptest.NewRequest(http.MethodPost, "/organizations/o/workspaces/w/execution-profiles"+suffix, strings.NewReader(`{"name":"Java","description":"Backend","runtime":"opencode","toolchainImage":"example.com/java:21"}`))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "execution_images_not_configured") {
			t.Fatalf("unexpected registry error on %q: %d %s", suffix, response.Code, response.Body.String())
		}
	}
}
