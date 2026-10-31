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

func TestGitHubCopilotProvider(t *testing.T) {
	provider, ok := ProviderFor(GitHubCopilot)
	if !ok {
		t.Fatal("GitHub Copilot provider is not registered")
	}
	if provider.Version() != "1.0.105" || provider.DefaultImage() != "orkest/agentctl-github-copilot:1.0.105" {
		t.Fatalf("unexpected GitHub Copilot release: version=%q image=%q", provider.Version(), provider.DefaultImage())
	}
	if err := provider.ValidateDefinition(Definition{ModelProvider: "GitHub-Copilot"}); err != nil {
		t.Fatalf("valid GitHub Copilot provider was rejected: %v", err)
	}
	if err := provider.ValidateDefinition(Definition{ModelProvider: "openai"}); err == nil {
		t.Fatal("non-Copilot model provider was accepted")
	}
}
