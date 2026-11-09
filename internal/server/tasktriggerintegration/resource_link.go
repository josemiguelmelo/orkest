package tasktriggerintegration

import (
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

type ResourceLinkSnapshot struct {
	Integration          Integration
	Link                 ResourceLink
	RepositorySelected   bool
	CurrentTaskID        string
	CurrentLinkCreatedAt time.Time
	ExistingTaskID       string
	OwnerTaskIDs         []string
	Suppressed           bool
}

type ResourceLinkChange struct {
	Linked     bool
	Suppressed bool
}

func PlanResourceLink(s ResourceLinkSnapshot, remove bool) (ResourceLinkChange, error) {
	if remove {
		return ResourceLinkChange{Suppressed: true}, nil
	}
	if !s.RepositorySelected || s.CurrentTaskID != "" {
		return ResourceLinkChange{}, problem.ErrConflict
	}
	if s.ExistingTaskID != "" && s.ExistingTaskID != s.Link.TaskID {
		return ResourceLinkChange{}, problem.ErrConflict
	}
	for _, owner := range s.OwnerTaskIDs {
		if owner != s.Link.TaskID {
			return ResourceLinkChange{}, problem.ErrConflict
		}
	}
	return ResourceLinkChange{Linked: true}, nil
}
