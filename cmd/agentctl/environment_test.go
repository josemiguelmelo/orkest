package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	t.Setenv("AGENTCTL_WORK_DIR", "/from-shell")
	// Ensure variables introduced by the loader are restored after the test.
	t.Setenv("AGENTCTL_STATE_DIR", "")
	if err := os.Unsetenv("AGENTCTL_STATE_DIR"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# local configuration\nAGENTCTL_WORK_DIR=/from-file\nAGENTCTL_STATE_DIR=\"/state with spaces\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnvironment(path); err != nil {
		t.Fatal(err)
	}
	if got := environmentDefault("AGENTCTL_WORK_DIR", ""); got != "/from-shell" {
		t.Fatalf("existing environment overwritten: %q", got)
	}
	if got := environmentDefault("AGENTCTL_STATE_DIR", ""); got != "/state with spaces" {
		t.Fatalf("dotenv value not loaded: %q", got)
	}
}

func TestLoadEnvironmentMissingAndMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := loadEnvironment(path); err != nil {
		t.Fatalf("missing .env should be optional: %v", err)
	}
	if err := os.WriteFile(path, []byte("SECRET='sensitive-unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := loadEnvironment(path)
	if err == nil {
		t.Fatal("malformed .env accepted")
	}
	if strings.Contains(err.Error(), "sensitive") {
		t.Fatal("error leaked file content")
	}
}
