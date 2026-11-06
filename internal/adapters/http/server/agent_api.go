package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
)

type agentAPI struct{ agents *agent.AgentDefinitionUseCase }

func (a agentAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/agents", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{agentID}", func(r chi.Router) {
			r.Get("/", a.get)
			r.Put("/", a.update)
			r.Delete("/", a.delete)
		})
	})
}

func (a agentAPI) list(w http.ResponseWriter, r *http.Request) {
	v, err := a.agents.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, err, http.StatusOK)
}

func (a agentAPI) get(w http.ResponseWriter, r *http.Request) {
	v, err := a.agents.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID"))
	respond(w, v, err, http.StatusOK)
}

func (a agentAPI) create(w http.ResponseWriter, r *http.Request) {
	var request agentDefinitionRequest
	if !decodeBody(w, r, &request) {
		return
	}
	body := request.AgentDefinition
	body.Configuration = request.Configuration
	v, err := a.agents.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body)
	respond(w, v, err, http.StatusCreated)
}

func (a agentAPI) update(w http.ResponseWriter, r *http.Request) {
	var request agentUpdateRequest
	if !decodeBody(w, r, &request) {
		return
	}
	body := request.AgentDefinition
	body.Configuration = request.Configuration
	if request.ExecutionProfileID == nil || request.RuntimeProfileID == nil {
		current, err := a.agents.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID"))
		if err != nil {
			respond(w, nil, err, http.StatusOK)
			return
		}
		body.ExecutionProfileID, body.RuntimeProfileID = current.ExecutionProfileID, current.RuntimeProfileID
	}
	if request.ExecutionProfileID != nil {
		body.ExecutionProfileID = *request.ExecutionProfileID
	}
	if request.RuntimeProfileID != nil {
		body.RuntimeProfileID = *request.RuntimeProfileID
	}
	value, err := a.agents.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID"), body)
	respond(w, value, err, http.StatusOK)
}

func (a agentAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.agents.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID")), http.StatusNoContent)
}
