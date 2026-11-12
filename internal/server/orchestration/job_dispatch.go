package orchestration

import (
	"context"
	"encoding/json"
	"fmt"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

type JobSessionStore interface {
	ReserveJobSessionRun(context.Context, jobmodel.Run, orchestrationmodel.RunConfig) error
	TaskCommand(context.Context, string, string, string, string, string) (bool, error)
	MarkJobTimeout(context.Context, string) error
}

// JobDispatcher uses occurrence identities, never workflow task/conversation rows.
type JobDispatcher struct {
	configuration *TaskDispatcher
	sessions      JobSessionStore
	completion    *LaunchCompletionService
}

func NewJobDispatcher(configuration *TaskDispatcher, sessions JobSessionStore, completion *LaunchCompletionService) *JobDispatcher {
	return &JobDispatcher{configuration: configuration, sessions: sessions, completion: completion}
}

func (d *JobDispatcher) JobAccepted(ctx context.Context, run jobmodel.Run) (bool, error) {
	return d.sessions.TaskCommand(ctx, run.OrganizationID, run.WorkspaceID, run.ID, run.CommandID, "")
}

func (d *JobDispatcher) StartJob(ctx context.Context, run jobmodel.Run) error {
	h := d.configuration
	config, agent, err := h.freshAgentConfiguration(ctx, run.ActorID, run.OrganizationID, run.WorkspaceID, run.ID, run.Input.AgentID)
	if err != nil {
		return err
	}
	config.RunID, config.TaskSessionID = run.RunID, run.ID+":session"
	config.ExecutionProfileID = agent.ExecutionProfileID
	if err = h.resolveHostMounts(ctx, &config); err != nil {
		return err
	}
	if err = h.resolveExecutionImage(ctx, run.WorkspaceID, &config); err != nil {
		return err
	}
	if err = h.resolveAgentCapabilities(ctx, run.OrganizationID, run.WorkspaceID, agent, &config); err != nil {
		return err
	}
	if h.environmentResolver != nil {
		config.Environment, err = h.environmentResolver.Resolve(ctx, run.WorkspaceID)
		if err != nil {
			return err
		}
	}
	if config.Environment == nil {
		config.Environment = map[string]string{}
	}
	for k, v := range agent.Configuration {
		config.Environment[k] = v
	}
	repos, err := h.repositories.List(ctx, run.ActorID, run.OrganizationID, run.WorkspaceID)
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, id := range run.RepositoryIDs {
		selected[id] = true
	}
	for _, r := range repos {
		if selected[r.ID] {
			config.Repositories = append(config.Repositories, orchestrationmodel.WorkspaceRepository{ID: r.ID, ProviderName: r.ProviderName, Name: r.Name, URL: r.URL, IntegrationID: r.IntegrationID, ProviderRepositoryID: r.ProviderRepositoryID, DefaultBranch: r.DefaultBranch})
			delete(selected, r.ID)
		}
	}
	if len(selected) > 0 {
		return modelerrors.ErrNotFound
	}
	if len(config.Repositories) > 0 {
		if err = h.resolveWorkspaceGitCredentials(ctx, run.ActorID, run.OrganizationID, run.WorkspaceID, &config); err != nil {
			return err
		}
	}
	resources := make([]taskPromptRepository, 0, len(config.Repositories))
	for _, r := range config.Repositories {
		resources = append(resources, taskPromptRepository{ID: r.ID, Name: r.Name, ProviderName: r.ProviderName, URL: r.URL, Path: "repositories/" + r.ProviderName + "/" + r.ID})
	}
	encoded, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	config.InitialCommandID = run.CommandID
	config.InitialPrompt = fmt.Sprintf("Scheduled job: %s\nOccurrence: %s\nScheduled for: %s\n\n%s\n\nRepositories (fixed invocation scope): %s\nOperate only within these resources and the configured tool account permissions. For changes use an occurrence-specific branch. Reconcile an existing external effect before repeating an uncertain operation. Finish with per-resource outcomes, exceptions and report/MR links. Results are agent-reported, not independently verified.", run.Input.Name, run.ID, run.ScheduledFor.UTC().Format("2006-01-02T15:04:05Z"), run.Input.Instructions, encoded)
	// Task-specific built-in tools are absent. Agent-selected external MCP servers
	// keep their configured accounts/scopes; no mailbox connector is fabricated.
	if err = config.Validate(); err != nil {
		return err
	}
	if err = d.sessions.ReserveJobSessionRun(ctx, run, config); err != nil {
		return err
	}
	config.ReservedRun = true
	_, err = h.launches.StartWithPrompt(ctx, config, config.InitialCommandID, config.InitialPrompt)
	return err
}

func (d *JobDispatcher) StopJob(ctx context.Context, run jobmodel.Run) error {
	launches, err := d.configuration.launches.repo.ListTaskLaunches(ctx, run.OrganizationID, run.WorkspaceID, run.ID)
	if err != nil {
		return err
	}
	launch, found := findLaunch(launches, run.RunID)
	if !found {
		return modelerrors.ErrNotFound
	}
	// A startup callback may still create a process. Do not claim termination before
	// launch has resolved; the durable cancellation flag is retried by the worker.
	if launch.Status == orchestrationmodel.LaunchStarting || launch.Status == orchestrationmodel.LaunchProcessStarted {
		return nil
	}
	if launch.Status == orchestrationmodel.LaunchCompleted || launch.Status == orchestrationmodel.LaunchFailed {
		return nil
	}
	if !run.CancelRequested {
		if err = d.sessions.MarkJobTimeout(ctx, run.ID); err != nil {
			return err
		}
	}
	launch.Status = orchestrationmodel.LaunchFailedPendingClose
	if err = d.configuration.launches.repo.SetLaunchStatus(ctx, PlanLaunchStatus(run.RunID, launch.Status)); err != nil {
		return err
	}
	return d.completion.ClosePending(ctx, launch)
}
