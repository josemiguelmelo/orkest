package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/artifact"
)

type ArtifactPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewArtifactPostgresRepository(db *PostgresDatabase) *ArtifactPostgresRepository {
	return &ArtifactPostgresRepository{pool: db.pool, queries: sqlc.New(db.pool)}
}

func artifactRevision(row sqlc.ArtifactPublicationRow) (artifact.Revision, error) {
	value := artifact.Revision{ID: row.ID, ArtifactID: row.ArtifactID, Number: row.Number, Kind: row.Kind, Title: row.Title, OutputName: row.OutputName, CreatedBy: row.CreatedBy, ProducingRunID: row.ProducingRunID.String, StageID: row.StageID, StageVersion: row.StageVersion, CreatedAt: row.CreatedAt.Time}
	err := json.Unmarshal(row.Content, &value.Content)
	return value, err
}

func (r *ArtifactPostgresRepository) List(ctx context.Context, scope artifact.Scope) ([]artifact.Revision, error) {
	rows, err := r.queries.ListTaskArtifacts(ctx, sqlc.ListTaskArtifactsParams{WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID})
	if err != nil {
		return nil, err
	}
	out := make([]artifact.Revision, 0, len(rows))
	for _, row := range rows {
		value, err := artifactRevision(sqlc.ArtifactPublicationRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (r *ArtifactPostgresRepository) Revisions(ctx context.Context, scope artifact.Scope, id string) ([]artifact.Revision, error) {
	rows, err := r.queries.ListArtifactRevisions(ctx, sqlc.ListArtifactRevisionsParams{WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID, ArtifactID: id})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, modelerrors.ErrNotFound
	}
	out := make([]artifact.Revision, 0, len(rows))
	for _, row := range rows {
		value, err := artifactRevision(sqlc.ArtifactPublicationRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (r *ArtifactPostgresRepository) Publish(ctx context.Context, scope artifact.Scope, input artifact.PublishInput) (artifact.Revision, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return artifact.Revision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	state, err := q.LockArtifactTask(ctx, sqlc.LockArtifactTaskParams{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, ID: scope.TaskID})
	if err != nil {
		return artifact.Revision{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	request, err := json.Marshal(input)
	if err != nil {
		return artifact.Revision{}, err
	}
	previous, err := q.ArtifactPublication(ctx, sqlc.ArtifactPublicationParams{WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID, IdempotencyKey: input.IdempotencyKey})
	if err == nil {
		var old artifact.PublishInput
		if json.Unmarshal(previous.Request, &old) != nil {
			return artifact.Revision{}, modelerrors.ErrConflict
		}
		normalized, _ := json.Marshal(old)
		if string(normalized) != string(request) || previous.CreatedBy != scope.UserID || previous.ProducingRunID.String != scope.RunID {
			return artifact.Revision{}, modelerrors.ErrConflict
		}
		return artifactRevision(previous)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return artifact.Revision{}, err
	}
	if scope.RunID != "" {
		version, e := q.ArtifactPublishingRun(ctx, sqlc.ArtifactPublishingRunParams{RunID: scope.RunID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return artifact.Revision{}, e
		}
		if errors.Is(e, pgx.ErrNoRows) || version != state.StageVersion {
			return artifact.Revision{}, modelerrors.ErrConflict
		}
	}
	id := input.ArtifactID
	if id == "" {
		id, err = ids.NewUUID()
		if err != nil {
			return artifact.Revision{}, err
		}
		err = q.CreateArtifact(ctx, sqlc.CreateArtifactParams{ID: id, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID, Kind: input.Kind, OutputName: input.OutputName})
		if err != nil {
			return artifact.Revision{}, err
		}
	}
	current, err := q.LockArtifact(ctx, sqlc.LockArtifactParams{WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID, ID: id})
	if err != nil {
		return artifact.Revision{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if current.Revision != input.ExpectedRevision || current.Kind != input.Kind || current.OutputName != input.OutputName {
		return artifact.Revision{}, modelerrors.ErrConflict
	}
	revisionID, err := ids.NewUUID()
	if err != nil {
		return artifact.Revision{}, err
	}
	content, err := json.Marshal(input.Content)
	if err != nil {
		return artifact.Revision{}, err
	}
	row, err := q.InsertArtifactRevision(ctx, sqlc.InsertArtifactRevisionParams{ID: revisionID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID, ArtifactID: id, Number: current.Revision + 1, Title: input.Title, Content: content, CreatedBy: scope.UserID, ProducingRunID: scope.RunID, StageID: state.Status, StageVersion: state.StageVersion, IdempotencyKey: input.IdempotencyKey, Request: request})
	if err != nil {
		return artifact.Revision{}, err
	}
	if err = q.AdvanceArtifactRevision(ctx, id); err != nil {
		return artifact.Revision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return artifact.Revision{}, err
	}
	return artifactRevision(sqlc.ArtifactPublicationRow{ID: row.ID, ArtifactID: id, Number: row.Number, Kind: input.Kind, Title: row.Title, OutputName: input.OutputName, Content: row.Content, CreatedBy: row.CreatedBy, ProducingRunID: row.ProducingRunID, StageID: row.StageID, StageVersion: row.StageVersion, CreatedAt: row.CreatedAt})
}
