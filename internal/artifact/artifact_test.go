package artifact

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

func TestPublishValidation(t *testing.T) {
	text := "# Plan"
	valid := PublishInput{Kind: "plan", Title: "Plan", IdempotencyKey: "publish-1", Content: Content{MediaType: "text/markdown", Text: &text}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*PublishInput)
	}{
		{"missing location", func(in *PublishInput) { in.Content.Text = nil }},
		{"two locations", func(in *PublishInput) { in.Content.URL = "https://example.test/pr/1" }},
		{"credentials", func(in *PublishInput) { in.Content.Text = nil; in.Content.URL = "https://token@example.test/pr/1" }},
		{"unsafe URL", func(in *PublishInput) { in.Content.Text = nil; in.Content.URL = "javascript:alert(1)" }},
		{"invalid JSON", func(in *PublishInput) { in.Content.MediaType = "application/json" }},
		{"missing concurrency version", func(in *PublishInput) { in.ArtifactID = "existing" }},
		{"missing identity", func(in *PublishInput) { in.ExpectedRevision = 1 }},
		{"invalid kind", func(in *PublishInput) { in.Kind = "bad kind" }},
		{"missing retry key", func(in *PublishInput) { in.IdempotencyKey = "" }},
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
	service := NewService(nil, deniedAccess{})
	if _, err := service.Publish(context.Background(), Scope{}, PublishInput{}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.List(context.Background(), Scope{}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := service.Revisions(context.Background(), Scope{}, "artifact"); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatal(err)
	}
}
