package githubcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

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
