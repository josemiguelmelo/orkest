package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

func (d *TaskDispatcher) StartDirectChat(ctx context.Context, userID, organizationID, workspaceID, agentID, conversationID, commandID, prompt string) error {
	sessions, ok := d.sessions.(directSessionRepository)
	if !ok {
		return errors.New("direct session storage unavailable")
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate direct chat run ID: %w", err)
	}
	config, continuing, err := sessions.CurrentDirectSession(ctx, organizationID, workspaceID, conversationID)
	if err != nil {
		return err
	}
	if continuing {
		err = d.refreshSessionMCPServers(ctx, workspaceID, &config)
	} else {
		config, err = d.createDirectSession(ctx, userID, organizationID, workspaceID, agentID, conversationID)
	}
	if err != nil {
		return err
	}
	if d.environmentResolver != nil {
		config.Environment, err = d.environmentResolver.Resolve(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("resolve workspace environment: %w", err)
		}
	}
	config.RunID, config.RestoreSession = runID, continuing
	if err := d.refreshOrkestMCPServer(userID, organizationID, workspaceID, conversationID, &config); err != nil {
		return err
	}
	config.InitialCommandID, config.InitialPrompt = commandID, prompt
	if err := config.Validate(); err != nil {
		return err
	}
	return d.launchDirectSession(ctx, sessions, config, continuing && strings.TrimSpace(prompt) != "")
}

func (d *TaskDispatcher) createDirectSession(ctx context.Context, userID, organizationID, workspaceID, agentID, conversationID string) (orchestrationmodel.RunConfig, error) {
	value := taskmodel.Task{ID: conversationID, WorkspaceID: workspaceID, AssigneeAgentID: agentID}
	config, definition, err := d.freshConfiguration(ctx, userID, organizationID, value)
	if err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	config.ExecutionProfileID = definition.ExecutionProfileID
	if err := d.resolveHostMounts(ctx, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	if strings.TrimSpace(definition.SystemPrompt) == "" && strings.TrimSpace(definition.Description) == "" {
		config.Agent.Instructions = "Assist the user with their requests."
	}
	config.TaskSessionID, err = ids.NewUUID()
	if err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	if err := d.resolveAgentCapabilities(ctx, organizationID, workspaceID, definition, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	if err := d.resolveExecutionImage(ctx, workspaceID, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	return config, nil
}

func (d *TaskDispatcher) launchDirectSession(ctx context.Context, sessions directSessionRepository, config orchestrationmodel.RunConfig, withPrompt bool) error {
	if withPrompt && strings.TrimSpace(config.InitialCommandID) == "" {
		commandID, err := ids.NewUUID()
		if err != nil {
			return err
		}
		config.InitialCommandID = commandID
	}
	if err := sessions.ReserveDirectSessionRun(ctx, config); err != nil {
		return err
	}
	config.ReservedRun = true
	if withPrompt {
		_, err := d.launches.StartWithPrompt(ctx, config, config.InitialCommandID, config.InitialPrompt)
		return err
	}
	_, err := d.launches.Start(ctx, config)
	return err
}
