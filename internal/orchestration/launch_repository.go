package orchestration

import "context"

// LaunchRepository persists launch lifecycle state and ordered agent activity.
type LaunchRepository interface {
	CreateLaunch(context.Context, Launch) error
	UpdateLaunch(context.Context, Launch) error
	SaveEvent(context.Context, Event) error
}
