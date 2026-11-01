package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
)

// ValidateConfiguration rejects keys not declared by the selected provider.
func ValidateConfiguration(runtime Runtime, values map[string]string) error {
	provider, ok := ProviderFor(runtime)
	if !ok {
		return fmt.Errorf("unsupported runtime")
	}
	allowed := make(map[string]struct{})
	for _, option := range provider.ConfigurationSchema() {
		allowed[option.Name] = struct{}{}
	}
	for key, value := range values {
		if _, ok := allowed[key]; !ok || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid runtime configuration")
		}
	}
	return nil
}

// NormalizeConfiguration drops blank values so empty form controls do not
// replace inherited workspace or profile values.
func NormalizeConfiguration(values map[string]string) map[string]string {
	clean := make(map[string]string, len(values))
	for key, value := range values {
		if value != "" {
			clean[key] = value
		}
	}
	return clean
}

// ValidateRequiredEnvironment checks required provider variables after
// workspace and agent/profile values have been combined for a run.
func ValidateRequiredEnvironment(runtime Runtime, values map[string]string) error {
	provider, ok := ProviderFor(runtime)
	if !ok {
		return fmt.Errorf("unsupported runtime")
	}
	for _, option := range provider.ConfigurationSchema() {
		if option.Required && strings.TrimSpace(values[option.Name]) == "" {
			return fmt.Errorf("runtime configuration %s is required", option.Name)
		}
	}
	return nil
}

// SealConfiguration encrypts provider configuration before persistence.
func SealConfiguration(box *secretbox.Box, values map[string]string) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if box == nil {
		return nil, secretbox.ErrKeyUnavailable
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return box.Seal(string(encoded))
}

// OpenConfiguration decrypts persisted provider configuration.
func OpenConfiguration(box *secretbox.Box, ciphertext []byte) (map[string]string, error) {
	if len(ciphertext) == 0 {
		return map[string]string{}, nil
	}
	if box == nil {
		return nil, secretbox.ErrKeyUnavailable
	}
	plain, err := box.Open(ciphertext)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if err = json.Unmarshal([]byte(plain), &values); err != nil {
		return nil, fmt.Errorf("decode runtime configuration: %w", err)
	}
	return values, nil
}
