package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
)

type codeRepositoryAPI struct{ repositories *coderepository.Service }

func (a codeRepositoryAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/code-repositories", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{repositoryID}", func(r chi.Router) {
			r.Get("/", a.get)
			r.Delete("/", a.delete)
			r.Post("/clone", a.clone)
			r.Post("/sync", a.sync)
		})
	})
}

func (a codeRepositoryAPI) list(w http.ResponseWriter, r *http.Request) {
	v, err := a.repositories.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, err, http.StatusOK)
}

func (a codeRepositoryAPI) get(w http.ResponseWriter, r *http.Request) {
	v, err := a.repositories.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "repositoryID"))
	respond(w, v, err, http.StatusOK)
}

func (a codeRepositoryAPI) create(w http.ResponseWriter, r *http.Request) {
	var body coderepository.CreateRequest
	if !decodeBody(w, r, &body) {
		return
	}
	v, err := a.repositories.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body.ProviderName, body.URL)
	respond(w, v, err, http.StatusCreated)
}

func (a codeRepositoryAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.repositories.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "repositoryID")), http.StatusNoContent)
}

func (a codeRepositoryAPI) clone(w http.ResponseWriter, r *http.Request) {
	err := a.repositories.Clone(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "repositoryID"))
	respond(w, nil, err, http.StatusNoContent)
}

func (a codeRepositoryAPI) sync(w http.ResponseWriter, r *http.Request) {
	err := a.repositories.Sync(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "repositoryID"))
	respond(w, nil, err, http.StatusNoContent)
}
