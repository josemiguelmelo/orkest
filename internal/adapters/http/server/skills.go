package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/skill"
)

type skillAPI struct {
	service *skill.CatalogService
	scope   skill.Scope
}

func (a skillAPI) routes(r chi.Router, base string) {
	r.Route(base, func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{skillID}", func(r chi.Router) {
			r.Get("/", a.get)
			r.Put("/", a.update)
			r.Delete("/", a.delete)
		})
	})
}

func (a skillAPI) list(w http.ResponseWriter, r *http.Request) {
	items, err := a.service.List(r.Context(), currentUserID(r), a.scope, chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, items, err, http.StatusOK)
}

func (a skillAPI) get(w http.ResponseWriter, r *http.Request) {
	item, err := a.service.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "skillID"))
	respond(w, item, err, http.StatusOK)
}

func (a skillAPI) create(w http.ResponseWriter, r *http.Request) {
	var body skill.Input
	if !decodeBody(w, r, &body) {
		return
	}
	item, err := a.service.Create(r.Context(), currentUserID(r), a.scope, chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body.SkillMD)
	respond(w, item, err, http.StatusCreated)
}

func (a skillAPI) update(w http.ResponseWriter, r *http.Request) {
	var body skill.Input
	if !decodeBody(w, r, &body) {
		return
	}
	item, err := a.service.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "skillID"), body.SkillMD)
	respond(w, item, err, http.StatusOK)
}

func (a skillAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "skillID")), http.StatusNoContent)
}
