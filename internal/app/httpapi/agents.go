package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agent"
)

type agentAPI struct{ agents *agent.Service }

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
	var body agent.AgentDefinition
	if !decodeBody(w, r, &body) {
		return
	}
	v, err := a.agents.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body)
	respond(w, v, err, http.StatusCreated)
}

func (a agentAPI) update(w http.ResponseWriter, r *http.Request) {
	var request struct {
		agent.AgentDefinition
		ExecutionProfileID *string `json:"executionProfileId"`
		RuntimeProfileID   *string `json:"runtimeProfileId"`
	}
	if !decodeBody(w, r, &request) {
		return
	}
	body := request.AgentDefinition
	if request.ExecutionProfileID != nil {
		body.ExecutionProfileID = *request.ExecutionProfileID
	} else {
		current, err := a.agents.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID"))
		if err != nil {
			respond(w, nil, err, http.StatusOK)
			return
		}
		body.ExecutionProfileID = current.ExecutionProfileID
	}
	if request.RuntimeProfileID != nil {
		body.RuntimeProfileID = *request.RuntimeProfileID
	} else {
		current, err := a.agents.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID"))
		if err != nil {
			respond(w, nil, err, http.StatusOK)
			return
		}
		body.RuntimeProfileID = current.RuntimeProfileID
	}
	v, err := a.agents.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID"), body)
	respond(w, v, err, http.StatusOK)
}

func (a agentAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.agents.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "agentID")), http.StatusNoContent)
}
