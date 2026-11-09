package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/server/workspacemount"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type WorkspaceHostMountsRepository struct{ queries *sqlc.Queries }

var _ workspacemount.WorkspaceMountRepository = (*WorkspaceHostMountsRepository)(nil)

func NewWorkspaceHostMountsRepository(database *PostgresDatabase) *WorkspaceHostMountsRepository {
	return &WorkspaceHostMountsRepository{queries: sqlc.New(database.pool)}
}

func (r *WorkspaceHostMountsRepository) List(ctx context.Context, workspaceID string) ([]mountmodel.Mount, error) {
	encoded, err := r.queries.ListWorkspaceHostMounts(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []mountmodel.Mount{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace host mounts: %w", err)
	}
	var mounts []mountmodel.Mount
	if err := json.Unmarshal(encoded, &mounts); err != nil {
		return nil, fmt.Errorf("decode workspace host mounts: %w", err)
	}
	return mounts, nil
}

func (r *WorkspaceHostMountsRepository) Put(ctx context.Context, workspaceID string, mounts []mountmodel.Mount) error {
	if mounts == nil {
		mounts = []mountmodel.Mount{}
	}
	encoded, err := json.Marshal(mounts)
	if err != nil {
		return fmt.Errorf("encode workspace host mounts: %w", err)
	}
	return r.queries.PutWorkspaceHostMounts(ctx, sqlc.PutWorkspaceHostMountsParams{WorkspaceID: workspaceID, Mounts: encoded})
}
