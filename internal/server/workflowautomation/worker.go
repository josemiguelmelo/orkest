// Package workflowautomation coordinates durable task workflow events and execution commands.
package workflowautomation

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type (
	EventProcessor interface {
		ProcessNextEvent(context.Context) (bool, error)
	}
	ExecutionCommandProcessor interface {
		ProcessNextCommand(context.Context) (bool, error)
	}
)

type WorkflowAutomationWorker struct {
	events    EventProcessor
	execution ExecutionCommandProcessor
	logger    *slog.Logger
}

func NewWorkflowAutomationWorker(events EventProcessor, execution ExecutionCommandProcessor, logger *slog.Logger) *WorkflowAutomationWorker {
	return &WorkflowAutomationWorker{events: events, execution: execution, logger: logger}
}

// Run separates durable event processing from delivery so a slow worker launch
// cannot block unrelated workflow events. Each delivery worker claims on demand.
func (w *WorkflowAutomationWorker) Run(ctx context.Context) {
	var work sync.WaitGroup
	work.Go(func() { w.poll(ctx, "event", w.events.ProcessNextEvent) })
	for range 4 {
		work.Go(func() { w.poll(ctx, "execution_command", w.execution.ProcessNextCommand) })
	}
	work.Wait()
}

func (w *WorkflowAutomationWorker) poll(ctx context.Context, operation string, process func(context.Context) (bool, error)) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil {
			found, err := process(ctx)
			if err != nil {
				if ctx.Err() == nil {
					w.logger.Error("workflow processing failed", "operation", operation, "reason", "processing_failed")
				}
				break
			}
			if !found {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOne processes one event and delivers one command. It is also useful for
// deterministic recovery tests and bounded worker shutdown.
func (w *WorkflowAutomationWorker) ProcessOne(ctx context.Context) (bool, error) {
	processed, err := w.events.ProcessNextEvent(ctx)
	if err != nil {
		return false, err
	}
	delivered, err := w.execution.ProcessNextCommand(ctx)
	return processed || delivered, err
}
