package postgres

import (
	"fmt"

	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

// dispatchProtocolFromSurface maps a resolved upstream binding surface to the
// protocol value exposed by preview candidates. Rule matching itself belongs
// to commercial.Service and the shared runtime resolver.
func dispatchProtocolFromSurface(id surface.ID) (domain.UpstreamProtocol, error) {
	switch id {
	case surface.OpenAIChat:
		return domain.ProtocolOpenAIChat, nil
	case surface.OpenAIResponses:
		return domain.ProtocolOpenAIResponses, nil
	case surface.OpenAIEmbeddings:
		return domain.ProtocolOpenAIEmbeddings, nil
	case surface.AnthropicMessages:
		return domain.ProtocolAnthropicMessages, nil
	case surface.GeminiText, surface.GeminiImages:
		return domain.ProtocolGeminiGenerate, nil
	case surface.GeminiEmbeddings:
		return domain.ProtocolGeminiEmbeddings, nil
	case surface.OpenAIImages:
		return domain.ProtocolOpenAIImages, nil
	default:
		return "", fmt.Errorf("unsupported dispatch client surface %q", id)
	}
}
