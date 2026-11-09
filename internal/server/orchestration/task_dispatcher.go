package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	skillmodel "github.com/josemiguelmelo/orkest/internal/server/skill/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	workspaceenvmodel "github.com/josemiguelmelo/orkest/internal/server/workspaceenv/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type taskAgentGetter interface {
	Get(context.Context, string, string, string, string) (agentmodel.Agent, error)
}

// TaskDispatcher resolves an assigned agent definition and starts its task run.
type TaskDispatcher struct {
	sessions            TaskSessionStore
	artifacts           TaskArtifactReader
	workflows           TaskWorkflowReader
	agents              taskAgentGetter
	profiles            ExecutionImageResolver
	launches            *LaunchService
	mcpServers          TaskMCPResolver
	skillResolver       TaskSkillResolver
	environmentResolver TaskEnvironmentResolver
	mounts              TaskMountResolver
	repositories        TaskRepositoryReader
	agentTools          TaskToolResolver
	codingCredentials   TaskCredentialResolver
}

type (
	TaskArtifactReader interface {
		List(context.Context, artifactmodel.Scope) ([]artifactmodel.Revision, error)
	}
	TaskWorkflowReader interface {
		Get(context.Context, string, string, string, string) (workflowmodel.Definition, error)
	}
	ExecutionImageResolver interface {
		Resolve(context.Context, string, string, agentmodel.Runtime) (string, error)
	}
	TaskMCPResolver interface {
		Resolve(context.Context, string, []string) ([]mcpservermodel.RuntimeServer, error)
	}
	TaskSkillResolver interface {
		ResolveForAgent(context.Context, string, string, []string) ([]skillmodel.Skill, error)
	}
	TaskEnvironmentResolver interface {
		Resolve(context.Context, string) (workspaceenvmodel.RuntimeValues, error)
	}
	TaskMountResolver interface {
		Resolve(context.Context, string) ([]mountmodel.Mount, error)
	}
	TaskRepositoryReader interface {
		List(context.Context, string, string, string) ([]coderepositorymodel.CodeRepository, error)
	}
	TaskToolResolver interface {
		RuntimeServerForRun(string, string, string, string, string) (mcpservermodel.RuntimeServer, error)
	}
	TaskCredentialResolver interface {
		CodingCredential(context.Context, string, string) (coderepositorymodel.Credentials, error)
		ListIntegrations(context.Context, string, string, string) ([]coderepositorymodel.CodingIntegration, error)
	}
)

// TaskSessionStore supplies only selection, reservation, and command identity.
type TaskSessionStore interface {
	SessionRepository
	TaskCommand(context.Context, string, string, string, string, string) (bool, error)
}

type TaskDispatcherDependencies struct {
	Agents       taskAgentGetter
	Launches     *LaunchService
	Sessions     TaskSessionStore
	MCPServers   TaskMCPResolver
	Profiles     ExecutionImageResolver
	Skills       TaskSkillResolver
	Environment  TaskEnvironmentResolver
	Mounts       TaskMountResolver
	Repositories TaskRepositoryReader
	Credentials  TaskCredentialResolver
	Tools        TaskToolResolver
	Artifacts    TaskArtifactReader
	Workflows    TaskWorkflowReader
}

func NewTaskDispatcher(deps TaskDispatcherDependencies) (*TaskDispatcher, error) {
	if deps.Agents == nil || deps.Launches == nil || deps.Sessions == nil {
		return nil, errors.New("agents, launches, and sessions are required")
	}
	if deps.Artifacts != nil && deps.Workflows == nil {
		return nil, errors.New("artifact resolution requires workflows")
	}
	return &TaskDispatcher{
		agents:              deps.Agents,
		launches:            deps.Launches,
		sessions:            deps.Sessions,
		mcpServers:          deps.MCPServers,
		profiles:            deps.Profiles,
		skillResolver:       deps.Skills,
		environmentResolver: deps.Environment,
		mounts:              deps.Mounts,
		repositories:        deps.Repositories,
		codingCredentials:   deps.Credentials,
		agentTools:          deps.Tools,
		artifacts:           deps.Artifacts,
		workflows:           deps.Workflows,
	}, nil
}

func (d *TaskDispatcher) StartTask(ctx context.Context, userID, organizationID string, command taskmodel.StartCommand) error {
	value := command.Task
	if strings.TrimSpace(value.AssigneeAgentID) == "" {
		return errors.New("task has no assigned agent")
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate task run ID: %w", err)
	}
	commandID := command.CommandID
	if commandID == "" {
		commandID, err = ids.NewUUID()
		if err != nil {
			return err
		}
	}
	sessions := d.sessions
	var config orchestrationmodel.RunConfig
	var continuing bool
	if command.SessionMode != "new" {
		config, continuing, err = d.continueTaskSession(ctx, userID, organizationID, value, runID)
		if err != nil {
			return err
		}
	}
	if !continuing {
		config, err = d.createTaskSession(ctx, userID, organizationID, value, runID)
		if err != nil {
			return err
		}
	}
	if err = d.refreshOrkestMCPServer(userID, organizationID, value.WorkspaceID, value.ID, &config); err != nil {
		return err
	}
	artifact, err := d.artifactContext(ctx, userID, organizationID, value, &config)
	if err != nil {
		return err
	}
	prompt, err := composeTaskPrompt(value, &config, artifact, command.PromptText)
	if err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || strings.TrimSpace(prompt) == "" || len(prompt) > orchestrationmodel.MaxPromptBytes {
		return orchestrationmodel.ErrCommand
	}
	config.InitialCommandID, config.InitialPrompt = commandID, prompt
	config.DeliveryToken = command.DeliveryToken
	if err := sessions.ReserveSessionRun(ctx, config, value.StageVersion); err != nil {
		return err
	}
	config.ReservedRun = true
	if _, err = d.launches.StartWithPrompt(ctx, config, commandID, prompt); err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return nil
}

func (d *TaskDispatcher) TaskCommandAccepted(ctx context.Context, orgID, workspaceID, taskID, commandID, prompt string) (bool, error) {
	return d.sessions.TaskCommand(ctx, orgID, workspaceID, taskID, commandID, prompt)
}
