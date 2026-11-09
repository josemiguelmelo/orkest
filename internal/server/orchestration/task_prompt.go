package orchestration

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type taskPromptRepository struct {
	ID           string `json:"id"`
	ProviderName string `json:"providerName"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	Path         string `json:"path"`
}

type taskPromptSnapshot struct {
	ID             string                 `json:"id"`
	PlatformTaskID string                 `json:"platformTaskId"`
	SourceProvider string                 `json:"sourceProvider,omitempty"`
	SourceID       string                 `json:"sourceId,omitempty"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	Labels         []string               `json:"labels"`
	Repositories   []taskPromptRepository `json:"repositories"`
}

func taskPrompt(value taskmodel.Task, repositories []orchestrationmodel.WorkspaceRepository) (string, error) {
	taskID := value.ID
	if strings.TrimSpace(value.SourceID) != "" {
		taskID = value.SourceID
	}
	snapshot := taskPromptSnapshot{ID: taskID, PlatformTaskID: value.ID, SourceProvider: value.SourceProvider, SourceID: value.SourceID, Title: value.Title, Description: value.Description, Labels: value.Labels}
	for _, repository := range repositories {
		snapshot.Repositories = append(snapshot.Repositories, taskPromptRepository{ID: repository.ID, ProviderName: repository.ProviderName, Name: repository.Name, URL: repository.URL, Path: "repositories/" + repository.ProviderName + "/" + repository.ID})
	}
	sort.Slice(snapshot.Repositories, func(i, j int) bool { return snapshot.Repositories[i].ID < snapshot.Repositories[j].ID })
	snapshot.Labels = append([]string(nil), snapshot.Labels...)
	sort.Strings(snapshot.Labels)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode task context: %w", err)
	}
	return "Work on the following task within the scope of its current workflow stage, when provided. Use its external source reference as the main task ID when present; the Orkest platform ID is included for traceability. The selected repositories are checked out under the listed paths on their current default or preserved branches. Inspect them and create or switch to a task-specific branch only when the requested changes require one. Reuse an existing task branch for review iterations:\n\n" + string(encoded), nil
}
