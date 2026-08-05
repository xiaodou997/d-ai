package application

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	MaxPromptInputBytes   = 128 * 1024
	MaxPromptPlaceholders = 128
	MaxPromptNameRunes    = 80
)

var (
	ErrPromptInputTooLarge      = errors.New("prompt input is too large")
	ErrPromptPlaceholderInvalid = errors.New("prompt placeholder is invalid")
	ErrPromptPlaceholderMissing = errors.New("prompt placeholder is not bound")
	ErrPromptVariableMissing    = errors.New("prompt variable is missing")
)

type PromptSnapshot struct {
	PromptID     string
	PromptName   string
	Revision     int
	RenderedText string
}

type PromptResolveInput struct {
	Input     string
	Variables map[string]string
	Bindings  []RuntimePromptBinding
}

type ResolvedPrompt struct {
	Instruction string
	Input       string
	Snapshots   []PromptSnapshot
}

func (r ResolvedPrompt) CombinedText() string {
	parts := make([]string, 0, 2)
	if value := strings.TrimSpace(r.Instruction); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(r.Input); value != "" {
		parts = append(parts, value)
	}
	return strings.Join(parts, "\n\n")
}

type PromptStrategyResolver interface {
	Resolve(PromptResolveInput) (ResolvedPrompt, error)
}

func ResolvePrompt(strategy PromptStrategy, input PromptResolveInput) (ResolvedPrompt, error) {
	resolver, ok := promptStrategyRegistry[strategy]
	if !ok {
		return ResolvedPrompt{}, fmt.Errorf("unsupported prompt strategy %q", strategy)
	}
	return resolver.Resolve(input)
}

// ExtractPromptVariables returns the normalized placeholder names used by one
// prompt body. It shares the runtime parser so management and execution accept
// exactly the same Unicode names and brace syntax.
func ExtractPromptVariables(templateText string) ([]string, error) {
	placeholders, err := parsePromptPlaceholders(templateText)
	if err != nil {
		return nil, err
	}
	unique := make(map[string]struct{}, len(placeholders))
	for _, placeholder := range placeholders {
		unique[placeholder.Name] = struct{}{}
	}
	return sortedKeys(unique), nil
}

var promptStrategyRegistry = map[PromptStrategy]PromptStrategyResolver{
	PromptStrategyNone:            nonePromptStrategy{},
	PromptStrategyCallerVariables: callerVariablesPromptStrategy{},
	PromptStrategyBoundExact:      boundExactPromptStrategy{},
}

type nonePromptStrategy struct{}

func (nonePromptStrategy) Resolve(input PromptResolveInput) (ResolvedPrompt, error) {
	if err := validatePromptInput(input.Input); err != nil {
		return ResolvedPrompt{}, err
	}
	return ResolvedPrompt{Input: strings.TrimSpace(input.Input)}, nil
}

type callerVariablesPromptStrategy struct{}

func (callerVariablesPromptStrategy) Resolve(input PromptResolveInput) (ResolvedPrompt, error) {
	if err := validatePromptInput(input.Input); err != nil {
		return ResolvedPrompt{}, err
	}
	if len(input.Bindings) != 1 || input.Bindings[0].Role != PromptBindingSystem && input.Bindings[0].Role != PromptBindingInputTemplate {
		return ResolvedPrompt{}, fmt.Errorf("caller_variables strategy requires one primary prompt binding")
	}
	binding := input.Bindings[0]
	rendered, _, err := renderExactTemplate(binding.TemplateText, input.Variables, ErrPromptVariableMissing)
	if err != nil {
		return ResolvedPrompt{}, err
	}
	return ResolvedPrompt{
		Instruction: strings.TrimSpace(rendered),
		Input:       strings.TrimSpace(input.Input),
		Snapshots: []PromptSnapshot{{
			PromptID: binding.PromptID, PromptName: binding.PromptName,
			Revision: binding.PromptRevision, RenderedText: strings.TrimSpace(rendered),
		}},
	}, nil
}

type boundExactPromptStrategy struct{}

func (boundExactPromptStrategy) Resolve(input PromptResolveInput) (ResolvedPrompt, error) {
	if err := validatePromptInput(input.Input); err != nil {
		return ResolvedPrompt{}, err
	}
	requested, err := parsePromptPlaceholders(input.Input)
	if err != nil {
		return ResolvedPrompt{}, err
	}
	bindings := make(map[string]RuntimePromptBinding, len(input.Bindings))
	for _, binding := range input.Bindings {
		name, err := NormalizePromptName(binding.PromptName)
		if err != nil {
			return ResolvedPrompt{}, err
		}
		if _, exists := bindings[name]; exists {
			return ResolvedPrompt{}, fmt.Errorf("duplicate bound prompt name %q", binding.PromptName)
		}
		bindings[name] = binding
	}

	requestedNames := make(map[string]struct{}, len(requested))
	for _, placeholder := range requested {
		requestedNames[placeholder.Name] = struct{}{}
	}
	values := make(map[string]string, len(requestedNames))
	snapshots := make(map[string]PromptSnapshot, len(requestedNames))
	missing := make(map[string]struct{})
	for name := range requestedNames {
		binding, exists := bindings[name]
		if !exists {
			missing[name] = struct{}{}
			continue
		}
		rendered, _, err := renderExactTemplate(binding.TemplateText, input.Variables, ErrPromptVariableMissing)
		if err != nil {
			return ResolvedPrompt{}, fmt.Errorf("render bound prompt %q: %w", binding.PromptName, err)
		}
		values[name] = rendered
		snapshots[name] = PromptSnapshot{
			PromptID: binding.PromptID, PromptName: binding.PromptName,
			Revision: binding.PromptRevision, RenderedText: strings.TrimSpace(rendered),
		}
	}
	if len(missing) > 0 {
		return ResolvedPrompt{}, fmt.Errorf("%w: %s", ErrPromptPlaceholderMissing, strings.Join(sortedKeys(missing), ", "))
	}
	rendered, used, err := renderExactTemplate(input.Input, values, ErrPromptPlaceholderMissing)
	if err != nil {
		return ResolvedPrompt{}, err
	}
	outSnapshots := make([]PromptSnapshot, 0, len(used))
	for _, name := range used {
		outSnapshots = append(outSnapshots, snapshots[name])
	}
	return ResolvedPrompt{Input: strings.TrimSpace(rendered), Snapshots: outSnapshots}, nil
}

type promptPlaceholder struct {
	Name       string
	Start, End int
}

func parsePromptPlaceholders(input string) ([]promptPlaceholder, error) {
	if err := validatePromptInput(input); err != nil {
		return nil, err
	}
	if strings.Contains(input, "｛") || strings.Contains(input, "｝") {
		return nil, fmt.Errorf("%w: use ASCII {{ and }}", ErrPromptPlaceholderInvalid)
	}
	items := make([]promptPlaceholder, 0)
	for cursor := 0; cursor < len(input); {
		openOffset := strings.Index(input[cursor:], "{{")
		if openOffset < 0 {
			break
		}
		start := cursor + openOffset
		closeOffset := strings.Index(input[start+2:], "}}")
		if closeOffset < 0 {
			return nil, fmt.Errorf("%w: missing closing braces", ErrPromptPlaceholderInvalid)
		}
		end := start + 2 + closeOffset + 2
		rawName := input[start+2 : end-2]
		if strings.Contains(rawName, "{{") {
			return nil, fmt.Errorf("%w: nested opening braces", ErrPromptPlaceholderInvalid)
		}
		name, err := NormalizePromptName(rawName)
		if err != nil {
			return nil, err
		}
		items = append(items, promptPlaceholder{Name: name, Start: start, End: end})
		if len(items) > MaxPromptPlaceholders {
			return nil, fmt.Errorf("%w: more than %d placeholders", ErrPromptPlaceholderInvalid, MaxPromptPlaceholders)
		}
		cursor = end
	}
	return items, nil
}

func renderExactTemplate(templateText string, values map[string]string, missingError error) (string, []string, error) {
	placeholders, err := parsePromptPlaceholders(templateText)
	if err != nil {
		return "", nil, err
	}
	if len(placeholders) == 0 {
		return templateText, []string{}, nil
	}
	normalizedValues := make(map[string]string, len(values))
	originalNames := make(map[string]string, len(values))
	for key, value := range values {
		name, err := NormalizePromptName(key)
		if err != nil {
			return "", nil, err
		}
		if original, exists := originalNames[name]; exists && original != key {
			return "", nil, fmt.Errorf("%w: duplicate normalized name %q", ErrPromptPlaceholderInvalid, name)
		}
		normalizedValues[name] = value
		originalNames[name] = key
	}
	missingSet := map[string]struct{}{}
	usedSet := map[string]struct{}{}
	var out strings.Builder
	previous := 0
	for _, placeholder := range placeholders {
		out.WriteString(templateText[previous:placeholder.Start])
		value, ok := normalizedValues[placeholder.Name]
		if !ok {
			missingSet[placeholder.Name] = struct{}{}
			out.WriteString(templateText[placeholder.Start:placeholder.End])
		} else {
			usedSet[placeholder.Name] = struct{}{}
			out.WriteString(value)
		}
		previous = placeholder.End
	}
	out.WriteString(templateText[previous:])
	if len(missingSet) > 0 {
		missing := sortedKeys(missingSet)
		return "", nil, fmt.Errorf("%w: %s", missingError, strings.Join(missing, ", "))
	}
	return out.String(), sortedKeys(usedSet), nil
}

func NormalizePromptName(value string) (string, error) {
	value = norm.NFC.String(strings.TrimSpace(value))
	if value == "" || strings.Contains(value, "{{") || strings.Contains(value, "}}") || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%w: invalid name %q", ErrPromptPlaceholderInvalid, value)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > MaxPromptNameRunes {
		return "", fmt.Errorf("%w: name exceeds %d characters", ErrPromptPlaceholderInvalid, MaxPromptNameRunes)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: name contains a control character", ErrPromptPlaceholderInvalid)
		}
	}
	return value, nil
}

func validatePromptInput(value string) error {
	if len(value) > MaxPromptInputBytes {
		return ErrPromptInputTooLarge
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: input is not valid UTF-8", ErrPromptPlaceholderInvalid)
	}
	return nil
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
