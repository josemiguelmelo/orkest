package orchestration

import (
	"context"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
)

// SessionRepository owns task session selection and atomic run reservation.
type SessionRepository interface {
	CurrentSession(context.Context, string, string, string) (runconfig.RunConfig, bool, error)
	ReserveSessionRun(context.Context, runconfig.RunConfig) error
}

type PromptDelivery struct{ CommandID, Text string }

type PromptOutbox interface {
	RecordPrompt(context.Context, string, string, string) (bool, error)
	PromptDelivered(context.Context, string, string) error
	PendingPrompts(context.Context, string) ([]PromptDelivery, error)
}
