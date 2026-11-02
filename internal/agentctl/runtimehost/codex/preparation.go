// Package codex provides Codex-specific host preparation for runtimehost.
package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
)

const (
	authFileEnvironment = "ORKEST_CODEX_AUTH_FILE"
	authFileRelative    = "home/.codex/auth.json"
	maxAuthFileBytes    = 1 << 20
)

// Preparation copies opt-in host Codex credentials into the private runtime
// home and configures the ACP auth method from the saved auth mode.
type Preparation struct{}

func (Preparation) Prepare(ctx context.Context, input runtimehost.RuntimePreparationInput) (runtimehost.RuntimePreparationResult, error) {
	if err := ctx.Err(); err != nil {
		return runtimehost.RuntimePreparationResult{}, err
	}
	granted, copiedAuth, err := prepareAuthFile(input.GrantedEnvironment, input.Paths.RuntimeRoot, input.Restoring)
	if err != nil {
		return runtimehost.RuntimePreparationResult{}, err
	}
	plan := input.Plan
	if copiedAuth != nil {
		if plan.Files == nil {
			plan.Files = make(map[string][]byte)
		}
		plan.Files[authFileRelative] = copiedAuth
	}
	authForMode := copiedAuth
	if authForMode == nil {
		authForMode, err = readSavedAuthFile(input.Paths.RuntimeRoot)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return runtimehost.RuntimePreparationResult{}, &agentruntime.LaunchError{Message: "read saved Codex auth state failed", Cause: err}
		}
	}
	if authMode(authForMode) == "chatgpt" {
		plan.Environment = replaceEnvironmentValue(plan.Environment, "DEFAULT_AUTH_REQUEST", `{"methodId":"chat-gpt"}`)
		plan.Environment = removeEnvironmentValue(plan.Environment, "NO_BROWSER")
	}
	return runtimehost.RuntimePreparationResult{
		Plan:               plan,
		GrantedEnvironment: granted,
		SecretValues:       authSecretValues(authForMode),
	}, nil
}

func prepareAuthFile(granted []string, runtimeRoot string, restoring bool) ([]string, []byte, error) {
	var source string
	filtered := make([]string, 0, len(granted))
	for _, entry := range granted {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name != authFileEnvironment {
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
	target := filepath.Join(runtimeRoot, filepath.FromSlash(authFileRelative))
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
	if !info.Mode().IsRegular() || info.Size() > maxAuthFileBytes {
		return nil, nil, errors.New("local Codex auth file must be a regular file no larger than 1 MiB")
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return nil, nil, fmt.Errorf("read local Codex auth file: %w", err)
	}
	if len(contents) > maxAuthFileBytes || !json.Valid(contents) {
		return nil, nil, errors.New("local Codex auth file must contain valid JSON no larger than 1 MiB")
	}
	return filtered, contents, nil
}

func readSavedAuthFile(runtimeRoot string) ([]byte, error) {
	return os.ReadFile(filepath.Join(runtimeRoot, filepath.FromSlash(authFileRelative)))
}

func authMode(contents []byte) string {
	var auth struct {
		Mode string `json:"auth_mode"`
	}
	if json.Unmarshal(contents, &auth) != nil {
		return ""
	}
	return auth.Mode
}

func authSecretValues(contents []byte) []string {
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

func replaceEnvironmentValue(environment []string, name, value string) []string {
	result := removeEnvironmentValue(environment, name)
	return append(result, name+"="+value)
}

func removeEnvironmentValue(environment []string, name string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if key != name {
			result = append(result, entry)
		}
	}
	return result
}
