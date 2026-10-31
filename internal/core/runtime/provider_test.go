package runtime

import "testing"

func TestCodexProvider(t *testing.T) {
	provider, ok := ProviderFor(Codex)
	if !ok {
		t.Fatal("Codex provider is not registered")
	}
	if provider.Version() != "2.3.0" || provider.DefaultImage() != "orkest/agentctl-codex:2.3.0" {
		t.Fatalf("unexpected Codex release: version=%q image=%q", provider.Version(), provider.DefaultImage())
	}
	if err := provider.ValidateDefinition(Definition{ModelProvider: "OpenAI"}); err != nil {
		t.Fatalf("valid OpenAI provider was rejected: %v", err)
	}
	if err := provider.ValidateDefinition(Definition{ModelProvider: "anthropic"}); err == nil {
		t.Fatal("non-OpenAI model provider was accepted")
	}
}
