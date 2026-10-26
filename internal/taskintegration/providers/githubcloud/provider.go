package githubcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

const projectItemsQuery = `query($owner:String!,$number:Int!,$cursor:String){ OWNER_LOOKUP { projectV2(number:$number) { id items(first:100,after:$cursor) { nodes { id content { __typename ... on Issue { id number state repository { nameWithOwner } url title body labels(first:100) { nodes { name } } } ... on PullRequest { id url title body labels(first:100) { nodes { name } } } ... on DraftIssue { id title body } } } pageInfo { hasNextPage endCursor } } } } }`

const ProviderID taskintegration.TaskIntegrationSourceProvider = "github_cloud"

type GitHubCloudConfig struct {
	ProjectURL string `json:"projectUrl" jsonschema:"format=uri,pattern=^https://github\\.com/(orgs|users)/[^/]+/projects/[1-9][0-9]*(/views/[1-9][0-9]*)?$" jsonschema_description:"GitHub Cloud Projects v2 board URL."`
}

type GitHubCloudProvider struct{ Client *http.Client }

var _ taskintegration.SourceIntegrationProvider = GitHubCloudProvider{}

func NewGitHubCloudProvider() GitHubCloudProvider {
	return GitHubCloudProvider{Client: &http.Client{Timeout: 20 * time.Second}}
}

func (GitHubCloudProvider) Provider() taskintegration.TaskIntegrationSourceProvider {
	return ProviderID
}

func (GitHubCloudProvider) ConfigSchema() json.RawMessage {
	return taskintegration.GenerateProviderConfigSchema(GitHubCloudConfig{}, "GitHub Cloud configuration")
}

func (GitHubCloudProvider) ValidateConfig(raw json.RawMessage) error {
	config, err := taskintegration.DecodeProviderConfig[GitHubCloudConfig](raw)
	if err != nil {
		return &taskintegration.ConfigurationError{Code: "github_config_invalid"}
	}
	if strings.TrimSpace(config.ProjectURL) == "" {
		return &taskintegration.ConfigurationError{Code: "github_project_url_required"}
	}
	if _, err := parseProjectURL(config.ProjectURL); err != nil {
		return &taskintegration.ConfigurationError{Code: "github_project_url_invalid"}
	}
	return nil
}

type projectPage struct {
	ID    string `json:"id"`
	Items struct {
		Nodes []struct {
			ID      string `json:"id"`
			Content struct {
				TypeName   string `json:"__typename"`
				ID         string `json:"id"`
				Number     int    `json:"number"`
				State      string `json:"state"`
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
				URL    string `json:"url"`
				Title  string `json:"title"`
				Body   string `json:"body"`
				Labels struct {
					Nodes []struct {
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"labels"`
			} `json:"content"`
		} `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"items"`
}

type graphQLResponse struct {
	Data struct {
		Organization *struct {
			ProjectV2 *projectPage `json:"projectV2"`
		} `json:"organization"`
		User *struct {
			ProjectV2 *projectPage `json:"projectV2"`
		} `json:"user"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"errors"`
}

func (p GitHubCloudProvider) Validate(ctx context.Context, raw json.RawMessage, token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("GitHub token is required")
	}
	if err := p.ValidateConfig(raw); err != nil {
		return err
	}
	config, err := taskintegration.DecodeProviderConfig[GitHubCloudConfig](raw)
	if err != nil {
		return err
	}
	config.ProjectURL = strings.TrimSpace(config.ProjectURL)
	target, err := parseProjectURL(config.ProjectURL)
	if err != nil {
		return err
	}
	page, err := p.fetchPage(ctx, target, token, "")
	if err != nil {
		return err
	}
	if page == nil {
		return &taskintegration.ValidationError{Code: "github_project_not_found_or_not_accessible"}
	}
	return nil
}

func (p GitHubCloudProvider) Discover(ctx context.Context, raw json.RawMessage, token string, filters []string) ([]taskintegration.DiscoveredTask, error) {
	if err := p.ValidateConfig(raw); err != nil {
		return nil, err
	}
	config, err := taskintegration.DecodeProviderConfig[GitHubCloudConfig](raw)
	if err != nil {
		return nil, err
	}
	config.ProjectURL = strings.TrimSpace(config.ProjectURL)
	target, err := parseProjectURL(config.ProjectURL)
	if err != nil {
		return nil, err
	}
	out := make([]taskintegration.DiscoveredTask, 0)
	cursor := ""
	for {
		page, fetchErr := p.fetchPage(ctx, target, token, cursor)
		if fetchErr != nil {
			return nil, fetchErr
		}
		if page == nil {
			return nil, &taskintegration.ValidationError{Code: "github_project_not_found_or_not_accessible"}
		}
		for _, node := range page.Items.Nodes {
			content := node.Content
			if content.TypeName != "Issue" && content.TypeName != "DraftIssue" {
				continue
			}
			labels := make([]string, 0, len(content.Labels.Nodes))
			for _, label := range content.Labels.Nodes {
				labels = append(labels, label.Name)
			}
			if !matchesLabels(labels, filters) {
				continue
			}
			id, sourceURL := "github:draft:"+node.ID, config.ProjectURL
			if content.TypeName == "Issue" {
				id = fmt.Sprintf("%s#%d", content.Repository.NameWithOwner, content.Number)
				sourceURL = content.URL
			}
			out = append(out, taskintegration.DiscoveredTask{ID: id, URL: sourceURL, Title: content.Title, Description: content.Body, Labels: labels, Closed: content.State == "CLOSED"})
		}
		if !page.Items.PageInfo.HasNextPage {
			break
		}
		cursor = page.Items.PageInfo.EndCursor
	}
	return out, nil
}

func (p GitHubCloudProvider) fetchPage(ctx context.Context, target ProjectTarget, token, cursor string) (*projectPage, error) {
	query := strings.Replace(projectItemsQuery, "OWNER_LOOKUP", "organization(login:$owner)", 1)
	if target.OwnerType == "user" {
		query = strings.Replace(projectItemsQuery, "OWNER_LOOKUP", "user(login:$owner)", 1)
	}
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": map[string]any{"owner": target.Owner, "number": target.Number, "cursor": nullableCursor(cursor)},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, &taskintegration.ValidationError{Code: "github_api_unavailable"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, githubStatusError(resp.StatusCode)
	}
	var result graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &taskintegration.ValidationError{Code: "github_project_response_invalid"}
	}
	if len(result.Errors) > 0 {
		for _, graphError := range result.Errors {
			message := strings.ToLower(graphError.Message)
			if strings.Contains(message, "permission") || strings.Contains(message, "not accessible") || strings.Contains(message, "insufficient scope") {
				return nil, &taskintegration.ValidationError{Code: "github_project_permission_denied"}
			}
			if strings.Contains(message, "could not resolve to a project") {
				return nil, &taskintegration.ValidationError{Code: "github_project_not_found_or_not_accessible"}
			}
		}
		return nil, &taskintegration.ValidationError{Code: "github_graphql_query_failed"}
	}
	if target.OwnerType == "user" && result.Data.User != nil {
		return result.Data.User.ProjectV2, nil
	}
	if target.OwnerType == "organization" && result.Data.Organization != nil {
		return result.Data.Organization.ProjectV2, nil
	}
	return nil, nil
}

func githubStatusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return &taskintegration.ValidationError{Code: "github_token_invalid"}
	case http.StatusForbidden:
		return &taskintegration.ValidationError{Code: "github_project_permission_denied"}
	default:
		return &taskintegration.ValidationError{Code: "github_api_status_" + strconv.Itoa(status)}
	}
}

func matchesLabels(labels, filters []string) bool {
	for _, filter := range filters {
		found := false
		for _, label := range labels {
			if strings.EqualFold(label, filter) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func nullableCursor(cursor string) any {
	if cursor == "" {
		return nil
	}
	return cursor
}

func (p GitHubCloudProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}
