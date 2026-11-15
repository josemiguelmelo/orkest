package agentctl

import (
	"sort"
	"strings"
)

// ResolveEnvironment applies the shared precedence rule: selected host values
// provide defaults, then explicit run configuration overrides them. Exclusions
// apply only to inherited host values so an explicit binding remains usable.
func ResolveEnvironment(hostEnvironment, allowedHostNames []string, overrides map[string]string, excludedHostNames []string) []string {
	allowed := make(map[string]struct{}, len(allowedHostNames))
	for _, name := range allowedHostNames {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	excluded := make(map[string]struct{}, len(excludedHostNames))
	for _, name := range excludedHostNames {
		excluded[strings.TrimSpace(name)] = struct{}{}
	}
	values := make([]string, 0, len(allowed)+len(overrides))
	positions := make(map[string]int, len(allowed)+len(overrides))
	for _, entry := range hostEnvironment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok = allowed[name]; !ok {
			continue
		}
		if _, blocked := excluded[name]; blocked {
			continue
		}
		if _, duplicate := positions[name]; duplicate {
			continue
		}
		positions[name] = len(values)
		values = append(values, entry)
	}
	overrideNames := make([]string, 0, len(overrides))
	for name := range overrides {
		overrideNames = append(overrideNames, name)
	}
	sort.Strings(overrideNames)
	for _, name := range overrideNames {
		value := overrides[name]
		entry := name + "=" + value
		if index, exists := positions[name]; exists {
			values[index] = entry
			continue
		}
		positions[name] = len(values)
		values = append(values, entry)
	}
	return values
}
