package clientsecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
)

type cipherBox struct {
	aead cipher.AEAD
}

var (
	globalMu  sync.RWMutex
	globalBox *cipherBox
)

func Configure(masterKey string) error {
	box, err := newCipherBox(masterKey)
	if err != nil {
		return err
	}
	globalMu.Lock()
	globalBox = box
	globalMu.Unlock()
	return nil
}

func Encrypt(secret string) (string, error) {
	box, err := getConfigured()
	if err != nil {
		return "", err
	}
	return box.encrypt(secret)
}

func Decrypt(encrypted string) (string, error) {
	box, err := getConfigured()
	if err != nil {
		return "", err
	}
	return box.decrypt(encrypted)
}

func getConfigured() (*cipherBox, error) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalBox == nil {
		return nil, fmt.Errorf("sensitive configuration cipher is not configured")
	}
	return globalBox, nil
}

func newCipherBox(masterKey string) (*cipherBox, error) {
	key, err := parseMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm cipher: %w", err)
	}
	return &cipherBox{aead: aead}, nil
}

func parseMasterKey(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("security.secret_master_key is required")
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && validKeySize(len(decoded)) {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil && validKeySize(len(decoded)) {
		return decoded, nil
	}
	if validKeySize(len(value)) {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("secret master key must be 16/24/32 bytes, or base64 encoded key")
}

func validKeySize(size int) bool {
	return size == 16 || size == 24 || size == 32
}

func (b *cipherBox) encrypt(secret string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := b.aead.Seal(nil, nonce, []byte(secret), nil)
	payload := append(nonce, ciphertext...)
	return "v1:" + base64.RawStdEncoding.EncodeToString(payload), nil
}

func (b *cipherBox) decrypt(encrypted string) (string, error) {
	value := strings.TrimSpace(encrypted)
	if value == "" {
		return "", fmt.Errorf("empty encrypted secret")
	}
	if !strings.HasPrefix(value, "v1:") {
		return "", fmt.Errorf("unsupported client secret format")
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", fmt.Errorf("decode client secret payload: %w", err)
	}
	nonceSize := b.aead.NonceSize()
	if len(payload) < nonceSize {
		return "", fmt.Errorf("invalid client secret payload")
	}
	plaintext, err := b.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt client secret: %w", err)
	}
	return string(plaintext), nil
}
