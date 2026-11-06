package orchestration

import (
	"context"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// SessionRepository owns task session selection and atomic run reservation.
type SessionRepository interface {
	CurrentSession(context.Context, string, string, string) (orchestrationmodel.RunConfig, bool, error)
	ReserveSessionRun(context.Context, orchestrationmodel.RunConfig, int64) error
}

type directSessionRepository interface {
	CurrentDirectSession(context.Context, string, string, string) (orchestrationmodel.RunConfig, bool, error)
	ReserveDirectSessionRun(context.Context, orchestrationmodel.RunConfig) error
}

type PromptDelivery struct{ CommandID, Text string }

type PromptOutbox interface {
	RecordPrompt(context.Context, string, string, string) (bool, error)
	PromptDelivered(context.Context, string, string) error
	PendingPrompts(context.Context, string) ([]PromptDelivery, error)
}
