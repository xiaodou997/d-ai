package clientsecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

const (
	currentPrefix  = "enc:v1:"
	legacyPrefix   = "v1:"
	providerPrefix = "aesgcm:v1:"
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type keyMaterial struct {
	id   string
	raw  string
	aead cipher.AEAD
}

// Keyring contains the active encryption key and a bounded set of previous
// keys. Previous keys allow rolling deployments to decrypt existing records
// while all new writes use the active key.
type Keyring struct {
	activeID string
	keys     map[string]keyMaterial
}

var (
	globalMu      sync.RWMutex
	globalKeyring *Keyring
)

// NewKeyring validates the active key and all grace-period keys. The map key
// is the stable key ID stored in ciphertext; its value is the raw or base64
// encoded key accepted by parseMasterKey.
func NewKeyring(activeID, activeKey string, previous map[string]string) (*Keyring, error) {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" {
		activeID = "v1"
	}
	if err := validateKeyID(activeID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(activeKey) == "" {
		return nil, fmt.Errorf("security.secret_master_key is required")
	}
	keys := make(map[string]keyMaterial, len(previous)+1)
	active, err := newKeyMaterial(activeID, activeKey)
	if err != nil {
		return nil, fmt.Errorf("active secret key: %w", err)
	}
	keys[activeID] = active
	for id, raw := range previous {
		id = strings.TrimSpace(id)
		if err := validateKeyID(id); err != nil {
			return nil, err
		}
		if id == activeID {
			return nil, fmt.Errorf("previous secret key ID %q duplicates active key ID", id)
		}
		if _, exists := keys[id]; exists {
			return nil, fmt.Errorf("duplicate previous secret key ID %q", id)
		}
		material, err := newKeyMaterial(id, raw)
		if err != nil {
			return nil, fmt.Errorf("previous secret key %q: %w", id, err)
		}
		keys[id] = material
	}
	return &Keyring{activeID: activeID, keys: keys}, nil
}

func validateKeyID(id string) error {
	if !keyIDPattern.MatchString(id) {
		return fmt.Errorf("invalid secret master key ID %q", id)
	}
	return nil
}

func newKeyMaterial(id, raw string) (keyMaterial, error) {
	key, err := parseMasterKey(raw)
	if err != nil {
		return keyMaterial{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return keyMaterial{}, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return keyMaterial{}, fmt.Errorf("create gcm cipher: %w", err)
	}
	return keyMaterial{id: id, raw: strings.TrimSpace(raw), aead: aead}, nil
}

// Configure preserves the original single-key API for tests and older
// callers. New ciphertext is still written in the versioned key-ID format.
func Configure(masterKey string) error {
	keyring, err := NewKeyring("legacy", masterKey, nil)
	if err != nil {
		return err
	}
	return ConfigureKeyring(keyring)
}

func ConfigureKeyring(keyring *Keyring) error {
	if keyring == nil || keyring.activeID == "" {
		return fmt.Errorf("sensitive configuration keyring is required")
	}
	globalMu.Lock()
	globalKeyring = keyring
	globalMu.Unlock()
	return nil
}

func IsConfigured() bool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalKeyring != nil
}

func ActiveKeyID() string {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalKeyring == nil {
		return ""
	}
	return globalKeyring.activeID
}

func Encrypt(secret string) (string, error) {
	keyring, err := getConfigured()
	if err != nil {
		return "", err
	}
	return keyring.encrypt(secret)
}

func Decrypt(encrypted string) (string, error) {
	keyring, err := getConfigured()
	if err != nil {
		return "", err
	}
	return keyring.decrypt(encrypted)
}

// NeedsReencrypt reports whether ciphertext is in a legacy format or was
// encrypted with a previous key. Callers can use it for opportunistic online
// migration after a key rotation.
func NeedsReencrypt(encrypted string) bool {
	keyring, err := getConfigured()
	if err != nil {
		return false
	}
	value := strings.TrimSpace(encrypted)
	if !strings.HasPrefix(value, currentPrefix) {
		return true
	}
	parts := strings.SplitN(strings.TrimPrefix(value, currentPrefix), ":", 2)
	return len(parts) != 2 || parts[0] != keyring.activeID
}

// Reencrypt decrypts with the active or grace key and emits ciphertext using
// the current active key. It deliberately creates a fresh nonce every time.
func Reencrypt(encrypted string) (string, error) {
	plain, err := Decrypt(encrypted)
	if err != nil {
		return "", err
	}
	return Encrypt(plain)
}

func getConfigured() (*Keyring, error) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalKeyring == nil {
		return nil, fmt.Errorf("sensitive configuration cipher is not configured")
	}
	return globalKeyring, nil
}

func (k *Keyring) encrypt(secret string) (string, error) {
	material := k.keys[k.activeID]
	nonce := make([]byte, material.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := material.aead.Seal(nil, nonce, []byte(secret), nil)
	return currentPrefix + material.id + ":" +
		base64.RawStdEncoding.EncodeToString(nonce) + ":" +
		base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (k *Keyring) decrypt(encrypted string) (string, error) {
	value := strings.TrimSpace(encrypted)
	if value == "" {
		return "", fmt.Errorf("empty encrypted secret")
	}
	if strings.HasPrefix(value, currentPrefix) {
		return k.decryptCurrent(value)
	}
	if strings.HasPrefix(value, legacyPrefix) {
		payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, legacyPrefix))
		if err != nil {
			return "", fmt.Errorf("decode legacy client secret payload: %w", err)
		}
		for _, material := range k.keys {
			if plaintext, ok := openPayload(material.aead, payload); ok {
				return plaintext, nil
			}
		}
		return "", fmt.Errorf("decrypt legacy client secret: no configured key matched")
	}
	if strings.HasPrefix(value, providerPrefix) {
		return k.decryptLegacyProvider(value)
	}
	return "", fmt.Errorf("unsupported client secret format")
}

func (k *Keyring) decryptCurrent(value string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(value, currentPrefix), ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid encrypted secret format")
	}
	material, ok := k.keys[parts[0]]
	if !ok {
		return "", fmt.Errorf("secret master key ID %q is not configured", parts[0])
	}
	nonce, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode encrypted secret nonce: %w", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode encrypted secret body: %w", err)
	}
	if len(nonce) != material.aead.NonceSize() {
		return "", fmt.Errorf("invalid encrypted secret nonce size")
	}
	plaintext, err := material.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt encrypted secret: %w", err)
	}
	return string(plaintext), nil
}

func (k *Keyring) decryptLegacyProvider(value string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(value, providerPrefix), ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid legacy provider ciphertext")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode legacy provider nonce: %w", err)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode legacy provider body: %w", err)
	}
	for _, material := range k.keys {
		hashed := sha256.Sum256([]byte(material.raw))
		block, err := aes.NewCipher(hashed[:])
		if err != nil {
			continue
		}
		aead, err := cipher.NewGCM(block)
		if err != nil || len(nonce) != aead.NonceSize() {
			continue
		}
		if plaintext, err := aead.Open(nil, nonce, ciphertext, nil); err == nil {
			return string(plaintext), nil
		}
	}
	return "", fmt.Errorf("decrypt legacy provider secret: no configured key matched")
}

func openPayload(aead cipher.AEAD, payload []byte) (string, bool) {
	nonceSize := aead.NonceSize()
	if len(payload) < nonceSize {
		return "", false
	}
	plaintext, err := aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", false
	}
	return string(plaintext), true
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
