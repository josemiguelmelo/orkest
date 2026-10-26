package jiraserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

const ProviderID taskintegration.TaskIntegrationSourceProvider = "jira_server"

type JiraServerConfig struct {
	BaseURL  string   `json:"baseUrl" jsonschema:"format=uri,pattern=^https?://[^/?#]+(?:/[^?#]*)?$" jsonschema_description:"Base URL of the Jira Server instance."`
	Projects []string `json:"projects" jsonschema:"minItems=1,uniqueItems=true" jsonschema_description:"Jira project keys to import."`
}

type JiraServerProvider struct{ Client *http.Client }

var _ taskintegration.SourceIntegrationProvider = JiraServerProvider{}

func NewJiraServerProvider() JiraServerProvider {
	return JiraServerProvider{Client: &http.Client{Timeout: 20 * time.Second}}
}

func (JiraServerProvider) Provider() taskintegration.TaskIntegrationSourceProvider {
	return ProviderID
}

func (JiraServerProvider) ConfigSchema() json.RawMessage {
	return taskintegration.GenerateProviderConfigSchema(JiraServerConfig{}, "Jira Server configuration")
}

func (JiraServerProvider) ValidateConfig(raw json.RawMessage) error {
	config, err := taskintegration.DecodeProviderConfig[JiraServerConfig](raw)
	if err != nil {
		return &taskintegration.ConfigurationError{Code: "jira_config_invalid"}
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return &taskintegration.ConfigurationError{Code: "jira_base_url_required"}
	}
	if len(config.Projects) == 0 {
		return &taskintegration.ConfigurationError{Code: "jira_projects_required"}
	}
	seenProjects := make(map[string]struct{}, len(config.Projects))
	for i, project := range config.Projects {
		config.Projects[i] = strings.TrimSpace(project)
		if config.Projects[i] == "" {
			return &taskintegration.ConfigurationError{Code: "jira_projects_invalid"}
		}
		if _, exists := seenProjects[config.Projects[i]]; exists {
			return &taskintegration.ConfigurationError{Code: "jira_projects_invalid"}
		}
		seenProjects[config.Projects[i]] = struct{}{}
	}
	if err := parseServerURL(config.BaseURL); err != nil {
		return &taskintegration.ConfigurationError{Code: "jira_base_url_invalid"}
	}
	return nil
}

func parseServerURL(raw string) error {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid Jira Server URL")
	}
	return nil
}

func (p JiraServerProvider) Validate(ctx context.Context, raw json.RawMessage, token string) error {
	if err := p.ValidateConfig(raw); err != nil {
		return err
	}
	c, err := taskintegration.DecodeProviderConfig[JiraServerConfig](raw)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("jira URL and personal access token are required")
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/rest/api/2/myself", nil)
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

func (p JiraServerProvider) Discover(ctx context.Context, raw json.RawMessage, token string, labels []string) ([]taskintegration.DiscoveredTask, error) {
	if err := p.ValidateConfig(raw); err != nil {
		return nil, err
	}
	c, err := taskintegration.DecodeProviderConfig[JiraServerConfig](raw)
	if err != nil {
		return nil, err
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	projects := make([]string, 0, len(c.Projects))
	for _, v := range c.Projects {
		v = strings.TrimSpace(v)
		projects = append(projects, "'"+strings.ReplaceAll(v, "'", "\\'")+"'")
	}
	jql := "project in (" + strings.Join(projects, ",") + ")"
	if len(labels) > 0 {
		q := make([]string, 0, len(labels))
		for _, v := range labels {
			q = append(q, "'"+strings.ReplaceAll(v, "'", "\\'")+"'")
		}
		jql += " AND labels in (" + strings.Join(q, ",") + ")"
	}
	out := []taskintegration.DiscoveredTask{}
	start := 0
	for {
		endpoint := c.BaseURL + "/rest/api/2/search"
		u, _ := url.Parse(endpoint)
		query := u.Query()
		query.Set("jql", jql)
		query.Set("startAt", fmt.Sprint(start))
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
		var result struct {
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
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("jira returned status %d", resp.StatusCode)
		}
		for _, issue := range result.Issues {
			description, _ := issue.Fields.Description.(string)
			out = append(out, taskintegration.DiscoveredTask{ID: issue.Key, URL: c.BaseURL + "/browse/" + issue.Key, Title: issue.Fields.Summary, Description: description, Labels: issue.Fields.Labels, Closed: issue.Fields.Status.StatusCategory.Key == "done"})
		}
		start += len(result.Issues)
		if len(result.Issues) == 0 || start >= result.Total {
			break
		}
	}
	return out, nil
}

func (p JiraServerProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}
