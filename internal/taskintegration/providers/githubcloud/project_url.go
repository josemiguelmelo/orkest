package githubcloud

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type ProjectTarget struct {
	OwnerType string
	Owner     string
	Number    int
}

func parseProjectURL(raw string) (ProjectTarget, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.RawQuery != "" || u.Fragment != "" {
		return ProjectTarget{}, fmt.Errorf("invalid GitHub Cloud project URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || len(parts) > 6 || (parts[0] != "orgs" && parts[0] != "users") || parts[1] == "" || parts[2] != "projects" {
		return ProjectTarget{}, fmt.Errorf("invalid GitHub Cloud project URL")
	}
	if len(parts) == 6 && parts[4] != "views" {
		return ProjectTarget{}, fmt.Errorf("invalid GitHub Cloud project URL")
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number < 1 {
		return ProjectTarget{}, fmt.Errorf("invalid GitHub Cloud project number")
	}
	ownerType := "organization"
	if parts[0] == "users" {
		ownerType = "user"
	}
	return ProjectTarget{OwnerType: ownerType, Owner: parts[1], Number: number}, nil
}
