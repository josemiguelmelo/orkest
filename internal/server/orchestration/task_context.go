package orchestration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

const taskContextVersion = 1

type taskStageContext struct {
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Version      int64  `json:"version"`
}

func composeTaskPrompt(value taskmodel.Task, config *orchestrationmodel.RunConfig, artifact string, userText string) (string, error) {
	task, err := taskPrompt(value, config.Repositories)
	if err != nil {
		return "", err
	}
	stageBytes, err := json.Marshal(taskStageContext{Name: value.StageName, Description: value.StageDescription, Instructions: value.StageCustomPrompt, Version: value.StageVersion})
	if err != nil {
		return "", fmt.Errorf("encode stage context: %w", err)
	}
	sections := map[string]string{"task": task, "stage": "Stage context:\n" + string(stageBytes), "artifacts": artifact}
	fingerprints := make(map[string]string, len(sections))
	for key, section := range sections {
		sum := sha256.Sum256([]byte(section))
		fingerprints[key] = hex.EncodeToString(sum[:])
	}
	newSession := !config.RestoreSession
	parts := make([]string, 0, 4)
	if newSession {
		contextParts := []string{task, sections["stage"]}
		if artifact != "" {
			contextParts = append(contextParts, artifact)
		}
		config.Agent.Instructions = strings.TrimSpace(config.Agent.Instructions + "\n\nTask session context:\n" + strings.Join(contextParts, "\n\n"))
	} else if config.TaskContext == nil || config.TaskContext.Version != taskContextVersion {
		parts = append(parts, task, sections["stage"])
		if artifact != "" {
			parts = append(parts, artifact)
		}
	} else {
		for _, key := range []string{"task", "stage", "artifacts"} {
			if config.TaskContext.Fingerprints[key] == fingerprints[key] {
				continue
			}
			if sections[key] == "" {
				parts = append(parts, fmt.Sprintf("Task context update (%s): this section is now empty or removed.", key))
			} else {
				parts = append(parts, fmt.Sprintf("Task context update (%s):\n%s", key, sections[key]))
			}
		}
	}
	config.TaskContext = &orchestrationmodel.TaskContextBaseline{Version: taskContextVersion, Fingerprints: fingerprints}
	if userText != "" {
		if len(parts) > 0 {
			parts = append(parts, "User message:\n"+userText)
		} else {
			return userText, nil
		}
	} else if len(parts) == 0 {
		parts = append(parts, "Continue work within the current workflow stage.")
	}
	return strings.Join(parts, "\n\n"), nil
}
