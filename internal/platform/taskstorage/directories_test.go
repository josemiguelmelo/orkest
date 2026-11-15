package taskstorage

import (
	"path/filepath"
	"testing"
)

func TestDirectoriesPreservesHistoricalScopeHash(t *testing.T) {
	work, state := Directories("/data", "org", "workspace", "task")
	base := filepath.Join("/data", "tasks", "961e0f4564a3db8eeae39c31bea4d9026bb33bc064377d6252b33e63ee4224e4")
	if work != filepath.Join(base, "work") || state != filepath.Join(base, "agentctl-state") {
		t.Fatalf("unexpected task paths: %q %q", work, state)
	}
}

func TestDirectoriesEscapesScopeAndSeparatesTasks(t *testing.T) {
	workA, _ := Directories("/data", `org"one`, "workspace", "task")
	workB, _ := Directories("/data", "org", "workspace", "task")
	workC, _ := Directories("/data", `org"one`, "workspace", "other")
	workD, _ := Directories("/data", `org"one`, "different-workspace", "task")
	base := filepath.Join("/data", "tasks", "e59adfab54aca57d1887f8b8586e4ffa2aeb17663528802f1eb69bdbe620ee5c", "work")
	if workA != base {
		t.Fatalf("escaped scope path = %q, want %q", workA, base)
	}
	if workA == workB || workA == workC || workA == workD {
		t.Fatal("scope values did not isolate storage paths")
	}
}
