package form

import (
	"encoding/json"
	"testing"
)

func TestFormValidation(t *testing.T) {
	schema, err := Compile(json.RawMessage(`{"type":"object","properties":{"manager":{"type":"string","oneOf":[{"const":"pnpm","title":"pnpm"},{"const":"npm","title":"npm"}]},"manager_custom":{"type":"string"},"targets":{"type":"array","items":{"type":"string","enum":["web","server"]},"minItems":1,"uniqueItems":true},"count":{"type":"integer","minimum":1}},"required":["manager","targets"]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, answer string
		valid        bool
	}{
		{"choices", `{"manager":"pnpm","targets":["web"],"count":2}`, true},
		{"custom", `{"manager":"npm","manager_custom":"bun","targets":["server"]}`, true},
		{"missing", `{"manager":"npm"}`, false},
		{"unknown option", `{"manager":"yarn","targets":["web"]}`, false},
		{"duplicate", `{"manager":"npm","targets":["web","web"]}`, false},
		{"constraint", `{"manager":"npm","targets":["web"],"count":0}`, false},
		{"extra", `{"manager":"npm","targets":["web"],"injected":"value"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateAnswer(schema, json.RawMessage(test.answer)); (err == nil) != test.valid {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

func TestUnsupportedFormsFailBeforeDisplay(t *testing.T) {
	for _, raw := range []string{
		`{"type":"object","properties":{"nested":{"type":"object","properties":{}}}}`,
		`{"type":"object","properties":{"value":{"$ref":"https://example.test/schema"}}}`,
		`{"type":"object","properties":{"value":{"type":"string","pattern":".*"}}}`,
		`{"type":"object","properties":{"value":{"type":"string","_meta":{"codex":{"isSecret":true}}}}}`,
		`{"type":"object","properties":{"value":{"type":"array","items":{"type":"array","items":{"type":"string"}}}}}`,
	} {
		if _, err := Compile(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted unsupported form: %s", raw)
		}
	}
}
