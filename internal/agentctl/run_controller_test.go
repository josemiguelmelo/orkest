package agentctl

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func runConfig() RunConfig {
	return RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: ModelSelection{Provider: "provider", ID: "model"}, SystemPrompt: "Work on this task"}
}

func TestConfigurationValidation(t *testing.T) {
	for _, field := range []string{"organization", "workspace", "task", "run", "provider", "model", "prompt", "runtime"} {
		t.Run(field, func(t *testing.T) {
			config := runConfig()
			switch field {
			case "organization":
				config.OrganizationID = " "
			case "workspace":
				config.WorkspaceID = ""
			case "task":
				config.TaskID = ""
			case "run":
				config.RunID = ""
			case "provider":
				config.Model.Provider = ""
			case "model":
				config.Model.ID = ""
			case "prompt":
				config.SystemPrompt = "\t"
			case "runtime":
				config.Runtime = ""
			}
			service := NewRunController()
			var validation *ValidationError
			if err := service.Configure(config); !errors.As(err, &validation) {
				t.Fatalf("expected field validation error, got %v", err)
			}
			expected := map[string]string{
				"organization": "organizationId", "workspace": "workspaceId",
				"task": "taskId", "run": "runId", "provider": "model.provider",
				"model": "model.id", "prompt": "systemPrompt", "runtime": "runtime",
			}[field]
			if validation.Field != expected {
				t.Fatalf("got field %q, want %q", validation.Field, expected)
			}
			if service.Status().Configured {
				t.Fatal("invalid request changed state")
			}
		})
	}
}

func TestRunBinding(t *testing.T) {
	service := NewRunController()
	if service.Status().Configured {
		t.Fatal("new service is configured")
	}
	config := runConfig()
	for range 2 {
		if err := service.Configure(config); err != nil {
			t.Fatal(err)
		}
	}
	config.RunID = "another-run"
	if err := service.Configure(config); !errors.Is(err, ErrConfigurationConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := service.Configure(runConfig()); err != nil {
		t.Fatalf("conflict changed binding: %v", err)
	}
	if status := service.Status(); !status.Configured || status.ExecutionAvailable {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestConcurrentConfiguration(t *testing.T) {
	service := NewRunController()
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			config := runConfig()
			config.RunID = fmt.Sprintf("run-%d", i)
			results <- service.Configure(config)
			_ = service.Status()
		})
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrConfigurationConflict) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d distinct configurations", accepted)
	}
}
