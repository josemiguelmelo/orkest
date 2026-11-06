package codex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestPreparationCopiesAuthAndSelectsChatGPT(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "auth.json")
	want := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"local-token"}}`)
	if err := os.WriteFile(source, want, 0o600); err != nil {
		t.Fatal(err)
	}
	input := runtimehost.RuntimePreparationInput{
		Paths: agentruntime.ProvisioningPaths{RuntimeRoot: root},
		Plan: agentruntime.ProvisioningPlan{
			Directories: []string{"home/.codex"},
			Environment: []string{"NO_BROWSER=1", "DEFAULT_AUTH_REQUEST={}"},
		},
		GrantedEnvironment: []string{"KEEP=value", authFileEnvironment + "=" + source},
	}
	result, err := (Preparation{}).Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.GrantedEnvironment, []string{"KEEP=value"}) {
		t.Fatalf("runtime environment retained auth source path: %v", result.GrantedEnvironment)
	}
	if string(result.Plan.Files[authFileRelative]) != string(want) {
		t.Fatalf("auth file in plan = %q, want %q", result.Plan.Files[authFileRelative], want)
	}
	if !reflect.DeepEqual(result.Plan.Environment, []string{`DEFAULT_AUTH_REQUEST={"methodId":"chat-gpt"}`}) {
		t.Fatalf("plan environment = %v", result.Plan.Environment)
	}
	if !reflect.DeepEqual(result.SecretValues, []string{"local-token"}) {
		t.Fatalf("secret values = %v", result.SecretValues)
	}
}

func TestPreparationPreservesSavedAuthOnRestore(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, filepath.FromSlash(authFileRelative))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"saved"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (Preparation{}).Prepare(context.Background(), runtimehost.RuntimePreparationInput{
		Paths:              agentruntime.ProvisioningPaths{RuntimeRoot: root},
		Plan:               agentruntime.ProvisioningPlan{Environment: []string{"NO_BROWSER=1"}},
		GrantedEnvironment: []string{authFileEnvironment + "=/missing/auth.json"},
		Restoring:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"auth_mode":"chatgpt","tokens":{"access_token":"saved"}}` {
		t.Fatalf("saved auth was overwritten: %s", got)
	}
	if len(result.Plan.Files) != 0 || !reflect.DeepEqual(result.SecretValues, []string{"saved"}) {
		t.Fatalf("restore result unexpectedly replaced saved auth: %+v", result)
	}
}

func TestPreparationRejectsInvalidAuthSettings(t *testing.T) {
	for _, entry := range []string{authFileEnvironment + "=relative/auth.json", authFileEnvironment + "=/missing/auth.json"} {
		_, err := (Preparation{}).Prepare(context.Background(), runtimehost.RuntimePreparationInput{
			Paths:              agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir()},
			GrantedEnvironment: []string{entry},
		})
		if err == nil {
			t.Fatalf("expected invalid auth setting error for %q", entry)
		}
	}
	_, err := (Preparation{}).Prepare(context.Background(), runtimehost.RuntimePreparationInput{
		Paths:              agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir()},
		GrantedEnvironment: []string{authFileEnvironment + "=/one", authFileEnvironment + "=/two"},
	})
	if err == nil {
		t.Fatal("expected duplicate auth setting error")
	}
}

func TestAuthSecretValuesCollectSensitiveFields(t *testing.T) {
	got := authSecretValues([]byte(`{"tokens":{"access_token":"access","refresh_token":"refresh"},"accountId":"account","api_key":"api"}`))
	want := []string{"access", "api", "refresh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Codex auth secrets = %v, want %v", got, want)
	}
}
