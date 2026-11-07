// Package form validates the bounded JSON Schema subset supported by user-input forms.
package form

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const MaxBytes = 64 * 1024

var ErrInvalid = errors.New("invalid or unsupported input form")

// Compile rejects shapes the UI cannot render before compiling validation rules.
// References and nested objects are deliberately excluded: forms never fetch schemas.
func Compile(raw json.RawMessage) (*jsonschema.Schema, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return nil, ErrInvalid
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	schema, ok := document.(map[string]any)
	if err != nil || !ok || schema["type"] != "object" {
		return nil, ErrInvalid
	}
	if err := allowedKeys(schema, []string{"type", "properties", "required", "title", "description", "additionalProperties", "$schema"}); err != nil {
		return nil, err
	}
	if dialect, exists := schema["$schema"]; exists {
		name, ok := dialect.(string)
		if !ok || !slices.Contains([]string{"https://json-schema.org/draft/2020-12/schema", "https://json-schema.org/draft/2019-09/schema", "http://json-schema.org/draft-07/schema#"}, name) {
			return nil, ErrInvalid
		}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 || len(properties) > 32 {
		return nil, ErrInvalid
	}
	for name, value := range properties {
		field, ok := value.(map[string]any)
		if !ok || name == "" || len(name) > 200 || validateField(field, false) != nil {
			return nil, ErrInvalid
		}
	}
	// Unexpected answer keys must never be forwarded to a runtime.
	schema["additionalProperties"] = false
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const location = "https://orkest.invalid/input-form.json"
	if err := compiler.AddResource(location, schema); err != nil {
		return nil, ErrInvalid
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return nil, ErrInvalid
	}
	return compiled, nil
}

func validateField(field map[string]any, item bool) error {
	keys := []string{"type", "title", "description", "default", "enum", "enumNames", "oneOf", "minLength", "maxLength", "minimum", "maximum", "format", "items", "minItems", "maxItems", "uniqueItems", "_meta"}
	if allowedKeys(field, keys) != nil || containsSecret(field["_meta"]) {
		return ErrInvalid
	}
	kind, _ := field["type"].(string)
	if !slices.Contains([]string{"string", "boolean", "integer", "number", "array"}, kind) {
		return ErrInvalid
	}
	if value, exists := field["format"]; exists {
		format, ok := value.(string)
		if !ok || !slices.Contains([]string{"date", "time", "date-time", "email", "uri"}, format) {
			return ErrInvalid
		}
	}
	if choices, exists := field["oneOf"]; exists {
		options, ok := choices.([]any)
		if !ok || len(options) == 0 || len(options) > 100 || kind != "string" {
			return ErrInvalid
		}
		for _, option := range options {
			value, ok := option.(map[string]any)
			if !ok || allowedKeys(value, []string{"const", "title", "description"}) != nil {
				return ErrInvalid
			}
			if _, ok := value["const"].(string); !ok {
				return ErrInvalid
			}
		}
	}
	if choices, exists := field["enum"]; exists {
		options, ok := choices.([]any)
		if !ok || len(options) == 0 || len(options) > 100 || kind != "string" {
			return ErrInvalid
		}
		for _, option := range options {
			if _, ok := option.(string); !ok {
				return ErrInvalid
			}
		}
	}
	if kind == "array" {
		items, ok := field["items"].(map[string]any)
		if item || !ok || items["type"] != "string" {
			return ErrInvalid
		}
		return validateField(items, true)
	}
	return nil
}

func allowedKeys(value map[string]any, allowed []string) error {
	for key := range value {
		if !slices.Contains(allowed, key) {
			return ErrInvalid
		}
	}
	return nil
}

func containsSecret(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			if (key == "isSecret" && nested == true) || containsSecret(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if containsSecret(nested) {
				return true
			}
		}
	}
	return false
}

func ValidateAnswer(schema *jsonschema.Schema, content json.RawMessage) error {
	if len(content) == 0 || len(content) > MaxBytes {
		return ErrInvalid
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return ErrInvalid
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("%w: answer does not match the requested fields", ErrInvalid)
	}
	return nil
}
