package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	stageintegration "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

type callbackIngestor interface {
	Ingest(context.Context, string, stageintegrationmodel.CallbackRequest) (stageintegrationmodel.IngestionReceipt, error)
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
	receipt, err := a.ingestor.Ingest(r.Context(), chi.URLParam(r, "connectionID"), stageintegrationmodel.CallbackRequest{Method: r.Method, Headers: map[string][]string(r.Header.Clone()), Body: body})
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
