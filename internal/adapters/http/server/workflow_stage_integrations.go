package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	stageintegration "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
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
	var input stageintegration.ConnectionInput
	if !decodeBody(w, r, &input) {
		return
	}
	value, err := a.service.Create(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), input)
	respond(w, value, err, http.StatusCreated)
}

func (a workflowStageIntegrationAPI) update(w http.ResponseWriter, r *http.Request) {
	var input stageintegration.ConnectionInput
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
	value, err := a.service.Status(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "connectionID"))
	respond(w, value, err, http.StatusOK)
}

func (a workflowStageIntegrationAPI) retry(w http.ResponseWriter, r *http.Request) {
	respond(w, nil, a.service.Retry(r.Context(), currentUserID(r), chi.URLParam(r, "orgID"), chi.URLParam(r, "workspaceID"), chi.URLParam(r, "eventID")), http.StatusNoContent)
}

type callbackIngestor interface {
	Ingest(context.Context, string, stageintegration.CallbackRequest) (stageintegration.IngestionReceipt, error)
}

type workflowStageCallbackAPI struct{ ingestor callbackIngestor }

// callback forwards original bytes and headers. Authentication, delivery identity
// and event decoding belong entirely to the selected integration adapter.
func (a workflowStageCallbackAPI) callback(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, stageintegration.MaxCallbackBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "callback_too_large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	receipt, err := a.ingestor.Ingest(r.Context(), chi.URLParam(r, "connectionID"), stageintegration.CallbackRequest{Method: r.Method, Headers: map[string][]string(r.Header.Clone()), Body: body})
	if err != nil {
		switch {
		case errors.Is(err, stageintegration.ErrUnauthenticated):
			writeError(w, http.StatusUnauthorized, "invalid_callback_authentication")
		case errors.Is(err, modelerrors.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found")
		case errors.Is(err, modelerrors.ErrInvalid):
			writeError(w, http.StatusBadRequest, "invalid_request")
		case errors.Is(err, modelerrors.ErrConflict):
			writeError(w, http.StatusConflict, "conflict")
		default:
			writeError(w, http.StatusServiceUnavailable, "callback_unavailable")
		}
		return
	}
	status := http.StatusAccepted
	if receipt.Duplicate || receipt.Ignored {
		status = http.StatusOK
	}
	respond(w, receipt, nil, status)
}
