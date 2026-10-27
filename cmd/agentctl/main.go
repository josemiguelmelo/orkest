package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/josemiguelmelo/orkest/internal/agentctl/daemon"
	sharedacp "github.com/josemiguelmelo/orkest/internal/agentctl/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
	"github.com/josemiguelmelo/orkest/internal/core/environment"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := environment.LoadEnvironment(".env"); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}

	address := flag.String("listen", environment.EnvironmentDefault("AGENTCTL_LISTEN", "127.0.0.1:8081"), "agentctl HTTP listen address")
	binary := flag.String("opencode-binary", environment.EnvironmentDefault("AGENTCTL_OPENCODE_BINARY", "opencode"), "pinned OpenCode executable")
	workDir := flag.String("work-dir", environment.EnvironmentDefault("AGENTCTL_WORK_DIR", ""), "absolute task working directory")
	stateDir := flag.String("state-dir", environment.EnvironmentDefault("AGENTCTL_STATE_DIR", ""), "absolute durable task state directory")

	runID := flag.String("run-id", environment.EnvironmentDefault("AGENTCTL_RUN_ID", ""), "recover only this run from shared task storage")

	var env []string
	flag.Func("env", "environment variable name granted to the provider (repeatable)", func(name string) error {
		return appendGrantedEnvironmentVariable(&env, name)
	})

	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	host := runtimehost.New(opencode.Profile{Binary: *binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: *workDir, StateDir: *stateDir, GrantedEnv: env})
	if err := daemon.RunAgentctl(ctx, logger, *address, host, *stateDir, *runID); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}
}
