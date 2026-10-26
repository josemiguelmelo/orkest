package orchestration

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type LaunchService struct {
	repo          LaunchRepository
	launcher      AgentLauncher
	client        AgentctlClient
	communication AgentCommunication
}

func NewLaunchService(repo LaunchRepository, launcher AgentLauncher, client AgentctlClient, communication AgentCommunication) (*LaunchService, error) {
	if repo == nil || launcher == nil || client == nil || communication == nil {
		return nil, errors.New("launch dependencies are required")
	}
	return &LaunchService{repo: repo, launcher: launcher, client: client, communication: communication}, nil
}

// Start persists each lifecycle transition. The communication context should be the
// platform lifecycle context; canceling it disconnects without stopping agentctl.
func (s *LaunchService) Start(ctx context.Context, config agentctl.RunConfig) (Launch, error) {
	if err := config.Validate(); err != nil {
		return Launch{}, err
	}
	launch := Launch{RunID: config.RunID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Status: LaunchStarting}
	if err := s.repo.CreateLaunch(ctx, launch); err != nil {
		return Launch{}, fmt.Errorf("create launch record: %w", err)
	}
	fail := func(stage string, cause error) (Launch, error) {
		launch.Status, launch.FailureStage, launch.FailureReason, launch.UpdatedAt = LaunchFailed, stage, sanitizedFailure(stage), time.Now().UTC()
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		persistErr := s.repo.UpdateLaunch(persistCtx, launch)
		if persistErr != nil {
			return launch, fmt.Errorf("%s failed; persist launch failure: %w", stage, persistErr)
		}
		return launch, fmt.Errorf("%s failed: %w", stage, cause)
	}
	process, err := s.launcher.LaunchAgentctl(ctx, config)
	launch.ProcessID, launch.Endpoint = strings.TrimSpace(process.ID), strings.TrimSpace(process.Endpoint)
	if err != nil {
		return fail("process_start", err)
	}
	if launch.ProcessID == "" || !validEndpoint(launch.Endpoint) {
		return fail("process_start", errors.New("executor returned invalid process identity or endpoint"))
	}
	if err = s.client.WaitHealthy(ctx, launch.Endpoint); err != nil {
		return fail("health_check", err)
	}
	launch.Status, launch.UpdatedAt = LaunchProcessStarted, time.Now().UTC()
	if err = s.repo.UpdateLaunch(ctx, launch); err != nil {
		return launch, fmt.Errorf("persist process started: %w", err)
	}
	if err = s.client.Configure(ctx, launch.Endpoint, config); err != nil {
		return fail("configure", err)
	}
	if err = s.communication.Start(launch); err != nil {
		return fail("connect", err)
	}
	if err = s.client.StartAgent(ctx, launch.Endpoint); err != nil {
		return fail("agent_start", err)
	}
	launch.Status, launch.UpdatedAt = LaunchReady, time.Now().UTC()
	if err = s.repo.UpdateLaunch(ctx, launch); err != nil {
		return launch, fmt.Errorf("persist ready launch: %w", err)
	}
	return launch, nil
}

// StartWithPrompt starts a fresh agent session and submits its first prompt.
func (s *LaunchService) StartWithPrompt(ctx context.Context, config agentctl.RunConfig, commandID, prompt string) (Launch, error) {
	if strings.TrimSpace(commandID) == "" || strings.TrimSpace(prompt) == "" {
		return Launch{}, errors.New("command ID and prompt are required")
	}
	launch, err := s.Start(ctx, config)
	if err != nil {
		return launch, err
	}
	if err = s.client.Prompt(ctx, launch.Endpoint, commandID, prompt); err != nil {
		return launch, fmt.Errorf("submit initial agent prompt: %w", err)
	}
	return launch, nil
}

func (s *LaunchService) TaskLaunches(ctx context.Context, orgID, workspaceID, taskID string) ([]Launch, error) {
	reader, ok := s.repo.(TaskLaunchReader)
	if !ok {
		return nil, errors.New("task launch history is unavailable")
	}
	return reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
}

func (s *LaunchService) TaskEvents(ctx context.Context, orgID, workspaceID, taskID, runID string, after uint64) ([]Event, error) {
	reader, ok := s.repo.(TaskLaunchReader)
	if !ok {
		return nil, errors.New("task activity history is unavailable")
	}
	launches, err := reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, launch := range launches {
		if launch.RunID == runID {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("task run not found")
	}
	return reader.ListTaskLaunchEvents(ctx, runID, after)
}

func (s *LaunchService) SendTaskPrompt(ctx context.Context, orgID, workspaceID, taskID, runID, commandID, prompt string) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	if launch.Status != LaunchReady {
		return errors.New("task run is not active")
	}
	client, ok := s.client.(InteractiveAgentctlClient)
	if !ok {
		return errors.New("agent interaction is unavailable")
	}
	return client.Prompt(ctx, launch.Endpoint, commandID, prompt)
}

func (s *LaunchService) CancelTaskPrompt(ctx context.Context, orgID, workspaceID, taskID, runID, commandID string) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	client, ok := s.client.(InteractiveAgentctlClient)
	if !ok {
		return errors.New("agent interaction is unavailable")
	}
	return client.CancelPrompt(ctx, launch.Endpoint, commandID)
}

func (s *LaunchService) RespondTaskPermission(ctx context.Context, orgID, workspaceID, taskID, runID string, response agentctl.PermissionResponse) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	events, err := s.TaskEvents(ctx, orgID, workspaceID, taskID, runID, 0)
	if err != nil {
		return err
	}
	valid := false
	for _, event := range events {
		if event.Payload.Type == agentctl.EventPermissionRequested && event.Payload.RequestID == response.RequestID && event.Payload.SessionID == response.SessionID {
			if event.Payload.Permission != nil {
				for _, option := range event.Payload.Permission.Options {
					if option.ID == response.OptionID {
						valid = true
					}
				}
			}
		}
		if event.Payload.Type == agentctl.EventPermissionResponse && event.Payload.RequestID == response.RequestID {
			valid = false
		}
	}
	if !valid {
		return errors.New("permission request is no longer pending")
	}
	client, ok := s.client.(InteractiveAgentctlClient)
	if !ok {
		return errors.New("agent interaction is unavailable")
	}
	return client.RespondPermission(ctx, launch.Endpoint, response)
}

func (s *LaunchService) taskLaunch(ctx context.Context, orgID, workspaceID, taskID, runID string) (Launch, error) {
	reader, ok := s.repo.(TaskLaunchReader)
	if !ok {
		return Launch{}, errors.New("task launch history is unavailable")
	}
	launches, err := reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return Launch{}, err
	}
	for _, launch := range launches {
		if launch.RunID == runID {
			return launch, nil
		}
	}
	return Launch{}, errors.New("task run not found")
}

func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func sanitizedFailure(stage string) string { return "launch step failed: " + stage }
