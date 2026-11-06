package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
)

var _ orchestration.DirectConversationStore = (*AgentLaunchPostgresRepository)(nil)

func (r *AgentLaunchPostgresRepository) CreateDirectConversation(ctx context.Context, id, orgID, workspaceID, agentID string) (orchestration.DirectConversation, error) {
	row, err := r.queries.CreateDirectConversation(ctx, sqlc.CreateDirectConversationParams{ID: id, OrganizationID: orgID, WorkspaceID: workspaceID, AgentID: agentID})
	if err != nil {
		return orchestration.DirectConversation{}, fmt.Errorf("create direct conversation: %w", err)
	}
	return orchestration.DirectConversation{ID: row.ID, AgentID: row.AgentID, CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339Nano)}, nil
}

func (r *AgentLaunchPostgresRepository) GetDirectConversation(ctx context.Context, orgID, workspaceID, id string) (orchestration.DirectConversation, error) {
	row, err := r.queries.GetDirectConversation(ctx, sqlc.GetDirectConversationParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return orchestration.DirectConversation{}, modelerrors.ErrNotFound
	}
	if err != nil {
		return orchestration.DirectConversation{}, fmt.Errorf("get direct conversation: %w", err)
	}
	item := orchestration.DirectConversation{ID: row.ID, AgentID: row.AgentID, CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339Nano)}
	return item, nil
}

func (r *AgentLaunchPostgresRepository) ListDirectConversations(ctx context.Context, orgID, workspaceID string) ([]orchestration.DirectConversation, error) {
	rows, err := r.queries.ListDirectConversations(ctx, sqlc.ListDirectConversationsParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	items := []orchestration.DirectConversation{}
	for _, row := range rows {
		items = append(items, orchestration.DirectConversation{ID: row.ID, AgentID: row.AgentID, CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339Nano)})
	}
	return items, nil
}

func (r *AgentLaunchPostgresRepository) DeleteDirectConversation(ctx context.Context, orgID, workspaceID, conversationID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin direct conversation deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if _, err = q.LockDirectConversation(ctx, sqlc.LockDirectConversationParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: conversationID}); errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock direct conversation: %w", err)
	}
	if _, err = q.ActiveDirectConversationLaunch(ctx, sqlc.ActiveDirectConversationLaunchParams{WorkspaceID: workspaceID, TaskID: conversationID}); err == nil {
		return modelerrors.ErrConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check direct conversation activity: %w", err)
	}
	if err = q.DeleteDirectConversationLaunches(ctx, sqlc.DeleteDirectConversationLaunchesParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: conversationID}); err != nil {
		return fmt.Errorf("delete direct conversation runs: %w", err)
	}
	if err = q.DeleteDirectConversationSessions(ctx, sqlc.DeleteDirectConversationSessionsParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: conversationID}); err != nil {
		return fmt.Errorf("delete direct conversation sessions: %w", err)
	}
	if _, err = q.DeleteDirectConversation(ctx, sqlc.DeleteDirectConversationParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: conversationID}); err != nil {
		return fmt.Errorf("delete direct conversation: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit direct conversation deletion: %w", err)
	}
	return nil
}

func (r *AgentLaunchPostgresRepository) CurrentDirectSession(ctx context.Context, orgID, workspaceID, conversationID string) (orchestration.RunConfig, bool, error) {
	return r.CurrentSession(ctx, orgID, workspaceID, conversationID)
}

func (r *AgentLaunchPostgresRepository) ReserveDirectSessionRun(ctx context.Context, config orchestration.RunConfig) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	_, err = q.LockDirectConversation(ctx, sqlc.LockDirectConversationParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, ID: config.TaskID})
	if errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = q.ActiveDirectConversationLaunch(ctx, sqlc.ActiveDirectConversationLaunchParams{WorkspaceID: config.WorkspaceID, TaskID: config.TaskID})
	if err == nil {
		return modelerrors.ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if config.RestoreSession {
		current, err := q.CurrentTaskAgentSession(ctx, sqlc.CurrentTaskAgentSessionParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID})
		if err != nil {
			return err
		}
		if current.ID != config.TaskSessionID {
			return modelerrors.ErrConflict
		}
		data, err := r.sessionSnapshot(config)
		if err != nil {
			return err
		}
		if _, err = q.UpdateTaskAgentSessionConfiguration(ctx, sqlc.UpdateTaskAgentSessionConfigurationParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Configuration: data}); err != nil {
			return err
		}
	} else {
		data, err := r.sessionSnapshot(config)
		if err != nil {
			return err
		}
		if err = q.RetireTaskAgentSession(ctx, sqlc.RetireTaskAgentSessionParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID}); err != nil {
			return err
		}
		if err = q.InsertTaskAgentSession(ctx, sqlc.InsertTaskAgentSessionParams{ID: config.TaskSessionID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Configuration: data}); err != nil {
			return err
		}
	}
	if err = q.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{ExecutionImage: config.ExecutionImage, RunID: config.RunID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, TaskSessionID: pgtype.Text{String: config.TaskSessionID, Valid: true}, Status: "starting"}); err != nil {
		return err
	}
	if config.InitialCommandID != "" {
		if err = q.InsertPromptOutbox(ctx, sqlc.InsertPromptOutboxParams{RunID: config.RunID, CommandID: config.InitialCommandID, Prompt: config.InitialPrompt}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
