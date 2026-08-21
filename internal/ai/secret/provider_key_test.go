package secret

import (
	"strings"
	"testing"
)

func TestProviderKeyCodecRoundTrip(t *testing.T) {
	codec := NewProviderKeyCodec("0123456789abcdef0123456789abcdef")

	ciphertext, err := codec.Encrypt("provider-secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if !strings.HasPrefix(ciphertext, aesGCMPrefix) {
		t.Fatalf("Encrypt() ciphertext = %q, want legacy provider prefix", ciphertext)
	}
	plaintext, err := codec.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if plaintext != "provider-secret" {
		t.Fatalf("Decrypt() plaintext = %q", plaintext)
	}
}

func TestNilProviderKeyCodecFailsClosed(t *testing.T) {
	var codec *ProviderKeyCodec
	if _, err := codec.Encrypt("secret"); err == nil {
		t.Fatal("nil Encrypt() error = nil")
	}
	if _, err := codec.Decrypt("ciphertext"); err == nil {
		t.Fatal("nil Decrypt() error = nil")
	}
}
