package model

import (
	"encoding/json"
	"fmt"
)

type ProviderError struct {
	Code   string
	Status int
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("coding provider request failed (%s)", e.Code)
}

type Credentials struct {
	Token          string
	ProviderConfig json.RawMessage
}

type RepositoryPage struct {
	Repositories []RepositoryMetadata `json:"repositories"`
	NextPage     int                  `json:"nextPage,omitempty"`
}

type RepositoryMetadata struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CloneURL      string `json:"cloneUrl"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"defaultBranch"`
}
