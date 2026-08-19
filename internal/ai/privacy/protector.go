package privacy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	defaultPlaceholderPrefix = "DAI"
	maxRules                 = 64
	maxPatternLength         = 4096
)

var (
	ruleIDPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	prefixPattern      = regexp.MustCompile(`^[A-Z0-9_]{1,32}$`)
	defaultRuleConfigs = []RuleConfig{
		{ID: "email", Name: "邮箱", Pattern: `(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`, Enabled: true, System: true},
		{ID: "phone", Name: "中国大陆手机号", Pattern: `(?:\+?86[ -]?)?1[3-9][0-9]{9}\b`, Enabled: true, System: true},
		{ID: "bearer", Name: "Bearer Token", Pattern: `(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{16,}`, Enabled: true, System: true},
		{ID: "jwt", Name: "JWT", Pattern: `\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`, Enabled: true, System: true},
		{ID: "api_key", Name: "API Key", Pattern: `\b(?:sk|key|token)[-_][A-Za-z0-9_-]{16,}\b`, Enabled: true, System: true},
	}
)

type RuleConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Enabled bool   `json:"enabled"`
	System  bool   `json:"system"`
}

type Config struct {
	Rules             []RuleConfig `json:"rules"`
	PlaceholderPrefix string       `json:"placeholderPrefix"`
}

type rule struct {
	id string
	re *regexp.Regexp
}

// Mapping is request-scoped and must never be persisted or logged. It is used
// only to restore values in the response of the same upstream request.
type Mapping struct {
	values map[string]string
}

type Protector struct {
	rules             []rule
	placeholderPrefix string
}

func DefaultConfig() Config {
	rules := make([]RuleConfig, len(defaultRuleConfigs))
	copy(rules, defaultRuleConfigs)
	return Config{Rules: rules, PlaceholderPrefix: defaultPlaceholderPrefix}
}

func NewProtector() *Protector {
	protector, err := NewProtectorWithConfig(DefaultConfig())
	if err != nil {
		panic(err)
	}
	return protector
}

func NewProtectorWithConfig(config Config) (*Protector, error) {
	normalized, err := ValidateConfig(config)
	if err != nil {
		return nil, err
	}
	rules := make([]rule, 0, len(normalized.Rules))
	for _, item := range normalized.Rules {
		if !item.Enabled {
			continue
		}
		compiled, err := regexp.Compile(item.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile rule %q: %w", item.ID, err)
		}
		rules = append(rules, rule{id: item.ID, re: compiled})
	}
	return &Protector{rules: rules, placeholderPrefix: normalized.PlaceholderPrefix}, nil
}

func ValidateConfig(config Config) (Config, error) {
	prefix := strings.ToUpper(strings.TrimSpace(config.PlaceholderPrefix))
	if !prefixPattern.MatchString(prefix) {
		return Config{}, fmt.Errorf("占位符前缀只能包含 1-32 位大写字母、数字或下划线")
	}
	if len(config.Rules) > maxRules {
		return Config{}, fmt.Errorf("脱敏规则不能超过 %d 条", maxRules)
	}

	seen := make(map[string]struct{}, len(config.Rules))
	rules := make([]RuleConfig, 0, len(config.Rules))
	for index, item := range config.Rules {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Pattern = strings.TrimSpace(item.Pattern)
		if !ruleIDPattern.MatchString(item.ID) {
			return Config{}, fmt.Errorf("第 %d 条规则标识只能包含字母、数字、下划线或连字符", index+1)
		}
		if item.Name == "" || len([]rune(item.Name)) > 64 {
			return Config{}, fmt.Errorf("规则 %q 的名称不能为空且不能超过 64 个字符", item.ID)
		}
		if item.Pattern == "" || len(item.Pattern) > maxPatternLength {
			return Config{}, fmt.Errorf("规则 %q 的正则不能为空且不能超过 %d 个字符", item.ID, maxPatternLength)
		}
		if _, exists := seen[item.ID]; exists {
			return Config{}, fmt.Errorf("规则标识 %q 重复", item.ID)
		}
		if _, err := regexp.Compile(item.Pattern); err != nil {
			return Config{}, fmt.Errorf("规则 %q 的正则无效: %w", item.ID, err)
		}
		seen[item.ID] = struct{}{}
		rules = append(rules, item)
	}
	return Config{Rules: rules, PlaceholderPrefix: prefix}, nil
}

func (p *Protector) RedactJSON(body []byte) ([]byte, *Mapping, error) {
	if p == nil || len(bytes.TrimSpace(body)) == 0 {
		return body, nil, nil
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		protected, mapping := p.RedactText(body)
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
			placeholder := fmt.Sprintf("__%s_PII_%s_%d__", p.placeholderPrefix, strings.ToUpper(rule.id), len(mapping.values)+1)
			for strings.Contains(value, placeholder) {
				placeholder += "_"
			}
			mapping.values[placeholder] = original
			return placeholder
		})
	}
	return value
}

func (p *Protector) RedactText(body []byte) ([]byte, *Mapping) {
	mapping := &Mapping{values: make(map[string]string)}
	return []byte(p.redactString(string(body), mapping)), mapping
}
