package model

import (
	"strings"
	"testing"
)

func TestCustomPromptValidation(t *testing.T) {
	for _, tt := range []struct {
		name, prompt string
		valid        bool
	}{
		{"empty", "", true},
		{"multiline", "  Analyse\n  repositories  ", true},
		{"boundary", strings.Repeat("é", MaxCustomPromptBytes/2), true},
		{"oversized", strings.Repeat("é", MaxCustomPromptBytes/2+1), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := DefaultDefinition("Default")
			d.ID = "workflow"
			d.Stages[0].CustomPrompt = tt.prompt
			err := d.Validate()
			if (err == nil) != tt.valid {
				t.Fatal("unexpected validation", err)
			}
			if tt.valid && d.Stages[0].CustomPrompt != strings.TrimSpace(tt.prompt) {
				t.Fatal("formatting not preserved")
			}
		})
	}
}
