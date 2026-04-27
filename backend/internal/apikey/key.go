package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const Prefix = "sk-ai-"

func Generate() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return Prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func Hash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func PrefixForDisplay(key string) string {
	if len(key) <= 14 {
		return key
	}
	return key[:14]
}

func ExtractBearer(authHeader string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", errors.New("missing bearer token")
	}
	key := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
	if key == "" {
		return "", errors.New("empty bearer token")
	}
	if !strings.HasPrefix(key, Prefix) {
		return "", errors.New("invalid api key prefix")
	}
	return key, nil
}
