package egress

import (
	"strings"

	"xiaodou/uni-ai-api/internal/domain"
)

// Policy describes the public contract for one runtime response. It is derived
// from the selected route, not inferred from response bytes.
type Policy struct {
	PublicModel        string
	UpstreamModel      string
	Protocol           domain.UpstreamProtocol
	ProviderCode       string
	EndpointBaseURL    string
	Aliases            []string
	AllowVersionSuffix bool
}

// IsConfigured reports whether there is enough context to rewrite model
// identity fields.
func (p Policy) IsConfigured() bool {
	return p.PublicModel != ""
}

// IsModelIdentity reports whether value is an upstream-facing model identifier
// that must be presented as PublicModel.
func (p Policy) IsModelIdentity(value string) bool {
	if value == "" || value == p.PublicModel {
		return false
	}
	if p.UpstreamModel != "" && value == p.UpstreamModel {
		return true
	}
	for _, alias := range p.Aliases {
		if alias != "" && value == alias {
			return true
		}
	}
	return p.AllowVersionSuffix && isVersionedPublicModel(value, p.PublicModel)
}

// SensitiveTerms returns exact internal strings that should not be echoed in
// runtime error messages.
func (p Policy) SensitiveTerms() []string {
	terms := make([]string, 0, 2+len(p.Aliases))
	if p.UpstreamModel != "" && p.UpstreamModel != p.PublicModel {
		terms = append(terms, p.UpstreamModel)
	}
	if p.EndpointBaseURL != "" {
		terms = append(terms, p.EndpointBaseURL)
	}
	for _, alias := range p.Aliases {
		if alias != "" && alias != p.PublicModel && alias != p.UpstreamModel {
			terms = append(terms, alias)
		}
	}
	return terms
}

func isVersionedPublicModel(value, publicModel string) bool {
	if publicModel == "" || !strings.HasPrefix(value, publicModel+"-") {
		return false
	}
	suffix := strings.TrimPrefix(value, publicModel+"-")
	if suffix == "" {
		return false
	}
	// Date/snapshot suffixes start with a digit, e.g.
	// gpt-5.4-mini-2026-03-17 or claude-sonnet-4-20250514. This deliberately
	// avoids folding sibling model names such as gpt-4o or gpt-4-turbo into
	// gpt-4.
	return suffix[0] >= '0' && suffix[0] <= '9'
}
