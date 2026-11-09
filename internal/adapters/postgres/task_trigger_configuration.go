package postgres

import (
	"context"
	"sort"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func (r *TaskTriggerIntegrationPostgresRepository) RuleWorkflows(ctx context.Context, workspace string, rules []trigger.Rule) ([]workflowmodel.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	ids := make([]string, 0, len(rules))
	seen := make(map[string]bool)
	for _, rule := range rules {
		if !seen[rule.WorkflowID] {
			ids = append(ids, rule.WorkflowID)
			seen[rule.WorkflowID] = true
		}
	}
	sort.Strings(ids)
	definitions := make([]workflowmodel.Definition, 0, len(ids))
	for _, id := range ids {
		row, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspace, ID: id})
		if err != nil {
			return nil, mapError(err, problem.ErrInvalid, problem.ErrConflict)
		}
		definition, err := workflowDefinition(ctx, q, row)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}
