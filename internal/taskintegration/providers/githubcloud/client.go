package githubcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

const projectItemsQuery = `query($owner:String!,$number:Int!,$cursor:String){ OWNER_LOOKUP { projectV2(number:$number) { id items(first:100,after:$cursor) { nodes { id content { __typename ... on Issue { id number state repository { nameWithOwner } url title body labels(first:100) { nodes { name } } } ... on PullRequest { id url title body labels(first:100) { nodes { name } } } ... on DraftIssue { id title body } } } pageInfo { hasNextPage endCursor } } } } }`

type projectPage struct {
	ID    string `json:"id"`
	Items struct {
		Nodes []struct {
			ID      string `json:"id"`
			Content struct {
				TypeName   string `json:"__typename"`
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
