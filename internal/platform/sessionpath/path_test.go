package sessionpath

import (
	"path/filepath"
	"testing"
)

func TestPersistedSessionLayout(t *testing.T) {
	task := filepath.Join("/state", "961e0f4564a3db8eeae39c31bea4d9026bb33bc064377d6252b33e63ee4224e4")
	want := filepath.Join(task, "sessions", "3f3af1ecebbd1410ab417ec0d27bbfcb5d340e177ae159b59fc8626c2dfd9175")
	if got := Root("/state", "org", "workspace", "task", "session"); got != want {
		t.Fatalf("persisted session path changed: %s", got)
	}
	if got := Root("/state", "org", "workspace", "task", ""); got != task {
		t.Fatalf("legacy task path changed: %s", got)
	}
	if got := Home("/state", "org", "workspace", "task", "session"); got != filepath.Join(want, "home") {
		t.Fatalf("incorrect session home: %s", got)
	}
	for _, scope := range [][4]string{
		{"other", "workspace", "task", "session"},
		{"org", "other", "task", "session"},
		{"org", "workspace", "other", "session"},
		{"org", "workspace", "task", "other"},
	} {
		if got := Root("/state", scope[0], scope[1], scope[2], scope[3]); got == want {
			t.Fatal("storage paths crossed scope boundaries")
		}
	}
}
