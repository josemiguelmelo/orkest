package workflowautomation

import (
	"context"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type TaskTransitionRepository interface {
	WithTaskTransition(context.Context, string, string, string, string, TransitionCommand, func(TransitionSnapshot, TransitionCommand) (TransitionEffects, error)) (taskmodel.Task, error)
}

type AutomationEventRepository interface {
	WithAutomationEvent(context.Context, func(AutomationEventSnapshot) (AutomationEventEffects, error)) (bool, error)
}
