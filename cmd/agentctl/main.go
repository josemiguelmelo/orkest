package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/josemiguelmelo/orkest/internal/agentruntime/opencode"
	"github.com/josemiguelmelo/orkest/internal/app"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := loadEnvironment(".env"); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}
	address := flag.String("listen", environmentDefault("AGENTCTL_LISTEN", "127.0.0.1:8081"), "agentctl HTTP listen address")
	binary := flag.String("opencode-binary", environmentDefault("AGENTCTL_OPENCODE_BINARY", "opencode"), "pinned OpenCode executable")
	workDir := flag.String("work-dir", environmentDefault("AGENTCTL_WORK_DIR", ""), "absolute task working directory")
	stateDir := flag.String("state-dir", environmentDefault("AGENTCTL_STATE_DIR", ""), "absolute durable task state directory")
	var env []string
	flag.Func("env", "environment variable name granted to the provider (repeatable)", func(name string) error {
		if strings.TrimSpace(name) == "" || strings.Contains(name, "=") {
			return fmt.Errorf("invalid environment variable name")
		}
		value, ok := os.LookupEnv(name)
		if !ok {
			return fmt.Errorf("environment variable %s is unset", name)
		}
		env = append(env, name+"="+value)
		return nil
	})
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.RunAgentctl(ctx, logger, *address, &opencode.Provider{Binary: *binary, WorkDir: *workDir, StateDir: *stateDir, Env: env}, *stateDir); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}
}
