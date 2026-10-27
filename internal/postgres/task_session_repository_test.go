package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
)

func testTaskSessions(t *testing.T, database *PostgresDatabase, orgID, workspaceID, taskID string) {
	t.Helper()
	ctx := context.Background()
	box, err := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewAgentLaunchPostgresRepository(database, box)
	newID := func() string {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	configs := []runconfig.RunConfig{
		{RuntimeConfig: runconfig.RuntimeConfig{
			OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, TaskSessionID: newID(), RunID: newID(), Runtime: "opencode",
			Agent: runconfig.AgentDefinition{Name: "original", Instructions: "original"}, Model: runconfig.ModelSelection{Provider: "test", ID: "model"},
			MCPServers:  []runconfig.MCPServer{{ID: "mcp", Name: "mcp", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "private"}}},
			Environment: map[string]string{"TOKEN": "environment-secret"},
		}, InitialCommandID: "initial", InitialPrompt: "hello"},
	}
	second := configs[0]
	second.TaskSessionID, second.RunID = newID(), newID()
	configs = append(configs, second)
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := range configs {
		wg.Go(func() {
			if err := repo.ReserveSessionRun(ctx, configs[i]); err == nil {
				results <- i
			}
		})
	}
	wg.Wait()
	close(results)
	winner := -1
	for i := range results {
		if winner != -1 {
			t.Fatal("two executions reserved the same task")
		}
		winner = i
	}
	if winner == -1 {
		t.Fatal("no reservation succeeded")
	}
	config := configs[winner]
	current, exists, err := repo.CurrentSession(ctx, orgID, workspaceID, taskID)
	if err != nil || !exists || current.TaskSessionID != config.TaskSessionID {
		t.Fatal("current session mismatch", err)
	}
	if len(current.MCPServers[0].Headers) != 0 {
		t.Fatal("credentials persisted in session snapshot")
	}
	if current.Environment["TOKEN"] != "environment-secret" {
		t.Fatalf("environment snapshot lost value: %#v", current.Environment)
	}
	var persisted string
	if err := database.pool.QueryRow(ctx, `SELECT configuration::text FROM task_agent_sessions WHERE id=$1`, config.TaskSessionID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, "environment-secret") {
		t.Fatal("plaintext environment value persisted in task session")
	}
	if _, exists, err := repo.CurrentSession(ctx, "other-org", workspaceID, taskID); err != nil || exists {
		t.Fatal("session crossed organization boundary", err)
	}
	if _, err := repo.RecordPrompt(ctx, config.RunID, "initial", "changed"); !errors.Is(err, interaction.ErrCommand) {
		t.Fatal("outbox accepted changed command", err)
	}
	pending, err := repo.PendingPrompts(ctx, config.RunID)
	if err != nil || len(pending) != 1 || pending[0].Text != "hello" {
		t.Fatal("initial prompt not durable", err)
	}
	if err := repo.PromptDelivered(ctx, config.RunID, "initial"); err != nil {
		t.Fatal(err)
	}
	delivered, err := repo.RecordPrompt(ctx, config.RunID, "initial", "hello")
	if err != nil || !delivered {
		t.Fatal("delivery acknowledgement lost", err)
	}
	if err := repo.SetLaunchStatus(ctx, config.RunID, orchestration.LaunchFailed); err != nil {
		t.Fatal(err)
	}
	next := config
	next.RunID = newID()
	next.RestoreSession = true
	next.Environment = map[string]string{"TOKEN": "rotated-secret"}
	if err := repo.ReserveSessionRun(ctx, next); err != nil {
		t.Fatal("session could not continue in a new run", err)
	}
	current, exists, err = repo.CurrentSession(ctx, orgID, workspaceID, taskID)
	if err != nil || !exists || current.Environment["TOKEN"] != "rotated-secret" {
		t.Fatalf("updated run environment snapshot mismatch: %#v, %v", current.Environment, err)
	}
	launches, err := repo.ListTaskLaunches(ctx, orgID, workspaceID, taskID)
	if err != nil || len(launches) != 2 || launches[0].TaskSessionID != config.TaskSessionID {
		t.Fatal("run/session linkage lost", err)
	}
	if err := repo.SetLaunchStatus(ctx, next.RunID, orchestration.LaunchFailed); err != nil {
		t.Fatal(err)
	}
}
