package cliauth

import (
	"os"
	"path/filepath"
	"strings"
)

func environmentMap(environment []string, reserved ...string) map[string]string {
	values := make(map[string]string, len(environment)+4)
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		remove := false
		for _, key := range reserved {
			if name == key || strings.HasPrefix(name, key) {
				remove = true
				break
			}
		}
		if !remove {
			values[name] = value
		}
	}
	return values
}

func prependPath(values map[string]string, directory string) []string {
	runtimePath := values["PATH"]
	if strings.TrimSpace(runtimePath) == "" {
		runtimePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	values["PATH"] = directory + string(os.PathListSeparator) + runtimePath
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func writeCLIWrapper(directory, name, script string) error {
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
