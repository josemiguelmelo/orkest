package orchestration

import (
	"context"
)

// LaunchRepository persists launch lifecycle state and ordered agent activity.
type LaunchRepository interface {
	CreateLaunch(context.Context, Launch) error
	UpdateLaunch(context.Context, Launch) error
	ListReconnectableLaunches(context.Context) ([]Launch, error)
	SaveEvent(context.Context, Event) error
	SaveTerminalEvent(context.Context, Event, LaunchStatus) error
	SetLaunchStatus(context.Context, LaunchStatusEffects) error
}

type TaskLaunchReader interface {
	ListTaskLaunches(context.Context, string, string, string) ([]Launch, error)
	ListTaskLaunchEvents(context.Context, string, uint64) ([]Event, error)
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
