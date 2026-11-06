package runtimehost

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func sessionStorageError(config configmodel.RuntimeConfig, store sessionStorage, restoring bool, err error) error {
	operation := "load_session"
	if !restoring {
		operation = "reserve_session_creation"
	}
	logAttrs := []any{
		"organization_id", config.OrganizationID,
		"workspace_id", config.WorkspaceID,
		"task_id", config.TaskID,
		"run_id", config.RunID,
		"task_session_id", config.TaskSessionID,
		"operation", operation,
		"restoring", restoring,
		"storage_root", store.Root(config),
		"error", err,
	}
	var compatibilityError *sessionManifestCompatibilityError
	if errors.As(err, &compatibilityError) {
		logAttrs = append(logAttrs, "compatibility_mismatches", compatibilityError.Mismatches)
		if slices.Contains(compatibilityError.Mismatches, "configuration_mismatch") {
			if compatibilityError.Legacy {
				logAttrs = append(logAttrs, "configuration_mismatch_detail", "legacy_manifest_has_no_field_fingerprints")
			} else {
				logAttrs = append(logAttrs, "configuration_mismatch_fields", compatibilityError.Fields)
			}
		}
	}
	slog.Default().Error("runtime durable session storage operation failed", logAttrs...)
	message := "could not reserve durable session storage; check storage availability and permissions"
	switch {
	case errors.Is(err, errSessionAlreadyExists):
		message = "saved session already exists; continue that session or explicitly start a new session"
	case errors.Is(err, os.ErrExist):
		message = "session creation was already reserved by a previous attempt; reconcile it or explicitly start a new session"
	default:
		message += ": " + err.Error()
	}
	return &agentruntime.LaunchError{Message: message, Cause: err}
}

func (h *Host) registerSessionSecrets(session *Session, config configmodel.RuntimeConfig, prepared preparedRuntime) {
	for _, entry := range h.environment.GrantedEnv {
		name, value, ok := strings.Cut(entry, "=")
		name = strings.ToLower(name)
		if ok && (strings.Contains(name, "key") || strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "password")) {
			session.callbacks.AddSecretValues(value)
		}
	}
	for _, value := range config.Environment {
		if value != "" {
			session.callbacks.AddSecretValues(value)
		}
	}
	session.callbacks.AddSecretValues(prepared.SecretValues...)
	for _, credential := range config.GitCredentials {
		if credential.Token != "" {
			session.callbacks.AddSecretValues(credential.Token, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+credential.Token)))
		}
	}
	for _, server := range config.MCPServers {
		for _, value := range server.Headers {
			session.callbacks.AddSecretValues(value)
		}
	}
}
