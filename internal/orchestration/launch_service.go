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
	completion    *LaunchCompletionService
}

var ErrTaskRunNotFound = errors.New("task run not found")

func NewLaunchService(repo LaunchRepository, launcher AgentLauncher, client AgentctlClient, communication AgentCommunication, completions ...*LaunchCompletionService) (*LaunchService, error) {
	if repo == nil || launcher == nil || client == nil || communication == nil {
		return nil, errors.New("launch dependencies are required")
	}
	service := &LaunchService{repo: repo, launcher: launcher, client: client, communication: communication}
	if len(completions) > 0 {
		service.completion = completions[0]
	}
	return service, nil
}

// Start persists each lifecycle transition. The communication context should be the
// platform lifecycle context; canceling it disconnects without stopping agentctl.
func (s *LaunchService) Start(ctx context.Context, config agentctl.RunConfig) (Launch, error) {
	if err := config.Validate(); err != nil {
		return Launch{}, err
	}
	launch := Launch{RunID: config.RunID, TaskSessionID: config.TaskSessionID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Status: LaunchStarting}
	if !config.ReservedRun {
		if err := s.repo.CreateLaunch(ctx, launch); err != nil {
			return Launch{}, fmt.Errorf("create launch record: %w", err)
		}
	}
	fail := func(stage string, cause error) (Launch, error) {
		launch.Status, launch.FailureStage, launch.FailureReason, launch.UpdatedAt = LaunchFailed, stage, sanitizedFailure(stage), time.Now().UTC()
		if config.TaskSessionID != "" {
			launch.Status = LaunchFailedPendingClose
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		persistErr := s.repo.UpdateLaunch(persistCtx, launch)
		if persistErr != nil {
			return launch, fmt.Errorf("%s failed; persist launch failure: %w", stage, persistErr)
		}
		if s.completion != nil && launch.Status == LaunchFailedPendingClose {
			// Persisted pending cleanup retains task ownership until termination is confirmed.
			_ = s.completion.ClosePending(context.WithoutCancel(ctx), launch)
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
	if config.RestoreSession {
		restorer, ok := s.client.(interface {
			RestoreAgent(context.Context, string) error
		})
		if !ok {
			return fail("agent_restore", errors.New("agent restoration unavailable"))
		}
		err = restorer.RestoreAgent(ctx, launch.Endpoint)
	} else {
		err = s.client.StartAgent(ctx, launch.Endpoint)
	}
	if err != nil {
		return fail("agent_start", err)
	}
	launch.Status, launch.UpdatedAt = LaunchReady, time.Now().UTC()
	if err = s.repo.UpdateLaunch(ctx, launch); err != nil {
		return launch, fmt.Errorf("persist ready launch: %w", err)
	}
	return launch, nil
}

// StartWithPrompt creates or restores the configured session and submits the initial prompt.
func (s *LaunchService) StartWithPrompt(ctx context.Context, config agentctl.RunConfig, commandID, prompt string) (Launch, error) {
	if strings.TrimSpace(commandID) == "" || strings.TrimSpace(prompt) == "" {
		return Launch{}, errors.New("command ID and prompt are required")
	}
	launch, err := s.Start(ctx, config)
	if err != nil {
		return launch, err
	}
	if err = s.deliverPrompt(ctx, launch, commandID, prompt); err != nil {
		return launch, fmt.Errorf("submit initial agent prompt: %w", err)
	}
	return launch, nil
}

func (s *LaunchService) TaskLaunches(ctx context.Context, orgID, workspaceID, taskID string) ([]Launch, error) {
	reader, ok := s.repo.(TaskLaunchReader)
	if !ok {
		return nil, errors.New("task launch history is unavailable")
	}
	launches, err := reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for i := range launches {
		if launches[i].Status == LaunchReady {
			status, err := s.client.Status(ctx, launches[i].Endpoint)
			if err == nil && (launches[i].TaskSessionID == "" || matchesExecution(launches[i], status)) {
				launches[i].ExecutionState, launches[i].QueuePaused, launches[i].QueueDepth, launches[i].RecoveryBlocked = status.State, status.QueuePaused, status.QueueDepth, status.RecoveryBlocked
			}
		}
	}
	return launches, nil
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
		return nil, ErrTaskRunNotFound
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
	return s.deliverPrompt(ctx, launch, commandID, prompt)
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
			if launch.TaskSessionID != "" {
				status, err := s.client.Status(ctx, launch.Endpoint)
				if err != nil {
					return Launch{}, err
				}
				if !matchesExecution(launch, status) {
					return Launch{}, errors.New("agent execution identity mismatch")
				}
			}
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

func (s *LaunchService) deliverPrompt(ctx context.Context, launch Launch, commandID, prompt string) error {
	outbox, durable := s.repo.(PromptOutbox)
	if durable {
		delivered, err := outbox.RecordPrompt(ctx, launch.RunID, commandID, prompt)
		if err != nil {
			return err
		}
		if delivered {
			return nil
		}
	}
	if launch.TaskSessionID != "" {
		status, err := s.client.Status(ctx, launch.Endpoint)
		if err != nil {
			return err
		}
		if !matchesExecution(launch, status) {
			return errors.New("agent execution identity mismatch")
		}
	}
	if err := s.client.Prompt(ctx, launch.Endpoint, commandID, prompt); err != nil {
		return err
	}
	if durable {
		return outbox.PromptDelivered(ctx, launch.RunID, commandID)
	}
	return nil
}

func (s *LaunchService) TaskQueue(ctx context.Context, orgID, workspaceID, taskID, runID string, discard bool) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	if discard && s.completion != nil {
		status, statusErr := s.client.Status(ctx, launch.Endpoint)
		if statusErr == nil && status.RecoveryBlocked {
			// Explicit discard may abandon failed restoration, but ownership is released
			// only after the executor confirms the entire environment has stopped.
			launch.Status = LaunchFailedPendingClose
			if err := s.repo.SetLaunchStatus(ctx, launch.RunID, launch.Status); err != nil {
				return err
			}
			return s.completion.ClosePending(ctx, launch)
		}
	}
	client, ok := s.client.(interface {
		Queue(context.Context, string, bool) error
	})
	if !ok {
		return errors.New("queue control unavailable")
	}
	return client.Queue(ctx, launch.Endpoint, discard)
}

func matchesExecution(launch Launch, status agentctl.Status) bool {
	return status.OrganizationID == launch.OrganizationID && status.WorkspaceID == launch.WorkspaceID && status.TaskID == launch.TaskID && status.RunID == launch.RunID && status.TaskSessionID == launch.TaskSessionID
}
