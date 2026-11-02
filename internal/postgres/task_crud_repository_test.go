package postgres

import (
	"context"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

func testTaskCRUDTransaction(t *testing.T, db *PostgresDatabase, orgID, workspaceID, actorID, agentID string) {
	t.Helper()
	ctx := context.Background()
	newID := func() string {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	definition := workflow.DefaultDefinition("Task CRUD")
	definition.ID, definition.ConfiguredBy = newID(), actorID
	definition.InitialStageID = "in_review"
	definition.Stages[2].ResponsibleAgentID = agentID
	if _, err := NewWorkflowDefinitionPostgresRepository(db).Create(ctx, workspaceID, definition, workflow.PlanConfigurationChange); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskPostgresRepository(db)
	service := task.NewTaskService(tasks, NewTaskCreationPostgresRepository(db), NewTaskUpdatePostgresRepository(db), newTaskWorkflowService(tasks), workflowTestAccess{})
	created, err := service.Create(ctx, actorID, orgID, workspaceID, task.TaskInput{WorkflowID: definition.ID, Title: "  Original  ", Labels: []string{" keep ", "keep"}})
	if err != nil || created.Status != "in_review" || created.AssigneeAgentID != agentID || created.StageVersion != 1 || created.Title != "Original" || len(created.Labels) != 1 || !created.AutomationPending {
		t.Fatalf("creation: %+v, %v", created, err)
	}
	edit := task.TaskInput{Title: "Edited", Description: "Updated", Status: created.Status, AssigneeAgentID: agentID, StageVersion: created.StageVersion, CommandID: newID()}
	updated, err := service.Update(ctx, actorID, orgID, workspaceID, created.ID, edit)
	if err != nil || updated.Title != "Edited" || updated.StageVersion != 1 {
		t.Fatalf("content update: %+v, %v", updated, err)
	}
	if _, err = service.Update(ctx, actorID, orgID, workspaceID, created.ID, edit); err != nil {
		t.Fatal("content replay", err)
	}
	edit.Title, edit.Status, edit.StartWork, edit.CommandID = "Combined", "todo", true, newID()
	updated, err = service.Update(ctx, actorID, orgID, workspaceID, created.ID, edit)
	if err != nil || updated.Title != "Combined" || updated.Status != "todo" || updated.StageVersion != 2 {
		t.Fatalf("combined update: %+v, %v", updated, err)
	}
	edit.Title, edit.Status, edit.StartWork, edit.CommandID, edit.StageVersion = "Rejected", "in_progress", false, newID(), updated.StageVersion
	edit.RepositoryIDs = []string{newID()}
	if _, err = service.Update(ctx, actorID, orgID, workspaceID, created.ID, edit); err == nil {
		t.Fatal("missing repository accepted")
	}
	restored, err := tasks.Task(ctx, orgID, workspaceID, created.ID)
	if err != nil || restored.Title != "Combined" || restored.Status != "todo" || restored.StageVersion != 2 {
		t.Fatalf("partial update committed: %+v, %v", restored, err)
	}
	var events, starts, rejected int
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_events WHERE workspace_id=$1 AND task_id=$2", workspaceID, created.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND options->>'action'='start'", workspaceID, created.ID).Scan(&starts); err != nil {
		t.Fatal(err)
	}
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND id=$3", workspaceID, created.ID, edit.CommandID).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	if events != 2 || starts != 0 || rejected != 0 {
		t.Fatalf("unexpected atomic effects: events=%d, starts=%d, rejected=%d", events, starts, rejected)
	}
}
