package model

import (
	"time"
)

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
	EventQuestionRequested   EventType = "question_requested"
	EventQuestionAnswered    EventType = "question_answered"
	EventQuestionCancelled   EventType = "question_cancelled"
	EventQuestionUncertain   EventType = "question_delivery_uncertain"
	EventPermissionRequested EventType = "permission_requested"
	EventPermissionResponse  EventType = "permission_response"
	EventCancelRequested     EventType = "cancel_requested"
)

type Update struct {
	Kind       string         `json:"kind"`
	Text       string         `json:"text,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	Title      string         `json:"title,omitempty"`
	Status     string         `json:"status,omitempty"`
	ToolKind   string         `json:"toolKind,omitempty"`
	Input      string         `json:"input,omitempty"`
	Output     string         `json:"output,omitempty"`
	Truncated  bool           `json:"truncated,omitempty"`
	Locations  []ToolLocation `json:"locations,omitzero"`
	Content    []ToolContent  `json:"content,omitzero"`
}

type ToolLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type ToolContent struct {
	Kind      string `json:"kind"`
	Path      string `json:"path,omitempty"`
	OldText   string `json:"oldText,omitempty"`
	NewText   string `json:"newText,omitempty"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// AgentActivityEvent is sensitive task activity, persisted before subscribers are notified.
type AgentActivityEvent struct {
	At               time.Time         `json:"at"`
	Sequence         uint64            `json:"sequence"`
	Type             EventType         `json:"type"`
	OrganizationID   string            `json:"organizationId"`
	WorkspaceID      string            `json:"workspaceId"`
	TaskID           string            `json:"taskId"`
	RunID            string            `json:"runId"`
	TaskSessionID    string            `json:"taskSessionId,omitempty"`
	SessionID        string            `json:"sessionId"`
	CommandID        string            `json:"commandId,omitempty"`
	RequestID        string            `json:"requestId,omitempty"`
	Text             string            `json:"text,omitempty"`
	Update           *Update           `json:"update,omitempty"`
	Permission       *Permission       `json:"permission,omitempty"`
	Question         *Question         `json:"question,omitempty"`
	QuestionResponse *QuestionResponse `json:"questionResponse,omitempty"`
	StopReason       string            `json:"stopReason,omitempty"`
	Error            string            `json:"error,omitempty"`
}
