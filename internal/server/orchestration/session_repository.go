package orchestration

import (
	"context"
)

// SessionRepository owns task session selection and atomic run reservation.
type SessionRepository interface {
	CurrentSession(context.Context, string, string, string) (RunConfig, bool, error)
	ReserveSessionRun(context.Context, RunConfig, int64) error
}

type directSessionRepository interface {
	CurrentDirectSession(context.Context, string, string, string) (RunConfig, bool, error)
	ReserveDirectSessionRun(context.Context, RunConfig) error
}

type PromptDelivery struct{ CommandID, Text string }

type PromptOutbox interface {
	RecordPrompt(context.Context, string, string, string) (bool, error)
	PromptDelivered(context.Context, string, string) error
	PendingPrompts(context.Context, string) ([]PromptDelivery, error)
}
