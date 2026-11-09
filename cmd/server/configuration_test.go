package main

import (
	"os"
	"testing"
)

func TestLoadServerConfigurationGitIdentity(t *testing.T) {
	tests := []struct {
		name, authorName, authorEmail, wantName, wantEmail string
	}{
		{"unset", "", "", "Orkest Agent", "agent@orkest.local"},
		{"empty", "", "", "Orkest Agent", "agent@orkest.local"},
		{"custom", "Build Agent", "build@example.com", "Build Agent", "build@example.com"},
		{"name only", "Build Agent", "", "Build Agent", "agent@orkest.local"},
		{"email only", "", "build@example.com", "Orkest Agent", "build@example.com"},
		{"whitespace", " \t", " \t", "Orkest Agent", "agent@orkest.local"},
		{"trimmed", " Build Agent ", " build@example.com ", "Build Agent", "build@example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ORKEST_GIT_AUTHOR_NAME", test.authorName)
			t.Setenv("ORKEST_GIT_AUTHOR_EMAIL", test.authorEmail)
			if test.name == "unset" {
				for _, name := range []string{"ORKEST_GIT_AUTHOR_NAME", "ORKEST_GIT_AUTHOR_EMAIL"} {
					if err := os.Unsetenv(name); err != nil {
						t.Fatal(err)
					}
				}
			}
			config := loadServerConfiguration()
			if config.GitAuthorName != test.wantName || config.GitAuthorEmail != test.wantEmail {
				t.Fatalf("Git identity = %q <%s>, want %q <%s>", config.GitAuthorName, config.GitAuthorEmail, test.wantName, test.wantEmail)
			}
		})
	}
}
