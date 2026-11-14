package workflowstageintegration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

type statusRepository struct {
	WorkflowStageIntegrationRepository
	items    []stageintegrationmodel.Status
	position stageintegrationmodel.StatusPosition
}

func (r *statusRepository) ListStatus(_ context.Context, _, _ string, position stageintegrationmodel.StatusPosition) ([]stageintegrationmodel.Status, error) {
	r.position = position
	if position.ID != "" {
		return []stageintegrationmodel.Status{r.items[len(r.items)-1]}, nil
	}
	return r.items, nil
}

func TestStatusPaginationValidatesAndRoundTripsCursor(t *testing.T) {
	repo := &statusRepository{}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 51; i++ {
		repo.items = append(repo.items, stageintegrationmodel.Status{ID: fmt.Sprint(i), CreatedAt: now})
	}
	s := NewWorkflowStageIntegrationService(repo, nil, &workerAccess{}, workerCoding{}, nil)
	page, err := s.Status(t.Context(), "user", "org", "workspace", "connection", "")
	if err != nil || len(page.Items) != 50 || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	next, err := s.Status(t.Context(), "user", "org", "workspace", "connection", page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.NextCursor != "" || repo.position.ID != "49" || !repo.position.CreatedAt.Equal(now) {
		t.Fatalf("next=%+v position=%+v err=%v", next, repo.position, err)
	}
	for _, cursor := range []string{"invalid!", "e30", strings.Repeat("a", 1025)} {
		if _, err := s.Status(t.Context(), "user", "org", "workspace", "connection", cursor); err == nil {
			t.Fatalf("invalid cursor accepted: %q", cursor)
		}
	}
}
