package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
)

type codingIntegrationAPI struct{ service *coderepository.Service }

func (a codingIntegrationAPI) routes(r chi.Router) {
	r.Get("/coding-providers", a.providers)
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/coding-integrations", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{integrationID}", func(r chi.Router) {
			r.Put("/", a.update)
			r.Delete("/", a.delete)
			r.Post("/validate", a.validate)
			r.Get("/repositories", a.repositories)
		})
	})
}

func (a codingIntegrationAPI) providers(w http.ResponseWriter, r *http.Request) {
	respond(w, a.service.ProviderCatalog(), nil, http.StatusOK)
}

func (a codingIntegrationAPI) list(w http.ResponseWriter, r *http.Request) {
	v, e := a.service.ListIntegrations(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, e, http.StatusOK)
}

func (a codingIntegrationAPI) create(w http.ResponseWriter, r *http.Request) {
	var body coderepository.CodingIntegrationInput
	if !decodeBody(w, r, &body) {
		return
	}
	v, e := a.service.CreateIntegration(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body)
	respond(w, v, e, http.StatusCreated)
}

func (a codingIntegrationAPI) update(w http.ResponseWriter, r *http.Request) {
	var body coderepository.CodingIntegrationInput
	if !decodeBody(w, r, &body) {
		return
	}
	e := a.service.UpdateIntegration(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID"), body)
	respond(w, nil, e, http.StatusNoContent)
}

func (a codingIntegrationAPI) delete(w http.ResponseWriter, r *http.Request) {
	e := a.service.DeleteIntegration(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID"))
	respond(w, nil, e, http.StatusNoContent)
}

func (a codingIntegrationAPI) validate(w http.ResponseWriter, r *http.Request) {
	e := a.service.ValidateIntegration(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID"))
	respond(w, map[string]bool{"connected": e == nil}, e, http.StatusOK)
}

func (a codingIntegrationAPI) repositories(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
	v, e := a.service.BrowseRepositories(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "integrationID"), page, perPage)
	respond(w, v, e, http.StatusOK)
}
