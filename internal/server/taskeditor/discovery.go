package taskeditor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type TaskReader interface {
	AuthorizeExecution(context.Context, string, string, string, string, bool) error
	Get(context.Context, string, string, string, string) (taskmodel.Task, error)
}

type Checkout struct {
	RegistrationID string
	ProviderName   string
	Name           string
}

// Root pairs an editor-visible path with a private executor-owned storage reference.
// StorageRef is deterministic and non-secret; only the producing backend resolves it.
type Root struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	StorageRef string `json:"-"`
}

type Inspection struct {
	CompatibilityKey string `json:"-"` // Private adapter configuration identity; never a credential.
	Roots            []Root
	Missing          []MissingCheckout
}

type MissingCheckout struct {
	RegistrationID string `json:"registrationId"`
	ProviderName   string `json:"providerName"`
	Name           string `json:"name"`
}

type CheckoutInspector interface {
	Inspect(context.Context, string, string, string, []Checkout) (Inspection, error)
}

type Discovery struct {
	CompatibilityKey string            `json:"-"`
	Roots            []Root            `json:"roots"`
	Missing          []MissingCheckout `json:"missing"`
	Manifest         []byte            `json:"-"`
}

// Discover authorizes the task and inspects its durable work directory.
func Discover(ctx context.Context, tasks TaskReader, inspector CheckoutInspector, userID, orgID, workspaceID, taskID string) (Discovery, error) {
	if tasks == nil || inspector == nil {
		return Discovery{}, errors.New("task reader and checkout inspector are required")
	}
	if err := tasks.AuthorizeExecution(ctx, userID, orgID, workspaceID, taskID, true); err != nil {
		return Discovery{}, err
	}
	task, err := tasks.Get(ctx, userID, orgID, workspaceID, taskID)
	if err != nil {
		return Discovery{}, err
	}
	selected := make([]Checkout, 0, len(task.Repositories))
	for _, repository := range task.Repositories {
		selected = append(selected, Checkout{RegistrationID: repository.ID, ProviderName: repository.ProviderName, Name: repository.Name})
	}
	sort.Slice(selected, func(left, right int) bool {
		if selected[left].ProviderName != selected[right].ProviderName {
			return selected[left].ProviderName < selected[right].ProviderName
		}
		if selected[left].RegistrationID != selected[right].RegistrationID {
			return selected[left].RegistrationID < selected[right].RegistrationID
		}
		return selected[left].Name < selected[right].Name
	})
	inspection, err := inspector.Inspect(ctx, orgID, workspaceID, taskID, selected)
	if err != nil {
		return Discovery{}, err
	}
	manifest, err := json.Marshal(struct {
		Folders []Root `json:"folders"`
	}{inspection.Roots})
	if err != nil {
		return Discovery{}, fmt.Errorf("encode workspace manifest: %w", err)
	}
	return Discovery{CompatibilityKey: inspection.CompatibilityKey, Roots: inspection.Roots, Missing: inspection.Missing, Manifest: manifest}, nil
}

// DisplayName keeps readable repository labels while disambiguating duplicates.
func DisplayName(roots []Root, name, provider, registrationID string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = provider + "/" + registrationID
	}
	base := name
	for suffix := 1; ; suffix++ {
		found := false
		for _, existing := range roots {
			if existing.Name == name {
				found = true
				break
			}
		}
		if !found {
			return name
		}
		if suffix == 1 {
			name = base + " (" + provider + "/" + registrationID + ")"
		} else {
			name = fmt.Sprintf("%s (%s/%s) %d", base, provider, registrationID, suffix)
		}
	}
}
