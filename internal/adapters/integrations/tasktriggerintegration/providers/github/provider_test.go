package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func signedRequest(event, body string) trigger.WebhookRequest {
	mac := hmac.New(sha256.New, []byte("test-webhook-secret"))
	_, _ = mac.Write([]byte(body))
	return trigger.WebhookRequest{Body: []byte(body), Headers: map[string]string{"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-Github-Delivery": "delivery-1", "X-Github-Event": event}}
}

func TestVerifyAndDecode(t *testing.T) {
	review := `{"action":"submitted","repository":{"id":123,"full_name":"owner/repo"},"pull_request":{"number":4},"review":{"id":9,"submitted_at":"2026-11-08T12:00:00Z"}}`
	merged := `{"action":"closed","repository":{"id":123,"full_name":"owner/repo"},"pull_request":{"number":4,"merged":true,"merged_at":"2026-11-08T12:00:00Z"}}`
	for _, tc := range []struct{ name, kind, body, want string }{
		{"review", "pull_request_review", review, ReviewSubmitted},
		{"merged", "pull_request", merged, PullRequestMerged},
		{"closed unmerged", "pull_request", `{"action":"closed","repository":{"id":123},"pull_request":{"number":4,"merged":false}}`, ""},
		{"draft review", "pull_request_review", `{"action":"edited","repository":{"id":123}}`, ""},
		{"ping", "ping", `{}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, events, err := NewGitHubProvider().VerifyAndDecode(context.Background(), signedRequest(tc.kind, tc.body), json.RawMessage(`{}`), "test-webhook-secret")
			if err != nil || id != "delivery-1" {
				t.Fatalf("id=%s err=%v", id, err)
			}
			if tc.want == "" {
				if len(events) != 0 {
					t.Fatal(events)
				}
				return
			}
			if len(events) != 1 || events[0].Type != tc.want || events[0].ReferenceURL != "https://github.com/owner/repo/pull/4" {
				t.Fatalf("events=%+v", events)
			}
		})
	}
	t.Run("tampered bytes", func(t *testing.T) {
		request := signedRequest("pull_request_review", review)
		request.Body = append(request.Body, ' ')
		_, _, err := NewGitHubProvider().VerifyAndDecode(context.Background(), request, json.RawMessage(`{}`), "test-webhook-secret")
		if !errors.Is(err, problem.ErrForbidden) {
			t.Fatal(err)
		}
	})
	t.Run("bad signature before parsing", func(t *testing.T) {
		_, _, err := NewGitHubProvider().VerifyAndDecode(context.Background(), trigger.WebhookRequest{Body: []byte("invalid")}, nil, "test-webhook-secret")
		if !errors.Is(err, problem.ErrForbidden) {
			t.Fatal(err)
		}
	})
}

func TestWebhookRepositoriesAreNotConfigured(t *testing.T) {
	provider := NewGitHubProvider()
	config := json.RawMessage(`{}`)
	for _, id := range []string{"123", "456"} {
		body := `{"action":"submitted","repository":{"id":` + id + `,"full_name":"owner/repo"},"pull_request":{"number":4},"review":{"id":9,"submitted_at":"2026-11-08T12:00:00Z"}}`
		_, events, err := provider.VerifyAndDecode(context.Background(), signedRequest("pull_request_review", body), config, "test-webhook-secret")
		if err != nil || len(events) != 1 || events[0].RepositoryID != id {
			t.Fatalf("repository %s: %+v %v", id, events, err)
		}
	}
	// Stored lists from the previous configuration format no longer restrict events.
	body := `{"action":"submitted","repository":{"id":456,"full_name":"owner/repo"},"pull_request":{"number":4},"review":{"id":9,"submitted_at":"2026-11-08T12:00:00Z"}}`
	_, events, err := provider.VerifyAndDecode(context.Background(), signedRequest("pull_request_review", body), json.RawMessage(`{"repositoryIds":["123"]}`), "test-webhook-secret")
	if err != nil || len(events) != 1 || events[0].RepositoryID != "456" {
		t.Fatalf("retired repository list restricted webhook: %+v %v", events, err)
	}
}

func TestConfigurationAndResourceValidation(t *testing.T) {
	provider := NewGitHubProvider()
	if err := provider.ValidateConfig(json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`null`, `[]`, `{"repositoryIds":["123"]}`, `{"repositoryId":"123"}`} {
		if err := provider.ValidateConfig(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted invalid configuration %s", raw)
		}
	}
	for _, link := range []trigger.ResourceLink{{RepositoryID: "0", ResourceID: "4"}, {RepositoryID: "123", ResourceID: "0"}} {
		if err := provider.ValidateResource(json.RawMessage(`{}`), link); err == nil {
			t.Fatalf("accepted invalid resource %+v", link)
		}
	}
	if err := provider.ValidateResource(json.RawMessage(`{"repositoryIds":["123"]}`), trigger.ResourceLink{RepositoryID: "456", ResourceID: "4"}); err != nil {
		t.Fatalf("retired repository list restricted resource: %v", err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(provider.Descriptor().ConfigSchema, &schema); err != nil || len(schema.Properties) != 0 {
		t.Fatalf("unexpected provider settings: %s %v", provider.Descriptor().ConfigSchema, err)
	}
}
