package postgres

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestChooseProviderProtocol(t *testing.T) {
	tests := []struct {
		name            string
		capability      domain.CapabilityType
		client          domain.UpstreamProtocol
		supported       []domain.UpstreamProtocol
		allowConversion bool
		stream          bool
		wantProvider    domain.UpstreamProtocol
		wantBucket      int
		wantOK          bool
	}{
		{name: "passthrough", capability: domain.CapabilityChat, client: domain.ProtocolOpenAIChat, supported: []domain.UpstreamProtocol{domain.ProtocolOpenAIChat}, wantProvider: domain.ProtocolOpenAIChat, wantOK: true},
		{name: "conversion disabled", capability: domain.CapabilityChat, client: domain.ProtocolAnthropicMessages, supported: []domain.UpstreamProtocol{domain.ProtocolOpenAIChat}},
		{name: "cross family chat", capability: domain.CapabilityChat, client: domain.ProtocolAnthropicMessages, supported: []domain.UpstreamProtocol{domain.ProtocolOpenAIChat}, allowConversion: true, wantProvider: domain.ProtocolOpenAIChat, wantBucket: 3, wantOK: true},
		{name: "embedding bridge", capability: domain.CapabilityEmbedding, client: domain.ProtocolOpenAIEmbeddings, supported: []domain.UpstreamProtocol{domain.ProtocolGeminiEmbeddings}, allowConversion: true, wantProvider: domain.ProtocolGeminiEmbeddings, wantBucket: 1, wantOK: true},
		{name: "streaming image bridge", capability: domain.CapabilityImage, client: domain.ProtocolOpenAIImages, supported: []domain.UpstreamProtocol{domain.ProtocolGeminiGenerate}, allowConversion: true, stream: true, wantProvider: domain.ProtocolGeminiGenerate, wantBucket: 1, wantOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, bucket, ok := chooseProviderProtocol(test.capability, test.client, test.supported, test.allowConversion, test.stream)
			if ok != test.wantOK || provider != test.wantProvider || bucket != test.wantBucket {
				t.Fatalf("chooseProviderProtocol() = (%q, %d, %t), want (%q, %d, %t)", provider, bucket, ok, test.wantProvider, test.wantBucket, test.wantOK)
			}
		})
	}
}
