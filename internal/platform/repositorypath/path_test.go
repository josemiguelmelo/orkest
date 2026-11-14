package repositorypath

import (
	"path/filepath"
	"testing"
)

func TestRelativeValidatesIdentityAndPreservesLayout(t *testing.T) {
	path, err := Relative("github", "registration-123")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("repositories", "github", "registration-123"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	for _, identity := range []string{"", ".", "..", "../escape", "a/b"} {
		if _, err := Relative("github", identity); err == nil {
			t.Errorf("Relative accepted identity %q", identity)
		}
	}
	for _, provider := range []string{"", ".", "..", "../escape", "a/b"} {
		if _, err := Relative(provider, "registration"); err == nil {
			t.Errorf("Relative accepted provider %q", provider)
		}
	}
}
