package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
)

func testTaskTriggerRuleActionsRepair(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	// Reproduce the early 0041 schema only in the migration suite's disposable schema.
	if _, err := pool.Exec(ctx, `
ALTER TABLE task_trigger_rules DROP COLUMN action, DROP COLUMN prompt_text;
ALTER TABLE task_trigger_rules ADD CONSTRAINT task_trigger_rules_check CHECK(source_stage_id<>target_stage_id);
DELETE FROM goose_db_version WHERE version_id>=45;
INSERT INTO organizations(id,name) VALUES('rule-repair-org','Repair');
INSERT INTO workspaces(id,organization_id,name) VALUES('rule-repair-workspace','rule-repair-org','Repair');
INSERT INTO task_trigger_integrations(id,workspace_id,name,provider,config,enabled,configured_by,encrypted_secret)
VALUES('rule-repair-integration','rule-repair-workspace','Repair','github','{}',true,'fixture','encrypted-fixture');
INSERT INTO task_trigger_rules(integration_id,workspace_id,id,event_type,workflow_id,source_stage_id,target_stage_id)
VALUES('rule-repair-integration','rule-repair-workspace','existing','pull_request.merged','rule-repair-workspace','todo','in_review');
`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id='rule-repair-org'`); err != nil {
			t.Error(err)
		}
	}()
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("repair historical trigger rules", err)
	}
	queries := sqlc.New(pool)
	rule := sqlc.InsertTaskTriggerRuleParams{
		IntegrationID: "rule-repair-integration", WorkspaceID: "rule-repair-workspace",
		ID: "start", EventType: "pull_request.review_submitted", WorkflowID: "rule-repair-workspace",
		SourceStageID: "todo", TargetStageID: "todo", Action: "start_work", PromptText: "Address the review",
	}
	if err := queries.InsertTaskTriggerRule(ctx, rule); err != nil {
		t.Fatal("save same-stage start after repair", err)
	}
	// Reapplying the repair to the current schema must preserve configured actions.
	if _, err := pool.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id>=45`); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("repair current schema", err)
	}
	rules, err := queries.ListTaskTriggerRules(ctx, rule.IntegrationID)
	if err != nil || len(rules) != 2 {
		t.Fatalf("read repaired rules: %v %v", rules, err)
	}
	if rules[0].Action != "move_task" || rules[0].PromptText != "" || rules[0].TargetStageID != "in_review" {
		t.Fatalf("legacy rule changed: %+v", rules[0])
	}
	if rules[1].Action != rule.Action || rules[1].PromptText != rule.PromptText {
		t.Fatalf("configured start changed: %+v", rules[1])
	}
	for _, invalid := range []sqlc.InsertTaskTriggerRuleParams{
		{Action: "move_task"},
		{Action: "unknown"},
		{Action: "start_work", PromptText: strings.Repeat("x", 32769)},
	} {
		rule.ID = "invalid"
		rule.EventType = "invalid"
		rule.Action, rule.PromptText = invalid.Action, invalid.PromptText
		if err := queries.InsertTaskTriggerRule(ctx, rule); err == nil {
			t.Fatalf("invalid rule accepted: action=%q prompt length=%d", rule.Action, len(rule.PromptText))
		}
	}
}
