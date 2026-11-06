package workflowautomation_test

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

type blockedDelivery struct {
	entered chan struct{}
	active  atomic.Int32
}

func (b *blockedDelivery) ProcessNextCommand(ctx context.Context) (bool, error) {
	b.active.Add(1)
	defer b.active.Add(-1)
	b.entered <- struct{}{}
	<-ctx.Done()
	return false, ctx.Err()
}

type countedEvents struct{ processed chan struct{} }

func (e countedEvents) ProcessNextEvent(context.Context) (bool, error) {
	select {
	case e.processed <- struct{}{}:
	default:
	}
	return false, nil
}

// Previously ProcessOne coupled events to a delivery taking arbitrarily long.
// Occupying every delivery worker must still permit another event polling cycle.
func TestWorkflowEventsProgressWhileAllDeliveriesBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	delivery := &blockedDelivery{entered: make(chan struct{}, 4)}
	events := countedEvents{processed: make(chan struct{}, 4)}
	worker := workflowautomation.NewWorkflowAutomationWorker(events, delivery, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	for range 4 {
		select {
		case <-delivery.entered:
		case <-time.After(3 * time.Second):
			t.Fatal("delivery workers did not start")
		}
	}
	<-events.processed
	select {
	case <-events.processed:
	case <-time.After(3 * time.Second):
		t.Fatal("slow delivery blocked events")
	}
	cancel()
	<-done
	if delivery.active.Load() != 0 {
		t.Fatal("delivery workers not joined")
	}
}
