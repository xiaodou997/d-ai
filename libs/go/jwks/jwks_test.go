package jwks

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"
)

func TestParseRSAPublicKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	key := Key{
		Kty: "RSA",
		Kid: "test-key",
		N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
	}

	got, err := ParseRSAPublicKey(key)
	if err != nil {
		t.Fatalf("ParseRSAPublicKey returned error: %v", err)
	}

	if got.E != privateKey.PublicKey.E {
		t.Fatalf("E = %d, want %d", got.E, privateKey.PublicKey.E)
	}
	if got.N.Cmp(privateKey.PublicKey.N) != 0 {
		t.Fatal("N does not match")
	}
}
