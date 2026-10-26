package taskintegration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrEncryptionKeyUnavailable = errors.New("integration encryption key is not configured")

type SecretBox struct{ key []byte }

func NewSecretBox(hexKey string) (*SecretBox, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("ORKEST_INTEGRATION_ENCRYPTION_KEY must be 64 hexadecimal characters")
	}
	return &SecretBox{key: key}, nil
}

func (b *SecretBox) Seal(plain string) ([]byte, error) {
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return []byte(base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil))), nil
}

func (b *SecretBox) Open(ciphertext []byte) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(ciphertext))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted credential")
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(decoded) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted credential")
	}
	nonce, body := decoded[:gcm.NonceSize()], decoded[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt integration credential")
	}
	return string(plain), nil
}
