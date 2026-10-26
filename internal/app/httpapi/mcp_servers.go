package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/mcpserver"
)

type mcpServerAPI struct{ service *mcpserver.MCPServerService }

func (a mcpServerAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/mcp-servers", func(r chi.Router) {
		r.Get("/", a.list)
		r.Post("/", a.create)
		r.Route("/{serverID}", func(r chi.Router) { r.Put("/", a.update); r.Delete("/", a.delete) })
	})
}

func (a mcpServerAPI) list(w http.ResponseWriter, r *http.Request) {
	v, err := a.service.List(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, err, http.StatusOK)
}

func (a mcpServerAPI) create(w http.ResponseWriter, r *http.Request) {
	var body mcpserver.Input
	if !decodeBody(w, r, &body) {
		return
	}
	v, err := a.service.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), body)
	respond(w, v, err, http.StatusCreated)
}

func (a mcpServerAPI) update(w http.ResponseWriter, r *http.Request) {
	var body mcpserver.Input
	if !decodeBody(w, r, &body) {
		return
	}
	v, err := a.service.Update(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "serverID"), body)
	respond(w, v, err, http.StatusOK)
}

func (a mcpServerAPI) delete(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.service.Delete(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "serverID")), http.StatusNoContent)
}
