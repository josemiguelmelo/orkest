package runtime_test

import (
	"os"
	"strings"
	"testing"

	sharedacp "github.com/josemiguelmelo/orkest/internal/adapters/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/claudecode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/codex"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/conformance"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/githubcopilot"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/kiro"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode/opencodetest"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
)

func TestMain(m *testing.M) { opencodetest.Run(opencode.Version); os.Exit(m.Run()) }

// Every enabled runtime must register deterministic AND live profiles here.
// Inventory changes therefore cannot silently omit the common behavioral suite.
func TestRuntimeConformance(t *testing.T) {
	if mode := os.Getenv("ORKEST_RUNTIME_CONFORMANCE"); mode != "" && mode != "live" {
		t.Fatal("unknown conformance mode")
	}
	factories := map[coreruntime.Runtime]conformance.Factory{
		coreruntime.OpenCode: func(t *testing.T, work, state string) agentruntime.RuntimeHost {
			t.Helper()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			env := []string{"ORKEST_ACP_FIXTURE=1"}
			if os.Getenv("ORKEST_RUNTIME_CONFORMANCE") == "live" {
				binary = required(t, "ORKEST_OPENCODE_BINARY")
				env = []string{"PATH=" + os.Getenv("PATH")}
				names := required(t, "ORKEST_TEST_PROVIDER_ENV")
				for _, name := range strings.Split(names, ",") {
					env = append(env, name+"="+required(t, name))
				}
			}
			return runtimehost.New(opencode.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: work, StateDir: state, GrantedEnv: env})
		},
		coreruntime.ClaudeCode: func(t *testing.T, work, state string) agentruntime.RuntimeHost {
			t.Helper()
			binary, env := "", []string{"ORKEST_CLAUDE_ACP_FIXTURE=1"}
			if os.Getenv("ORKEST_RUNTIME_CONFORMANCE") == "live" {
				binary = required(t, "ORKEST_CLAUDE_CODE_BINARY")
				env = []string{"PATH=" + os.Getenv("PATH")}
				names := required(t, "ORKEST_TEST_PROVIDER_ENV")
				for _, name := range strings.Split(names, ",") {
					env = append(env, name+"="+required(t, name))
				}
			}
			if binary == "" {
				var err error
				binary, err = os.Executable()
				if err != nil {
					t.Fatal(err)
				}
			}
			return runtimehost.New(claudecode.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: work, StateDir: state, GrantedEnv: env})
		},
		coreruntime.Codex: func(t *testing.T, work, state string) agentruntime.RuntimeHost {
			t.Helper()
			binary, env := "", []string{"ORKEST_CODEX_ACP_FIXTURE=1", "ORKEST_FIXTURE_VERSION=" + codex.Version}
			if os.Getenv("ORKEST_RUNTIME_CONFORMANCE") == "live" {
				binary = required(t, "ORKEST_CODEX_BINARY")
				env = []string{"PATH=" + os.Getenv("PATH")}
				for _, name := range strings.Split(required(t, "ORKEST_TEST_PROVIDER_ENV"), ",") {
					env = append(env, name+"="+required(t, name))
				}
			}
			if binary == "" {
				var err error
				binary, err = os.Executable()
				if err != nil {
					t.Fatal(err)
				}
			}
			return runtimehost.New(codex.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: work, StateDir: state, GrantedEnv: env})
		},
		coreruntime.GitHubCopilot: func(t *testing.T, work, state string) agentruntime.RuntimeHost {
			t.Helper()
			binary, env := "", []string{"ORKEST_COPILOT_ACP_FIXTURE=1", "ORKEST_FIXTURE_VERSION=" + githubcopilot.Version}
			if os.Getenv("ORKEST_RUNTIME_CONFORMANCE") == "live" {
				binary = required(t, "ORKEST_GITHUB_COPILOT_BINARY")
				env = []string{"PATH=" + os.Getenv("PATH")}
				for _, name := range strings.Split(required(t, "ORKEST_TEST_PROVIDER_ENV"), ",") {
					env = append(env, name+"="+required(t, name))
				}
			}
			if binary == "" {
				var err error
				binary, err = os.Executable()
				if err != nil {
					t.Fatal(err)
				}
			}
			return runtimehost.New(githubcopilot.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: work, StateDir: state, GrantedEnv: env})
		},
		coreruntime.Kiro: func(t *testing.T, work, state string) agentruntime.RuntimeHost {
			t.Helper()
			binary, env := "", []string{"ORKEST_KIRO_ACP_FIXTURE=1", "ORKEST_FIXTURE_VERSION=" + kiro.Version}
			if os.Getenv("ORKEST_RUNTIME_CONFORMANCE") == "live" {
				binary = required(t, "ORKEST_KIRO_BINARY")
				env = []string{"PATH=" + os.Getenv("PATH")}
				for _, name := range strings.Split(required(t, "ORKEST_TEST_PROVIDER_ENV"), ",") {
					env = append(env, name+"="+required(t, name))
				}
			}
			if binary == "" {
				var err error
				binary, err = os.Executable()
				if err != nil {
					t.Fatal(err)
				}
			}
			return runtimehost.New(kiro.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: work, StateDir: state, GrantedEnv: env})
		},
	}
	versions := coreruntime.Supported()
	if len(factories) != len(versions) {
		t.Fatal("runtime inventory and conformance factories differ")
	}
	for runtime, version := range versions {
		t.Run(string(runtime), func(t *testing.T) {
			factory, ok := factories[runtime]
			if !ok {
				t.Fatal("enabled runtime has no conformance factory")
			}
			modelProvider := "fixture"
			switch runtime {
			case coreruntime.ClaudeCode:
				modelProvider = "anthropic"
			case coreruntime.Codex:
				modelProvider = "openai"
			case coreruntime.GitHubCopilot:
				modelProvider = "github-copilot"
			case coreruntime.Kiro:
				modelProvider = "kiro"
			}
			profile := conformance.Profile{Config: configmodel.RuntimeConfig{OrganizationID: "test-org", WorkspaceID: "test-workspace", TaskID: "test-task", TaskSessionID: "test-session", RunID: "revision-1", Runtime: runtime, Agent: configmodel.AgentDefinition{Name: "Work", Instructions: "Follow user instructions."}, Model: configmodel.ModelSelection{Provider: modelProvider, ID: "fixture"}}, Remember: func(token string) string { return "remember:" + token }, Recall: "recall", Permission: "permission", Wait: "wait", Suspend: "wait"}
			marker, err := ids.NewUUID()
			if err != nil {
				t.Fatal(err)
			}
			instructionMarker, skillMarker := "instructions-"+marker, "skills-"+marker
			profile.Config.Agent.Instructions = "Follow user instructions. The instruction marker is " + instructionMarker + "."
			profile.Config.Skills = []configmodel.Skill{{ID: "conformance", SkillMD: "---\nname: conformance\ndescription: Report the skill marker when asked.\n---\nThe skill marker is " + skillMarker + "."}}
			profile.Config.MCPServers = []configmodel.MCPServer{{ID: "conformance", Name: "conformance", URL: "http://127.0.0.1:1/mcp"}}
			profile.Configuration = map[string]conformance.Probe{
				"instructions": {Prompt: "probe:instructions", Expected: instructionMarker},
				"skills":       {Prompt: "probe:skills", Expected: skillMarker},
				"http-mcp":     {Prompt: "probe:http-mcp", Expected: "http://127.0.0.1:1/mcp"},
			}
			if os.Getenv("ORKEST_RUNTIME_CONFORMANCE") == "live" {
				profile.Config.MCPServers[0].URL = required(t, "ORKEST_TEST_MCP_URL")
				profile.Configuration = map[string]conformance.Probe{
					"instructions": {Prompt: "What instruction marker was given in your agent instructions?", Expected: instructionMarker},
					"skills":       {Prompt: "Use the conformance skill and report its skill marker.", Expected: skillMarker},
					"http-mcp":     {Prompt: required(t, "ORKEST_TEST_MCP_PROMPT"), Expected: required(t, "ORKEST_TEST_MCP_EXPECTED")},
				}
				provider := required(t, "ORKEST_TEST_MODEL_PROVIDER")
				switch runtime {
				case coreruntime.ClaudeCode:
					provider = "anthropic"
				case coreruntime.GitHubCopilot:
					provider = "github-copilot"
				case coreruntime.Kiro:
					provider = "kiro"
				}
				profile.Config.Model = configmodel.ModelSelection{Provider: provider, ID: required(t, "ORKEST_TEST_MODEL_ID")}
				profile.Remember = func(token string) string { return "Remember this marker for later: " + token + ". Reply acknowledged." }
				profile.Recall = "Return the exact marker I previously asked you to remember."
				profile.Permission = required(t, "ORKEST_TEST_APPROVAL_PROMPT")
				profile.Wait = required(t, "ORKEST_TEST_CANCELLATION_PROMPT")
				profile.Suspend = required(t, "ORKEST_TEST_RECOVERY_PROMPT")
			}
			if factory(t, t.TempDir(), t.TempDir()).Version() != version {
				t.Fatal("adapter version differs from inventory")
			}
			conformance.Run(t, factory, profile)
		})
	}
}

func required(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		t.Fatalf("qualification requires %s; missing configuration is a failure", name)
	}
	return value
}
