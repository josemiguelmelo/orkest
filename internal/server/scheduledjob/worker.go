package scheduledjob

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

type JobDeliveryRepository interface {
	Materialize(context.Context) (bool, error)
	Claim(context.Context, string) (jobmodel.Run, bool, error)
	FinishClaim(context.Context, jobmodel.Run, string, string) error
	Expired(context.Context) ([]jobmodel.Run, error)
}
type (
	JobDispatcher interface {
		StartJob(context.Context, jobmodel.Run) error
		JobAccepted(context.Context, jobmodel.Run) (bool, error)
		StopJob(context.Context, jobmodel.Run) error
	}
	JobAuthorizer interface {
		Authorize(context.Context, string, string, string, bool) error
	}
	ScheduledJobWorker struct {
		repository JobDeliveryRepository
		dispatcher JobDispatcher
		access     JobAuthorizer
		logger     *slog.Logger
	}
)

func NewScheduledJobWorker(r JobDeliveryRepository, d JobDispatcher, a JobAuthorizer, l *slog.Logger) *ScheduledJobWorker {
	return &ScheduledJobWorker{repository: r, dispatcher: d, access: a, logger: l}
}

func (w *ScheduledJobWorker) Deliver(ctx context.Context) (bool, error) {
	token, err := ids.NewUUID()
	if err != nil {
		return false, err
	}
	run, found, err := w.repository.Claim(ctx, token)
	if err != nil || !found {
		return found, err
	}
	accepted, err := w.dispatcher.JobAccepted(ctx, run)
	if err != nil {
		return true, err
	}
	state, reason := "running", ""
	if !accepted {
		if run.ClaimAttempt > 1 {
			state, reason = "delivery_uncertain", "acceptance_not_established"
		} else if time.Since(run.CreatedAt) > time.Hour {
			state, reason = "missed", "capacity_wait_expired"
		} else {
			err = w.access.Authorize(ctx, run.ActorID, run.OrganizationID, run.WorkspaceID, true)
			if err == nil {
				err = w.dispatcher.StartJob(ctx, run)
			}
			if ctx.Err() != nil {
				return true, ctx.Err()
			}
			if err != nil {
				accepted, checkErr := w.dispatcher.JobAccepted(ctx, run)
				if checkErr != nil {
					return true, checkErr
				}
				if !accepted {
					state, reason = "failed", "dispatch_failed"
				}
			}
		}
	}
	return true, w.repository.FinishClaim(ctx, run, state, reason)
}

// Calendar materialization and timeout cleanup do not wait behind slow launches.
func (w *ScheduledJobWorker) Run(ctx context.Context) {
	var group sync.WaitGroup
	group.Go(func() { w.poll(ctx, "materialize", 20, w.repository.Materialize) })
	group.Go(func() { w.poll(ctx, "deliver", 4, w.Deliver) })
	group.Go(func() {
		w.poll(ctx, "expiration", 1, func(ctx context.Context) (bool, error) {
			runs, err := w.repository.Expired(ctx)
			if err != nil {
				return false, err
			}
			for _, run := range runs {
				if err = w.dispatcher.StopJob(ctx, run); err != nil {
					return false, err
				}
			}
			return false, nil
		})
	})
	group.Wait()
}

func (w *ScheduledJobWorker) poll(ctx context.Context, operation string, limit int, process func(context.Context) (bool, error)) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		for range limit {
			found, err := process(ctx)
			if err != nil {
				if ctx.Err() == nil {
					w.logger.Error("scheduled job processing failed", "operation", operation, "reason", "processing_unavailable")
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
