package scheduledjob

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type scopedJobAccess struct{ role string }

func (a scopedJobAccess) GetWorkspace(_ context.Context, _ string, org, workspace string) (workspacemodel.Workspace, error) {
	if org != "org" || workspace != "workspace" {
		return workspacemodel.Workspace{}, modelerrors.ErrNotFound
	}
	return workspacemodel.Workspace{Role: a.role}, nil
}

func TestScheduledJobViewerCannotMutate(t *testing.T) {
	s := NewScheduledJobService(nil, scopedJobAccess{role: "viewer"}, nil, nil)
	ctx := context.Background()
	if _, err := s.Save(ctx, "user", "org", "workspace", "job", 0, jobmodel.Input{}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.RunNow(ctx, "user", "org", "workspace", "job", "key"); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Run(ctx, "user", "org", "workspace", "job", "occurrence", true); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.CancelPending(ctx, "user", "org", "workspace", "job", "occurrence"); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
}

func TestScheduledJobScopeCheckedBeforeRepository(t *testing.T) {
	s := NewScheduledJobService(nil, scopedJobAccess{role: "member"}, nil, nil)
	if _, err := s.Get(context.Background(), "user", "another-org", "workspace", "job"); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.Run(context.Background(), "user", "org", "other-workspace", "job", "occurrence", false); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatal(err)
	}
}
