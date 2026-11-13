package model

import "encoding/json"

type CallbackRequest struct {
	Method  string
	Headers map[string][]string
	Body    []byte
}

type CallbackInput struct {
	Request CallbackRequest
	Config  json.RawMessage
	Secret  string
}

type CallbackResult struct {
	DeliveryID string
	Events     []ProviderEvent
	Ignored    bool
}

type IngestionReceipt struct {
	Duplicate bool `json:"duplicate"`
	Ignored   bool `json:"ignored"`
}
