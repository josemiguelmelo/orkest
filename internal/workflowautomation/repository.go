package workflowautomation

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/task"
)

type TaskTransitionRepository interface {
	WithTaskTransition(context.Context, string, string, string, string, TransitionCommand, func(TransitionSnapshot, TransitionCommand) (TransitionEffects, error)) (task.Task, error)
}

type AutomationEventRepository interface {
	WithAutomationEvent(context.Context, func(AutomationEventSnapshot) (AutomationEventEffects, error)) (bool, error)
}
