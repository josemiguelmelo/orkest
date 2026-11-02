package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
	"github.com/josemiguelmelo/orkest/internal/workflow"
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
	repo := NewWorkflowPostgresRepository(db)
	d, err := repo.Definition(ctx, workspaceID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	agents := NewAgentPostgresRepository(db)
	assigned, err := agents.CreateAgent(ctx, newID(), workspaceID, agent.AgentDefinition{Name: "Builder", Runtime: agent.RuntimeOpenCode, Provider: "test", Model: "test", PermissionPolicy: agent.PermissionPolicyAsk})
	if err != nil {
		t.Fatal(err)
	}
	secondWorkflowID := newID()
	secondDefinition := workflow.DefaultDefinition("Support")
	secondDefinition.ID = secondWorkflowID
	secondDefinition.Stages = []workflow.Stage{
		secondDefinition.Stages[1],
		secondDefinition.Stages[0],
		secondDefinition.Stages[3],
		secondDefinition.Stages[2],
	}
	secondDefinition.Stages[0].ResponsibleAgentID = assigned.ID
	secondDefinition.ConfiguredBy = actorID
	if _, err = repo.Create(ctx, workspaceID, secondDefinition); err != nil {
		t.Fatal("create second workflow with shared stage IDs", err)
	}
	if definitions, listErr := repo.Definitions(ctx, workspaceID); listErr != nil || len(definitions) != 2 {
		t.Fatal("workspace workflows were not listed", len(definitions), listErr)
	}
	original := d
	d.Stages[1].ResponsibleAgentID = foreignAgent
	if _, err = repo.Save(ctx, workspaceID, d); !errors.Is(err, modelerrors.ErrInvalid) {
		t.Fatal("foreign workspace agent accepted", err)
	}
	d.Stages[1].ResponsibleAgentID = assigned.ID
	d.Stages = append(d.Stages[:2], append([]workflow.Stage{{ID: "quality", Name: "Quality", AutoAdvance: false}}, d.Stages[2:]...)...)
	d, err = repo.Save(ctx, workspaceID, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(ctx, workspaceID, original); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("stale workflow write accepted", err)
	}
	tasks := NewTaskPostgresRepository(db)
	taskID := newID()
	value, err := tasks.CreateTask(ctx, taskID, workspaceID, task.TaskInput{WorkflowID: workspaceID, Title: "Workflow", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	secondTaskID := newID()
	secondTask, err := tasks.CreateTask(ctx, secondTaskID, workspaceID, task.TaskInput{WorkflowID: secondWorkflowID, Title: "Support task", Status: "todo"})
	if err != nil || secondTask.WorkflowID != secondWorkflowID {
		t.Fatal("task was not associated with its selected workflow", secondTask, err)
	}
	if err = repo.Delete(ctx, workspaceID, secondWorkflowID); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("workflow with an associated task was deleted", err)
	}
	input := task.TaskInput{Title: value.Title, Status: "in_progress", StageVersion: value.StageVersion, CommandID: newID()}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, e := tasks.TransitionTask(ctx, actorID, orgID, workspaceID, taskID, input); results <- e })
	}
	wg.Wait()
	close(results)
	// Repeated command delivery is idempotent, and only one command is queued.
	for e := range results {
		if e != nil {
			t.Fatal("idempotent move failed", e)
		}
	}
	value, err = tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if value.StageVersion != 2 || value.StageWorkState != "queued" || value.AssigneeAgentID != assigned.ID {
		t.Fatalf("entry effects missing: %+v", value)
	}
	q := sqlc.New(db.pool)
	var count int
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2", workspaceID, taskID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate start command", count, err)
	}
	d.Stages = append(d.Stages[:1], d.Stages[2:]...)
	if _, err = repo.Save(ctx, workspaceID, d); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("occupied stage removed", err)
	}
	// Use durable launch records without launching an external runtime.
	launches := NewAgentLaunchPostgresRepository(db)
	runID := newID()
	if err = launches.CreateLaunch(ctx, orchestration.Launch{RunID: runID, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Status: orchestration.LaunchReady}); err != nil {
		t.Fatal(err)
	}
	if err = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: taskID, WorkflowRunID: pgtype.Text{String: runID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err = launches.SetLaunchStatus(ctx, runID, orchestration.LaunchCompleted); err != nil {
		t.Fatal(err)
	}
	value, err = tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil || value.Status != "quality" || value.StageVersion != 3 {
		t.Fatal("successful run did not advance", value, err)
	}
	if err = launches.SetLaunchStatus(ctx, runID, orchestration.LaunchCompleted); err != nil {
		t.Fatal(err)
	}
	again, err := tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil || again.StageVersion != 3 {
		t.Fatal("completion replay advanced task", again, err)
	}
	// A late completion from an earlier run in the same stage cannot finish
	// the newer run. Failures leave stage identity and position unchanged.
	oldRun, newRun := newID(), newID()
	for _, run := range []string{oldRun, newRun} {
		if err = launches.CreateLaunch(ctx, orchestration.Launch{RunID: run, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Status: orchestration.LaunchReady}); err != nil {
			t.Fatal(err)
		}
		if err = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: taskID, WorkflowRunID: pgtype.Text{String: run, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if run == oldRun {
			if err = launches.SetLaunchStatus(ctx, run, orchestration.LaunchFailed); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = launches.SetLaunchStatus(ctx, oldRun, orchestration.LaunchCompleted); err != nil {
		t.Fatal(err)
	}
	value, err = tasks.Task(ctx, orgID, workspaceID, taskID)
	if err != nil || value.StageWorkState != "failed" {
		t.Fatal("old completion changed newer stage work", value, err)
	}
	if err = launches.SetLaunchStatus(ctx, newRun, orchestration.LaunchFailed); err != nil {
		t.Fatal(err)
	}
	// Competing moves with distinct command identities must not both win.
	a := task.TaskInput{Title: value.Title, Status: "in_review", StageVersion: 3}
	b := a
	b.Status = "todo"
	results = make(chan error, 2)
	wg.Go(func() { _, e := tasks.TransitionTask(ctx, actorID, orgID, workspaceID, taskID, a); results <- e })
	wg.Go(func() { _, e := tasks.TransitionTask(ctx, actorID, orgID, workspaceID, taskID, b); results <- e })
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
	if _, e = repo.Save(ctx, workspaceID, definition); e != nil {
		t.Fatal(e)
	}
	integrationID := newID()
	if _, e = NewPostgresTaskIntegrationRepository(db).Create(ctx, taskintegration.TaskIntegration{ID: integrationID, WorkspaceID: workspaceID, WorkflowID: secondWorkflowID, Provider: "github_cloud", Name: "Source", Config: []byte(`{}`), Labels: []string{}, Interval: "5m"}, "encrypted-test"); e != nil {
		t.Fatal(e)
	}
	importer := NewPostgresImportedTaskRepository(db)
	item := taskintegration.DiscoveredTask{ID: "external-1", Title: "Imported", Labels: []string{}}
	for range 2 {
		if e = importer.UpsertImportedTask(ctx, workspaceID, integrationID, secondWorkflowID, "github_cloud", item); e != nil {
			t.Fatal(e)
		}
	}
	imported, e := q.ImportedWorkflowTask(ctx, sqlc.ImportedWorkflowTaskParams{WorkspaceID: workspaceID, SourceProvider: "github_cloud", SourceID: pgtype.Text{String: item.ID, Valid: true}, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}})
	if e != nil {
		t.Fatal(e)
	}
	importedTask, e := tasks.Task(ctx, orgID, workspaceID, imported.ID)
	if e != nil || importedTask.WorkflowID != secondWorkflowID || importedTask.AssigneeAgentID != assigned.ID || importedTask.StageWorkState != "queued" {
		t.Fatal("import entry effects missing", importedTask, e)
	}
	if e = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2", workspaceID, imported.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("polling duplicated stage start", count, e)
	}
	item.Closed = true
	if e = importer.UpsertImportedTask(ctx, workspaceID, integrationID, secondWorkflowID, "github_cloud", item); e != nil {
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
			definition.Stages[i].AutoStart = true
			definition.Stages[i].ResponsibleAgentID = assigned.ID
		}
	}
	if _, e = repo.Save(ctx, workspaceID, definition); e != nil {
		t.Fatal(e)
	}
	chainID := newID()
	chain, e := tasks.CreateTask(ctx, chainID, workspaceID, task.TaskInput{WorkflowID: workspaceID, Title: "Chain", Status: "in_progress", ActorID: actorID})
	if e != nil {
		t.Fatal(e)
	}
	chainRun := newID()
	if e = launches.CreateLaunch(ctx, orchestration.Launch{RunID: chainRun, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: chain.ID, Status: orchestration.LaunchReady}); e != nil {
		t.Fatal(e)
	}
	if e = q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: workspaceID, ID: chain.ID, WorkflowRunID: pgtype.Text{String: chainRun, Valid: true}}); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = launches.SetLaunchStatus(ctx, chainRun, orchestration.LaunchCompleted); e != nil {
			t.Fatal(e)
		}
	}
	chain, e = tasks.Task(ctx, orgID, workspaceID, chain.ID)
	if e != nil || chain.Status != "quality" || chain.StageWorkState != "queued" || chain.StageVersion != 2 {
		t.Fatal("destination start not queued", chain, e)
	}
	if e = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2", workspaceID, chain.ID).Scan(&count); e != nil || count != 2 {
		t.Fatal("chain duplicated or lost a start", count, e)
	}
}
