package main

import (
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/claudecode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/codex"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/githubcopilot"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/kiro"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	codexpreparation "github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost/codex"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

func runtimeConfiguration(runtime coreruntime.Runtime) (agentruntime.RuntimeProfile, runtimehost.RuntimePreparationStrategy, error) {
	preparation := runtimehost.NoopRuntimePreparationStrategy{}
	switch runtime {
	case coreruntime.OpenCode:
		return opencode.Profile{}, preparation, nil
	case coreruntime.ClaudeCode:
		return claudecode.Profile{}, preparation, nil
	case coreruntime.Codex:
		return codex.Profile{}, codexpreparation.Preparation{}, nil
	case coreruntime.GitHubCopilot:
		return githubcopilot.Profile{}, preparation, nil
	case coreruntime.Kiro:
		return kiro.Profile{}, preparation, nil
	default:
		return nil, nil, fmt.Errorf("unsupported runtime %q", runtime)
	}
}
