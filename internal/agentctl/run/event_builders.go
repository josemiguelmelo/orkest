package run

import (
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func promptAcceptedEvent(commandID, text string) interaction.Event {
	return interaction.Event{Type: interaction.EventPromptAccepted, CommandID: commandID, Text: text}
}

func promptResultEvent(commandID string, reason agentruntime.StopReason, err error) interaction.Event {
	if err != nil {
		return interaction.Event{Type: interaction.EventPromptFailed, CommandID: commandID, Error: "provider prompt failed; outcome may be uncertain"}
	}
	return interaction.Event{Type: interaction.EventPromptCompleted, CommandID: commandID, StopReason: string(reason)}
}

func updateEvent(commandID string, update interaction.Update) interaction.Event {
	return interaction.Event{Type: interaction.EventUpdate, CommandID: commandID, Update: &update}
}

func permissionRequestedEvent(commandID, requestID string, permission interaction.Permission) interaction.Event {
	return interaction.Event{Type: interaction.EventPermissionRequested, CommandID: commandID, RequestID: requestID, Permission: &permission}
}

func permissionResponseEvent(commandID string, response interaction.PermissionResponse) interaction.Event {
	return interaction.Event{Type: interaction.EventPermissionResponse, CommandID: commandID, RequestID: response.RequestID, Text: response.OptionID}
}

func cancelRequestedEvent(commandID string) interaction.Event {
	return interaction.Event{Type: interaction.EventCancelRequested, CommandID: commandID}
}
