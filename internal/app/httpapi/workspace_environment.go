package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/workspaceenv"
)

type workspaceEnvironmentAPI struct{ service *workspaceenv.Service }

func (a workspaceEnvironmentAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/environment", func(r chi.Router) {
		r.Get("/", a.list)
		r.Put("/{name}", a.put)
		r.Delete("/{name}", a.delete)
	})
}

func (a workspaceEnvironmentAPI) list(w http.ResponseWriter, r *http.Request) {
	values, err := a.service.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, values, err, http.StatusOK)
}

func (a workspaceEnvironmentAPI) put(w http.ResponseWriter, r *http.Request) {
	var input workspaceenv.Input
	if !decodeBody(w, r, &input) {
		return
	}
	err := a.service.Put(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "name"), input)
	respond(w, map[string]string{"status": "updated"}, err, http.StatusOK)
}

func (a workspaceEnvironmentAPI) delete(w http.ResponseWriter, r *http.Request) {
	err := a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "name"))
	respond(w, nil, err, http.StatusNoContent)
}
