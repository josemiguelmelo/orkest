package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	taskintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/taskintegration/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

func testWorkspaceWorkflow(t *testing.T, db *PostgresDatabase, orgID, actorID, foreignAgent string) {
	t.Helper()
	ctx := context.Background()
	newID := func() string {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	workspaceID := newID()
	workspaces := NewWorkspacePostgresRepository(db)
	if _, err := workspaces.CreateWorkspace(ctx, workspaceID, orgID, "Workflow test"); err != nil {
		t.Fatal(err)
	}
	repo := NewWorkflowDefinitionPostgresRepository(db)
	d, err := repo.Definition(ctx, workspaceID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	agents := NewAgentPostgresRepository(db, nil)
	assigned, err := agents.CreateAgent(ctx, newID(), workspaceID, agentmodel.AgentDefinition{Name: "Builder", Runtime: agentmodel.RuntimeOpenCode, Provider: "test", Model: "test", PermissionPolicy: agentmodel.PermissionPolicyAsk})
	if err != nil {
		t.Fatal(err)
	}
	secondWorkflowID := newID()
	secondDefinition := workflowmodel.DefaultDefinition("Support")
	secondDefinition.ID = secondWorkflowID
	secondDefinition.Stages = []workflowmodel.Stage{
		secondDefinition.Stages[1],
		secondDefinition.Stages[0],
		secondDefinition.Stages[3],
		secondDefinition.Stages[2],
	}
	secondDefinition.Stages[0].ResponsibleAgentID = assigned.ID
	secondDefinition.InitialStageID = "in_progress"
	secondDefinition.ClosedStageID = "in_review"
	secondDefinition.Rules = secondDefinition.Rules[:1]
	secondDefinition.ConfiguredBy = actorID
	if _, err = repo.Create(ctx, workspaceID, secondDefinition, workflow.PlanConfigurationChange); err != nil {
		t.Fatal("create second workflow with shared stage IDs", err)
	}
	if definitions, listErr := repo.Definitions(ctx, workspaceID); listErr != nil || len(definitions) != 2 {
		t.Fatal("workspace workflows were not listed", len(definitions), listErr)
	}
	original := d
	d.Stages[1].ResponsibleAgentID = foreignAgent
	if _, err = repo.Save(ctx, workspaceID, d, workflow.PlanConfigurationChange); !errors.Is(err, modelerrors.ErrInvalid) {
		t.Fatal("foreign workspace agent accepted", err)
	}
	d.Stages[1].ResponsibleAgentID = assigned.ID
	d.Stages = append(d.Stages[:2], append([]workflowmodel.Stage{{ID: "quality", Name: "Quality"}}, d.Stages[2:]...)...)
	for i := range d.Rules {
		if d.Rules[i].Action == workflowmodel.MoveTask {
			d.Rules[i].TargetStageID = "quality"
		}
	}
	d, err = repo.Save(ctx, workspaceID, d, workflow.PlanConfigurationChange)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, workspaceID, original, workflow.PlanConfigurationChange); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("stale workflow write accepted", err)
	}
	tasks := NewTaskPostgresRepository(db)
	worker := newWorkflowTestWorker(tasks)
	taskID := newID()
	value, err := createWorkflowTask(ctx, tasks, taskID, workspaceID, taskmodel.TaskInput{WorkflowID: workspaceID, Title: "Workflow", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	secondTaskID := newID()
	secondTask, err := createWorkflowTask(ctx, tasks, secondTaskID, workspaceID, taskmodel.TaskInput{WorkflowID: secondWorkflowID, Title: "Support task", Status: "todo"})
	if err != nil || secondTask.WorkflowID != secondWorkflowID {
		t.Fatal("task was not associated with its selected workflow", secondTask, err)
	}
	if err = repo.Delete(ctx, workspaceID, secondWorkflowID, workflow.ValidateWorkflowDeletion); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("workflow with an associated task was deleted", err)
	}
	input := workflowautomation.TransitionRequest{Action: "move", StageID: "in_progress", StageVersion: value.StageVersion, CommandID: newID()}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, taskID, input)
			results <- e
		})
	}
	wg.Wait()
	close(results)
	// Repeated command delivery is idempotent, and only one command is queued.
	for e := range results {
		if e != nil {
			t.Fatal("idempotent move failed", e)
		}
	}
	processWorkflowCommands(t, ctx, worker)
	value, err = tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if value.StageVersion != 2 || value.StageWorkState != "running" || value.AssigneeAgentID != assigned.ID {
		t.Fatalf("entry effects missing: %+v", value)
	}
	q := sqlc.New(db.pool)
	var count int
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND options->>'action'='start'", workspaceID, taskID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate start command", count, err)
	}
	d.Stages = append(d.Stages[:1], d.Stages[2:]...)
	kept := []workflowmodel.AutomationRule{}
	for _, rule := range d.Rules {
		if rule.SourceStageID != "in_progress" && rule.TargetStageID != "in_progress" {
			kept = append(kept, rule)
		}
	}
	d.Rules = kept
	if _, err = repo.Save(ctx, workspaceID, d, workflow.PlanConfigurationChange); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("occupied stage removed", err)
	}
	// Use durable launch records without launching an external runtime.
	launches := NewAgentLaunchPostgresRepository(db, nil)
	runID := newID()
	if err = launches.CreateLaunch(ctx, orchestrationmodel.Launch{RunID: runID, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Status: orchestrationmodel.LaunchReady}); err != nil {
		t.Fatal(err)
	}
	if err = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: taskID, WorkflowRunID: pgtype.Text{String: runID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, orchestrationmodel.LaunchCompleted)); err != nil {
		t.Fatal(err)
	}
	processWorkflowCommands(t, ctx, worker)
	value, err = tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil || value.Status != "quality" || value.StageVersion != 3 {
		t.Fatal("successful run did not advance", value, err)
	}
	if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, orchestrationmodel.LaunchCompleted)); err != nil {
		t.Fatal(err)
	}
	processWorkflowCommands(t, ctx, worker)
	again, err := tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil || again.StageVersion != 3 {
		t.Fatal("completion replay advanced task", again, err)
	}
	// A late completion from an earlier run in the same stage cannot finish
	// the newer run. Failures leave stage identity and position unchanged.
	oldRun, newRun := newID(), newID()
	for _, run := range []string{oldRun, newRun} {
		if err = launches.CreateLaunch(ctx, orchestrationmodel.Launch{RunID: run, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Status: orchestrationmodel.LaunchReady}); err != nil {
			t.Fatal(err)
		}
		if err = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: taskID, WorkflowRunID: pgtype.Text{String: run, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if run == oldRun {
			if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(run, orchestrationmodel.LaunchFailed)); err != nil {
				t.Fatal(err)
			}
			processWorkflowCommands(t, ctx, worker)
		}
	}
	if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(oldRun, orchestrationmodel.LaunchCompleted)); err != nil {
		t.Fatal(err)
	}
	value, err = tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil || value.StageWorkState != "failed" {
		t.Fatal("old completion changed newer stage work", value, err)
	}
	if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(newRun, orchestrationmodel.LaunchFailed)); err != nil {
		t.Fatal(err)
	}
	processWorkflowCommands(t, ctx, worker)
	// Competing moves with distinct command identities must not both win.
	a := workflowautomation.TransitionRequest{Action: "move", StageID: "in_review", StageVersion: 3, CommandID: newID()}
	b := a
	b.StageID = "todo"
	b.CommandID = newID()
	results = make(chan error, 2)
	wg.Go(func() {
		_, e := newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, taskID, a)
		results <- e
	})
	wg.Go(func() {
		_, e := newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, taskID, b)
		results <- e
	})
	wg.Wait()
	close(results)
	successes := 0
	for e := range results {
		if e == nil {
			successes++
		} else if !errors.Is(e, modelerrors.ErrConflict) {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent transitions both committed", successes)
	}
	// New imports enter the first stage once; polling does not repeat its
	// assignment/start effects. Source closure moves to the final stage.
	definition, e := repo.Definition(ctx, workspaceID, secondWorkflowID)
	if e != nil {
		t.Fatal(e)
	}
	definition.ConfiguredBy = actorID
	if _, e = repo.Save(ctx, workspaceID, definition, workflow.PlanConfigurationChange); e != nil {
		t.Fatal(e)
	}
	integrationID := newID()
	if _, e = NewPostgresTaskIntegrationRepository(db).Create(ctx, taskintegrationmodel.TaskIntegration{ID: integrationID, WorkspaceID: workspaceID, WorkflowID: secondWorkflowID, Provider: "github_cloud", Name: "Source", Config: []byte(`{}`), Labels: []string{}, Interval: "5m"}, "encrypted-test"); e != nil {
		t.Fatal(e)
	}
	importer := NewPostgresImportedTaskRepository(db)
	item := taskintegrationmodel.DiscoveredTask{ID: "external-1", Title: "Imported", Labels: []string{}}
	for range 2 {
		if e = importer.UpsertImportedTask(ctx, workspaceID, integrationID, secondWorkflowID, "github_cloud", item, testImportedTaskEntryDecider); e != nil {
			t.Fatal(e)
		}
	}
	imported, e := q.ImportedWorkflowTask(ctx, sqlc.ImportedWorkflowTaskParams{WorkspaceID: workspaceID, SourceProvider: "github_cloud", SourceID: pgtype.Text{String: item.ID, Valid: true}, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}})
	if e != nil {
		t.Fatal(e)
	}
	processWorkflowCommands(t, ctx, worker)
	importedTask, e := tasks.Task(ctx, orgID, workspaceID, imported.ID)
	if e != nil || importedTask.WorkflowID != secondWorkflowID || importedTask.AssigneeAgentID != assigned.ID || importedTask.StageWorkState != "running" {
		t.Fatal("import entry effects missing", importedTask, e)
	}
	if e = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND options->>'action'='start'", workspaceID, imported.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("polling duplicated stage start", count, e)
	}
	item.Closed = true
	if e = importer.UpsertImportedTask(ctx, workspaceID, integrationID, secondWorkflowID, "github_cloud", item, testImportedTaskEntryDecider); e != nil {
		t.Fatal(e)
	}
	importedTask, e = tasks.Task(ctx, orgID, workspaceID, imported.ID)
	if e != nil || !importedTask.Closed || importedTask.Status != "in_review" {
		t.Fatal("source closure did not use final stage", importedTask, e)
	}
	// Successful completion atomically queues the destination's automatic work.
	definition, e = repo.Definition(ctx, workspaceID, workspaceID)
	if e != nil {
		t.Fatal(e)
	}
	for i := range definition.Stages {
		if definition.Stages[i].ID == "quality" {
			definition.Rules = append(definition.Rules, workflowmodel.AutomationRule{ID: "quality-start", Enabled: true, Event: workflowmodel.StageEntered, SourceStageID: "quality", Action: workflowmodel.StartWork})
			definition.Stages[i].ResponsibleAgentID = assigned.ID
		}
	}
	if _, e = repo.Save(ctx, workspaceID, definition, workflow.PlanConfigurationChange); e != nil {
		t.Fatal(e)
	}
	chainID := newID()
	chain, e := createWorkflowTask(ctx, tasks, chainID, workspaceID, taskmodel.TaskInput{WorkflowID: workspaceID, Title: "Chain", Status: "in_progress", ActorID: actorID})
	if e != nil {
		t.Fatal(e)
	}
	processWorkflowCommands(t, ctx, worker)
	chainRun := newID()
	if e = launches.CreateLaunch(ctx, orchestrationmodel.Launch{RunID: chainRun, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: chain.ID, Status: orchestrationmodel.LaunchReady}); e != nil {
		t.Fatal(e)
	}
	if e = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: chain.ID, WorkflowRunID: pgtype.Text{String: chainRun, Valid: true}}); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(chainRun, orchestrationmodel.LaunchCompleted)); e != nil {
			t.Fatal(e)
		}
		processWorkflowCommands(t, ctx, worker)
	}
	chain, e = tasks.Task(ctx, orgID, workspaceID, chain.ID)
	if e != nil || chain.Status != "quality" || chain.StageWorkState != "running" || chain.StageVersion != 2 {
		t.Fatal("destination start not queued", chain, e)
	}
	if e = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND options->>'action'='start'", workspaceID, chain.ID).Scan(&count); e != nil || count != 2 {
		t.Fatal("chain duplicated or lost a start", count, e)
	}
	t.Run("event admission", func(t *testing.T) { testWorkflowEventAdmission(t, db, orgID, workspaceID, actorID, assigned.ID) })
	t.Run("latest configuration", func(t *testing.T) { testLatestWorkflowConfiguration(t, db, orgID, actorID, workspaceID, assigned.ID) })
	t.Run("task CRUD transaction", func(t *testing.T) { testTaskCRUDTransaction(t, db, orgID, workspaceID, actorID, assigned.ID) })
	t.Run("transition policy transaction", func(t *testing.T) { testTaskTransitionPolicyTransaction(t, db, orgID, workspaceID, actorID) })
	t.Run("transitions preserve edits", func(t *testing.T) { testTransitionPreservesContent(t, db, orgID, actorID, workspaceID) })
}

func testImportedTaskEntryDecider(snapshot task.ImportedTaskSnapshot) (task.ImportedTaskEffects, error) {
	return task.NewTaskService(nil, nil, nil, nil, nil).PlanImportedTask(snapshot)
}

func testLatestWorkflowConfiguration(t *testing.T, db *PostgresDatabase, orgID, actorID, workspaceID, agentID string) {
	t.Helper()
	for _, scenario := range []string{"disable advancement", "reorder stages", "remove future stage", "change destination policy", "manual completion"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			newID := func() string {
				id, err := ids.NewUUID()
				if err != nil {
					t.Fatal(err)
				}
				return id
			}
			repo := NewWorkflowDefinitionPostgresRepository(db)
			d := workflowmodel.DefaultDefinition(scenario)
			d.ID = newID()
			d.Rules[0].Enabled = scenario != "manual completion"
			d, err := repo.Create(ctx, workspaceID, d, workflow.PlanConfigurationChange)
			if err != nil {
				t.Fatal(err)
			}
			tasks := NewTaskPostgresRepository(db)
			worker := newWorkflowTestWorker(tasks)
			value, err := createWorkflowTask(ctx, tasks, newID(), workspaceID, taskmodel.TaskInput{WorkflowID: d.ID, Title: "Original", Status: "in_progress", AssigneeAgentID: agentID, ActorID: actorID})
			if err != nil {
				t.Fatal(err)
			}
			processWorkflowCommands(t, ctx, worker)
			launches := NewAgentLaunchPostgresRepository(db, nil)
			q := sqlc.New(db.pool)
			runID := newID()
			if scenario != "manual completion" {
				if err = launches.CreateLaunch(ctx, orchestrationmodel.Launch{RunID: runID, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: value.ID, Status: orchestrationmodel.LaunchReady}); err != nil {
					t.Fatal(err)
				}
				if err = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: value.ID, WorkflowRunID: pgtype.Text{String: runID, Valid: true}}); err != nil {
					t.Fatal(err)
				}
			}
			expectedStage, expectedState := "in_review", taskmodel.WorkStateIdle
			switch scenario {
			case "disable advancement", "manual completion":
				for i := range d.Rules {
					if d.Rules[i].Action == workflowmodel.MoveTask {
						d.Rules[i].Enabled = false
					}
				}
				expectedStage, expectedState = "in_progress", taskmodel.WorkStateCompleted
			case "reorder stages":
				d.Stages[0], d.Stages[2] = d.Stages[2], d.Stages[0]
				expectedStage = "in_review"
			case "remove future stage":
				d.Stages = append(d.Stages[:2], d.Stages[3:]...)
				expectedStage = "done"
				for i := range d.Rules {
					if d.Rules[i].Action == workflowmodel.MoveTask {
						d.Rules[i].TargetStageID = "done"
					}
				}
			case "change destination policy":
				d.Rules = append(d.Rules, workflowmodel.AutomationRule{ID: "review-start", Enabled: true, Event: workflowmodel.StageEntered, SourceStageID: "in_review", Action: workflowmodel.StartWork})
				d.Stages[2].ResponsibleAgentID = agentID
				expectedState = taskmodel.WorkStateRunning
			}
			d.Stages[1].Name = "Updated build"
			if _, err = repo.Save(ctx, workspaceID, d, workflow.PlanConfigurationChange); err != nil {
				t.Fatal(err)
			}
			if scenario == "manual completion" {
				_, err = newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, value.ID, workflowautomation.TransitionRequest{Action: "complete", StageVersion: value.StageVersion, CommandID: newID()})
			} else {
				err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, orchestrationmodel.LaunchCompleted))
				if err == nil {
					processWorkflowCommands(t, ctx, worker)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			processWorkflowCommands(t, ctx, worker)
			got, err := tasks.Task(ctx, orgID, workspaceID, value.ID)
			if err != nil || got.Status != expectedStage || got.StageWorkState != expectedState {
				t.Fatalf("latest configuration not applied: %+v, %v", got, err)
			}
			if got.Status == "in_progress" && got.StageName != "Updated build" {
				t.Fatal("task metadata uses old configuration", got.StageName)
			}
		})
	}
}

func testTransitionPreservesContent(t *testing.T, db *PostgresDatabase, orgID, actorID, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskPostgresRepository(db)
	value, err := createWorkflowTask(ctx, tasks, id, workspaceID, taskmodel.TaskInput{WorkflowID: workspaceID, Title: "Original", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	// Hold configuration lock so transition admission waits while content is edited.
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = sqlc.New(tx).LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: workspaceID}); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, id, workflowautomation.TransitionRequest{Action: "move", StageID: "in_review", StageVersion: value.StageVersion, CommandID: id})
		result <- e
	}()
	_, err = sqlc.New(db.pool).UpdateTask(ctx, sqlc.UpdateTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Title: "Concurrent edit", Description: "Keep this description", Status: "todo", Labels: []string{"keep"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	got, err := tasks.Task(ctx, orgID, workspaceID, id)
	if err != nil || got.Title != "Concurrent edit" || got.Description != "Keep this description" || len(got.Labels) != 1 || got.Labels[0] != "keep" || got.Status != "in_review" {
		t.Fatalf("transition overwrote content: %+v, %v", got, err)
	}
}

type workflowTestAccess struct{}

func (workflowTestAccess) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: "admin"}, nil
}

type workflowTestDispatcher struct{}

func (workflowTestDispatcher) StartTask(context.Context, string, string, taskmodel.StartCommand) error {
	return nil
}

func (workflowTestDispatcher) TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error) {
	return true, nil
}

func newWorkflowTestWorker(repo *TaskPostgresRepository) *workflowautomation.WorkflowAutomationWorker {
	service := task.NewTaskService(repo, NewTaskCreationPostgresRepository(&PostgresDatabase{pool: repo.pool}), NewTaskUpdatePostgresRepository(&PostgresDatabase{pool: repo.pool}), newTaskWorkflowService(repo), workflowTestAccess{})
	execution := workflowautomation.NewWorkflowExecutionService(NewExecutionCommandPostgresRepository(&PostgresDatabase{pool: repo.pool}), service, workflowTestDispatcher{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return workflowautomation.NewWorkflowAutomationWorker(newTaskWorkflowService(repo), execution, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func processWorkflowCommands(t *testing.T, ctx context.Context, worker *workflowautomation.WorkflowAutomationWorker) {
	t.Helper()
	for i := 0; i < 100; i++ {
		found, err := worker.ProcessOne(ctx)
		if err != nil {
			t.Fatal("deliver workflow command", err)
		}
		if !found {
			return
		}
	}
	t.Fatal("workflow command delivery did not drain")
}

func newWorkflowTestService(repo *TaskPostgresRepository) *workflowautomation.WorkflowAutomationService {
	return newTaskWorkflowService(repo)
}

func newTaskWorkflowService(repo *TaskPostgresRepository) *workflowautomation.WorkflowAutomationService {
	database := &PostgresDatabase{pool: repo.pool}
	return workflowautomation.NewWorkflowAutomationService(NewTaskTransitionPostgresRepository(database), NewAutomationEventPostgresRepository(database), workflowTestAccess{}, workflowautomation.WorkflowRuleEvaluator{})
}

func createWorkflowTask(ctx context.Context, repo *TaskPostgresRepository, id, workspaceID string, input taskmodel.TaskInput) (taskmodel.Task, error) {
	return NewTaskCreationPostgresRepository(&PostgresDatabase{pool: repo.pool}).WithTaskCreation(ctx, id, workspaceID, input, func(snapshot task.TaskCreationSnapshot, input taskmodel.TaskInput) (task.TaskCreationEffects, error) {
		if input.Status == "" {
			input.Status = snapshot.Workflow.InitialStageID
		}
		plan, err := workflowautomation.PlanTaskEntry(snapshot.Workflow, input.Status, input.AssigneeAgentID)
		if err != nil {
			return task.TaskCreationEffects{}, err
		}
		input.AssigneeAgentID = plan.AssigneeAgentID
		return task.TaskCreationEffects{Input: input, EmitStageEntry: true, Events: []workflow.EventIntent{workflow.StageEvent(snapshot.TaskID, 1, workflowmodel.StageEntered, "", input.CommandID, "")}}, nil
	})
}
