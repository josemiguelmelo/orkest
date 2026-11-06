package httpapi

import (
	"errors"
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl/protocol"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// writeServiceError exposes known domain errors and sanitizes unexpected failures.
func writeServiceError(w http.ResponseWriter, err error, status int, code, message string) {
	var validation *configmodel.ValidationError
	if errors.As(err, &validation) {
		protocol.WriteValidationError(w, http.StatusBadRequest, "invalid_configuration", &protocol.ValidationError{
			Field:   validation.Field,
			Message: validation.Error(),
		})
		return
	}
	var launchError *agentruntime.LaunchError
	if errors.As(err, &launchError) {
		protocol.WriteError(w, http.StatusBadGateway, "launch_failed", launchError.Message)
		return
	}
	var runtimeError *agentruntime.RuntimeError
	if errors.As(err, &runtimeError) {
		protocol.WriteError(w, http.StatusBadGateway, "launch_failed", runtimeError.Message)
		return
	}
	for _, known := range []struct {
		err    error
		status int
		code   string
	}{
		{agentrun.ErrRecoveryBlocked, http.StatusConflict, "recovery_blocked"},
		{agentrun.ErrConfigurationConflict, http.StatusConflict, "configuration_conflict"},
		{agentrun.ErrNotConfigured, http.StatusConflict, "not_configured"},
		{agentrun.ErrLaunchConflict, http.StatusConflict, "launch_conflict"},
		{agentruntime.ErrRuntimeHostUnavailable, http.StatusServiceUnavailable, "provider_unavailable"},
		{interaction.ErrCommand, http.StatusBadRequest, "invalid_command"},
		{interaction.ErrPermission, http.StatusConflict, "invalid_permission"},
		{interaction.ErrConversationBusy, http.StatusConflict, "conversation_busy"},
		{interaction.ErrConversationInterrupted, http.StatusConflict, "conversation_interrupted"},
		{interaction.ErrConversation, http.StatusConflict, "conversation_unavailable"},
		{interaction.ErrCommandLimit, http.StatusConflict, "command_limit"},
		{interaction.ErrCancellation, http.StatusBadGateway, "cancellation_failed"},
		{agentrun.ErrJournal, http.StatusServiceUnavailable, "storage_unavailable"},
	} {
		if errors.Is(err, known.err) {
			status, code, message = known.status, known.code, known.err.Error()
			break
		}
	}
	protocol.WriteError(w, status, code, message)
}
