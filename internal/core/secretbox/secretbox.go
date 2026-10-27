package secretbox

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

var ErrKeyUnavailable = errors.New("integration encryption key is not configured")

type Box struct{ key []byte }

func New(hexKey string) (*Box, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("ORKEST_INTEGRATION_ENCRYPTION_KEY must be 64 hexadecimal characters")
	}
	return &Box{key: key}, nil
}

func (b *Box) Seal(plain string) ([]byte, error) {
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

func (b *Box) Open(ciphertext []byte) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(ciphertext))
	if err != nil {
		return "", errors.New("invalid encrypted credential")
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
		return "", errors.New("invalid encrypted credential")
	}
	plain, err := gcm.Open(nil, decoded[:gcm.NonceSize()], decoded[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("decrypt integration credential")
	}
	return string(plain), nil
}
