package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	executionprofilemodel "github.com/josemiguelmelo/orkest/internal/server/executionprofile/model"
)

type ExecutionProfileRepository struct {
	db *PostgresDatabase
	q  *sqlc.Queries
}

func NewExecutionProfileRepository(db *PostgresDatabase) *ExecutionProfileRepository {
	return &ExecutionProfileRepository{db: db, q: sqlc.New(db.pool)}
}

func profileModel(p sqlc.ExecutionProfile) executionprofilemodel.Profile {
	return executionprofilemodel.Profile{ID: p.ID, WorkspaceID: p.WorkspaceID, Input: executionprofilemodel.Input{Name: p.Name, Description: p.Description, Runtime: coreruntime.Runtime(p.Runtime), ToolchainImage: p.ToolchainImage}, Generation: p.Generation, Status: p.Status, ExecutionImage: p.ExecutionImage, BuildError: p.BuildError, CreatedAt: p.CreatedAt.Time, UpdatedAt: p.UpdatedAt.Time}
}

func buildModel(b sqlc.ExecutionProfileBuild) executionprofilemodel.Build {
	return executionprofilemodel.Build{RuntimeVersion: b.RuntimeVersion, ResolvedBase: b.ResolvedBase, ResolvedToolchain: b.ResolvedToolchain, ID: b.ID, ProfileID: b.ProfileID, WorkspaceID: b.WorkspaceID, Generation: b.Generation, Runtime: coreruntime.Runtime(b.Runtime), ToolchainImage: b.ToolchainImage, BaseImage: b.BaseImage, Destination: b.Destination, Status: b.Status, ExecutionImage: b.ExecutionImage, BuildError: b.BuildError, Owner: b.Owner, CreatedAt: b.CreatedAt.Time}
}

func (r *ExecutionProfileRepository) List(ctx context.Context, ws string) ([]executionprofilemodel.Profile, error) {
	rows, err := r.q.ListExecutionProfiles(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := make([]executionprofilemodel.Profile, 0, len(rows))
	for _, row := range rows {
		out = append(out, profileModel(row))
	}
	return out, nil
}

func (r *ExecutionProfileRepository) Get(ctx context.Context, ws, id string) (executionprofilemodel.Profile, error) {
	row, err := r.q.GetExecutionProfile(ctx, sqlc.GetExecutionProfileParams{WorkspaceID: ws, ID: id})
	return profileModel(row), mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *ExecutionProfileRepository) Save(ctx context.Context, p executionprofilemodel.Profile, b executionprofilemodel.Build, create bool) (executionprofilemodel.Profile, error) {
	tx, err := r.db.pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	var row sqlc.ExecutionProfile
	if create {
		row, err = q.InsertExecutionProfile(ctx, sqlc.InsertExecutionProfileParams{ID: p.ID, WorkspaceID: p.WorkspaceID, Name: p.Name, Description: p.Description, Runtime: string(p.Runtime), ToolchainImage: p.ToolchainImage})
	} else {
		row, err = q.UpdateExecutionProfile(ctx, sqlc.UpdateExecutionProfileParams{WorkspaceID: p.WorkspaceID, ID: p.ID, Name: p.Name, Description: p.Description, ToolchainImage: p.ToolchainImage, Generation: p.Generation})
	}
	if err != nil {
		return p, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	err = q.InsertExecutionBuild(ctx, sqlc.InsertExecutionBuildParams{ID: b.ID, WorkspaceID: p.WorkspaceID, ProfileID: p.ID, Generation: row.Generation, Runtime: string(b.Runtime), ToolchainImage: b.ToolchainImage, BaseImage: b.BaseImage, Destination: b.Destination, RuntimeVersion: b.RuntimeVersion})
	if err != nil {
		return p, err
	}
	return profileModel(row), tx.Commit(ctx)
}

func (r *ExecutionProfileRepository) Builds(ctx context.Context, ws, id string) ([]executionprofilemodel.Build, error) {
	rows, err := r.q.ListExecutionBuilds(ctx, sqlc.ListExecutionBuildsParams{WorkspaceID: ws, ProfileID: id})
	out := make([]executionprofilemodel.Build, 0, len(rows))
	for _, row := range rows {
		out = append(out, buildModel(row))
	}
	return out, err
}

func (r *ExecutionProfileRepository) Claim(ctx context.Context, owner string) (executionprofilemodel.Build, bool, error) {
	tx, err := r.db.pool.Begin(ctx)
	if err != nil {
		return executionprofilemodel.Build{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	row, err := q.ClaimExecutionBuild(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return executionprofilemodel.Build{}, false, nil
	}
	if err != nil {
		return executionprofilemodel.Build{}, false, err
	}
	if err = q.PromoteExecutionBuild(ctx, sqlc.PromoteExecutionBuildParams{ID: row.ProfileID, Generation: row.Generation, Status: row.Status}); err != nil {
		return executionprofilemodel.Build{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return executionprofilemodel.Build{}, false, err
	}
	return buildModel(row), true, nil
}

func (r *ExecutionProfileRepository) Heartbeat(ctx context.Context, id, owner string) error {
	n, err := r.q.HeartbeatExecutionBuild(ctx, sqlc.HeartbeatExecutionBuildParams{ID: id, Owner: owner})
	if err == nil && n == 0 {
		return modelerrors.ErrConflict
	}
	return err
}

func (r *ExecutionProfileRepository) Finish(ctx context.Context, b executionprofilemodel.Build, status, image, diagnostic string) error {
	tx, err := r.db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	row, err := q.FinishExecutionBuild(ctx, sqlc.FinishExecutionBuildParams{ID: b.ID, Owner: b.Owner, Status: status, ExecutionImage: image, BuildError: diagnostic})
	if err != nil {
		return err
	}
	if err = q.PromoteExecutionBuild(ctx, sqlc.PromoteExecutionBuildParams{ID: row.ProfileID, Generation: row.Generation, Status: status, ExecutionImage: image, BuildError: diagnostic}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ExecutionProfileRepository) PinInputs(ctx context.Context, b executionprofilemodel.Build) error {
	n, err := r.q.PinExecutionBuildInputs(ctx, sqlc.PinExecutionBuildInputsParams{ID: b.ID, Owner: b.Owner, ResolvedToolchain: b.ResolvedToolchain, ResolvedBase: b.ResolvedBase})
	if err == nil && n != 1 {
		return modelerrors.ErrConflict
	}
	return err
}
