package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	stage "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
)

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

func (r *WorkflowStageIntegrationPostgresRepository) Candidates(ctx context.Context, now time.Time) ([]stage.Candidate, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
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
	type scopedID struct{ WorkspaceID, ID string }
	definitions := map[scopedID]workflowmodel.Definition{}
	artifacts := map[scopedID][]artifactmodel.Revision{}
	repositories := map[scopedID][]codemodel.CodeRepository{}
	for _, row := range rows {
		c := integrationConnection(row.WorkflowStageIntegration).Connection
		workflowKey := scopedID{WorkspaceID: c.WorkspaceID, ID: row.WorkflowID}
		taskKey := scopedID{WorkspaceID: c.WorkspaceID, ID: row.TaskID}
		var trigger workflowmodel.IntegrationTrigger
		if err = json.Unmarshal(row.Integration, &trigger); err != nil {
			return nil, err
		}
		candidate := stage.Candidate{Connection: c, OrganizationID: row.OrganizationID, TaskID: row.TaskID, WorkflowID: row.WorkflowID, StageID: row.StageID, StageVersion: row.StageVersion, ConfigurationVersion: row.ConfigurationVersion, RuleID: row.RuleID, ActorID: row.ActorID, ActionID: trigger.ActionID, Parameters: trigger.Parameters}
		candidate.ObservationID = stableIntegrationID(c.ID, c.Version, row.TaskID, row.StageVersion, row.ConfigurationVersion, row.RuleID)
		if row.ObservationID != "" {
			observation := stage.Observation{ID: row.ObservationID, TaskID: row.TaskID, RuleID: row.RuleID, StageVersion: row.StageVersion, ActivatedAt: row.ActivatedAt.Time, ReconciledAt: row.ReconciledAt.Time, AttemptedAt: row.AttemptedAt.Time, RevisionIDs: row.RevisionIds, LastError: row.LastError}
			if err = json.Unmarshal(row.Subjects, &observation.Subjects); err != nil {
				return nil, err
			}
			candidate.Observation = &observation
		}
		// Defer unresolved inputs still in backoff to the next scan. Do not
		// return an incomplete candidate that could become due during a slow scan.
		if candidate.Observation != nil && len(candidate.Observation.Subjects) == 0 && !candidate.Observation.AttemptedAt.IsZero() && now.Sub(candidate.Observation.AttemptedAt) < 10*time.Second {
			continue
		}
		// Pinned observations never resolve artifacts again.
		if candidate.Observation == nil || len(candidate.Observation.Subjects) == 0 {
			definition, ok := definitions[workflowKey]
			if !ok {
				workflowRow, e := q.GetWorkflow(ctx, sqlc.GetWorkflowParams{WorkspaceID: c.WorkspaceID, ID: row.WorkflowID})
				if e != nil {
					return nil, e
				}
				definition, err = workflowDefinition(ctx, q, workflowRow)
				if err != nil {
					return nil, err
				}
				definitions[workflowKey] = definition
			}
			candidate.Definition = definition
			available, ok := artifacts[taskKey]
			if !ok {
				available = []artifactmodel.Revision{}
				artifactRows, e := q.ListTaskArtifacts(ctx, sqlc.ListTaskArtifactsParams{WorkspaceID: c.WorkspaceID, TaskID: row.TaskID})
				if e != nil {
					return nil, e
				}
				for _, artifact := range artifactRows {
					revision, e := artifactRevision(sqlc.ArtifactPublicationRow(artifact))
					if e != nil {
						return nil, e
					}
					available = append(available, revision)
				}
				artifacts[taskKey] = available
			}
			candidate.AvailableArtifacts = available
		}
		repos, ok := repositories[taskKey]
		if !ok {
			repos = []codemodel.CodeRepository{}
			repoRows, e := q.ListWorkflowStageTaskRepositories(ctx, sqlc.ListWorkflowStageTaskRepositoriesParams{WorkspaceID: c.WorkspaceID, TaskID: row.TaskID})
			if e != nil {
				return nil, e
			}
			for _, repo := range repoRows {
				repos = append(repos, codemodel.CodeRepository{ID: repo.ID, WorkspaceID: repo.WorkspaceID, IntegrationID: repo.IntegrationID.String, ProviderRepositoryID: repo.ProviderRepositoryID.String})
			}
			repositories[taskKey] = repos
		}
		candidate.Repositories = repos
		out = append(out, candidate)
	}
	return out, tx.Commit(ctx)
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
	tx, err := r.pool.Begin(ctx)
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
