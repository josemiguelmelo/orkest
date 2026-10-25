package rest

import (
	"errors"
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

// writeServiceError exposes known domain errors and sanitizes unexpected failures.
func writeServiceError(w http.ResponseWriter, err error, status int, code, message string) {
	var validation *agentctl.ValidationError
	if errors.As(err, &validation) {
		protocol.WriteValidationError(w, http.StatusBadRequest, "invalid_configuration", &protocol.ValidationError{
			Field:   validation.Field,
			Message: validation.Error(),
		})
		return
	}
	var launchError *agentctl.LaunchError
	if errors.As(err, &launchError) {
		protocol.WriteError(w, http.StatusBadGateway, "launch_failed", launchError.Message)
		return
	}
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{agentctl.ErrConfigurationConflict, http.StatusConflict, "configuration_conflict"},
		{agentctl.ErrNotConfigured, http.StatusConflict, "not_configured"},
		{agentctl.ErrLaunchConflict, http.StatusConflict, "launch_conflict"},
		{agentctl.ErrRuntimeProviderUnavailable, http.StatusServiceUnavailable, "provider_unavailable"},
		{agentctl.ErrCommand, http.StatusBadRequest, "invalid_command"},
		{agentctl.ErrPermission, http.StatusConflict, "invalid_permission"},
		{agentctl.ErrConversationBusy, http.StatusConflict, "conversation_busy"},
		{agentctl.ErrConversationInterrupted, http.StatusConflict, "conversation_interrupted"},
		{agentctl.ErrConversation, http.StatusConflict, "conversation_unavailable"},
		{agentctl.ErrCommandLimit, http.StatusConflict, "command_limit"},
		{agentctl.ErrCancellation, http.StatusBadGateway, "cancellation_failed"},
		{agentctl.ErrJournal, http.StatusServiceUnavailable, "storage_unavailable"},
	} {
		if errors.Is(err, known.err) {
			status, code, message = known.status, known.code, known.err.Error()
			break
		}
	}
	protocol.WriteError(w, status, code, message)
}
