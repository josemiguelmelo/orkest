package jiraserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type searchResponse struct {
	StartAt int `json:"startAt"`
	Total   int `json:"total"`
	Issues  []struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string   `json:"summary"`
			Description any      `json:"description"`
			Labels      []string `json:"labels"`
			Status      struct {
				StatusCategory struct {
					Key string `json:"key"`
				} `json:"statusCategory"`
			} `json:"status"`
		} `json:"fields"`
	} `json:"issues"`
}

func (p JiraServerProvider) validateConnection(ctx context.Context, baseURL, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/rest/api/2/myself", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client().Do(req)
	if err != nil {
		return fmt.Errorf("connect to Jira")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jira returned status %d", resp.StatusCode)
	}
	return nil
}

func (p JiraServerProvider) searchIssues(ctx context.Context, baseURL, jql string, startAt int, token string) (*searchResponse, error) {
	endpoint := baseURL + "/rest/api/2/search"
	u, _ := url.Parse(endpoint)
	query := u.Query()
	query.Set("jql", jql)
	query.Set("startAt", strconv.Itoa(startAt))
	query.Set("maxResults", "100")
	query.Set("fields", "summary,description,labels,status")
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover Jira issues")
	}
	defer func() { _ = resp.Body.Close() }()
	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira returned status %d", resp.StatusCode)
	}
	return &result, nil
}

func (p JiraServerProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}
