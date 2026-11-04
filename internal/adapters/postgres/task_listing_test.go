package postgres

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/task"
)

type taskQueryCounter struct {
	sqlc.DBTX
	queries int
}

func (c *taskQueryCounter) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.DBTX.Query(ctx, sql, args...)
}

func testTaskListingUsesBoundedScopedQueries(t *testing.T, db *PostgresDatabase, orgID, workspaceID, taskID string) {
	t.Helper()
	ctx := context.Background()
	repository := NewTaskPostgresRepository(db)
	expected, err := repository.Task(ctx, orgID, workspaceID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	counter := &taskQueryCounter{DBTX: db.pool}
	repository.queries = sqlc.New(counter)
	verify := func() {
		counter.queries = 0
		listed, err := repository.Tasks(ctx, orgID, workspaceID)
		if err != nil {
			t.Fatal(err)
		}
		if counter.queries != 2 {
			t.Fatalf("task listing used %d queries", counter.queries)
		}
		found := false
		for _, value := range listed {
			if value.ID == taskID {
				found = true
				if !reflect.DeepEqual(value.Repositories, expected.Repositories) {
					t.Fatalf("repository projection/order changed: %+v", value.Repositories)
				}
			}
		}
		if !found {
			t.Fatal("task missing from listing")
		}
	}
	verify()
	for range 5 {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = createWorkflowTask(ctx, repository, id, workspaceID, task.TaskInput{WorkflowID: workspaceID, Title: "Additional listing task"}); err != nil {
			t.Fatal(err)
		}
	}
	verify()
	for _, scope := range [][2]string{{"foreign-org", workspaceID}, {orgID, "foreign-workspace"}} {
		values, err := repository.Tasks(ctx, scope[0], scope[1])
		if err != nil || len(values) != 0 {
			t.Fatalf("foreign scope leaked tasks: %v %v", values, err)
		}
	}
}
