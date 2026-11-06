package model

type Value struct {
	Name   string `json:"name"`
	Secret bool   `json:"secret"`
	Value  string `json:"value,omitempty"`
}

type Input struct {
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

type StoredValue struct {
	Name      string
	Value     string
	Encrypted bool
}

type RuntimeValues map[string]string
