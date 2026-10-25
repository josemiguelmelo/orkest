package app

import (
	"context"
	"log/slog"
	"net/http"
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

	database, err := postgres.Initialize(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	users := identity.NewUserRegistrationService(postgres.NewUserPostgresRepository(database))
	organizations := organization.NewOrganizationService(postgres.NewOrganizationPostgresRepository(database))
	workspaces := workspace.NewWorkspaceService(postgres.NewWorkspacePostgresRepository(database), organizations)
	router, err := httpapi.NewRouter(users, organizations, workspaces)
	if err != nil {
		return err
	}

	server := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return httpserver.Run(ctx, logger, "platform", server)
}
