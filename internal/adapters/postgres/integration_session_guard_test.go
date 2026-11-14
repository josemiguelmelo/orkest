package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func TestPostgresRepositoriesIntegrationReservationGuards(t *testing.T) {
	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		if os.Getenv("DATABASE_URL") == "" && os.Getenv("POSTGRES_HOST") == "" {
			t.Skip("PostgreSQL connection settings required")
		}
		t.Fatal(err)
	}
	ctx := context.Background()
	database, err := initialize(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	nextID := func() string {
		id, e := ids.NewUUID()
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	user, err := NewUserPostgresRepository(database).RegisterUser(ctx, nextID(), "integration-reservation", "integration-reservation@example.test")
	if err != nil {
		t.Fatal(err)
	}
	defer deleteTestUser(t, database, user.ID)
	org, err := NewOrganizationPostgresRepository(database).CreateOrganization(ctx, nextID(), user.ID, "Integration reservation guards")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = database.pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, org.ID) }()
	workspaceID := nextID()
	if _, err = NewWorkspacePostgresRepository(database).CreateWorkspace(ctx, workspaceID, org.ID, "Guard workspace"); err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(database.pool)
	coding, err := q.InsertCodingIntegration(ctx, sqlc.InsertCodingIntegrationParams{ID: nextID(), WorkspaceID: workspaceID, Name: "GitHub", ProviderName: "github", ProviderConfig: json.RawMessage(`{}`), EncryptedCredential: "fixture-encrypted"})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := q.CreateWorkflowStageIntegration(ctx, sqlc.CreateWorkflowStageIntegrationParams{ID: nextID(), WorkspaceID: workspaceID, Provider: "github_pr", Name: "PRs", Enabled: true, CodingIntegrationID: coding.ID, RepositoryIds: []string{}, Config: json.RawMessage(`{}`), EncryptedSecret: "fixture-encrypted-secret", ConfiguredBy: user.ID})
	if err != nil {
		t.Fatal(err)
	}
	config := orchestrationmodel.RunConfig{RuntimeConfig: orchestrationmodel.RuntimeConfig{OrganizationID: org.ID, WorkspaceID: workspaceID, TaskID: nextID(), RunID: nextID()}, IntegrationWorkflowID: workspaceID, IntegrationActorID: user.ID, IntegrationGuard: &workflowmodel.IntegrationEvent{ConnectionID: connection.ID, ConnectionVersion: connection.Version, ConfigurationVersion: 1}}
	for _, test := range []struct {
		name   string
		change func(*orchestrationmodel.RunConfig)
		want   error
	}{
		{"valid", func(*orchestrationmodel.RunConfig) {}, nil},
		{"edited workflow", func(c *orchestrationmodel.RunConfig) { c.IntegrationGuard.ConfigurationVersion++ }, problem.ErrConflict},
		{"rotated connection", func(c *orchestrationmodel.RunConfig) { c.IntegrationGuard.ConnectionVersion++ }, problem.ErrConflict},
		{"wrong workspace", func(c *orchestrationmodel.RunConfig) { c.WorkspaceID = nextID() }, problem.ErrConflict},
		{"revoked actor", func(c *orchestrationmodel.RunConfig) { c.IntegrationActorID = nextID() }, problem.ErrForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := config
			guard := *config.IntegrationGuard
			candidate.IntegrationGuard = &guard
			test.change(&candidate)
			tx, e := database.pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			e = guardIntegrationConfiguration(ctx, sqlc.New(tx), candidate)
			if !errors.Is(e, test.want) {
				t.Fatalf("guard: got %v, want %v", e, test.want)
			}
		})
	}
	// A connection can be disabled after event planning but before reservation.
	if _, err = database.pool.Exec(ctx, `UPDATE workflow_stage_integrations SET enabled=false WHERE id=$1`, connection.ID); err != nil {
		t.Fatal(err)
	}
	launchRepository := NewAgentLaunchPostgresRepository(database, nil)
	if err = launchRepository.ReserveSessionRun(ctx, config, 0); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("disabled callback reserved a run: %v", err)
	}
	var count int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM agent_launches WHERE run_id=$1`, config.RunID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reservation side effects: count=%d err=%v", count, err)
	}
}
