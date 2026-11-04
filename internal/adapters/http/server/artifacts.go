package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/artifact"
)

type artifactAPI struct{ service *artifact.ArtifactService }

func (a artifactAPI) routes(r chi.Router) {
	const prefix = "/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/artifacts"
	r.Get(prefix, a.list)
	r.Get(prefix+"/{artifactID}/revisions", a.revisions)
}

func artifactScope(r *http.Request) artifact.Scope {
	return artifact.Scope{UserID: currentUserID(r), OrganizationID: chi.URLParam(r, "orgID"), WorkspaceID: chi.URLParam(r, "workspaceID"), TaskID: chi.URLParam(r, "taskID")}
}

func (a artifactAPI) list(w http.ResponseWriter, r *http.Request) {
	value, err := a.service.List(r.Context(), artifactScope(r))
	respond(w, value, err, http.StatusOK)
}

func (a artifactAPI) revisions(w http.ResponseWriter, r *http.Request) {
	value, err := a.service.Revisions(r.Context(), artifactScope(r), chi.URLParam(r, "artifactID"))
	respond(w, value, err, http.StatusOK)
}
