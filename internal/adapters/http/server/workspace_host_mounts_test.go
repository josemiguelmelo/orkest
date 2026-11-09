package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
	"github.com/josemiguelmelo/orkest/internal/server/workspacemount"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type mountRouteRepository struct{ mounts []mountmodel.Mount }

func (r *mountRouteRepository) List(context.Context, string) ([]mountmodel.Mount, error) {
	if r.mounts == nil {
		return []mountmodel.Mount{}, nil
	}
	return r.mounts, nil
}

func (r *mountRouteRepository) Put(_ context.Context, _ string, mounts []mountmodel.Mount) error {
	r.mounts = mounts
	return nil
}

type mountRouteAccess struct {
	role string
	err  error
}

func (a mountRouteAccess) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: a.role}, a.err
}

func TestWorkspaceHostMountRoutes(t *testing.T) {
	for _, test := range []struct {
		name, role, body string
		status           int
		accessErr        error
	}{
		{"admin", "admin", `{"mounts":[{"source":"/srv/input","target":"/mnt/host/input","readOnly":true}]}`, 200, nil},
		{"owner", "owner", `{"mounts":[]}`, 200, nil},
		{"member", "member", `{"mounts":[]}`, 403, nil},
		{"viewer", "viewer", `{"mounts":[]}`, 403, nil},
		{"custom source", "admin", `{"mounts":[{"source":"/srv/custom","target":"/mnt/host/input","readOnly":true}]}`, 200, nil},
		{"write access", "admin", `{"mounts":[{"source":"/srv/custom","target":"/mnt/host/output","readOnly":false}]}`, 200, nil},
		{"aws home", "admin", `{"mounts":[{"source":"/srv/aws","target":"~/.aws","readOnly":true}]}`, 200, nil},
		{"maven home", "admin", `{"mounts":[{"source":"/srv/settings.xml","target":"~/.m2/settings.xml","readOnly":true}]}`, 200, nil},
		{"gradle home", "admin", `{"mounts":[{"source":"/srv/gradle.properties","target":"~/.gradle/gradle.properties","readOnly":true}]}`, 200, nil},
		{"home traversal", "admin", `{"mounts":[{"source":"/srv/aws","target":"~/../session.json","readOnly":true}]}`, 400, nil},
		{"missing mounts", "admin", `{}`, 400, nil},
		{"null mounts", "admin", `{"mounts":null}`, 400, nil},
		{"missing access mode", "admin", `{"mounts":[{"source":"/srv/input","target":"/mnt/host/input"}]}`, 400, nil},
		{"reserved target", "admin", `{"mounts":[{"source":"/srv/input","target":"/workspace","readOnly":true}]}`, 400, nil},
		{"wrong workspace", "admin", `{"mounts":[]}`, 404, modelerrors.ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &mountRouteRepository{}
			service := workspacemount.New(repo, mountRouteAccess{role: test.role, err: test.accessErr}, true)
			router := chi.NewRouter()
			workspaceHostMountsAPI{service: service}.routes(router)
			for _, suffix := range []string{"", "/"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/organizations/org/workspaces/workspace/host-mounts"+suffix, strings.NewReader(test.body)))
				if response.Code != test.status {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			}
			if test.status != 200 && len(repo.mounts) != 0 {
				t.Fatal("invalid request mutated mounts")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/organizations/org/workspaces/workspace/host-mounts", nil))
			if errors.Is(test.accessErr, modelerrors.ErrNotFound) {
				if response.Code != http.StatusNotFound {
					t.Fatal("unauthorized settings read")
				}
			} else if response.Code != 200 || !strings.Contains(response.Body.String(), `"enabled":true,"mounts":[`) {
				t.Fatalf("settings shape incorrect: %s", response.Body.String())
			}
		})
	}
}
