package artifact

import (
	"context"

	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
)

type ArtifactRepository interface {
	Publish(context.Context, artifactmodel.Scope, artifactmodel.PublishInput) (artifactmodel.Revision, error)
	List(context.Context, artifactmodel.Scope) ([]artifactmodel.Revision, error)
	Revisions(context.Context, artifactmodel.Scope, string) ([]artifactmodel.Revision, error)
}

type TaskAccess interface {
	AuthorizeExecution(context.Context, string, string, string, string, bool) error
}

type ArtifactService struct {
	repository ArtifactRepository
	tasks      TaskAccess
}

func NewArtifactService(repository ArtifactRepository, tasks TaskAccess) *ArtifactService {
	return &ArtifactService{repository: repository, tasks: tasks}
}

func (s *ArtifactService) Publish(ctx context.Context, scope artifactmodel.Scope, input artifactmodel.PublishInput) (artifactmodel.Revision, error) {
	if err := s.tasks.AuthorizeExecution(ctx, scope.UserID, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, true); err != nil {
		return artifactmodel.Revision{}, err
	}
	if err := input.Validate(); err != nil {
		return artifactmodel.Revision{}, err
	}
	return s.repository.Publish(ctx, scope, input)
}

func (s *ArtifactService) List(ctx context.Context, scope artifactmodel.Scope) ([]artifactmodel.Revision, error) {
	if err := s.tasks.AuthorizeExecution(ctx, scope.UserID, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, false); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, scope)
}

func (s *ArtifactService) Revisions(ctx context.Context, scope artifactmodel.Scope, artifactID string) ([]artifactmodel.Revision, error) {
	if err := s.tasks.AuthorizeExecution(ctx, scope.UserID, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, false); err != nil {
		return nil, err
	}
	return s.repository.Revisions(ctx, scope, artifactID)
}
