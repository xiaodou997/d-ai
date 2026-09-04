package promptaudit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

var categoryAliases = map[string]string{
	"violent": "violent", "violence": "violent",
	"non violent illegal acts":      "non_violent_illegal_acts",
	"sexual content or sexual acts": "sexual_content_or_sexual_acts", "sexual": "sexual_content_or_sexual_acts",
	"pii": "pii", "personal identifying information": "pii", "personal identifiable information": "pii",
	"suicide and self harm": "suicide_and_self_harm", "suicide self harm": "suicide_and_self_harm",
	"unethical acts": "unethical_acts", "unethical": "unethical_acts",
	"politically sensitive topics": "politically_sensitive_topics", "political": "politically_sensitive_topics",
	"copyright violation": "copyright_violation", "copyright": "copyright_violation",
	"jailbreak": "jailbreak", "prompt injection": "jailbreak",
}

type GuardError struct {
	Code      string
	Retryable bool
	Timeout   bool
	Cause     error
}

func (e *GuardError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Code
}
func (e *GuardError) Unwrap() error { return e.Cause }

func NormalizeCategory(value string) string {
	n := strings.ToLower(strings.TrimSpace(value))
	n = strings.NewReplacer("_", " ", "-", " ", "&", " and ", "/", " ").Replace(n)
	n = strings.Join(strings.Fields(n), " ")
	if v, ok := categoryAliases[n]; ok {
		return v
	}
	return strings.ReplaceAll(n, " ", "_")
}

func ParseQwen3Guard(content string, enabledScanners []string) (*Result, error) {
	var safety, categoriesLine string
	var sawSafety, sawCategories bool
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "safety:"):
			if sawSafety {
				return nil, &GuardError{Code: ErrorInvalidResponse}
			}
			sawSafety = true
			safety = strings.TrimSpace(line[len("safety:"):])
		case strings.HasPrefix(lower, "categories:"):
			if sawCategories {
				return nil, &GuardError{Code: ErrorInvalidResponse}
			}
			sawCategories = true
			categoriesLine = strings.TrimSpace(line[len("categories:"):])
		}
	}
	switch strings.ToLower(safety) {
	case "safe":
		safety = "Safe"
	case "controversial":
		safety = "Controversial"
	case "unsafe":
		safety = "Unsafe"
	default:
		return nil, &GuardError{Code: ErrorInvalidResponse}
	}
	if !sawCategories || categoriesLine == "" {
		return nil, &GuardError{Code: ErrorInvalidResponse}
	}
	knownCatalog := map[string]struct{}{}
	for _, id := range ScannerIDs {
		knownCatalog[id] = struct{}{}
	}
	enabled := map[string]struct{}{}
	for _, id := range enabledScanners {
		enabled[NormalizeCategory(id)] = struct{}{}
	}
	known, matched, unknown := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, raw := range strings.Split(categoriesLine, ",") {
		if strings.EqualFold(strings.TrimSpace(raw), "none") || strings.EqualFold(strings.TrimSpace(raw), "n/a") || strings.TrimSpace(raw) == "" {
			continue
		}
		id := NormalizeCategory(raw)
		if _, ok := knownCatalog[id]; ok {
			known[id] = struct{}{}
			if _, ok := enabled[id]; ok {
				matched[id] = struct{}{}
			}
		} else {
			sum := sha256.Sum256([]byte(id))
			unknown[fmt.Sprintf("unknown:%x", sum[:8])] = struct{}{}
		}
	}
	result := &Result{Safety: safety, Categories: orderedKeys(known), MatchedScanners: orderedKeys(matched), UnknownCategories: sortedKeys(unknown), ScannerScores: map[string]float64{}, Decision: "pass", RiskLevel: "low", Action: "Allow"}
	score := 0.0
	if safety == "Controversial" {
		score = .5
		result.Decision, result.RiskLevel, result.Action = "flag", "medium", "Warn"
	}
	if safety == "Unsafe" {
		score = 1
		if len(matched) > 0 || len(unknown) > 0 || len(known) == 0 {
			result.Decision, result.RiskLevel, result.Action = "critical", "critical", "Block"
		} else {
			result.Decision, result.RiskLevel, result.Action = "flag", "high", "Warn"
		}
	}
	for id := range matched {
		result.ScannerScores[id] = score
		if safety == "Controversial" && (id == "jailbreak" || id == "pii" || id == "suicide_and_self_harm") {
			result.Decision, result.RiskLevel, result.Action = "critical", "critical", "Block"
		}
	}
	return result, nil
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func orderedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	copyMap := map[string]struct{}{}
	for k := range m {
		copyMap[k] = struct{}{}
	}
	for _, id := range ScannerIDs {
		if _, ok := copyMap[id]; ok {
			out = append(out, id)
			delete(copyMap, id)
		}
	}
	return append(out, sortedKeys(copyMap)...)
}

type Scanner interface {
	Scan(context.Context, Endpoint, string, string, []string) (*Result, error)
}

type OpenAICompatibleScanner struct {
	ClientFor func(Endpoint) (*http.Client, error)
}

func NewOpenAICompatibleScanner() *OpenAICompatibleScanner {
	return &OpenAICompatibleScanner{ClientFor: NewSecureHTTPClient}
}

func (s *OpenAICompatibleScanner) Scan(ctx context.Context, endpoint Endpoint, apiKey, chunk string, scanners []string) (*Result, error) {
	clientFor := s.ClientFor
	if clientFor == nil {
		clientFor = NewSecureHTTPClient
	}
	client, err := clientFor(endpoint)
	if err != nil {
		return nil, &GuardError{Code: ErrorUnavailable, Cause: err}
	}
	target, err := ChatCompletionsURL(endpoint.BaseURL)
	if err != nil {
		return nil, &GuardError{Code: ErrorUnavailable, Cause: err}
	}
	payload := map[string]any{"model": endpoint.Model, "messages": []map[string]string{{"role": "user", "content": chunk}}, "temperature": 0, "max_tokens": 64, "seed": 42}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, &GuardError{Code: ErrorUnavailable, Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &GuardError{Code: ErrorUnavailable, Retryable: true, Timeout: errors.Is(err, context.DeadlineExceeded), Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &GuardError{Code: ErrorUnavailable, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024+1))
	if err != nil {
		return nil, &GuardError{Code: ErrorUnavailable, Retryable: true, Cause: err}
	}
	if len(raw) > 256*1024 {
		return nil, &GuardError{Code: ErrorInvalidResponse}
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) == 0 {
		return nil, &GuardError{Code: ErrorInvalidResponse}
	}
	content, err := openAIContentText(envelope.Choices[0].Message.Content)
	if err != nil {
		return nil, &GuardError{Code: ErrorInvalidResponse}
	}
	result, err := ParseQwen3Guard(content, scanners)
	if err != nil {
		return nil, err
	}
	result.EndpointID, result.ScannerVersion = endpoint.ID, endpoint.Model
	return result, nil
}

func openAIContentText(value any) (string, error) {
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		return text, nil
	}
	items, ok := value.([]any)
	if !ok {
		return "", errors.New("prompt guard content is invalid")
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		object, _ := item.(map[string]any)
		if text, ok := object["text"].(string); ok && strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "", errors.New("prompt guard content is empty")
	}
	return strings.Join(parts, "\n"), nil
}
