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
			id, events, err := NewGitHubProvider().VerifyAndDecode(context.Background(), signedRequest(tc.kind, tc.body), json.RawMessage(`{"repositoryIds":["123"]}`), "test-webhook-secret")
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
		_, _, err := NewGitHubProvider().VerifyAndDecode(context.Background(), request, json.RawMessage(`{"repositoryIds":["123"]}`), "test-webhook-secret")
		if !errors.Is(err, problem.ErrForbidden) {
			t.Fatal(err)
		}
	})
	t.Run("wrong repository", func(t *testing.T) {
		_, _, err := NewGitHubProvider().VerifyAndDecode(context.Background(), signedRequest("pull_request_review", review), json.RawMessage(`{"repositoryIds":["999"]}`), "test-webhook-secret")
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

func TestMultipleRepositoryConfiguration(t *testing.T) {
	provider := NewGitHubProvider()
	config := json.RawMessage(`{"repositoryIds":["123","456"]}`)
	for _, id := range []string{"123", "456"} {
		body := `{"action":"submitted","repository":{"id":` + id + `,"full_name":"owner/repo"},"pull_request":{"number":4},"review":{"id":9,"submitted_at":"2026-11-08T12:00:00Z"}}`
		_, events, err := provider.VerifyAndDecode(context.Background(), signedRequest("pull_request_review", body), config, "test-webhook-secret")
		if err != nil || len(events) != 1 || events[0].RepositoryID != id {
			t.Fatalf("repository %s: %+v %v", id, events, err)
		}
	}
	for _, raw := range []string{`{}`, `{"repositoryIds":[]}`, `{"repositoryIds":["123","123"]}`, `{"repositoryIds":["0"]}`, `{"repositoryIds":["123"],"repositoryId":"456"}`} {
		if err := provider.ValidateConfig(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted invalid configuration %s", raw)
		}
	}
	if err := provider.ValidateConfig(json.RawMessage(`{"repositoryId":"123"}`)); err == nil {
		t.Fatal("accepted retired repositoryId setting")
	}
}
