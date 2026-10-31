package runtimehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	codexAuthFileEnvironment = "ORKEST_CODEX_AUTH_FILE"
	codexAuthFileRelative    = "home/.codex/auth.json"
	maxCodexAuthFileBytes    = 1 << 20
)

func prepareLocalCodexAuthFile(granted []string, runtimeRoot string, restoring bool) ([]string, []byte, error) {
	var source string
	filtered := make([]string, 0, len(granted))
	for _, entry := range granted {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name != codexAuthFileEnvironment {
			filtered = append(filtered, entry)
			continue
		}
		if source != "" {
			return nil, nil, errors.New("duplicate local Codex auth file setting")
		}
		source = value
	}
	if source == "" {
		return filtered, nil, nil
	}
	if !filepath.IsAbs(source) {
		return nil, nil, errors.New("local Codex auth file path must be absolute")
	}
	target := filepath.Join(runtimeRoot, filepath.FromSlash(codexAuthFileRelative))
	if restoring {
		if info, err := os.Lstat(target); err == nil {
			if !info.Mode().IsRegular() {
				return nil, nil, errors.New("saved Codex auth state is not a regular file")
			}
			return filtered, nil, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, nil, errors.New("inspect saved Codex auth state failed")
		}
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, nil, fmt.Errorf("local Codex auth file is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxCodexAuthFileBytes {
		return nil, nil, errors.New("local Codex auth file must be a regular file no larger than 1 MiB")
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return nil, nil, fmt.Errorf("read local Codex auth file: %w", err)
	}
	if len(contents) > maxCodexAuthFileBytes || !json.Valid(contents) {
		return nil, nil, errors.New("local Codex auth file must contain valid JSON no larger than 1 MiB")
	}
	return filtered, contents, nil
}

func codexAuthSecretValues(contents []byte) []string {
	var value any
	if json.Unmarshal(contents, &value) != nil {
		return nil
	}
	secrets := make([]string, 0)
	var visit func(any, string)
	visit = func(current any, key string) {
		switch item := current.(type) {
		case map[string]any:
			for childKey, child := range item {
				visit(child, strings.ToLower(childKey))
			}
		case []any:
			for _, child := range item {
				visit(child, key)
			}
		case string:
			if item != "" && (strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "api_key")) {
				secrets = append(secrets, item)
			}
		}
	}
	visit(value, "")
	sort.Strings(secrets)
	return secrets
}
