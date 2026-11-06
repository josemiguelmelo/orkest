package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/httplog"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	"github.com/josemiguelmelo/orkest/internal/server/taskintegration"
)

func respond(w http.ResponseWriter, value any, err error, success int) {
	if err != nil {
		respondError(w, err)
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

func respondError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	var dispatchError *task.DispatchError
	var integrationValidationError *taskintegration.ValidationError
	var integrationConfigurationError *taskintegration.ConfigurationError
	var codingProviderError *coderepositorymodel.ProviderError
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
}
