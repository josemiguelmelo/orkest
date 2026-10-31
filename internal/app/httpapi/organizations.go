package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/core/httplog"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/organization"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type organizationAPI struct {
	organizations *organization.OrganizationService
	workspaces    *workspace.WorkspaceService
}

func (a organizationAPI) routes(r chi.Router) {
	r.Get("/organizations", a.listOrganizations)
	r.Post("/organizations", a.createOrganization)
	r.Route("/organizations/{orgID}", func(r chi.Router) {
		r.Get("/", a.getOrganization)
		r.Patch("/", a.updateOrganization)
		r.Delete("/", a.deleteOrganization)
		r.Get("/members", a.listOrganizationMembers)
		r.Put("/members/{userID}", a.setOrganizationMember)
		r.Delete("/members/{userID}", a.removeOrganizationMember)
		r.Get("/workspaces", a.listWorkspaces)
		r.Post("/workspaces", a.createWorkspace)
		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Get("/", a.getWorkspace)
			r.Patch("/", a.updateWorkspace)
			r.Delete("/", a.deleteWorkspace)
			r.Get("/members", a.listWorkspaceMembers)
			r.Put("/members/{userID}", a.setWorkspaceMember)
			r.Delete("/members/{userID}", a.removeWorkspaceMember)
		})
	})
}

type resourceNameRequest struct {
	Name string `json:"name"`
}
type roleRequest struct {
	Role string `json:"role"`
}

func (a organizationAPI) listOrganizations(w http.ResponseWriter, r *http.Request) {
	v, e := a.organizations.ListOrganizations(r.Context(), currentUserID(r))
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) getOrganization(w http.ResponseWriter, r *http.Request) {
	v, e := a.organizations.GetOrganization(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"))
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) createOrganization(w http.ResponseWriter, r *http.Request) {
	var b resourceNameRequest
	if !decodeBody(w, r, &b) {
		return
	}
	v, e := a.organizations.CreateOrganization(r.Context(), currentUserID(r), b.Name)
	respond(w, v, e, http.StatusCreated)
}

func (a organizationAPI) updateOrganization(w http.ResponseWriter, r *http.Request) {
	var b resourceNameRequest
	if !decodeBody(w, r, &b) {
		return
	}
	v, e := a.organizations.UpdateOrganization(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), b.Name)
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.organizations.DeleteOrganization(r.Context(), currentUserID(r), chi.URLParam(r, "orgID")), http.StatusNoContent)
}

func (a organizationAPI) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	v, e := a.organizations.ListOrganizationMembers(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"))
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) setOrganizationMember(w http.ResponseWriter, r *http.Request) {
	var b roleRequest
	if !decodeBody(w, r, &b) {
		return
	}
	e := a.organizations.SetOrganizationMember(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "userID"), b.Role)
	respond(w, map[string]string{"status": "updated"}, e, http.StatusOK)
}

func (a organizationAPI) removeOrganizationMember(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.organizations.RemoveOrganizationMember(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "userID")), http.StatusNoContent)
}

func (a organizationAPI) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	v, e := a.workspaces.ListWorkspaces(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"))
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) getWorkspace(w http.ResponseWriter, r *http.Request) {
	v, e := a.workspaces.GetWorkspace(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var b resourceNameRequest
	if !decodeBody(w, r, &b) {
		return
	}
	v, e := a.workspaces.CreateWorkspace(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), b.Name)
	respond(w, v, e, http.StatusCreated)
}

func (a organizationAPI) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var b resourceNameRequest
	if !decodeBody(w, r, &b) {
		return
	}
	v, e := a.workspaces.UpdateWorkspace(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), b.Name)
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.workspaces.DeleteWorkspace(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID")), http.StatusNoContent)
}

func (a organizationAPI) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	v, e := a.workspaces.ListWorkspaceMembers(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"))
	respond(w, v, e, http.StatusOK)
}

func (a organizationAPI) setWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	var b roleRequest
	if !decodeBody(w, r, &b) {
		return
	}
	e := a.workspaces.SetWorkspaceMember(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "userID"), b.Role)
	respond(w, map[string]string{"status": "updated"}, e, http.StatusOK)
}

func (a organizationAPI) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.workspaces.RemoveWorkspaceMember(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "userID")), http.StatusNoContent)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func respond(w http.ResponseWriter, value any, err error, success int) {
	if err != nil {
		status, code := http.StatusInternalServerError, "internal_error"
		var dispatchError *task.DispatchError
		var integrationValidationError *taskintegration.ValidationError
		var integrationConfigurationError *taskintegration.ConfigurationError
		var codingProviderError *coderepository.ProviderError
		switch {
		case errors.Is(err, executionprofile.ErrNotConfigured):
			status, code = http.StatusServiceUnavailable, "execution_images_not_configured"
		case errors.Is(err, secretbox.ErrKeyUnavailable):
			status, code = http.StatusServiceUnavailable, "integration_encryption_not_configured"
		case errors.As(err, &integrationValidationError):
			status, code = http.StatusBadGateway, integrationValidationError.Code
			if code == "" {
				code = "integration_validation_failed"
			}
		case errors.As(err, &integrationConfigurationError):
			status, code = http.StatusBadRequest, integrationConfigurationError.Code
		case errors.As(err, &codingProviderError):
			status, code = codingProviderError.Status, codingProviderError.Code
			if status == 0 {
				status = http.StatusBadGateway
			}
			if code == "" {
				code = "coding_provider_request_failed"
			}
		case errors.As(err, &dispatchError):
			status, code = http.StatusBadGateway, "agent_dispatch_failed"
		case errors.Is(err, modelerrors.ErrInvalid):
			status, code = http.StatusBadRequest, "invalid_request"
		case errors.Is(err, modelerrors.ErrForbidden):
			status, code = http.StatusForbidden, "forbidden"
		case errors.Is(err, modelerrors.ErrNotFound):
			status, code = http.StatusNotFound, "not_found"
		case errors.Is(err, modelerrors.ErrConflict):
			status, code = http.StatusConflict, "conflict"
		}
		if status >= http.StatusInternalServerError {
			logger := httplog.LoggerFromWriter(w)
			if logger == nil {
				logger = slog.Default()
			}
			logger.Error("http handler failed", "error", err, "error_code", code)
		}
		writeError(w, status, code)
		return
	}
	if success == http.StatusNoContent {
		w.WriteHeader(success)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(success)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
