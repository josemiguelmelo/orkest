package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// commandController proves the REST transport can be tested without execution services.
type commandController struct{ err error }

func (c commandController) Status() agentrun.RunStatus                             { return agentrun.RunStatus{} }
func (c commandController) Configure(runconfig.RunConfig) error                    { return c.err }
func (c commandController) Launch() error                                          { return c.err }
func (c commandController) Prompt(string, string) error                            { return c.err }
func (c commandController) CancelPrompt(string) error                              { return c.err }
func (c commandController) RespondPermission(interaction.PermissionResponse) error { return c.err }

func TestServiceErrorsThroughREST(t *testing.T) {
	for _, tc := range []struct {
		name, path, code, field string
		err                     error
		status                  int
	}{
		{"validation", "/agent/configure", "invalid_configuration", "model.id", &runconfig.ValidationError{Field: "model.id"}, 400},
		{"binding", "/agent/configure", "configuration_conflict", "", agentrun.ErrConfigurationConflict, 409},
		{"runtime", "/agent/configure", "provider_unavailable", "", agentruntime.ErrRuntimeHostUnavailable, 503},
		{"not configured", "/agent/launch", "not_configured", "", agentrun.ErrNotConfigured, 409},
		{"launch conflict", "/agent/launch", "launch_conflict", "", agentrun.ErrLaunchConflict, 409},
		{"startup secret", "/agent/launch", "launch_failed", "", errors.New("SECRET"), 502},
		{"busy", "/agent/prompt", "conversation_busy", "", interaction.ErrConversationBusy, 409},
		{"interrupted", "/agent/prompt", "conversation_interrupted", "", interaction.ErrConversationInterrupted, 409},
		{"unavailable", "/agent/prompt", "conversation_unavailable", "", interaction.ErrConversation, 409},
		{"storage secret", "/agent/prompt", "storage_unavailable", "", fmt.Errorf("SECRET: %w", agentrun.ErrJournal), 503},
		{"unexpected secret", "/agent/prompt", "internal_error", "", errors.New("SECRET"), 500},
		{"cancellation", "/agent/cancel", "cancellation_failed", "", interaction.ErrCancellation, 502},
		{"permission", "/agent/permissions/respond", "invalid_permission", "", interaction.ErrPermission, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, err := NewServer(context.Background(), commandController{err: tc.err})
			if err != nil {
				t.Fatal(err)
			}
			response := request(server, http.MethodPost, tc.path, `{}`)
			var body protocol.Error
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || body.Code != tc.code || body.Field != tc.field || body.Message == "" {
				t.Fatalf("unexpected error: status=%d body=%+v", response.Code, body)
			}
			if strings.Contains(response.Body.String(), "SECRET") {
				t.Fatal("error leaked internal details")
			}
		})
	}
}

func (c commandController) RestoreSession() error { return c.err }
func (c commandController) CommandStatus(string) (interaction.CommandStatus, error) {
	return interaction.CommandStatus{}, c.err
}
func (c commandController) ResumeQueue() error { return c.err }

func (c commandController) DiscardQueue() error { return c.err }

func (c commandController) Subscribe(uint64) ([]interaction.Event, <-chan interaction.Event, func(), error) {
	return nil, nil, nil, interaction.ErrConversation
}
