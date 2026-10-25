package agentctl

import (
	"errors"
	"fmt"
	"time"
)

const (
	MaxPromptBytes = 256 * 1024
	MaxCommands    = 4096
)

var (
	ErrCommandLimit            = errors.New("conversation command limit reached")
	ErrCancellation            = errors.New("provider cancellation delivery failed")
	ErrConversation            = errors.New("conversation unavailable")
	ErrConversationBusy        = fmt.Errorf("%w: prompt already running", ErrConversation)
	ErrConversationInterrupted = fmt.Errorf("%w: reconciliation required", ErrConversation)
	ErrCommand                 = errors.New("invalid or conflicting command identity")
	ErrPermission              = errors.New("permission identity or option does not match a pending request")
)

// Event is sensitive task activity, persisted before subscribers are notified.
type Event struct {
	At             time.Time   `json:"at"`
	Sequence       uint64      `json:"sequence"`
	Type           string      `json:"type"`
	OrganizationID string      `json:"organizationId"`
	WorkspaceID    string      `json:"workspaceId"`
	TaskID         string      `json:"taskId"`
	RunID          string      `json:"runId"`
	SessionID      string      `json:"sessionId"`
	CommandID      string      `json:"commandId,omitempty"`
	RequestID      string      `json:"requestId,omitempty"`
	Text           string      `json:"text,omitempty"`
	Update         *Update     `json:"update,omitempty"`
	Permission     *Permission `json:"permission,omitempty"`
	StopReason     string      `json:"stopReason,omitempty"`
	Error          string      `json:"error,omitempty"`
}

type PermissionResponse struct {
	TaskID    string `json:"taskId"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
}
