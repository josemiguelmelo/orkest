package runtimehost

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBlockedRuntimeShutdownJoinsChild(t *testing.T) {
	if os.Getenv("ORKEST_BLOCKED_RUNTIME_FIXTURE") == "1" {
		time.Sleep(time.Minute)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestBlockedRuntimeShutdownJoinsChild$")
	cmd.Env = append(os.Environ(), "ORKEST_BLOCKED_RUNTIME_FIXTURE=1")
	// An inherited pipe must not prevent joining a child that ignores stdin EOF.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	child, err := startChildProcess(cmd, io.NopCloser(strings.NewReader("")), nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	var work sync.WaitGroup
	for range 3 {
		work.Go(func() {
			if closeErr := child.Close(); closeErr != nil {
				t.Error(closeErr)
			}
		})
	}
	done := make(chan struct{})
	go func() { work.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("blocked runtime shutdown did not join child")
	}
	select {
	case <-child.Done():
	default:
		t.Fatal("shutdown returned before child exit")
	}
}
