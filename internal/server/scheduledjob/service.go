package scheduledjob

import (
	"context"
	"strings"
	"time"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	repomodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type ScheduledJobRepository interface {
	List(context.Context, string, string) ([]jobmodel.Job, error)
	Get(context.Context, string, string, string) (jobmodel.Job, error)
	Save(context.Context, string, string, string, string, int64, jobmodel.Input, time.Time) (jobmodel.Job, error)
	Runs(context.Context, string, string, string, int, int) ([]jobmodel.Run, error)
	Run(context.Context, string, string, string, string) (jobmodel.Run, error)
	Enqueue(context.Context, string, string, string, string, string) (jobmodel.Run, error)
	CancelPending(context.Context, string, string, string, string) (bool, error)
}
type (
	WorkspaceAccess interface {
		GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
	}
	Agents interface {
		Get(context.Context, string, string, string, string) (agentmodel.Agent, error)
	}
	Repositories interface {
		List(context.Context, string, string, string) ([]repomodel.CodeRepository, error)
	}
	ScheduledJobService struct {
		repository   ScheduledJobRepository
		access       WorkspaceAccess
		agents       Agents
		repositories Repositories
	}
)

func NewScheduledJobService(r ScheduledJobRepository, a WorkspaceAccess, agents Agents, repos Repositories) *ScheduledJobService {
	return &ScheduledJobService{repository: r, access: a, agents: agents, repositories: repos}
}

func (s *ScheduledJobService) Authorize(ctx context.Context, user, org, workspace string, write bool) error {
	w, err := s.access.GetWorkspace(ctx, user, org, workspace)
	if err != nil {
		return err
	}
	if write && w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func (s *ScheduledJobService) List(ctx context.Context, user, org, workspace string) ([]jobmodel.Job, error) {
	if err := s.Authorize(ctx, user, org, workspace, false); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, org, workspace)
}

func (s *ScheduledJobService) Get(ctx context.Context, user, org, workspace, id string) (jobmodel.Job, error) {
	if err := s.Authorize(ctx, user, org, workspace, false); err != nil {
		return jobmodel.Job{}, err
	}
	return s.repository.Get(ctx, org, workspace, id)
}

func (s *ScheduledJobService) Save(ctx context.Context, user, org, workspace, id string, revision int64, in jobmodel.Input) (jobmodel.Job, error) {
	if err := s.Authorize(ctx, user, org, workspace, true); err != nil {
		return jobmodel.Job{}, err
	}
	in.Name, in.Instructions = strings.TrimSpace(in.Name), strings.TrimSpace(in.Instructions)
	if !validKey(id) || revision < 0 || in.Name == "" || len(in.Name) > 200 || in.Instructions == "" || len(in.Instructions) > 60000 || in.MaxDurationMinutes < 1 || in.MaxDurationMinutes > 1440 || len(in.RepositoryIDs) > 1000 {
		return jobmodel.Job{}, modelerrors.ErrInvalid
	}
	next, err := Next(in.Schedule, time.Now())
	if err != nil {
		return jobmodel.Job{}, err
	}
	if _, err = s.agents.Get(ctx, user, org, workspace, in.AgentID); err != nil {
		return jobmodel.Job{}, err
	}
	if in.RepositoryScope != "none" && in.RepositoryScope != "selected" && in.RepositoryScope != "all" {
		return jobmodel.Job{}, modelerrors.ErrInvalid
	}
	if in.RepositoryScope != "selected" && len(in.RepositoryIDs) > 0 {
		return jobmodel.Job{}, modelerrors.ErrInvalid
	}
	if in.RepositoryScope == "selected" {
		if len(in.RepositoryIDs) == 0 {
			return jobmodel.Job{}, modelerrors.ErrInvalid
		}
		available, e := s.repositories.List(ctx, user, org, workspace)
		if e != nil {
			return jobmodel.Job{}, e
		}
		allowed := map[string]bool{}
		for _, r := range available {
			allowed[r.ID] = true
		}
		seen := map[string]bool{}
		for _, id := range in.RepositoryIDs {
			if !allowed[id] || seen[id] {
				return jobmodel.Job{}, modelerrors.ErrInvalid
			}
			seen[id] = true
		}
	}
	if in.RepositoryIDs == nil {
		in.RepositoryIDs = []string{}
	}
	return s.repository.Save(ctx, user, org, workspace, id, revision, in, next)
}

func validKey(key string) bool {
	if len(key) < 1 || len(key) > 100 {
		return false
	}
	for _, r := range key {
		allowed := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-'
		if !allowed {
			return false
		}
	}
	return true
}

func (s *ScheduledJobService) Runs(ctx context.Context, user, org, workspace, job string, limit, offset int) ([]jobmodel.Run, error) {
	if _, err := s.Get(ctx, user, org, workspace, job); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 || offset > 1000000 {
		return nil, modelerrors.ErrInvalid
	}
	return s.repository.Runs(ctx, org, workspace, job, limit, offset)
}

func (s *ScheduledJobService) Run(ctx context.Context, user, org, workspace, job, id string, write bool) (jobmodel.Run, error) {
	if err := s.Authorize(ctx, user, org, workspace, write); err != nil {
		return jobmodel.Run{}, err
	}
	return s.repository.Run(ctx, org, workspace, job, id)
}

func (s *ScheduledJobService) RunNow(ctx context.Context, user, org, workspace, job, key string) (jobmodel.Run, error) {
	if err := s.Authorize(ctx, user, org, workspace, true); err != nil {
		return jobmodel.Run{}, err
	}
	if !validKey(key) {
		return jobmodel.Run{}, modelerrors.ErrInvalid
	}
	return s.repository.Enqueue(ctx, user, org, workspace, job, key)
}

func (s *ScheduledJobService) CancelPending(ctx context.Context, user, org, workspace, job, id string) (bool, error) {
	if err := s.Authorize(ctx, user, org, workspace, true); err != nil {
		return false, err
	}
	return s.repository.CancelPending(ctx, org, workspace, job, id)
}
