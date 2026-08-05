package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	plainPrefix  = "plain:"
	aesGCMPrefix = "aesgcm:v1:"
)

func EncryptProviderKey(master string, plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("provider key is required")
	}
	if master == "" {
		return "", errors.New("provider key master is required")
	}

	key := sha256.Sum256([]byte(master))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("create provider key cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create provider key gcm: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate provider key nonce: %w", err)
	}

	encrypted := aead.Seal(nil, nonce, []byte(plaintext), nil)
	return aesGCMPrefix +
		base64.RawURLEncoding.EncodeToString(nonce) +
		":" +
		base64.RawURLEncoding.EncodeToString(encrypted), nil
}

func DecryptProviderKey(master string, ciphertext string) (string, error) {
	if strings.HasPrefix(ciphertext, plainPrefix) {
		return strings.TrimPrefix(ciphertext, plainPrefix), nil
	}
	if !strings.HasPrefix(ciphertext, aesGCMPrefix) {
		return "", errors.New("unsupported provider key ciphertext format")
	}
	if master == "" {
		return "", errors.New("provider key master is required")
	}

	parts := strings.Split(strings.TrimPrefix(ciphertext, aesGCMPrefix), ":")
	if len(parts) != 2 {
		return "", errors.New("invalid provider key ciphertext")
	}

	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode provider key nonce: %w", err)
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode provider key body: %w", err)
	}

	key := sha256.Sum256([]byte(master))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("create provider key cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create provider key gcm: %w", err)
	}
	if len(nonce) != aead.NonceSize() {
		return "", errors.New("invalid provider key nonce size")
	}

	plaintext, err := aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt provider key: %w", err)
	}
	return string(plaintext), nil
}
