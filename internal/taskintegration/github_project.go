package taskintegration

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type GitHubProjectTarget struct {
	OwnerType string
	Owner     string
	Number    int
}

func ParseGitHubProjectURL(raw string) (GitHubProjectTarget, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.RawQuery != "" || u.Fragment != "" {
		return GitHubProjectTarget{}, fmt.Errorf("invalid GitHub project URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || len(parts) > 6 || (parts[0] != "orgs" && parts[0] != "users") || parts[1] == "" || parts[2] != "projects" {
		return GitHubProjectTarget{}, fmt.Errorf("invalid GitHub project URL")
	}
	if len(parts) == 6 && parts[4] != "views" {
		return GitHubProjectTarget{}, fmt.Errorf("invalid GitHub project URL")
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number < 1 {
		return GitHubProjectTarget{}, fmt.Errorf("invalid GitHub project number")
	}
	ownerType := "organization"
	if parts[0] == "users" {
		ownerType = "user"
	}
	return GitHubProjectTarget{OwnerType: ownerType, Owner: parts[1], Number: number}, nil
}
