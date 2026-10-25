package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/josemiguelmelo/orkest/internal/app/httpapi"
	"github.com/josemiguelmelo/orkest/internal/core/httpserver"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/organization"
	"github.com/josemiguelmelo/orkest/internal/postgres"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

// Run owns the platform HTTP lifecycle; it does not own agent processes.
func Run(ctx context.Context, logger *slog.Logger) error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" && os.Getenv("POSTGRES_PASSWORD") != "" {
		dsn = (&url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword("orkest", os.Getenv("POSTGRES_PASSWORD")),
			Host:     "127.0.0.1:5432",
			Path:     "/orkest",
			RawQuery: "sslmode=disable",
		}).String()
	}
	if dsn == "" {
		return errors.New("DATABASE_URL or POSTGRES_PASSWORD must be configured")
	}
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Close()
	users := identity.NewUserRegistrationService(postgres.NewUserPostgresRepository(store))
	organizations := organization.NewOrganizationService(postgres.NewOrganizationPostgresRepository(store))
	workspaces := workspace.NewWorkspaceService(postgres.NewWorkspacePostgresRepository(store), organizations)
	router, err := httpapi.NewRouter(users, organizations, workspaces)
	if err != nil {
		return err
	}

	server := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return httpserver.Run(ctx, logger, "platform", server)
}
