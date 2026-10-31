package runtimehost

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestPrepareLocalCodexAuthFileCopiesAndRemovesMountPath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "auth.json")
	want := []byte(`{"tokens":{"access_token":"local-token"}}`)
	if err := os.WriteFile(source, want, 0o600); err != nil {
		t.Fatal(err)
	}
	granted, contents, err := prepareLocalCodexAuthFile([]string{"KEEP=value", codexAuthFileEnvironment + "=" + source}, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(granted, []string{"KEEP=value"}) {
		t.Fatalf("runtime environment retained mount path: %v", granted)
	}
	if string(contents) != string(want) {
		t.Fatalf("copied auth contents = %q, want %q", contents, want)
	}
	plan := agentruntime.ProvisioningPlan{Directories: []string{"home/.codex"}, Files: map[string][]byte{codexAuthFileRelative: contents}}
	if _, err := applyProvisioningPlan(root, nil, nil, nil, plan); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(codexAuthFileRelative)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth file permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestPrepareLocalCodexAuthFilePreservesRestoredState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, filepath.FromSlash(codexAuthFileRelative))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"tokens":{"access_token":"rotated"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	granted, contents, err := prepareLocalCodexAuthFile([]string{codexAuthFileEnvironment + "=/mount/auth.json"}, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 0 || contents != nil {
		t.Fatalf("restoration should use saved state, got granted=%v contents=%q", granted, contents)
	}
}

func TestPrepareLocalCodexAuthFileRejectsInvalidSettings(t *testing.T) {
	for _, entry := range []string{codexAuthFileEnvironment + "=relative/auth.json", codexAuthFileEnvironment + "=/missing/auth.json"} {
		if _, _, err := prepareLocalCodexAuthFile([]string{entry}, t.TempDir(), false); err == nil {
			t.Fatalf("expected invalid auth file setting error for %q", entry)
		}
	}
	if _, _, err := prepareLocalCodexAuthFile([]string{codexAuthFileEnvironment + "=/one", codexAuthFileEnvironment + "=/two"}, t.TempDir(), false); err == nil {
		t.Fatal("expected duplicate auth file setting error")
	}
}

func TestCodexAuthSecretValuesCollectTokenFieldsOnly(t *testing.T) {
	got := codexAuthSecretValues([]byte(`{"tokens":{"access_token":"access","refresh_token":"refresh"},"accountId":"account","api_key":"api"}`))
	want := []string{"access", "api", "refresh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex auth secrets = %v, want %v", got, want)
	}
}
