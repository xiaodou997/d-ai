package transport

import (
	"errors"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

type providerSecretCodecStub struct {
	encryptInput string
	ciphertext   string
	decryptInput string
	plaintext    string
	err          error
}

func (s *providerSecretCodecStub) Encrypt(plaintext string) (string, error) {
	s.encryptInput = plaintext
	return s.ciphertext, s.err
}

func (s *providerSecretCodecStub) Decrypt(ciphertext string) (string, error) {
	s.decryptInput = ciphertext
	return s.plaintext, s.err
}

func TestRiskControlConfigWriteUsesProviderSecretCodec(t *testing.T) {
	apiKey := "moderation-secret"
	codec := &providerSecretCodecStub{ciphertext: "encrypted-secret"}

	got, err := riskControlConfigFromWriteDTO(riskControlConfigWriteDTO{
		Provider: riskControlProviderWriteDTO{APIKey: &apiKey},
	}, domain.RiskControlConfig{}, codec)
	if err != nil {
		t.Fatalf("riskControlConfigFromWriteDTO() error = %v", err)
	}
	if codec.encryptInput != apiKey {
		t.Fatalf("Encrypt() input = %q, want %q", codec.encryptInput, apiKey)
	}
	if got.Provider.APIKeyCiphertext != "encrypted-secret" {
		t.Fatalf("APIKeyCiphertext = %q", got.Provider.APIKeyCiphertext)
	}
}

func TestRiskControlConfigWritePreservesOrClearsCiphertextWithoutCodec(t *testing.T) {
	current := domain.RiskControlConfig{Provider: domain.RiskControlProviderConfig{APIKeyCiphertext: "existing"}}

	preserved, err := riskControlConfigFromWriteDTO(riskControlConfigWriteDTO{}, current, nil)
	if err != nil {
		t.Fatalf("preserve config error = %v", err)
	}
	if preserved.Provider.APIKeyCiphertext != "existing" {
		t.Fatalf("preserved ciphertext = %q", preserved.Provider.APIKeyCiphertext)
	}

	empty := ""
	cleared, err := riskControlConfigFromWriteDTO(riskControlConfigWriteDTO{
		Provider: riskControlProviderWriteDTO{APIKey: &empty},
	}, current, nil)
	if err != nil {
		t.Fatalf("clear config error = %v", err)
	}
	if cleared.Provider.APIKeyCiphertext != "" {
		t.Fatalf("cleared ciphertext = %q", cleared.Provider.APIKeyCiphertext)
	}
}

func TestRiskControlConfigWriteWrapsCodecFailure(t *testing.T) {
	apiKey := "moderation-secret"
	cause := errors.New("encryption unavailable")

	_, err := riskControlConfigFromWriteDTO(riskControlConfigWriteDTO{
		Provider: riskControlProviderWriteDTO{APIKey: &apiKey},
	}, domain.RiskControlConfig{}, &providerSecretCodecStub{err: cause})
	if !errors.Is(err, cause) {
		t.Fatalf("riskControlConfigFromWriteDTO() error = %v, want wrapped cause", err)
	}
}
