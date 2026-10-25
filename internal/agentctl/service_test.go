package agentctl

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func configuration() Configuration {
	return Configuration{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: Model{Provider: "provider", ID: "model"}, SystemPrompt: "Work on this task"}
}

func TestConfigurationValidation(t *testing.T) {
	for _, field := range []string{"organization", "workspace", "task", "run", "provider", "model", "prompt", "runtime"} {
		t.Run(field, func(t *testing.T) {
			config := configuration()
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
				config.Runtime = "other"
			}
			service := NewService()
			if err := service.Configure(config); err == nil {
				t.Fatal("accepted invalid configuration")
			}
			if service.Status().Configured {
				t.Fatal("invalid request changed state")
			}
		})
	}
}

func TestRunBinding(t *testing.T) {
	service := NewService()
	if service.Status().Configured {
		t.Fatal("new service is configured")
	}
	config := configuration()
	for range 2 {
		if err := service.Configure(config); err != nil {
			t.Fatal(err)
		}
	}
	config.RunID = "another-run"
	if err := service.Configure(config); !errors.Is(err, ErrConfigurationConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := service.Configure(configuration()); err != nil {
		t.Fatalf("conflict changed binding: %v", err)
	}
	if status := service.Status(); !status.Configured || status.ExecutionAvailable {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestConcurrentConfiguration(t *testing.T) {
	service := NewService()
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			config := configuration()
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
