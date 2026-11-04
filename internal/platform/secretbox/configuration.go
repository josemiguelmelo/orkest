package secretbox

import (
	"encoding/json"
	"fmt"
)

// SealConfiguration encrypts provider configuration before persistence.
func SealConfiguration(box *Box, values map[string]string) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if box == nil {
		return nil, ErrKeyUnavailable
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return box.Seal(string(encoded))
}

// OpenConfiguration decrypts persisted provider configuration.
func OpenConfiguration(box *Box, ciphertext []byte) (map[string]string, error) {
	if len(ciphertext) == 0 {
		return map[string]string{}, nil
	}
	if box == nil {
		return nil, ErrKeyUnavailable
	}
	plain, err := box.Open(ciphertext)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if err = json.Unmarshal([]byte(plain), &values); err != nil {
		return nil, fmt.Errorf("decode runtime configuration: %w", err)
	}
	return values, nil
}
