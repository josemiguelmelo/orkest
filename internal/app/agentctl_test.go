package app

import (
	"context"
	"log/slog"
	"testing"
)

type cancelOnListen struct {
	cancel context.CancelFunc
}

func (w cancelOnListen) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestRunAgentctlShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(cancelOnListen{cancel: cancel}, nil))
	if err := RunAgentctl(ctx, logger, "127.0.0.1:0", nil, ""); err != nil {
		t.Fatalf("shutdown on cancellation: %v", err)
	}
}
