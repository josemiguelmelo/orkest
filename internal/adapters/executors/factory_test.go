package executors

import (
	"context"
	"os"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

func TestFactorySelectsEditorCapabilitiesWithoutStartingResources(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   Kind
		editor *EditorConfig
		want   bool
	}{
		{name: "Docker disabled", kind: Docker},
		{name: "Docker editor", kind: Docker, editor: &EditorConfig{Image: "editor:qualified", ReadOnlyImage: "editor:qualified-ro"}, want: true},
		{name: "Local unsupported", kind: Local, editor: &EditorConfig{Image: "editor:qualified", ReadOnlyImage: "editor:qualified-ro"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			selected, err := New(context.Background(), Config{Kind: string(tc.kind), DataRoot: t.TempDir(), AgentctlBinary: binary, Editor: tc.editor})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = selected.Executor.Close() }()
			if (selected.Editor != nil) != tc.want {
				t.Fatalf("editor capability=%v want %v", selected.Editor != nil, tc.want)
			}
			if selected.Editor != nil {
				result, err := selected.Editor.Checkouts.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{ProviderName: "github", RegistrationID: "repo"}})
				if err != nil || len(result.Missing) != 1 {
					t.Fatalf("selected inspector: %+v %v", result, err)
				}
			}
		})
	}
}
