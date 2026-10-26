package taskintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/invopop/jsonschema"
)

// GenerateProviderConfigSchema builds a provider's public configuration schema
// from the same Go type used to decode its configuration.
func GenerateProviderConfigSchema(config any, title string) json.RawMessage {
	reflector := &jsonschema.Reflector{
		Anonymous:      true,
		DoNotReference: true,
		ExpandedStruct: true,
	}
	schema := reflector.Reflect(config)
	schema.Title = title
	data, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Errorf("marshal provider config schema: %w", err))
	}
	return data
}

// DecodeProviderConfig decodes one provider config value and rejects unknown fields.
func DecodeProviderConfig[T any](raw json.RawMessage) (T, error) {
	var config T
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return config, fmt.Errorf("provider config is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return config, fmt.Errorf("provider config must contain one JSON value")
		}
		return config, err
	}
	return config, nil
}
