// Package artifact owns durable task outputs and immutable revisions.
package artifact

import (
	"context"
	"encoding/json"
	"mime"
	"net/url"
	"regexp"
	"strings"
	"time"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

type Content struct {
	MediaType string         `json:"mediaType"`
	Text      *string        `json:"text,omitempty"`
	URL       string         `json:"url,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type PublishInput struct {
	ArtifactID       string  `json:"artifactId,omitempty"`
	ExpectedRevision int64   `json:"expectedRevision"`
	IdempotencyKey   string  `json:"idempotencyKey"`
	Kind             string  `json:"kind"`
	Title            string  `json:"title"`
	OutputName       string  `json:"outputName,omitempty"`
	Content          Content `json:"content"`
}

type Revision struct {
	ID             string    `json:"id"`
	ArtifactID     string    `json:"artifactId"`
	Number         int64     `json:"number"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	OutputName     string    `json:"outputName,omitempty"`
	Content        Content   `json:"content"`
	CreatedBy      string    `json:"createdBy"`
	ProducingRunID string    `json:"producingRunId,omitempty"`
	StageID        string    `json:"stageId"`
	StageVersion   int64     `json:"stageVersion"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Scope struct{ UserID, OrganizationID, WorkspaceID, TaskID, RunID string }

type ArtifactRepository interface {
	Publish(context.Context, Scope, PublishInput) (Revision, error)
	List(context.Context, Scope) ([]Revision, error)
	Revisions(context.Context, Scope, string) ([]Revision, error)
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

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)

func ValidName(value string) bool { return namePattern.MatchString(value) }

func (in *PublishInput) Validate() error {
	in.Title = strings.TrimSpace(in.Title)
	if !ValidName(in.Kind) || (in.OutputName != "" && !ValidName(in.OutputName)) || in.Title == "" || len(in.Title) > 500 || len(in.IdempotencyKey) == 0 || len(in.IdempotencyKey) > 200 || in.ExpectedRevision < 0 || ((in.ArtifactID == "") != (in.ExpectedRevision == 0)) {
		return modelerrors.ErrInvalid
	}
	if (in.Content.Text == nil) == (in.Content.URL == "") {
		return modelerrors.ErrInvalid
	}
	media, _, err := mime.ParseMediaType(in.Content.MediaType)
	if err != nil {
		return modelerrors.ErrInvalid
	}
	if in.Content.Text != nil {
		if len(*in.Content.Text) > 100000 {
			return modelerrors.ErrInvalid
		}
		if media == "application/json" && !json.Valid([]byte(*in.Content.Text)) {
			return modelerrors.ErrInvalid
		}
	}
	if in.Content.URL != "" {
		u, err := url.Parse(in.Content.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(in.Content.URL) > 4000 {
			return modelerrors.ErrInvalid
		}
	}
	encoded, err := json.Marshal(in.Content.Metadata)
	if err != nil || len(encoded) > 16000 {
		return modelerrors.ErrInvalid
	}
	return nil
}

func (s *ArtifactService) Publish(ctx context.Context, scope Scope, input PublishInput) (Revision, error) {
	if err := s.tasks.AuthorizeExecution(ctx, scope.UserID, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, true); err != nil {
		return Revision{}, err
	}
	if err := input.Validate(); err != nil {
		return Revision{}, err
	}
	return s.repository.Publish(ctx, scope, input)
}

func (s *ArtifactService) List(ctx context.Context, scope Scope) ([]Revision, error) {
	if err := s.tasks.AuthorizeExecution(ctx, scope.UserID, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, false); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, scope)
}

func (s *ArtifactService) Revisions(ctx context.Context, scope Scope, artifactID string) ([]Revision, error) {
	if err := s.tasks.AuthorizeExecution(ctx, scope.UserID, scope.OrganizationID, scope.WorkspaceID, scope.TaskID, false); err != nil {
		return nil, err
	}
	return s.repository.Revisions(ctx, scope, artifactID)
}
