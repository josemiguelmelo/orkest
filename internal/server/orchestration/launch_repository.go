package orchestration

import (
	"context"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// LaunchRepository persists launch lifecycle state and ordered agent activity.
type LaunchRepository interface {
	CreateLaunch(context.Context, orchestrationmodel.Launch) error
	UpdateLaunch(context.Context, orchestrationmodel.Launch) error
	ListReconnectableLaunches(context.Context) ([]orchestrationmodel.Launch, error)
	SaveEvent(context.Context, orchestrationmodel.Event) error
	SaveTerminalEvent(context.Context, orchestrationmodel.Event, orchestrationmodel.LaunchStatus) error
	SetLaunchStatus(context.Context, LaunchStatusEffects) error
}

type TaskLaunchReader interface {
	ListTaskLaunches(context.Context, string, string, string) ([]orchestrationmodel.Launch, error)
	ListTaskLaunchEvents(context.Context, string, uint64) ([]orchestrationmodel.Event, error)
}

// TaskExecutionRepository explicitly supplies the durable features consumed by task starts.
type TaskExecutionRepository interface {
	LaunchRepository
	TaskLaunchReader
	SessionRepository
	PromptOutbox
	TaskCommand(context.Context, string, string, string, string, string) (bool, error)
}

type ConnectionRepository interface {
	LaunchRepository
	PromptOutbox
}
