package runtimehost_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sharedacp "github.com/josemiguelmelo/orkest/internal/adapters/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

type questionToolProfile struct{ opencode.Profile }

func (p questionToolProfile) Descriptor() agentruntime.RuntimeDescriptor {
	descriptor := p.Profile.Descriptor()
	descriptor.UseQuestionTool = true
	return descriptor
}

func TestQuestionToolSessionRestoreAndFollowUp(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "with tool", true: "legacy"}[legacy], func(t *testing.T) {
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
			profile := opencode.Profile{Binary: binary}
			var initial agentruntime.RuntimeProfile = questionToolProfile{profile}
			if legacy {
				initial = profile
			}
			host := runtimehost.New(initial, sharedacp.Factory{}, environment)
			config := configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "first", Runtime: "opencode", Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}, Agent: configmodel.AgentDefinition{Name: "Work", Instructions: "Work"}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			first, err := host.Launch(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := first.Prompt(ctx, interaction.Command{ID: "first", Text: "remember:retained"}, agentruntime.PromptObserver{}); err != nil {
				t.Fatal(err)
			}
			_ = first.Close()
			before, err := filepath.Glob(filepath.Join(environment.StateDir, "*", "sessions", "*", "question-tool.json"))
			if err != nil {
				t.Fatal(err)
			}
			if legacy && len(before) != 0 {
				t.Fatal("legacy session unexpectedly received a tool")
			}
			var saved []byte
			if !legacy {
				if len(before) != 1 {
					t.Fatal("tool state missing")
				}
				saved, err = os.ReadFile(before[0])
				if err != nil {
					t.Fatal(err)
				}
			}
			host = runtimehost.New(questionToolProfile{profile}, sharedacp.Factory{}, environment)
			config.RunID = "second"
			restored, err := host.Restore(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = restored.Close() }()
			var text string
			if _, err := restored.Prompt(ctx, interaction.Command{ID: "follow-up", Text: "recall"}, agentruntime.PromptObserver{Update: func(update interaction.Update) error { text += update.Text; return nil }}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "retained") || restored.SessionID() != first.SessionID() {
				t.Fatal("restored session could not continue")
			}
			after, err := filepath.Glob(filepath.Join(environment.StateDir, "*", "sessions", "*", "question-tool.json"))
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				if len(after) != 0 {
					t.Fatal("restoration silently added a question tool")
				}
				return
			}
			current, err := os.ReadFile(after[0])
			if err != nil || string(current) != string(saved) {
				t.Fatal("restoration changed the endpoint or credential")
			}
			var state struct {
				Token string `json:"token"`
			}
			if json.Unmarshal(saved, &state) != nil {
				t.Fatal("invalid saved tool state")
			}
			text = ""
			if _, err := restored.Prompt(ctx, interaction.Command{ID: "probe", Text: "probe:http-mcp"}, agentruntime.PromptObserver{Update: func(update interaction.Update) error { text += update.Text; return nil }}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "orkest-questions") || strings.Contains(text, state.Token) {
				t.Fatal("private question tool was missing or leaked credentials")
			}
		})
	}
}
