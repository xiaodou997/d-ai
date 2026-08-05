package postgres

import (
	"errors"
	"strings"
	"testing"

	"xiaodou/dai/internal/ai/core/upstream"
	coreupstream "xiaodou/dai/internal/ai/core/upstream"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/secret"
)

func TestUpstreamProviderFamilyFromProtocol(t *testing.T) {
	cases := []struct {
		protocol domain.UpstreamProtocol
		want     upstream.ProviderFamily
	}{
		{protocol: domain.ProtocolOpenAIChat, want: upstream.ProviderFamilyOpenAICompatible},
		{protocol: domain.ProtocolOpenAIResponses, want: upstream.ProviderFamilyOpenAICompatible},
		{protocol: domain.ProtocolAnthropicMessages, want: upstream.ProviderFamilyAnthropic},
		{protocol: domain.ProtocolGeminiGenerate, want: upstream.ProviderFamilyGoogle},
		{protocol: domain.ProtocolGeminiEmbeddings, want: upstream.ProviderFamilyGoogle},
	}

	for _, tc := range cases {
		if got := upstreamProviderFamilyFromProtocol(tc.protocol); got != tc.want {
			t.Fatalf("protocol=%q providerFamily=%q want %q", tc.protocol, got, tc.want)
		}
	}
}

func TestDecryptDirectProviderKey(t *testing.T) {
	t.Parallel()

	const (
		master = "0123456789abcdef0123456789abcdef"
		plain  = "sk-test-123"
	)
	ciphertext, err := secret.EncryptProviderKey(master, plain)
	if err != nil {
		t.Fatalf("EncryptProviderKey: %v", err)
	}

	got, err := decryptDirectProviderKey(master, ciphertext)
	if err != nil {
		t.Fatalf("decryptDirectProviderKey: %v", err)
	}
	if got != plain {
		t.Fatalf("plaintext = %q, want %q", got, plain)
	}
}

// 解密失败的根因必须留在 rejection 里：主密钥配错与账号没配凭证，
// 两者都表现为「目标被静默剔除 → no_available_route」，日志里分不出来就没法排查。
func TestCredentialRejectionKeepsCause(t *testing.T) {
	rejection, ok := coreupstream.RuntimeBindingRejectionFromError(
		credentialRejection(errors.New("cipher: message authentication failed")))
	if !ok {
		t.Fatal("credentialRejection must produce a RuntimeBindingRejection")
	}
	if rejection.Code != coreupstream.BindingRejectionCredentialUnavailable {
		t.Fatalf("code = %q, want credential_unavailable", rejection.Code)
	}
	if !strings.Contains(rejection.Detail, "decrypt provider credential") {
		t.Fatalf("detail = %q, want it to name the decryption step", rejection.Detail)
	}
	if !strings.Contains(rejection.Detail, "message authentication failed") {
		t.Fatalf("detail = %q, want the underlying cause preserved", rejection.Detail)
	}
	// 仍要能被路由层当作「无绑定」跳过，而不是升级成 500。
	if !errors.Is(credentialRejection(errors.New("boom")), coreupstream.ErrNoRuntimeBinding) {
		t.Fatal("credential rejection must still unwrap to ErrNoRuntimeBinding")
	}
}
