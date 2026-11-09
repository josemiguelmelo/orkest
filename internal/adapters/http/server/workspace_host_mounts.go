package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workspacemount"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type workspaceHostMountsAPI struct {
	service *workspacemount.WorkspaceMountService
}

func (a workspaceHostMountsAPI) routes(r chi.Router) {
	r.Route("/organizations/{orgID}/workspaces/{workspaceID}/host-mounts", func(r chi.Router) {
		r.Get("/", a.get)
		r.Put("/", a.put)
	})
}

func (a workspaceHostMountsAPI) get(w http.ResponseWriter, r *http.Request) {
	settings, err := a.service.Get(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, settings, err, http.StatusOK)
}

func (a workspaceHostMountsAPI) put(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Mounts []struct {
			Source   string `json:"source"`
			Target   string `json:"target"`
			ReadOnly *bool  `json:"readOnly"`
		} `json:"mounts"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if input.Mounts == nil {
		respond(w, nil, modelerrors.ErrInvalid, http.StatusOK)
		return
	}
	mounts := make([]mountmodel.Mount, 0, len(input.Mounts))
	for _, m := range input.Mounts {
		if m.ReadOnly == nil {
			respond(w, nil, modelerrors.ErrInvalid, http.StatusOK)
			return
		}
		mounts = append(mounts, mountmodel.Mount{Source: m.Source, Target: m.Target, ReadOnly: *m.ReadOnly})
	}
	err := a.service.Put(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), mounts)
	respond(w, map[string]string{"status": "updated"}, err, http.StatusOK)
}
