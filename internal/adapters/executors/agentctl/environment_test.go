package agentctl

import (
	"reflect"
	"testing"
)

func TestResolveEnvironmentFiltersAndOverlaysWithoutReordering(t *testing.T) {
	got := ResolveEnvironment(
		[]string{"PATH=/bin", "TOKEN=host", "UNRELATED=value", "ORKEST_CODEX_AUTH_FILE=host-auth"},
		[]string{"PATH", "TOKEN", "ORKEST_CODEX_AUTH_FILE"},
		map[string]string{"TOKEN": "configured", "ORKEST_CODEX_AUTH_FILE": "configured-auth", "NEW": "value"},
		[]string{"ORKEST_CODEX_AUTH_FILE"},
	)
	want := []string{"PATH=/bin", "TOKEN=configured", "NEW=value", "ORKEST_CODEX_AUTH_FILE=configured-auth"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveEnvironment() = %q, want %q", got, want)
	}
}
