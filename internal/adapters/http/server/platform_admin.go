package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
	"github.com/josemiguelmelo/orkest/internal/server/platformadmin"
)

type PlatformRuntimeConfiguration struct {
	Executor       string            `json:"executor"`
	AgentctlImages map[string]string `json:"agentctlImages"`
	AgentMCPURL    string            `json:"agentMcpUrl"`
	AgentMCPSource string            `json:"agentMcpSource"`
	DataDirectory  string            `json:"dataDirectory"`
	CodexAuth      CodexAuthSource   `json:"codexAuth"`
	LoadedAt       time.Time         `json:"loadedAt"`
}

type CodexAuthSource struct {
	Configured bool   `json:"configured"`
	Type       string `json:"type"`
	Location   string `json:"location,omitempty"`
}

type platformAdminAPI struct {
	users         *identity.UserRegistrationService
	overview      *platformadmin.OverviewService
	configuration PlatformRuntimeConfiguration
}

func (a platformAdminAPI) routes(router chi.Router) {
	router.Get("/platform-admin/overview", a.getOverview)
	router.Get("/platform-admin/runtime-configuration", a.getRuntimeConfiguration)
}

func (a platformAdminAPI) authorize(w http.ResponseWriter, r *http.Request) bool {
	allowed, err := a.users.IsPlatformAdmin(r.Context(), currentUserID(r))
	if err != nil {
		respond(w, nil, err, http.StatusOK)
		return false
	}
	if !allowed {
		respond(w, nil, problem.ErrForbidden, http.StatusOK)
		return false
	}
	return true
}

func (a platformAdminAPI) getOverview(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(w, r) {
		return
	}
	value, err := a.overview.Get(r.Context())
	respond(w, value, err, http.StatusOK)
}

func (a platformAdminAPI) getRuntimeConfiguration(w http.ResponseWriter, r *http.Request) {
	if !a.authorize(w, r) {
		return
	}
	respond(w, a.configuration, nil, http.StatusOK)
}
