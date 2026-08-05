package surface

// ID identifies a concrete wire surface exposed to clients or used against
// upstreams. It intentionally replaces the overloaded "protocol" terminology
// in the existing AI runtime.
type ID string

const (
	OpenAIChat        ID = "openai_chat"
	OpenAIResponses   ID = "openai_responses"
	OpenAIEmbeddings  ID = "openai_embeddings"
	AnthropicMessages ID = "anthropic_messages"
	GeminiText        ID = "gemini_text"
	GeminiEmbeddings  ID = "gemini_embeddings"
	OpenAIImages      ID = "openai_images"
	GeminiImages      ID = "gemini_images"
)

// Family groups concrete surfaces into vendor-aligned families.
type Family string

const (
	FamilyOpenAICompatible Family = "openai_compatible"
	FamilyAnthropic        Family = "anthropic"
	FamilyGoogle           Family = "google"
	FamilyUnknown          Family = "unknown"
)

// Known returns the supported vNext surface set in stable order.
func Known() []ID {
	return []ID{
		OpenAIChat,
		OpenAIResponses,
		OpenAIEmbeddings,
		AnthropicMessages,
		GeminiText,
		GeminiEmbeddings,
		OpenAIImages,
		GeminiImages,
	}
}

// IsKnown reports whether id is part of the vNext surface catalog.
func IsKnown(id ID) bool {
	switch id {
	case OpenAIChat, OpenAIResponses, OpenAIEmbeddings, AnthropicMessages, GeminiText, GeminiEmbeddings, OpenAIImages, GeminiImages:
		return true
	default:
		return false
	}
}

// Family reports the vendor family for a concrete surface.
func (id ID) Family() Family {
	switch id {
	case OpenAIChat, OpenAIResponses, OpenAIEmbeddings, OpenAIImages:
		return FamilyOpenAICompatible
	case AnthropicMessages:
		return FamilyAnthropic
	case GeminiText, GeminiEmbeddings, GeminiImages:
		return FamilyGoogle
	default:
		return FamilyUnknown
	}
}

// IsText reports whether the surface primarily carries text-generation style
// requests.
func (id ID) IsText() bool {
	switch id {
	case OpenAIChat, OpenAIResponses, OpenAIEmbeddings, AnthropicMessages, GeminiText, GeminiEmbeddings:
		return true
	default:
		return false
	}
}

// IsImage reports whether the surface primarily carries image requests.
func (id ID) IsImage() bool {
	switch id {
	case OpenAIImages, GeminiImages:
		return true
	default:
		return false
	}
}
