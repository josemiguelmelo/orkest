package orchestration

import (
	"errors"
	"fmt"
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

// CommandStatus describes durable acceptance separately from runtime delivery.
type CommandStatus struct {
	CommandID string       `json:"commandId"`
	Status    CommandState `json:"status"`
}

type Command struct {
	ID   string
	Text string
}

// CommandState records the durable lifecycle of an accepted command.
type CommandState string

const (
	CommandQueued      CommandState = "queued"
	CommandRunning     CommandState = "running"
	CommandCompleted   CommandState = "completed"
	CommandFailed      CommandState = "failed"
	CommandInterrupted CommandState = "interrupted"
	CommandDiscarded   CommandState = "discarded"
)
