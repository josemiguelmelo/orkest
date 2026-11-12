package scheduledjob

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

type deliveryFixture struct {
	run                                     jobmodel.Run
	state, reason                           string
	accepted                                bool
	starts                                  int
	startError, acceptError, authorizeError error
}

func (f *deliveryFixture) Materialize(context.Context) (bool, error) { return false, nil }

func (f *deliveryFixture) Claim(context.Context, string) (jobmodel.Run, bool, error) {
	return f.run, true, nil
}

func (f *deliveryFixture) FinishClaim(_ context.Context, _ jobmodel.Run, state, reason string) error {
	f.state, f.reason = state, reason
	return nil
}

func (f *deliveryFixture) Expired(context.Context) ([]jobmodel.Run, error) { return nil, nil }

func (f *deliveryFixture) StartJob(context.Context, jobmodel.Run) error {
	f.starts++
	return f.startError
}

func (f *deliveryFixture) JobAccepted(context.Context, jobmodel.Run) (bool, error) {
	return f.accepted, f.acceptError
}

func (f *deliveryFixture) StopJob(context.Context, jobmodel.Run) error { return nil }

func (f *deliveryFixture) Authorize(context.Context, string, string, string, bool) error {
	return f.authorizeError
}

func TestDeliveryReconcilesBeforeReplacement(t *testing.T) {
	tests := []struct {
		name                         string
		attempt                      int32
		accepted                     bool
		startErr, acceptErr, authErr error
		want                         string
		starts                       int
	}{
		{name: "first delivery", attempt: 1, want: "running", starts: 1},
		{name: "accepted reservation", attempt: 2, accepted: true, want: "running"},
		{name: "expired unaccepted claim", attempt: 2, want: "delivery_uncertain"},
		{name: "ordinary launch failure", attempt: 1, startErr: errors.New("failed"), want: "failed", starts: 1},
		{name: "unavailable acceptance store", attempt: 2, acceptErr: errors.New("offline")},
		{name: "revoked actor", attempt: 1, authErr: errors.New("forbidden"), want: "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &deliveryFixture{run: jobmodel.Run{ClaimAttempt: tt.attempt, CreatedAt: time.Now()}, accepted: tt.accepted, startError: tt.startErr, acceptError: tt.acceptErr, authorizeError: tt.authErr}
			w := NewScheduledJobWorker(f, f, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
			_, err := w.Deliver(context.Background())
			if (tt.acceptErr != nil) != (err != nil) || f.state != tt.want || f.starts != tt.starts {
				t.Fatalf("state=%q starts=%d err=%v", f.state, f.starts, err)
			}
		})
	}
}
