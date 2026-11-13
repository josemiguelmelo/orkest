package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	stage "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
)

type WorkflowStageIntegrationPostgresRepository struct{ database *PostgresDatabase }

func NewWorkflowStageIntegrationPostgresRepository(database *PostgresDatabase) *WorkflowStageIntegrationPostgresRepository {
	return &WorkflowStageIntegrationPostgresRepository{database: database}
}

var _ stage.IntegrationRepository = (*WorkflowStageIntegrationPostgresRepository)(nil)

func integrationConnection(row sqlc.WorkflowStageIntegration) stage.StoredConnection {
	return stage.StoredConnection{Connection: stage.Connection{ID: row.ID, WorkspaceID: row.WorkspaceID, Provider: row.Provider, Name: row.Name, Enabled: row.Enabled, CodingIntegrationID: row.CodingIntegrationID, RepositoryIDs: row.RepositoryIds, Config: row.Config, ConfiguredBy: row.ConfiguredBy, Version: row.Version, UpdatedAt: row.UpdatedAt.Time}, EncryptedSecret: row.EncryptedSecret}
}

func (r *WorkflowStageIntegrationPostgresRepository) ListConnections(ctx context.Context, workspaceID string) ([]stage.Connection, error) {
	rows, err := sqlc.New(r.database.pool).ListWorkflowStageIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]stage.Connection, 0, len(rows))
	for _, row := range rows {
		out = append(out, integrationConnection(row).Connection)
	}
	return out, nil
}

func (r *WorkflowStageIntegrationPostgresRepository) GetConnection(ctx context.Context, workspaceID, id string) (stage.StoredConnection, error) {
	row, err := sqlc.New(r.database.pool).GetWorkflowStageIntegration(ctx, sqlc.GetWorkflowStageIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return stage.StoredConnection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	return integrationConnection(row), nil
}

func (r *WorkflowStageIntegrationPostgresRepository) CallbackConnection(ctx context.Context, id string) (stage.StoredConnection, error) {
	// Connection IDs are globally unique. Workspace scope always comes from this
	// stored row; no callback body can select a workspace.
	row, err := sqlc.New(r.database.pool).LocklessWorkflowStageCallbackIntegration(ctx, id)
	if err != nil {
		return stage.StoredConnection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	return integrationConnection(row), nil
}

func validateIntegrationRepositories(ctx context.Context, q *sqlc.Queries, c stage.Connection) error {
	rows, err := q.WorkflowStageIntegrationRepositories(ctx, sqlc.WorkflowStageIntegrationRepositoriesParams{WorkspaceID: c.WorkspaceID, IntegrationID: pgtype.Text{String: c.CodingIntegrationID, Valid: true}, RepositoryIds: c.RepositoryIDs})
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows) != len(c.RepositoryIDs) {
		return problem.ErrInvalid
	}
	_, err = q.WorkflowStageIntegrationCodingProvider(ctx, sqlc.WorkflowStageIntegrationCodingProviderParams{WorkspaceID: c.WorkspaceID, ID: c.CodingIntegrationID})
	return mapError(err, problem.ErrInvalid, problem.ErrConflict)
}

func (r *WorkflowStageIntegrationPostgresRepository) CreateConnection(ctx context.Context, in stage.StoredConnection) (stage.Connection, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return stage.Connection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	c := in.Connection
	if err = validateIntegrationRepositories(ctx, q, c); err != nil {
		return stage.Connection{}, err
	}
	row, err := q.CreateWorkflowStageIntegration(ctx, sqlc.CreateWorkflowStageIntegrationParams{ID: c.ID, WorkspaceID: c.WorkspaceID, Provider: c.Provider, Name: c.Name, Enabled: c.Enabled, CodingIntegrationID: c.CodingIntegrationID, RepositoryIds: c.RepositoryIDs, Config: c.Config, EncryptedSecret: in.EncryptedSecret, ConfiguredBy: c.ConfiguredBy})
	if err != nil {
		return stage.Connection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	return integrationConnection(row).Connection, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) UpdateConnection(ctx context.Context, in stage.StoredConnection) (stage.Connection, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return stage.Connection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	c := in.Connection
	old, err := q.LockWorkflowStageIntegration(ctx, c.ID)
	if err != nil {
		return stage.Connection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	if old.WorkspaceID != c.WorkspaceID {
		return stage.Connection{}, problem.ErrNotFound
	}
	if old.Provider != c.Provider || old.CodingIntegrationID != c.CodingIntegrationID {
		return stage.Connection{}, problem.ErrInvalid
	}
	if err = validateIntegrationRepositories(ctx, q, c); err != nil {
		return stage.Connection{}, err
	}
	row, err := q.UpdateWorkflowStageIntegration(ctx, sqlc.UpdateWorkflowStageIntegrationParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Version: c.Version, Name: c.Name, Enabled: c.Enabled, RepositoryIds: c.RepositoryIDs, Config: c.Config, EncryptedSecret: in.EncryptedSecret, ConfiguredBy: c.ConfiguredBy})
	if err != nil {
		return stage.Connection{}, mapError(err, problem.ErrConflict, problem.ErrConflict)
	}
	return integrationConnection(row).Connection, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) DeleteConnection(ctx context.Context, workspaceID, id string) error {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	old, err := q.LockWorkflowStageIntegration(ctx, id)
	if err != nil {
		return mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	if old.WorkspaceID != workspaceID {
		return problem.ErrNotFound
	}
	exists, err := q.WorkflowStageIntegrationReferenced(ctx, sqlc.WorkflowStageIntegrationReferencedParams{WorkspaceID: workspaceID, ConnectionID: id})
	if err != nil {
		return err
	}
	if exists {
		return problem.ErrConflict
	}
	_, err = q.DeleteWorkflowStageIntegration(ctx, sqlc.DeleteWorkflowStageIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockIntegrationConnection(ctx context.Context, q *sqlc.Queries, c stage.Connection) error {
	current, err := q.LockWorkflowStageIntegration(ctx, c.ID)
	if err != nil {
		return mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	if current.WorkspaceID != c.WorkspaceID || current.Version != c.Version || !current.Enabled {
		return problem.ErrConflict
	}
	return nil
}

func insertIntegrationEvents(ctx context.Context, q *sqlc.Queries, c stage.Connection, events []stage.ProviderEvent) error {
	for _, event := range events {
		subject, err := json.Marshal(event.Subject)
		if err != nil {
			return err
		}
		facts := event.Facts
		if len(facts) == 0 {
			facts = json.RawMessage(`{}`)
		}
		if err = q.InsertWorkflowStageProviderEvent(ctx, sqlc.InsertWorkflowStageProviderEventParams{WorkspaceID: c.WorkspaceID, ConnectionID: c.ID, SemanticID: event.ID, ActionID: event.ActionID, Subject: subject, OccurredAt: pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}, Facts: facts}); err != nil {
			return err
		}
	}
	return nil
}

func (r *WorkflowStageIntegrationPostgresRepository) RecordReceipt(ctx context.Context, c stage.Connection, result stage.CallbackResult) (bool, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationConnection(ctx, q, c); err != nil {
		return false, err
	}
	n, err := q.InsertWorkflowStageReceipt(ctx, sqlc.InsertWorkflowStageReceiptParams{WorkspaceID: c.WorkspaceID, ConnectionID: c.ID, DeliveryID: result.DeliveryID, ConnectionVersion: c.Version})
	if err != nil {
		return false, err
	}
	if n > 0 {
		if err = insertIntegrationEvents(ctx, q, c, result.Events); err != nil {
			return false, err
		}
	}
	return n == 0, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) RecordEvents(ctx context.Context, c stage.Connection, events []stage.ProviderEvent) error {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationConnection(ctx, q, c); err != nil {
		return err
	}
	if err = insertIntegrationEvents(ctx, q, c, events); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func observationFromRow(row sqlc.WorkflowStageIntegrationObservation) (stage.Observation, error) {
	out := stage.Observation{ID: row.ID, TaskID: row.TaskID, RuleID: row.RuleID, StageVersion: row.StageVersion, ActivatedAt: row.ActivatedAt.Time, ReconciledAt: row.ReconciledAt.Time, AttemptedAt: row.AttemptedAt.Time, RevisionIDs: row.RevisionIds, LastError: row.LastError}
	err := json.Unmarshal(row.Subjects, &out.Subjects)
	return out, err
}

func stableIntegrationID(parts ...any) string {
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return "wsi:" + hex.EncodeToString(sum[:])
}

func (r *WorkflowStageIntegrationPostgresRepository) Candidates(ctx context.Context) ([]stage.Candidate, error) {
	tx, err := r.database.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	rows, err := q.ListWorkflowStageCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]stage.Candidate, 0, len(rows))
	for _, row := range rows {
		c := stage.Connection{ID: row.ID, WorkspaceID: row.WorkspaceID, Provider: row.Provider, Name: row.Name, Enabled: row.Enabled, CodingIntegrationID: row.CodingIntegrationID, RepositoryIDs: row.RepositoryIds, Config: row.Config, Version: row.Version, ConfiguredBy: row.ConfiguredBy, UpdatedAt: row.UpdatedAt.Time}
		var trigger workflowmodel.IntegrationTrigger
		if err = json.Unmarshal(row.Integration, &trigger); err != nil {
			return nil, err
		}
		candidate := stage.Candidate{Connection: c, OrganizationID: row.OrganizationID, TaskID: row.TaskID, WorkflowID: row.WorkflowID, StageID: row.StageID, StageVersion: row.StageVersion, ConfigurationVersion: row.ConfigurationVersion, RuleID: row.RuleID, ActorID: c.ConfiguredBy, ActionID: trigger.ActionID, Parameters: trigger.Parameters}
		candidate.ObservationID = stableIntegrationID(c.ID, c.Version, row.TaskID, row.StageVersion, row.ConfigurationVersion, row.RuleID)
		obs, obsErr := q.GetWorkflowStageObservation(ctx, candidate.ObservationID)
		if obsErr == nil {
			mapped, e := observationFromRow(obs)
			if e != nil {
				return nil, e
			}
			candidate.Observation = &mapped
		} else if !errors.Is(obsErr, pgx.ErrNoRows) {
			return nil, obsErr
		}
		if err = r.candidateInputs(ctx, q, &candidate); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) candidateInputs(ctx context.Context, q *sqlc.Queries, c *stage.Candidate) error {
	row, err := q.GetWorkflow(ctx, sqlc.GetWorkflowParams{WorkspaceID: c.Connection.WorkspaceID, ID: c.WorkflowID})
	if err != nil {
		return err
	}
	definition, err := workflowDefinition(ctx, q, row)
	if err != nil {
		return err
	}
	c.ActorID = definition.ConfiguredBy
	c.Definition = definition
	artifacts, err := q.ListTaskArtifacts(ctx, sqlc.ListTaskArtifactsParams{WorkspaceID: c.Connection.WorkspaceID, TaskID: c.TaskID})
	if err != nil {
		return err
	}
	for _, row := range artifacts {
		revision, err := artifactRevision(sqlc.ArtifactPublicationRow(row))
		if err != nil {
			return err
		}
		c.AvailableArtifacts = append(c.AvailableArtifacts, revision)
	}

	repos, err := q.ListWorkflowStageTaskRepositories(ctx, sqlc.ListWorkflowStageTaskRepositoriesParams{WorkspaceID: c.Connection.WorkspaceID, TaskID: c.TaskID})
	if err != nil {
		return err
	}
	for _, repo := range repos {
		c.Repositories = append(c.Repositories, codemodel.CodeRepository{ID: repo.ID, WorkspaceID: repo.WorkspaceID, ProviderName: repo.ProviderName, URL: repo.Url, IntegrationID: repo.IntegrationID.String, ProviderRepositoryID: repo.ProviderRepositoryID.String, DefaultBranch: repo.DefaultBranch, Name: repo.DisplayName, Private: repo.IsPrivate})
	}
	return nil
}

// lockCandidate follows workflow -> connection -> task lock ordering. External
// API calls and authorization checks have already completed outside this scope.
func lockIntegrationCandidate(ctx context.Context, q *sqlc.Queries, c stage.Candidate) error {
	row, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: c.Connection.WorkspaceID, ID: c.WorkflowID})
	if err != nil {
		return err
	}
	if row.Version != c.ConfigurationVersion {
		return problem.ErrConflict
	}
	if err = lockIntegrationConnection(ctx, q, c.Connection); err != nil {
		return err
	}
	task, err := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: c.Connection.WorkspaceID, ID: c.TaskID})
	if err != nil {
		return err
	}
	if task.StageVersion != c.StageVersion || task.Status != c.StageID || task.WorkflowID != c.WorkflowID {
		return problem.ErrConflict
	}
	definition, err := workflowDefinition(ctx, q, row)
	if err != nil {
		return err
	}
	for _, rule := range definition.Rules {
		if rule.ID == c.RuleID && rule.Enabled && rule.Event == workflowmodel.IntegrationAction && rule.SourceStageID == c.StageID && rule.Integration != nil && rule.Integration.ConnectionID == c.Connection.ID && rule.Integration.ActionID == c.ActionID {
			return nil
		}
	}
	return problem.ErrConflict
}

func (r *WorkflowStageIntegrationPostgresRepository) SaveObservation(ctx context.Context, c stage.Candidate, obs stage.Observation) error {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationCandidate(ctx, q, c); err != nil {
		return err
	}
	subjects, err := json.Marshal(obs.Subjects)
	if err != nil {
		return err
	}
	if string(subjects) == "null" {
		subjects = []byte(`[]`)
	}
	_, err = q.InsertWorkflowStageObservation(ctx, sqlc.InsertWorkflowStageObservationParams{ID: c.ObservationID, WorkspaceID: c.Connection.WorkspaceID, ConnectionID: c.Connection.ID, ConnectionVersion: c.Connection.Version, TaskID: c.TaskID, WorkflowID: c.WorkflowID, RuleID: c.RuleID, StageID: c.StageID, StageVersion: c.StageVersion, ConfigurationVersion: c.ConfigurationVersion, ActionID: c.ActionID, ActivatedAt: pgtype.Timestamptz{Time: obs.ActivatedAt, Valid: true}, ReconciledAt: pgtype.Timestamptz{Time: obs.ReconciledAt, Valid: true}, RevisionIds: append([]string{}, obs.RevisionIDs...), Subjects: subjects, LastError: obs.LastError, AttemptedAt: pgtype.Timestamptz{Time: obs.AttemptedAt, Valid: true}})
	if err != nil {
		return err
	}
	stored, err := q.WorkflowStageAdmissionObservation(ctx, sqlc.WorkflowStageAdmissionObservationParams{WorkspaceID: c.Connection.WorkspaceID, ID: c.ObservationID})
	if err != nil {
		return err
	}
	previous, err := observationFromRow(stored)
	if err != nil {
		return err
	}
	if len(previous.Subjects) > 0 {
		if len(previous.Subjects) != len(obs.Subjects) || len(previous.RevisionIDs) != len(obs.RevisionIDs) {
			return problem.ErrConflict
		}
		for i, subject := range previous.Subjects {
			if subject.Kind != obs.Subjects[i].Kind || subject.ID != obs.Subjects[i].ID || subject.RepositoryID != obs.Subjects[i].RepositoryID {
				return problem.ErrConflict
			}
		}
		for i, id := range previous.RevisionIDs {
			if id != obs.RevisionIDs[i] {
				return problem.ErrConflict
			}
		}
		obs.ActivatedAt = previous.ActivatedAt
	}
	if err = q.UpdateWorkflowStageObservation(ctx, sqlc.UpdateWorkflowStageObservationParams{ID: c.ObservationID, ReconciledAt: pgtype.Timestamptz{Time: obs.ReconciledAt, Valid: true}, LastError: obs.LastError, ActivatedAt: pgtype.Timestamptz{Time: obs.ActivatedAt, Valid: true}, RevisionIds: append([]string{}, obs.RevisionIDs...), Subjects: subjects, AttemptedAt: pgtype.Timestamptz{Time: obs.AttemptedAt, Valid: true}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) Events(ctx context.Context, c stage.Candidate, obs stage.Observation) ([]stage.ProviderEvent, error) {
	rows, err := sqlc.New(r.database.pool).ListWorkflowStageProviderEvents(ctx, sqlc.ListWorkflowStageProviderEventsParams{ConnectionID: c.Connection.ID, ActionID: c.ActionID, ActivatedAt: pgtype.Timestamptz{Time: obs.ActivatedAt, Valid: true}, ObservationID: obs.ID})
	if err != nil {
		return nil, err
	}
	events := make([]stage.ProviderEvent, 0, len(rows))
	for _, row := range rows {
		event := stage.ProviderEvent{ID: row.SemanticID, ActionID: row.ActionID, OccurredAt: row.OccurredAt.Time, Facts: row.Facts}
		if err = json.Unmarshal(row.Subject, &event.Subject); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (r *WorkflowStageIntegrationPostgresRepository) AdmitEvent(ctx context.Context, c stage.Candidate, obs stage.Observation, event stage.ProviderEvent, promptContext string) error {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationCandidate(ctx, q, c); err != nil {
		return err
	}
	stored, err := q.WorkflowStageAdmissionObservation(ctx, sqlc.WorkflowStageAdmissionObservationParams{WorkspaceID: c.Connection.WorkspaceID, ID: obs.ID})
	if err != nil {
		return err
	}
	if stored.ID != c.ObservationID || stored.ConnectionVersion != c.Connection.Version || stored.ConfigurationVersion != c.ConfigurationVersion || !event.OccurredAt.After(stored.ActivatedAt.Time) {
		return problem.ErrConflict
	}
	exists, err := q.WorkflowStageAdmissionExists(ctx, sqlc.WorkflowStageAdmissionExistsParams{ObservationID: obs.ID, SemanticID: event.ID})
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	id := stableIntegrationID(obs.ID, event.ID)
	payload, err := json.Marshal(workflowmodel.IntegrationEvent{RuleID: c.RuleID, ConnectionID: c.Connection.ID, ConfigurationVersion: c.ConfigurationVersion, ConnectionVersion: c.Connection.Version, PromptContext: promptContext})
	if err != nil {
		return err
	}
	if err = q.EnqueueWorkflowIntegrationEvent(ctx, sqlc.EnqueueWorkflowIntegrationEventParams{ID: id, WorkspaceID: c.Connection.WorkspaceID, TaskID: c.TaskID, StageVersion: c.StageVersion, ActorID: c.ActorID, CausationID: event.ID, CorrelationID: obs.ID, Integration: payload, OccurredAt: pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}}); err != nil {
		return err
	}
	if err = q.InsertWorkflowStageAdmission(ctx, sqlc.InsertWorkflowStageAdmissionParams{ObservationID: obs.ID, SemanticID: event.ID, EventID: id}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) ListStatus(ctx context.Context, workspaceID, connectionID string) ([]stage.Status, error) {
	q := sqlc.New(r.database.pool)
	observations, err := q.ListWorkflowStageObservations(ctx, sqlc.ListWorkflowStageObservationsParams{WorkspaceID: workspaceID, ConnectionID: connectionID})
	if err != nil {
		return nil, err
	}
	out := make([]stage.Status, 0, len(observations))
	for _, obs := range observations {
		state := "observing"
		if obs.LastError != "" {
			state = "failed"
		}
		out = append(out, stage.Status{ID: obs.ID, TaskID: obs.TaskID, RuleID: obs.RuleID, StageVersion: obs.StageVersion, ActionID: obs.ActionID, State: state, Error: obs.LastError, CreatedAt: obs.CreatedAt.Time})
	}
	effects, err := q.ListWorkflowStageEffectStatus(ctx, sqlc.ListWorkflowStageEffectStatusParams{WorkspaceID: workspaceID, ConnectionID: connectionID})
	if err != nil {
		return nil, err
	}
	for _, effect := range effects {
		var event workflowmodel.IntegrationEvent
		if err = json.Unmarshal(effect.Integration, &event); err != nil {
			return nil, err
		}
		state := "pending"
		if effect.ProcessedAt.Valid {
			state = "processed"
		}
		if effect.ProcessingError != "" {
			state = "blocked"
		}
		diagnostic := effect.ProcessingError
		if effect.CommandState != "" {
			state = effect.CommandState
		}
		if effect.DeliveryError != "" {
			diagnostic = effect.DeliveryError
		}
		out = append(out, stage.Status{ID: effect.ID, TaskID: effect.TaskID, RuleID: event.RuleID, StageVersion: effect.StageVersion, ActionID: effect.ActionID, State: state, Error: diagnostic, CreatedAt: effect.OccurredAt.Time})
	}
	return out, nil
}

func (r *WorkflowStageIntegrationPostgresRepository) Retry(ctx context.Context, workspaceID, eventID string) error {
	n, err := sqlc.New(r.database.pool).RetryWorkflowStageEvent(ctx, sqlc.RetryWorkflowStageEventParams{WorkspaceID: workspaceID, ID: eventID})
	if err != nil {
		return err
	}
	if n != 1 {
		return problem.ErrConflict
	}
	return nil
}
