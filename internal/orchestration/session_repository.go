package orchestration

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

// SessionRepository owns task session selection and atomic run reservation.
type SessionRepository interface {
	CurrentSession(context.Context, string, string, string) (agentctl.RunConfig, bool, error)
	ReserveSessionRun(context.Context, agentctl.RunConfig) error
}

type PromptDelivery struct{ CommandID, Text string }

type PromptOutbox interface {
	RecordPrompt(context.Context, string, string, string) (bool, error)
	PromptDelivered(context.Context, string, string) error
	PendingPrompts(context.Context, string) ([]PromptDelivery, error)
}
