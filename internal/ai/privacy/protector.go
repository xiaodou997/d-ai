package privacy

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

type rule struct {
	name string
	re   *regexp.Regexp
}

// Mapping is request-scoped and must never be persisted or logged. It is used
// only to restore values in the response of the same upstream request.
type Mapping struct {
	values map[string]string
}

type Protector struct{ rules []rule }

func NewProtector() *Protector {
	return &Protector{rules: []rule{
		{name: "email", re: regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)},
		{name: "phone", re: regexp.MustCompile(`(?:\+?86[ -]?)?1[3-9][0-9]{9}\b`)},
		{name: "bearer", re: regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{16,}`)},
		{name: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)},
		{name: "api_key", re: regexp.MustCompile(`\b(?:sk|key|token)[-_][A-Za-z0-9_-]{16,}\b`)},
	}}
}

func (p *Protector) RedactJSON(body []byte) ([]byte, *Mapping, error) {
	if p == nil || len(bytes.TrimSpace(body)) == 0 {
		return body, nil, nil
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		protected, mapping := p.redactText(body)
		return protected, mapping, nil
	}
	mapping := &Mapping{values: make(map[string]string)}
	value = p.walk(value, mapping)
	redacted, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	return redacted, mapping, nil
}

func (p *Protector) RestoreJSON(body []byte, mapping *Mapping) []byte {
	if p == nil || mapping == nil || len(mapping.values) == 0 || len(body) == 0 {
		return body
	}
	var value any
	if json.Unmarshal(body, &value) != nil {
		return p.RestoreText(body, mapping)
	}
	value = restoreWalk(value, mapping)
	out, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return out
}

func (p *Protector) RestoreText(body []byte, mapping *Mapping) []byte {
	if mapping == nil || len(mapping.values) == 0 {
		return body
	}
	text := string(body)
	keys := make([]string, 0, len(mapping.values))
	for key := range mapping.values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		text = strings.ReplaceAll(text, key, mapping.values[key])
	}
	return []byte(text)
}

func (p *Protector) walk(value any, mapping *Mapping) any {
	switch item := value.(type) {
	case string:
		return p.redactString(item, mapping)
	case []any:
		for i := range item {
			item[i] = p.walk(item[i], mapping)
		}
	case map[string]any:
		for key, child := range item {
			item[key] = p.walk(child, mapping)
		}
	}
	return value
}

func restoreWalk(value any, mapping *Mapping) any {
	switch item := value.(type) {
	case string:
		for placeholder, original := range mapping.values {
			item = strings.ReplaceAll(item, placeholder, original)
		}
		return item
	case []any:
		for i := range item {
			item[i] = restoreWalk(item[i], mapping)
		}
	case map[string]any:
		for key, child := range item {
			item[key] = restoreWalk(child, mapping)
		}
	}
	return value
}

func (p *Protector) redactString(value string, mapping *Mapping) string {
	for _, rule := range p.rules {
		value = rule.re.ReplaceAllStringFunc(value, func(original string) string {
			for placeholder, previous := range mapping.values {
				if previous == original {
					return placeholder
				}
			}
			placeholder := "__DAI_PII_" + rule.name + "_" + string(rune('A'+len(mapping.values))) + "__"
			mapping.values[placeholder] = original
			return placeholder
		})
	}
	return value
}

func (p *Protector) redactText(body []byte) ([]byte, *Mapping) {
	mapping := &Mapping{values: make(map[string]string)}
	return []byte(p.redactString(string(body), mapping)), mapping
}
