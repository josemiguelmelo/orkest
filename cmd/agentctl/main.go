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

	"github.com/josemiguelmelo/orkest/internal/agentctl/daemon"
	"github.com/josemiguelmelo/orkest/internal/agentctl/gitprovider"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	sharedacp "github.com/josemiguelmelo/orkest/internal/agentctl/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/claudecode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/codex"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/githubcopilot"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/kiro"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
	"github.com/josemiguelmelo/orkest/internal/core/environment"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := environment.LoadEnvironment(".env"); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}

	address := flag.String("listen", environment.EnvironmentDefault("AGENTCTL_LISTEN", "127.0.0.1:8081"), "agentctl HTTP listen address")
	runtimeName := flag.String("runtime", "", "required runtime assigned to this agentctl worker (opencode|claude-code|codex|github-copilot|kiro)")
	workDir := flag.String("work-dir", environment.EnvironmentDefault("AGENTCTL_WORK_DIR", ""), "absolute task working directory")
	stateDir := flag.String("state-dir", environment.EnvironmentDefault("AGENTCTL_STATE_DIR", ""), "absolute durable task state directory")

	runID := flag.String("run-id", environment.EnvironmentDefault("AGENTCTL_RUN_ID", ""), "recover only this run from shared task storage")

	var env []string
	flag.Func("env", "environment variable name granted to the provider (repeatable)", func(name string) error {
		return appendGrantedEnvironmentVariable(&env, name)
	})

	flag.Parse()
	if strings.TrimSpace(*runtimeName) == "" {
		logger.Error("agentctl stopped", "error", "-runtime is required")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hostEnvironment := runtimehost.Environment{WorkDir: *workDir, StateDir: *stateDir, GrantedEnv: env}
	profile, err := runtimeProfile(coreruntime.Runtime(*runtimeName))
	if err != nil {
		logger.Error("agentctl stopped", "error", err, "runtime", *runtimeName)
		os.Exit(2)
	}
	host := runtimehost.New(profile, sharedacp.Factory{}, hostEnvironment, gitprovider.DefaultCLIAuthAdapters()...)
	if err := daemon.RunAgentctl(ctx, logger, *address, host, *stateDir, *runID); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}
}

func runtimeProfile(runtime coreruntime.Runtime) (agentruntime.RuntimeProfile, error) {
	switch runtime {
	case coreruntime.OpenCode:
		return opencode.Profile{}, nil
	case coreruntime.ClaudeCode:
		return claudecode.Profile{}, nil
	case coreruntime.Codex:
		return codex.Profile{}, nil
	case coreruntime.GitHubCopilot:
		return githubcopilot.Profile{}, nil
	case coreruntime.Kiro:
		return kiro.Profile{}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q", runtime)
	}
}
