// Package imagepayload classifies and decodes image bytes represented as
// data URLs or raw Base64 in fields that otherwise normally contain URLs.
package imagepayload

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

const inlineImageProbeLength = 512

// DecodeInlineImageValue decodes image payloads placed in a URL-like field.
// handled is false only when value should be treated as an external URL/path.
func DecodeInlineImageValue(value string) ([]byte, string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, "", false, nil
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		data, contentType, err := decodeDataURL(value)
		return data, contentType, true, err
	}
	if !LooksLikeInlineImageValue(value) {
		return nil, "", false, nil
	}
	data, contentType, err := DecodeBase64ImageBytes(value, "")
	return data, contentType, true, err
}

// LooksLikeInlineImageValue distinguishes raw image Base64 from URLs and
// paths without accepting arbitrary Base64 text as an image.
func LooksLikeInlineImageValue(value string) bool {
	value = removeASCIIWhitespace(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "data:") {
		return true
	}
	if strings.Contains(value, "://") || strings.HasPrefix(lower, "blob:") {
		return false
	}
	if len(value) < 64 {
		return false
	}
	sample := value
	if len(sample) > inlineImageProbeLength {
		sample = sample[:inlineImageProbeLength]
	}
	if mod := len(sample) % 4; mod != 0 {
		sample = sample[:len(sample)-mod]
	}
	if len(sample) < 32 {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(sample)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(sample)
	}
	return err == nil && len(data) > 0 && strings.HasPrefix(http.DetectContentType(data), "image/")
}

// DecodeBase64ImageBytes validates and decodes an image Base64 payload.
func DecodeBase64ImageBytes(value, contentType string) ([]byte, string, error) {
	value = removeASCIIWhitespace(strings.TrimSpace(value))
	if value == "" {
		return nil, "", errors.New("base64 image is empty")
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, "", errors.New("invalid base64 image")
	}
	if contentType == "" {
		contentType = http.DetectContentType(decoded)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "image/") {
		return nil, "", errors.New("base64 payload is not an image")
	}
	return decoded, contentType, nil
}

func decodeDataURL(value string) ([]byte, string, error) {
	metadata, payload, ok := strings.Cut(strings.TrimSpace(value), ",")
	if !ok {
		return nil, "", errors.New("invalid data url image")
	}
	metadata = strings.TrimPrefix(metadata, "data:")
	if metadata == "" {
		return nil, "", errors.New("invalid data url image")
	}
	if !strings.HasSuffix(strings.ToLower(metadata), ";base64") {
		return nil, "", errors.New("data url image must be base64 encoded")
	}
	contentType := strings.TrimSpace(metadata[:len(metadata)-len(";base64")])
	if contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return nil, "", errors.New("data url image must use image/* content type")
	}
	return DecodeBase64ImageBytes(payload, contentType)
}

func removeASCIIWhitespace(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, value)
}
