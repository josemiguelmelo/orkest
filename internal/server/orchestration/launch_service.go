package orchestration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type LaunchService struct {
	repo          TaskExecutionRepository
	executor      AgentctlStarter
	client        AgentctlClient
	communication AgentCommunication
	completion    *LaunchCompletionService
}

var ErrTaskRunNotFound = errors.New("task run not found")

func NewLaunchService(repo TaskExecutionRepository, executor AgentctlStarter, client AgentctlClient, communication AgentCommunication, completion *LaunchCompletionService) (*LaunchService, error) {
	if repo == nil || executor == nil || client == nil || communication == nil {
		return nil, errors.New("launch dependencies are required")
	}
	service := &LaunchService{repo: repo, executor: executor, client: client, communication: communication}
	service.completion = completion
	return service, nil
}

// Start persists each lifecycle transition. The communication context should be the
// platform lifecycle context; canceling it disconnects without stopping agentctl.
func (s *LaunchService) Start(ctx context.Context, config orchestrationmodel.RunConfig) (orchestrationmodel.Launch, error) {
	if err := config.Validate(); err != nil {
		return orchestrationmodel.Launch{}, err
	}
	launch := orchestrationmodel.Launch{ExecutionImage: config.ExecutionImage, RunID: config.RunID, TaskSessionID: config.TaskSessionID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Status: orchestrationmodel.LaunchStarting}
	if !config.ReservedRun {
		if err := s.repo.CreateLaunch(ctx, launch); err != nil {
			return orchestrationmodel.Launch{}, fmt.Errorf("create launch record: %w", err)
		}
	}
	fail := func(stage string, cause error) (orchestrationmodel.Launch, error) {
		launch.Status, launch.FailureStage, launch.FailureReason, launch.UpdatedAt = orchestrationmodel.LaunchFailed, stage, sanitizedFailure(stage, cause), time.Now().UTC()
		logAttrs := []any{
			"organization_id", launch.OrganizationID,
			"workspace_id", launch.WorkspaceID,
			"task_id", launch.TaskID,
			"run_id", launch.RunID,
			"task_session_id", launch.TaskSessionID,
			"failure_stage", stage,
			"error_type", fmt.Sprintf("%T", cause),
		}
		var response *AgentctlFailure
		if errors.As(cause, &response) && response.Code == "launch_failed" && response.Message != "" {
			// Agentctl launch errors expose provider-sanitized diagnostics. Preserve
			// the reason in Orkest logs so durable-storage failures can be diagnosed
			// even when the executor container has already exited.
			logAttrs = append(logAttrs, "agentctl_error_code", response.Code, "agentctl_error", response.Message)
		}
		slog.Default().Error("agent launch step failed", logAttrs...)
		if config.TaskSessionID != "" {
			launch.Status = orchestrationmodel.LaunchFailedPendingClose
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		persistErr := s.repo.UpdateLaunch(persistCtx, launch)
		if persistErr != nil {
			return launch, fmt.Errorf("%s failed; persist launch failure: %w", stage, persistErr)
		}
		if s.completion != nil && launch.Status == orchestrationmodel.LaunchFailedPendingClose {
			// Persisted pending cleanup retains task ownership until termination is confirmed.
			_ = s.completion.ClosePending(context.WithoutCancel(ctx), launch)
		}
		return launch, fmt.Errorf("%s failed: %w", stage, cause)
	}
	process, err := s.executor.StartAgentctl(ctx, config)
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
	launch.Status, launch.UpdatedAt = orchestrationmodel.LaunchProcessStarted, time.Now().UTC()
	if err = s.repo.UpdateLaunch(ctx, launch); err != nil {
		return launch, fmt.Errorf("persist process started: %w", err)
	}
	if err = s.client.Configure(ctx, launch.Endpoint, config); err != nil {
		return fail("configure", err)
	}
	if err = s.communication.Start(ctx, launch); err != nil {
		return fail("connect", err)
	}
	if config.RestoreSession {
		err = s.client.RestoreAgent(ctx, launch.Endpoint)
	} else {
		err = s.client.StartAgent(ctx, launch.Endpoint)
	}
	if err != nil {
		return fail("agent_start", err)
	}
	launch.Status, launch.UpdatedAt = orchestrationmodel.LaunchReady, time.Now().UTC()
	if err = s.repo.UpdateLaunch(ctx, launch); err != nil {
		return launch, fmt.Errorf("persist ready launch: %w", err)
	}
	return launch, nil
}

// StartWithPrompt creates or restores the configured session and submits the initial prompt.
func (s *LaunchService) StartWithPrompt(ctx context.Context, config orchestrationmodel.RunConfig, commandID, prompt string) (orchestrationmodel.Launch, error) {
	if strings.TrimSpace(commandID) == "" || strings.TrimSpace(prompt) == "" {
		return orchestrationmodel.Launch{}, errors.New("command ID and prompt are required")
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

func (s *LaunchService) TaskLaunches(ctx context.Context, orgID, workspaceID, taskID string) ([]orchestrationmodel.Launch, error) {
	reader := s.repo
	launches, err := reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	for i := range launches {
		if launches[i].Status == orchestrationmodel.LaunchReady {
			status, err := s.client.Status(ctx, launches[i].Endpoint)
			if err == nil && (launches[i].TaskSessionID == "" || matchesExecution(launches[i], status)) {
				launches[i].ExecutionState, launches[i].QueuePaused, launches[i].QueueDepth, launches[i].RecoveryBlocked = status.State, status.QueuePaused, status.QueueDepth, status.RecoveryBlocked
			}
		}
	}
	return launches, nil
}

func (s *LaunchService) TaskEvents(ctx context.Context, orgID, workspaceID, taskID, runID string, after uint64) ([]orchestrationmodel.Event, error) {
	reader := s.repo
	launches, err := reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return nil, err
	}
	if _, found := findLaunch(launches, runID); !found {
		return nil, ErrTaskRunNotFound
	}

	return reader.ListTaskLaunchEvents(ctx, runID, after)
}

func (s *LaunchService) SendTaskPrompt(ctx context.Context, orgID, workspaceID, taskID, runID, commandID, prompt string) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	if launch.Status != orchestrationmodel.LaunchReady {
		return errors.New("task run is not active")
	}
	return s.deliverPrompt(ctx, launch, commandID, prompt)
}

func (s *LaunchService) CancelTaskPrompt(ctx context.Context, orgID, workspaceID, taskID, runID, commandID string) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	client := s.client
	return client.CancelPrompt(ctx, launch.Endpoint, commandID)
}

func (s *LaunchService) RespondTaskPermission(ctx context.Context, orgID, workspaceID, taskID, runID string, response orchestrationmodel.PermissionResponse) error {
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
		if event.Payload.Type == orchestrationmodel.EventPermissionRequested && event.Payload.RequestID == response.RequestID && event.Payload.SessionID == response.SessionID {
			if event.Payload.Permission != nil {
				for _, option := range event.Payload.Permission.Options {
					if option.ID == response.OptionID {
						valid = true
					}
				}
			}
		}
		if event.Payload.Type == orchestrationmodel.EventPermissionResponse && event.Payload.RequestID == response.RequestID {
			valid = false
		}
	}
	if !valid {
		return errors.New("permission request is no longer pending")
	}
	client := s.client
	return client.RespondPermission(ctx, launch.Endpoint, response)
}

func (s *LaunchService) taskLaunch(ctx context.Context, orgID, workspaceID, taskID, runID string) (orchestrationmodel.Launch, error) {
	reader := s.repo
	launches, err := reader.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return orchestrationmodel.Launch{}, err
	}
	for _, launch := range launches {
		if launch.RunID == runID {
			if launch.TaskSessionID != "" {
				status, err := s.client.Status(ctx, launch.Endpoint)
				if err != nil {
					return orchestrationmodel.Launch{}, err
				}
				if !matchesExecution(launch, status) {
					return orchestrationmodel.Launch{}, errors.New("agent execution identity mismatch")
				}
			}
			return launch, nil
		}
	}
	return orchestrationmodel.Launch{}, errors.New("task run not found")
}

func validEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func sanitizedFailure(stage string, cause error) string {
	reason := "launch step failed: " + stage
	var response *AgentctlFailure
	if errors.As(cause, &response) && response.Code == "launch_failed" && response.Message != "" {
		message := strings.Join(strings.Fields(response.Message), " ")
		if len(message) > 500 {
			message = message[:500]
		}
		reason += ": " + message
	}
	return reason
}

func (s *LaunchService) deliverPrompt(ctx context.Context, launch orchestrationmodel.Launch, commandID, prompt string) error {
	outbox := s.repo
	delivered, err := outbox.RecordPrompt(ctx, launch.RunID, commandID, prompt)
	if err != nil {
		return err
	}
	if delivered {
		return nil
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
	return outbox.PromptDelivered(ctx, launch.RunID, commandID)
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
			launch.Status = orchestrationmodel.LaunchFailedPendingClose
			if err := s.repo.SetLaunchStatus(ctx, PlanLaunchStatus(launch.RunID, launch.Status)); err != nil {
				return err
			}
			return s.completion.ClosePending(ctx, launch)
		}
	}
	client := s.client
	return client.Queue(ctx, launch.Endpoint, discard)
}

func matchesExecution(launch orchestrationmodel.Launch, status orchestrationmodel.RunStatus) bool {
	return status.OrganizationID == launch.OrganizationID && status.WorkspaceID == launch.WorkspaceID && status.TaskID == launch.TaskID && status.RunID == launch.RunID && status.TaskSessionID == launch.TaskSessionID
}

func findLaunch(launches []orchestrationmodel.Launch, runID string) (orchestrationmodel.Launch, bool) {
	for _, launch := range launches {
		if launch.RunID == runID {
			return launch, true
		}
	}
	return orchestrationmodel.Launch{}, false
}

func (s *LaunchService) RespondTaskQuestion(ctx context.Context, orgID, workspaceID, taskID, runID string, response orchestrationmodel.QuestionResponse) error {
	launch, err := s.taskLaunch(ctx, orgID, workspaceID, taskID, runID)
	if err != nil {
		return err
	}
	if response.TaskID != taskID || response.RunID != runID || response.TaskSessionID != launch.TaskSessionID {
		return errors.New("question scope does not match the run")
	}
	return s.client.RespondQuestion(ctx, launch.Endpoint, response)
}
