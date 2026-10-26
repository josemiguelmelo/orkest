package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type taskAgentGetter interface {
	Get(context.Context, string, string, string, string) (agent.Agent, error)
}

// TaskDispatcher resolves an assigned agent definition and starts its task run.
type TaskDispatcher struct {
	agents   taskAgentGetter
	launches *LaunchService
}

func NewTaskDispatcher(agents taskAgentGetter, launches *LaunchService) (*TaskDispatcher, error) {
	if agents == nil || launches == nil {
		return nil, errors.New("task dispatch dependencies are required")
	}
	return &TaskDispatcher{agents: agents, launches: launches}, nil
}

func (d *TaskDispatcher) StartTask(ctx context.Context, userID, organizationID string, value task.Task) error {
	if strings.TrimSpace(value.AssigneeAgentID) == "" {
		return errors.New("task has no assigned agent")
	}
	definition, err := d.agents.Get(ctx, userID, organizationID, value.WorkspaceID, value.AssigneeAgentID)
	if err != nil {
		return fmt.Errorf("get assigned agent: %w", err)
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate task run ID: %w", err)
	}
	commandID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate initial prompt ID: %w", err)
	}
	systemPrompt := strings.TrimSpace(definition.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = strings.TrimSpace(definition.Description)
	}
	if systemPrompt == "" {
		systemPrompt = "Complete the provided task."
	}
	config := agentctl.RunConfig{
		OrganizationID: organizationID,
		WorkspaceID:    value.WorkspaceID,
		TaskID:         value.ID,
		RunID:          runID,
		Runtime:        definition.Runtime,
		Model:          agentctl.ModelSelection{Provider: definition.Provider, ID: definition.Model},
		SystemPrompt:   systemPrompt,
	}
	prompt, err := taskPrompt(value)
	if err != nil {
		return err
	}
	if _, err = d.launches.StartWithPrompt(ctx, config, commandID, prompt); err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return nil
}

func taskPrompt(value task.Task) (string, error) {
	snapshot := struct {
		Title       string           `json:"title"`
		Description string           `json:"description"`
		Labels      []string         `json:"labels"`
		Components  []task.Component `json:"components"`
	}{Title: value.Title, Description: value.Description, Labels: value.Labels, Components: value.Components}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode task context: %w", err)
	}
	return "Work on the following Orkest task. Use this task snapshot as the source of truth for the requested work:\n\n" + string(encoded), nil
}
