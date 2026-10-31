package runtimehost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// applyProvisioningPlan creates only relative paths beneath runtimeRoot and
// merges environment values after rejecting collisions with reserved names.
func applyProvisioningPlan(runtimeRoot string, granted, reservedNames, reservedPrefixes []string, plan agentruntime.ProvisioningPlan) ([]string, error) {
	if !filepath.IsAbs(runtimeRoot) {
		return nil, errors.New("runtime root must be absolute")
	}
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "create runtime configuration failed", Cause: err}
	}
	for _, directory := range plan.Directories {
		path, err := safeProvisionPath(runtimeRoot, directory)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "create runtime configuration failed", Cause: err}
		}
	}
	for name, contents := range plan.Files {
		path, err := safeProvisionPath(runtimeRoot, name)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "create runtime configuration failed", Cause: err}
		}
		if err := writeProvisionedFile(path, contents); err != nil {
			return nil, err
		}
	}
	return mergeEnvironment(granted, plan.Environment, reservedNames, reservedPrefixes)
}

// mergeEnvironment validates the explicitly granted and runtime-owned values
// without reading or changing the process environment.
func mergeEnvironment(granted, runtimeEnv, reservedNames, reservedPrefixes []string) ([]string, error) {
	env := make([]string, 0, len(granted)+len(runtimeEnv))
	seen := make(map[string]bool)
	for _, entry := range granted {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !validEnvironmentName(key) || strings.ContainsRune(entry, '\x00') {
			return nil, errors.New("invalid granted environment variable")
		}
		for _, name := range reservedNames {
			if key == name {
				return nil, errors.New("reserved environment variable")
			}
		}
		for _, prefix := range reservedPrefixes {
			if strings.HasPrefix(key, prefix) {
				return nil, errors.New("reserved environment variable")
			}
		}
		if seen[key] {
			return nil, errors.New("duplicate environment variable")
		}
		seen[key] = true
		env = append(env, entry)
	}
	for _, entry := range runtimeEnv {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !validEnvironmentName(key) || strings.ContainsRune(entry, '\x00') {
			return nil, errors.New("invalid runtime environment plan")
		}
		if key == "PATH" && seen[key] {
			continue
		}
		if seen[key] {
			return nil, errors.New("invalid runtime environment plan")
		}
		seen[key] = true
		env = append(env, entry)
	}
	return env, nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index, value := range name {
		valid := value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
		if index > 0 {
			valid = valid || value >= '0' && value <= '9'
		}
		if !valid {
			return false
		}
	}
	return true
}

func safeProvisionPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid runtime provisioning path")
	}
	rootPath, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("runtime provisioning root is unavailable")
	}
	current := rootPath
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		candidate := filepath.Join(current, component)
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr == nil {
			inside, relErr := filepath.Rel(rootPath, resolved)
			if relErr != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				return "", errors.New("runtime provisioning path escaped its root")
			}
			current = resolved
			continue
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", errors.New("runtime provisioning path is unavailable")
		}
		current = candidate
	}
	return current, nil
}

func writeProvisionedFile(path string, contents []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, err = f.Write(contents); err != nil {
			_ = f.Close()
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "write runtime configuration failed", Cause: err}
		}
		if err = f.Sync(); err != nil {
			_ = f.Close()
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "sync runtime configuration failed", Cause: err}
		}
		if err = f.Close(); err != nil {
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "close runtime configuration failed", Cause: err}
		}
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "write runtime configuration failed", Cause: err}
	}
	old, readErr := os.ReadFile(path)
	if readErr != nil || string(old) != string(contents) {
		return fmt.Errorf("runtime configuration conflicts with existing durable file")
	}
	return nil
}
