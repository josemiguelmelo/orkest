package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
)

type executionProfileAPI struct {
	service *executionprofile.ExecutionProfileService
}

func (a executionProfileAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/execution-profiles", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.save)
		r.Put("/{profileID}", a.save)
		r.Get("/{profileID}/builds", a.builds)
	})
}

func (a executionProfileAPI) list(w http.ResponseWriter, r *http.Request) {
	org, ws, _ := scopeParams(r)
	items, err := a.service.List(r.Context(), currentUserID(r), org, ws)
	respond(w, items, err, http.StatusOK)
}

func (a executionProfileAPI) save(w http.ResponseWriter, r *http.Request) {
	var body struct {
		executionprofile.Input
		Generation int64 `json:"generation"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	org, ws, _ := scopeParams(r)
	item, err := a.service.Save(r.Context(), currentUserID(r), org, ws, chi.URLParam(r, "profileID"), body.Generation, body.Input)
	respond(w, item, err, http.StatusAccepted)
}

func (a executionProfileAPI) builds(w http.ResponseWriter, r *http.Request) {
	org, ws, _ := scopeParams(r)
	items, err := a.service.Builds(r.Context(), currentUserID(r), org, ws, chi.URLParam(r, "profileID"))
	respond(w, items, err, http.StatusOK)
}
