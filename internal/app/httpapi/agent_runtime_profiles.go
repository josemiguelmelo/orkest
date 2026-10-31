package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentruntimeprofile"
)

type agentRuntimeProfileAPI struct{ service *agentruntimeprofile.Service }

func (a agentRuntimeProfileAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/agent-runtime-profiles", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Put("/{profileID}", a.update)
		r.Delete("/{profileID}", a.delete)
	})
}

func (a agentRuntimeProfileAPI) list(w http.ResponseWriter, r *http.Request) {
	v, err := a.service.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, err, http.StatusOK)
}

func (a agentRuntimeProfileAPI) create(w http.ResponseWriter, r *http.Request) {
	var input agentruntimeprofile.Profile
	if !decodeBody(w, r, &input) {
		return
	}
	v, err := a.service.Save(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), "", input)
	respond(w, v, err, http.StatusCreated)
}

func (a agentRuntimeProfileAPI) update(w http.ResponseWriter, r *http.Request) {
	var input agentruntimeprofile.Profile
	if !decodeBody(w, r, &input) {
		return
	}
	v, err := a.service.Save(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "profileID"), input)
	respond(w, v, err, http.StatusOK)
}

func (a agentRuntimeProfileAPI) delete(w http.ResponseWriter, r *http.Request) {
	err := a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "profileID"))
	respond(w, nil, err, http.StatusNoContent)
}
