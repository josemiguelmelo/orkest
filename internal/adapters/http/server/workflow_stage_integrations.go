package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	stageintegration "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

type workflowStageIntegrationAPI struct {
	service *stageintegration.WorkflowStageIntegrationService
}

func (a workflowStageIntegrationAPI) routes(r chi.Router) {
	r.Get("/workflow-stage-integration-providers", a.providers)
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/workflow-stage-integrations", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Post("/events/{eventID}/retry", a.retry)
		r.Route("/{connectionID}", func(r chi.Router) {
			r.Put("/", a.update)
			r.Delete("/", a.delete)
			r.Post("/validate", a.validate)
			r.Get("/status", a.status)
		})
	})
}

func (a workflowStageIntegrationAPI) providers(w http.ResponseWriter, _ *http.Request) {
	respond(w, a.service.Providers(), nil, http.StatusOK)
}

func (a workflowStageIntegrationAPI) list(w http.ResponseWriter, r *http.Request) {
	value, err := a.service.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, value, err, http.StatusOK)
}

func (a workflowStageIntegrationAPI) create(w http.ResponseWriter, r *http.Request) {
	var input stageintegrationmodel.ConnectionInput
	if !decodeBody(w, r, &input) {
		return
	}
	value, err := a.service.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), input)
	respond(w, value, err, http.StatusCreated)
}

func (a workflowStageIntegrationAPI) update(w http.ResponseWriter, r *http.Request) {
	var input stageintegrationmodel.ConnectionInput
	if !decodeBody(w, r, &input) {
		return
	}
	value, err := a.service.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "connectionID"), input)
	respond(w, value, err, http.StatusOK)
}

func (a workflowStageIntegrationAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "connectionID")), http.StatusNoContent)
}

func (a workflowStageIntegrationAPI) validate(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "ok"}, a.service.Validate(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "connectionID")), http.StatusOK)
}

func (a workflowStageIntegrationAPI) status(w http.ResponseWriter, r *http.Request) {
	value, err := a.service.Status(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "connectionID"), r.URL.Query().Get("cursor"))
	respond(w, value, err, http.StatusOK)
}

func (a workflowStageIntegrationAPI) retry(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.service.Retry(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "eventID")), http.StatusNoContent)
}
