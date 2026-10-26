package orchestration

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

// LaunchRepository persists launch lifecycle state and ordered agent activity.
type LaunchRepository interface {
	CreateLaunch(context.Context, Launch) error
	UpdateLaunch(context.Context, Launch) error
	ListReconnectableLaunches(context.Context) ([]Launch, error)
	SaveEvent(context.Context, Event) error
	SaveTerminalEvent(context.Context, Event, LaunchStatus) error
	SetLaunchStatus(context.Context, string, LaunchStatus) error
}

type TaskLaunchReader interface {
	ListTaskLaunches(context.Context, string, string, string) ([]Launch, error)
	ListTaskLaunchEvents(context.Context, string, uint64) ([]Event, error)
}

type InteractiveAgentctlClient interface {
	Prompt(context.Context, string, string, string) error
	CancelPrompt(context.Context, string, string) error
	RespondPermission(context.Context, string, agentctl.PermissionResponse) error
}
