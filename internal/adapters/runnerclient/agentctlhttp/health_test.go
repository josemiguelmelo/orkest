package agentctlhttp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type failedHealthTransport struct{ cause error }

func (f failedHealthTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.cause }

func TestHealthWaitRetainsCancellationAndLastFailure(t *testing.T) {
	connectionFailure := errors.New("connection refused")
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "deadline"}[deadline], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			client := NewHTTPAgentctlClient(&http.Client{Transport: failedHealthTransport{cause: connectionFailure}})
			client.pollEvery = time.Millisecond
			err := client.WaitHealthy(ctx, "http://agentctl.test")
			if !errors.Is(err, want) || !errors.Is(err, connectionFailure) {
				t.Fatalf("lost context or diagnostic cause: %v", err)
			}
		})
	}
}
