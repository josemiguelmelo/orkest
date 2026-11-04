package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConnectionStringFromEnvironmentPrefersURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://url-user:url-password@example.test:6432/url-db")
	t.Setenv("POSTGRES_HOST", "host-from-fields")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_USERNAME", "field-user")
	t.Setenv("POSTGRES_PASSWORD", "field-password")
	t.Setenv("POSTGRES_DB", "field-db")

	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		t.Fatal("connection string selection failed")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("selected DATABASE_URL could not be parsed")
	}
	if config.ConnConfig.Host != "example.test" || config.ConnConfig.Port != 6432 || config.ConnConfig.User != "url-user" || config.ConnConfig.Database != "url-db" {
		t.Fatalf("DATABASE_URL did not take precedence: host=%q port=%d user=%q database=%q", config.ConnConfig.Host, config.ConnConfig.Port, config.ConnConfig.User, config.ConnConfig.Database)
	}
}

func TestConnectionStringFromEnvironmentUsesAllFields(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_HOST", "2001:db8::1")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_USERNAME", "orkest user")
	t.Setenv("POSTGRES_PASSWORD", "secret with spaces")
	t.Setenv("POSTGRES_DB", "orkest database")

	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		t.Fatal("connection string construction failed")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("constructed connection string could not be parsed")
	}
	if config.ConnConfig.Host != "2001:db8::1" || config.ConnConfig.Port != 5433 || config.ConnConfig.User != "orkest user" || config.ConnConfig.Database != "orkest database" || config.ConnConfig.Password != "secret with spaces" {
		t.Fatal("constructed connection string did not preserve the provided settings")
	}
	if got := connectionInfo(config.ConnConfig); got != (PostgresConnectionInfo{Host: "2001:db8::1", Port: 5433, Database: "orkest database", User: "orkest user"}) {
		t.Fatalf("connection info does not match the non-secret settings: %+v", got)
	}
}

func TestConnectionStringFromEnvironmentReportsMissingFields(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_PORT", "")
	t.Setenv("POSTGRES_USERNAME", "")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("POSTGRES_DB", "orkest")

	_, err := connectionStringFromEnvironment()
	if err == nil || err.Error() != "configure PostgreSQL with DATABASE_URL or all of POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USERNAME, POSTGRES_PASSWORD, POSTGRES_DB (missing: POSTGRES_PORT, POSTGRES_USERNAME)" {
		t.Fatalf("unexpected missing-setting error: %v", err)
	}
}
