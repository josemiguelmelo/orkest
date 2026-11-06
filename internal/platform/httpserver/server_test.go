package httpserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerListenFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), logger, "test", &http.Server{Addr: listener.Addr().String()}); err == nil {
		t.Fatal("occupied address accepted")
	}
}

func TestHTTPServerDrainsActiveRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	// The log captures the ephemeral address after the listener has been bound.
	addresses := make(chan string, 1)
	logger := slog.New(&addressHandler{addresses: addresses})
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, logger, "test", &http.Server{Addr: "127.0.0.1:0", Handler: handler, ReadHeaderTimeout: time.Second})
	}()
	var address string
	select {
	case address = <-addresses:
	case <-time.After(time.Second):
		t.Fatal("server did not listen")
	}
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get("http://" + address)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				err = fmt.Errorf("status: %d", response.StatusCode)
			}
		}
		requestDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("shutdown returned before request finished: %v", err)
	default:
	}
	release <- struct{}{}
	for _, result := range []<-chan error{requestDone, done} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown did not finish")
		}
	}
}

type addressHandler struct{ addresses chan string }

func (*addressHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *addressHandler) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "address" {
			h.addresses <- attr.Value.String()
		}
		return true
	})
	return nil
}
func (h *addressHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *addressHandler) WithGroup(string) slog.Handler      { return h }
