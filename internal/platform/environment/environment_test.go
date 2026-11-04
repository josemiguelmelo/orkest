package environment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv("AGENTCTL_WORK_DIR", "/from-shell")
	t.Setenv("AGENTCTL_STATE_DIR", "")
	if err := os.Unsetenv("AGENTCTL_STATE_DIR"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# local configuration\nAGENTCTL_WORK_DIR=/from-file\nAGENTCTL_STATE_DIR=\"/state with spaces\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnvironment(path); err != nil {
		t.Fatal(err)
	}
	if got := EnvironmentDefault("AGENTCTL_WORK_DIR", ""); got != "/from-shell" {
		t.Fatalf("existing environment overwritten: %q", got)
	}
	if got := EnvironmentDefault("AGENTCTL_STATE_DIR", ""); got != "/state with spaces" {
		t.Fatalf("dotenv value not loaded: %q", got)
	}
}

func TestLoadEnvironmentMissingAndMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := LoadEnvironment(path); err != nil {
		t.Fatalf("missing .env should be optional: %v", err)
	}
	if err := os.WriteFile(path, []byte("SECRET='sensitive-unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := LoadEnvironment(path)
	if err == nil {
		t.Fatal("malformed .env accepted")
	}
	if strings.Contains(err.Error(), "sensitive") {
		t.Fatal("error leaked file content")
	}
}

func TestEnvironmentDefault(t *testing.T) {
	t.Setenv("CORE_ENVIRONMENT_DEFAULT", "configured")
	if got := EnvironmentDefault("CORE_ENVIRONMENT_DEFAULT", "fallback"); got != "configured" {
		t.Fatalf("got %q, want configured", got)
	}
	if got := EnvironmentDefault("CORE_ENVIRONMENT_DEFAULT_UNSET", "fallback"); got != "fallback" {
		t.Fatalf("got %q, want fallback", got)
	}
}
