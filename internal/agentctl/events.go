package agentctl

// EventType identifies the payload of an activity record. Values are stable wire names.
type EventType string

const (
	EventRuntimeStopped    EventType = "runtime_stopped"
	EventPromptStarted     EventType = "prompt_started"
	EventPromptInterrupted EventType = "prompt_interrupted"
	EventQueuePaused       EventType = "queue_paused"
	EventQueueResumed      EventType = "queue_resumed"
	EventQueueDiscarded    EventType = "queue_discarded"
	EventRunFailed         EventType = "run_failed"
	EventPromptContinued   EventType = "prompt_continued"
	EventRunCompleted      EventType = "run_completed"
	EventSessionRestored   EventType = "session_restored"

	EventPromptAccepted      EventType = "prompt_accepted"
	EventPromptCompleted     EventType = "prompt_completed"
	EventPromptFailed        EventType = "prompt_failed"
	EventUpdate              EventType = "update"
	EventPermissionRequested EventType = "permission_requested"
	EventPermissionResponse  EventType = "permission_response"
	EventCancelRequested     EventType = "cancel_requested"
)

func promptAcceptedEvent(commandID, text string) Event {
	return Event{Type: EventPromptAccepted, CommandID: commandID, Text: text}
}

func promptResultEvent(commandID, reason string, err error) Event {
	if err != nil {
		return Event{Type: EventPromptFailed, CommandID: commandID, Error: "provider prompt failed; outcome may be uncertain"}
	}
	return Event{Type: EventPromptCompleted, CommandID: commandID, StopReason: reason}
}

func updateEvent(commandID string, update Update) Event {
	return Event{Type: EventUpdate, CommandID: commandID, Update: &update}
}

func permissionRequestedEvent(commandID, requestID string, permission Permission) Event {
	return Event{Type: EventPermissionRequested, CommandID: commandID, RequestID: requestID, Permission: &permission}
}

func permissionResponseEvent(commandID string, response PermissionResponse) Event {
	return Event{Type: EventPermissionResponse, CommandID: commandID, RequestID: response.RequestID, Text: response.OptionID}
}

func cancelRequestedEvent(commandID string) Event {
	return Event{Type: EventCancelRequested, CommandID: commandID}
}
