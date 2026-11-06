package runtime

import (
	"fmt"
	"strings"
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
