package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

func (r *AgentLaunchPostgresRepository) CurrentSession(ctx context.Context, orgID, workspaceID, taskID string) (runconfig.RunConfig, bool, error) {
	row, err := r.queries.CurrentTaskAgentSession(ctx, sqlc.CurrentTaskAgentSessionParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID})
	if errors.Is(err, pgx.ErrNoRows) {
		return runconfig.RunConfig{}, false, nil
	}
	if err != nil {
		return runconfig.RunConfig{}, false, fmt.Errorf("read current task session: %w", err)
	}
	var config runconfig.RunConfig
	if err := json.Unmarshal(row.Configuration, &config); err != nil {
		return config, false, fmt.Errorf("decode session configuration: %w", err)
	}
	if config.EncryptedEnvironment != "" {
		if r.secretBox == nil {
			return config, false, secretbox.ErrKeyUnavailable
		}
		plain, err := r.secretBox.Open([]byte(config.EncryptedEnvironment))
		if err != nil {
			return config, false, errors.New("decrypt task session environment")
		}
		var secrets struct {
			Environment    map[string]string         `json:"environment"`
			GitCredentials []runconfig.GitCredential `json:"gitCredentials"`
		}
		if err := json.Unmarshal([]byte(plain), &secrets); err != nil {
			return config, false, errors.New("decode task session environment")
		}
		if secrets.Environment == nil && secrets.GitCredentials == nil {
			if err := json.Unmarshal([]byte(plain), &secrets.Environment); err != nil {
				return config, false, errors.New("decode task session environment")
			}
		}
		config.Environment = secrets.Environment
		config.GitCredentials = secrets.GitCredentials
		config.EncryptedEnvironment = ""
	}
	config.TaskSessionID = row.ID
	return config, true, nil
}

func (r *AgentLaunchPostgresRepository) ReserveSessionRun(ctx context.Context, config runconfig.RunConfig, expectedStageVersion int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if err := q.LockTaskSessionScope(ctx, sqlc.LockTaskSessionScopeParams{ID: config.TaskID, WorkspaceID: config.WorkspaceID}); err != nil {
		return err
	}
	if expectedStageVersion > 0 {
		state, e := q.TaskWorkflowState(ctx, sqlc.TaskWorkflowStateParams{WorkspaceID: config.WorkspaceID, ID: config.TaskID})
		if e != nil {
			return e
		}
		if state.StageVersion != expectedStageVersion {
			return modelerrors.ErrConflict
		}
	}
	if config.InitialCommandID != "" && expectedStageVersion > 0 {
		exists, e := q.WorkflowCommandAccepted(ctx, config.InitialCommandID)
		if e != nil {
			return e
		}
		if exists {
			return modelerrors.ErrConflict
		}
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
		data, err := r.sessionSnapshot(config)
		if err != nil {
			return err
		}
		if _, err := q.UpdateTaskAgentSessionConfiguration(ctx, sqlc.UpdateTaskAgentSessionConfigurationParams{OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Configuration: data}); err != nil {
			return err
		}
	} else {
		data, err := r.sessionSnapshot(config)
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
	if err := q.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{ExecutionImage: config.ExecutionImage, RunID: config.RunID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, TaskSessionID: pgtype.Text{String: config.TaskSessionID, Valid: true}, Status: string(orchestration.LaunchStarting)}); err != nil {
		return err
	}
	if err := q.BindWorkflowRun(ctx, sqlc.BindWorkflowRunParams{WorkspaceID: config.WorkspaceID, ID: config.TaskID, WorkflowRunID: pgtype.Text{String: config.RunID, Valid: true}}); err != nil {
		return err
	}
	for _, revisionID := range config.ArtifactRevisionIDs {
		count, err := q.PinRunArtifactInput(ctx, sqlc.PinRunArtifactInputParams{RunID: config.RunID, ID: revisionID})
		if err != nil {
			return err
		}
		if count != 1 {
			return modelerrors.ErrConflict
		}
	}
	if config.InitialCommandID != "" {
		if err := q.InsertPromptOutbox(ctx, sqlc.InsertPromptOutboxParams{RunID: config.RunID, CommandID: config.InitialCommandID, Prompt: config.InitialPrompt}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *AgentLaunchPostgresRepository) sessionSnapshot(config runconfig.RunConfig) ([]byte, error) {
	snapshot := config
	snapshot.MCPServers = append([]runconfig.MCPServer(nil), config.MCPServers...)
	for i := range snapshot.MCPServers {
		snapshot.MCPServers[i].Headers = nil
	}
	if len(snapshot.Environment) > 0 || len(snapshot.GitCredentials) > 0 {
		if r.secretBox == nil {
			return nil, secretbox.ErrKeyUnavailable
		}
		encoded, err := json.Marshal(struct {
			Environment    map[string]string         `json:"environment,omitempty"`
			GitCredentials []runconfig.GitCredential `json:"gitCredentials,omitempty"`
		}{snapshot.Environment, snapshot.GitCredentials})
		if err != nil {
			return nil, err
		}
		sealed, err := r.secretBox.Seal(string(encoded))
		if err != nil {
			return nil, err
		}
		snapshot.EncryptedEnvironment = string(sealed)
		snapshot.Environment = nil
		snapshot.GitCredentials = nil
	}
	return json.Marshal(snapshot)
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
		return false, interaction.ErrCommand
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
		return false, interaction.ErrCommand
	}
	return true, nil
}
