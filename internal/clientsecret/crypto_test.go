package clientsecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

func TestKeyringEncryptsWithActiveKeyIDAndRotates(t *testing.T) {
	oldKey := "0123456789abcdef0123456789abcdef"
	newKey := "abcdef0123456789abcdef0123456789"
	keyring, err := NewKeyring("v2", newKey, map[string]string{"v1": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigureKeyring(keyring); err != nil {
		t.Fatal(err)
	}

	ciphertext, err := Encrypt("rotate-me")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ciphertext, "enc:v1:v2:") {
		t.Fatalf("ciphertext = %q, want active key ID", ciphertext)
	}
	if NeedsReencrypt(ciphertext) {
		t.Fatal("active ciphertext unexpectedly needs re-encryption")
	}

	oldCiphertext, err := encryptLegacyCurrent(oldKey, "old-value")
	if err != nil {
		t.Fatal(err)
	}
	if !NeedsReencrypt(oldCiphertext) {
		t.Fatal("legacy ciphertext should need re-encryption")
	}
	if got, err := Decrypt(oldCiphertext); err != nil || got != "old-value" {
		t.Fatalf("Decrypt legacy ciphertext = %q, %v", got, err)
	}
	reencrypted, err := Reencrypt(oldCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(reencrypted, "enc:v1:v2:") || NeedsReencrypt(reencrypted) {
		t.Fatalf("reencrypted ciphertext = %q", reencrypted)
	}
}

func TestKeyringDecryptsLegacyProviderCiphertext(t *testing.T) {
	master := "0123456789abcdef0123456789abcdef"
	if err := Configure(master); err != nil {
		t.Fatal(err)
	}
	ciphertext, err := encryptLegacyProvider(master, "provider-secret")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(ciphertext)
	if err != nil || got != "provider-secret" {
		t.Fatalf("Decrypt legacy provider = %q, %v", got, err)
	}
}

func TestKeyringRejectsUnknownPreviousKeyID(t *testing.T) {
	keyring, err := NewKeyring("v2", "0123456789abcdef0123456789abcdef", map[string]string{"bad id": "abcdef0123456789abcdef0123456789"})
	if err == nil || keyring != nil {
		t.Fatalf("NewKeyring accepted invalid previous ID: %v", err)
	}
}

func encryptLegacyCurrent(master, plaintext string) (string, error) {
	key, err := parseMasterKey(master)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return legacyPrefix + base64.RawStdEncoding.EncodeToString(append(nonce, aead.Seal(nil, nonce, []byte(plaintext), nil)...)), nil
}

func encryptLegacyProvider(master, plaintext string) (string, error) {
	// This helper mirrors the pre-keyring provider format.
	key := sha256.Sum256([]byte(master))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return providerPrefix + base64.RawURLEncoding.EncodeToString(nonce) + ":" +
		base64.RawURLEncoding.EncodeToString(aead.Seal(nil, nonce, []byte(plaintext), nil)), nil
}
