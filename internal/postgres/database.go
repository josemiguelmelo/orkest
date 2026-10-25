package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/josemiguelmelo/orkest/db/migrations"
	"github.com/pressly/goose/v3"
)

type PostgresDatabase struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*PostgresDatabase, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("configure postgres: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	s := &PostgresDatabase{pool: pool}
	if err = runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *PostgresDatabase) Close() { s.pool.Close() }

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("configure goose: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}
