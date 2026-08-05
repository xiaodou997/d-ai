package runtimecompat

import (
	"fmt"

	"xiaodou/dai/internal/ai/core/catalog"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
)

func CapabilityToCore(capability domain.CapabilityType) catalog.Capability {
	switch capability {
	case domain.CapabilityEmbedding:
		return catalog.CapabilityEmbedding
	case domain.CapabilityImage:
		return catalog.CapabilityImageGeneration
	case domain.CapabilityAudioTTS:
		return catalog.CapabilityAudioTTS
	case domain.CapabilityAudioSTT:
		return catalog.CapabilityAudioSTT
	case domain.CapabilityVideo:
		return catalog.CapabilityVideoGeneration
	default:
		return catalog.CapabilityChat
	}
}

func CapabilityFromCore(capability catalog.Capability) domain.CapabilityType {
	switch capability {
	case catalog.CapabilityEmbedding:
		return domain.CapabilityEmbedding
	case catalog.CapabilityImageGeneration, catalog.CapabilityImageEdit:
		return domain.CapabilityImage
	case catalog.CapabilityAudioTTS:
		return domain.CapabilityAudioTTS
	case catalog.CapabilityAudioSTT:
		return domain.CapabilityAudioSTT
	case catalog.CapabilityVideoGeneration:
		return domain.CapabilityVideo
	default:
		return domain.CapabilityChat
	}
}

func ProtocolToSurface(protocol domain.UpstreamProtocol) (surface.ID, error) {
	switch protocol {
	case domain.ProtocolOpenAIChat:
		return surface.OpenAIChat, nil
	case domain.ProtocolOpenAIResponses:
		return surface.OpenAIResponses, nil
	case domain.ProtocolOpenAIEmbeddings:
		return surface.OpenAIEmbeddings, nil
	case domain.ProtocolAnthropicMessages:
		return surface.AnthropicMessages, nil
	case domain.ProtocolGeminiGenerate:
		return surface.GeminiText, nil
	case domain.ProtocolGeminiEmbeddings:
		return surface.GeminiEmbeddings, nil
	case domain.ProtocolOpenAIImages:
		return surface.OpenAIImages, nil
	default:
		return "", fmt.Errorf("runtimecompat: unsupported legacy protocol %q", protocol)
	}
}

func MustProtocolToSurface(protocol domain.UpstreamProtocol) surface.ID {
	id, err := ProtocolToSurface(protocol)
	if err != nil {
		return ""
	}
	return id
}

func ProtocolToSurfaceForCapability(protocol domain.UpstreamProtocol, capability catalog.Capability) (surface.ID, error) {
	if protocol == domain.ProtocolGeminiGenerate {
		switch capability {
		case catalog.CapabilityImageGeneration, catalog.CapabilityImageEdit:
			return surface.GeminiImages, nil
		default:
			return surface.GeminiText, nil
		}
	}
	return ProtocolToSurface(protocol)
}

func MustProtocolToSurfaceForCapability(protocol domain.UpstreamProtocol, capability catalog.Capability) surface.ID {
	id, err := ProtocolToSurfaceForCapability(protocol, capability)
	if err != nil {
		return ""
	}
	return id
}

func SurfaceToProtocol(id surface.ID) (domain.UpstreamProtocol, error) {
	switch id {
	case surface.OpenAIChat:
		return domain.ProtocolOpenAIChat, nil
	case surface.OpenAIResponses:
		return domain.ProtocolOpenAIResponses, nil
	case surface.OpenAIEmbeddings:
		return domain.ProtocolOpenAIEmbeddings, nil
	case surface.AnthropicMessages:
		return domain.ProtocolAnthropicMessages, nil
	case surface.GeminiText:
		return domain.ProtocolGeminiGenerate, nil
	case surface.GeminiEmbeddings:
		return domain.ProtocolGeminiEmbeddings, nil
	case surface.OpenAIImages:
		return domain.ProtocolOpenAIImages, nil
	default:
		return "", fmt.Errorf("runtimecompat: unsupported core surface %q", id)
	}
}

func SurfaceToProtocolForCapability(id surface.ID, capability catalog.Capability) (domain.UpstreamProtocol, error) {
	if id == surface.GeminiImages {
		switch capability {
		case catalog.CapabilityImageGeneration, catalog.CapabilityImageEdit:
			return domain.ProtocolGeminiGenerate, nil
		default:
			return "", fmt.Errorf("runtimecompat: gemini_images requires image capability, got %q", capability)
		}
	}
	return SurfaceToProtocol(id)
}
