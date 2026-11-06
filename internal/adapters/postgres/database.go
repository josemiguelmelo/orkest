package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
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
	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		return nil, err
	}
	return initialize(ctx, dsn)
}

func connectionStringFromEnvironment() (string, error) {
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		return dsn, nil
	}

	values := map[string]string{
		"POSTGRES_HOST":     os.Getenv("POSTGRES_HOST"),
		"POSTGRES_PORT":     os.Getenv("POSTGRES_PORT"),
		"POSTGRES_USERNAME": os.Getenv("POSTGRES_USERNAME"),
		"POSTGRES_PASSWORD": os.Getenv("POSTGRES_PASSWORD"),
		"POSTGRES_DB":       os.Getenv("POSTGRES_DB"),
	}
	missing := make([]string, 0, len(values))
	for _, name := range []string{"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USERNAME", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if values[name] == "" || (name != "POSTGRES_PASSWORD" && strings.TrimSpace(values[name]) == "") {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("configure PostgreSQL with DATABASE_URL or all of POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USERNAME, POSTGRES_PASSWORD, POSTGRES_DB (missing: %s)", strings.Join(missing, ", "))
	}

	return (&url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(strings.TrimSpace(values["POSTGRES_USERNAME"]), values["POSTGRES_PASSWORD"]),
		Host:   net.JoinHostPort(strings.TrimSpace(values["POSTGRES_HOST"]), strings.TrimSpace(values["POSTGRES_PORT"])),
		Path:   "/" + strings.TrimSpace(values["POSTGRES_DB"]),
	}).String(), nil
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
	return &PostgresDatabase{
		pool:           pool,
		ConnectionInfo: connectionInfo(pool.Config().ConnConfig),
	}, nil
}

func connectionInfo(config *pgx.ConnConfig) PostgresConnectionInfo {
	return PostgresConnectionInfo{
		Host:     config.Host,
		Port:     config.Port,
		Database: config.Database,
		User:     config.User,
	}
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
