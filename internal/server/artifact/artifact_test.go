package artifact

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
)

func TestPublishValidation(t *testing.T) {
	text := "# Plan"
	valid := artifactmodel.PublishInput{Kind: "plan", Title: "Plan", IdempotencyKey: "publish-1", Content: artifactmodel.Content{MediaType: "text/markdown", Text: &text}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*artifactmodel.PublishInput)
	}{
		{"missing location", func(in *artifactmodel.PublishInput) { in.Content.Text = nil }},
		{"two locations", func(in *artifactmodel.PublishInput) { in.Content.URL = "https://example.test/pr/1" }},
		{"credentials", func(in *artifactmodel.PublishInput) {
			in.Content.Text = nil
			in.Content.URL = "https://token@example.test/pr/1"
		}},
		{"unsafe URL", func(in *artifactmodel.PublishInput) { in.Content.Text = nil; in.Content.URL = "javascript:alert(1)" }},
		{"invalid JSON", func(in *artifactmodel.PublishInput) { in.Content.MediaType = "application/json" }},
		{"missing concurrency version", func(in *artifactmodel.PublishInput) { in.ArtifactID = "existing" }},
		{"missing identity", func(in *artifactmodel.PublishInput) { in.ExpectedRevision = 1 }},
		{"invalid kind", func(in *artifactmodel.PublishInput) { in.Kind = "bad kind" }},
		{"missing retry key", func(in *artifactmodel.PublishInput) { in.IdempotencyKey = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.change(&input)
			if !errors.Is(input.Validate(), modelerrors.ErrInvalid) {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

type deniedAccess struct{}

func (deniedAccess) AuthorizeExecution(context.Context, string, string, string, string, bool) error {
	return modelerrors.ErrForbidden
}

func TestAccessCheckedBeforeRepository(t *testing.T) {
	service := NewArtifactService(nil, deniedAccess{})
	if _, err := service.Publish(context.Background(), artifactmodel.Scope{}, artifactmodel.PublishInput{}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.List(context.Background(), artifactmodel.Scope{}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.Revisions(context.Background(), artifactmodel.Scope{}, "artifact"); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
}
