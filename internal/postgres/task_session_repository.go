package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

func (r *AgentLaunchPostgresRepository) CurrentSession(ctx context.Context, orgID, workspaceID, taskID string) (agentctl.RunConfig, bool, error) {
	row, err := r.queries.CurrentTaskAgentSession(ctx, sqlc.CurrentTaskAgentSessionParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID})
	if errors.Is(err, pgx.ErrNoRows) {
		return agentctl.RunConfig{}, false, nil
	}
	if err != nil {
		return agentctl.RunConfig{}, false, fmt.Errorf("read current task session: %w", err)
	}
	var config agentctl.RunConfig
	if err := json.Unmarshal(row.Configuration, &config); err != nil {
		return config, false, fmt.Errorf("decode session configuration: %w", err)
	}
	config.TaskSessionID = row.ID
	return config, true, nil
}

func (r *AgentLaunchPostgresRepository) ReserveSessionRun(ctx context.Context, config agentctl.RunConfig) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if err := q.LockTaskSessionScope(ctx, sqlc.LockTaskSessionScopeParams{ID: config.TaskID, WorkspaceID: config.WorkspaceID}); err != nil {
		return err
	}
	if _, err := q.ActiveTaskLaunch(ctx, sqlc.ActiveTaskLaunchParams{WorkspaceID: config.WorkspaceID, TaskID: config.TaskID}); !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return err
		}
		return modelerrors.ErrConflict
	}
	if config.RestoreSession {
		current, err := q.CurrentTaskAgentSession(ctx, sqlc.CurrentTaskAgentSessionParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID})
		if err != nil {
			return err
		}
		if current.ID != config.TaskSessionID {
			return modelerrors.ErrConflict
		}
	} else {
		snapshot := config
		snapshot.MCPServers = append([]agentctl.MCPServer(nil), config.MCPServers...)
		for i := range snapshot.MCPServers {
			snapshot.MCPServers[i].Headers = nil
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		if err := q.RetireTaskAgentSession(ctx, sqlc.RetireTaskAgentSessionParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID}); err != nil {
			return err
		}
		if err := q.InsertTaskAgentSession(ctx, sqlc.InsertTaskAgentSessionParams{ID: config.TaskSessionID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Configuration: data}); err != nil {
			return err
		}
	}
	if err := q.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{RunID: config.RunID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, TaskSessionID: pgtype.Text{String: config.TaskSessionID, Valid: true}, Status: string(orchestration.LaunchStarting)}); err != nil {
		return err
	}
	if config.InitialCommandID != "" {
		if err := q.InsertPromptOutbox(ctx, sqlc.InsertPromptOutboxParams{RunID: config.RunID, CommandID: config.InitialCommandID, Prompt: config.InitialPrompt}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *AgentLaunchPostgresRepository) RecordPrompt(ctx context.Context, runID, commandID, prompt string) (bool, error) {
	if err := r.queries.InsertPromptOutbox(ctx, sqlc.InsertPromptOutboxParams{RunID: runID, CommandID: commandID, Prompt: prompt}); err != nil {
		return false, err
	}
	previous, err := r.queries.GetPromptOutbox(ctx, sqlc.GetPromptOutboxParams{RunID: runID, CommandID: commandID})
	if err != nil {
		return false, err
	}
	if previous.Prompt != prompt {
		return false, agentctl.ErrCommand
	}
	return previous.Delivered, nil
}

func (r *AgentLaunchPostgresRepository) PromptDelivered(ctx context.Context, runID, commandID string) error {
	return r.queries.MarkPromptDelivered(ctx, sqlc.MarkPromptDeliveredParams{RunID: runID, CommandID: commandID})
}

func (r *AgentLaunchPostgresRepository) PendingPrompts(ctx context.Context, runID string) ([]orchestration.PromptDelivery, error) {
	rows, err := r.queries.PendingPromptOutbox(ctx, runID)
	if err != nil {
		return nil, err
	}
	prompts := make([]orchestration.PromptDelivery, 0, len(rows))
	for _, row := range rows {
		prompts = append(prompts, orchestration.PromptDelivery{CommandID: row.CommandID, Text: row.Prompt})
	}
	return prompts, nil
}

func (r *AgentLaunchPostgresRepository) TaskCommand(ctx context.Context, orgID, workspaceID, taskID, commandID, prompt string) (bool, error) {
	previous, err := r.queries.FindTaskCommand(ctx, sqlc.FindTaskCommandParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, CommandID: commandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if prompt != "" && prompt != previous {
		return false, agentctl.ErrCommand
	}
	return true, nil
}
