package runtimehost

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
)

func TestOwnershipSurvivesParentExit(t *testing.T) {
	switch os.Getenv("ORKEST_LEASE_HELPER") {
	case "child":
		time.Sleep(20 * time.Second)
		return
	case "parent":
		store := sessionStorage{StateDir: os.Getenv("ORKEST_LEASE_DIR")}
		owner, err := store.acquire(configmodel.RuntimeConfig{})
		if err != nil {
			os.Exit(2)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestOwnershipSurvivesParentExit$")
		cmd.Env = append(os.Environ(), "ORKEST_LEASE_HELPER=child")
		if _, err = startChildProcess(cmd, io.NopCloser(strings.NewReader("")), owner); err != nil {
			os.Exit(3)
		}
		if err = os.WriteFile(filepath.Join(store.StateDir, "pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	parent := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOwnershipSurvivesParentExit$")
	parent.Env = append(os.Environ(), "ORKEST_LEASE_HELPER=parent", "ORKEST_LEASE_DIR="+dir)
	if err := parent.Run(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	owner, err := (sessionStorage{StateDir: dir}).acquire(configmodel.RuntimeConfig{})
	if err == nil {
		_ = owner.Close()
		t.Fatal("parent exit released a live child's ownership lease")
	}
}
