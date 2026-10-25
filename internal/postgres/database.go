package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/josemiguelmelo/orkest/db/migrations"
	"github.com/pressly/goose/v3"
)

type PostgresDatabase struct {
	pool           *pgxpool.Pool
	ConnectionInfo PostgresConnectionInfo
}

// PostgresConnectionInfo contains non-secret details for the active connection.
type PostgresConnectionInfo struct {
	Host     string
	Port     uint16
	Database string
	User     string
}

// Initialize configures the connection pool, verifies connectivity, and applies migrations.
func Initialize(ctx context.Context) (*PostgresDatabase, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" && os.Getenv("POSTGRES_PASSWORD") != "" {
		port := os.Getenv("POSTGRES_PORT")
		if port == "" {
			port = "5432"
		}
		dsn = (&url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword("orkest", os.Getenv("POSTGRES_PASSWORD")),
			Host:     "127.0.0.1:" + port,
			Path:     "/orkest",
			RawQuery: "sslmode=disable",
		}).String()
	}
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL or POSTGRES_PASSWORD must be configured")
	}
	return initialize(ctx, dsn)
}

func initialize(ctx context.Context, dsn string) (*PostgresDatabase, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("configure postgres: %w", err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err = runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	connectionConfig := pool.Config().ConnConfig
	return &PostgresDatabase{
		pool: pool,
		ConnectionInfo: PostgresConnectionInfo{
			Host:     connectionConfig.Host,
			Port:     connectionConfig.Port,
			Database: connectionConfig.Database,
			User:     connectionConfig.User,
		},
	}, nil
}

func (database *PostgresDatabase) Close() {
	database.pool.Close()
}

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
