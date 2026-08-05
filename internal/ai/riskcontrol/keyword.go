package riskcontrol

import (
	"strings"

	"github.com/cloudflare/ahocorasick"

	"xiaodou/dai/internal/ai/domain"
)

// compiledEntry is a keyword entry after normalization. patternIndices
// records which AC pattern slots this entry occupies (1 for normalized
// only, 2 if a distinct stripped pattern was also added).
type compiledEntry struct {
	original              domain.KeywordEntry
	normalized            string
	requireWithNormalized []string
	patternIndices        []int
}

// KeywordMatch is the result of a successful keyword match.
type KeywordMatch struct {
	Entry    domain.KeywordEntry
	HitLayer string // "keyword" | "pinyin"
}

// KeywordEngine wraps two Aho-Corasick automata (main keyword library +
// optional pinyin library) and the normalization configuration. It is
// rebuilt whenever the config changes; the old instance is discarded and
// replaced atomically by ConfigService.
type KeywordEngine struct {
	textMatcher    *ahocorasick.Matcher
	textEntries    []compiledEntry
	textIndexMap   map[int]int // pattern index → entry index
	pinyinMatcher  *ahocorasick.Matcher
	pinyinEntries  []compiledEntry
	pinyinIndexMap map[int]int
	pinyinEnabled  bool
	homoglyphExtra map[string]string
}

// NewKeywordEngine compiles the keyword and pinyin libraries from the
// given config. Entries are normalized (NFKC + lowercase + t2s + homoglyph)
// so that AC matching is invariant to full-width/traditional/homoglyph
// bypass attempts. The pinyin automaton pre-compiles each word to its
// pinyin representation; at match time the input is also converted to
// pinyin, so "葳信" hits the "微信" entry.
func NewKeywordEngine(cfg domain.KeywordConfig) *KeywordEngine {
	e := &KeywordEngine{
		homoglyphExtra: cfg.HomoglyphMapExtra,
		pinyinEnabled:  cfg.Pinyin.Enabled,
	}

	// Compile main keyword library.
	e.textMatcher, e.textEntries, e.textIndexMap = compileEntries(cfg.Entries, cfg.HomoglyphMapExtra, false)

	// Compile pinyin library (words → pinyin).
	if cfg.Pinyin.Enabled {
		e.pinyinMatcher, e.pinyinEntries, e.pinyinIndexMap = compileEntries(cfg.Pinyin.Entries, cfg.HomoglyphMapExtra, true)
	}

	return e
}

// compileEntries builds an AC automaton from keyword entries. If usePinyin
// is true, each word is converted to its pinyin form before being added
// as a pattern; otherwise the word is normalized through the standard
// pipeline. Returns the matcher, the compiled entries, and a pattern→entry
// index map (because one entry may occupy two pattern slots: normalized
// and stripped).
func compileEntries(entries []domain.KeywordEntry, homoglyphExtra map[string]string, usePinyin bool) (*ahocorasick.Matcher, []compiledEntry, map[int]int) {
	if len(entries) == 0 {
		return nil, nil, nil
	}

	patterns := make([]string, 0, len(entries)*2)
	compiled := make([]compiledEntry, 0, len(entries))
	indexMap := make(map[int]int)

	for _, entry := range entries {
		if entry.Word == "" {
			continue
		}

		ce := compiledEntry{original: entry}

		if usePinyin {
			pw := textToPinyin(entry.Word)
			if pw == "" {
				continue
			}
			ce.normalized = pw
			for _, rw := range entry.RequireWith {
				if rw != "" {
					ce.requireWithNormalized = append(ce.requireWithNormalized, textToPinyin(rw))
				}
			}
			ce.patternIndices = []int{len(patterns)}
			indexMap[len(patterns)] = len(compiled)
			patterns = append(patterns, pw)
		} else {
			norm := Normalize(entry.Word, homoglyphExtra)
			ce.normalized = norm.Normalized
			for _, rw := range entry.RequireWith {
				if rw != "" {
					rwNorm := Normalize(rw, homoglyphExtra)
					ce.requireWithNormalized = append(ce.requireWithNormalized, rwNorm.Normalized)
				}
			}
			ce.patternIndices = []int{len(patterns)}
			indexMap[len(patterns)] = len(compiled)
			patterns = append(patterns, norm.Normalized)
			if norm.Stripped != norm.Normalized {
				ce.patternIndices = append(ce.patternIndices, len(patterns))
				indexMap[len(patterns)] = len(compiled)
				patterns = append(patterns, norm.Stripped)
			}
		}

		compiled = append(compiled, ce)
	}

	if len(patterns) == 0 {
		return nil, nil, nil
	}
	return ahocorasick.NewStringMatcher(patterns), compiled, indexMap
}

// Match runs the keyword engine against the input text. Returns nil if
// no keyword matches (or if all matches fail their require_with constraint).
// The main AC automaton is tried first (covers ~95% of hits); the pinyin
// automaton runs only if enabled and the main pass found nothing.
func (e *KeywordEngine) Match(text string) *KeywordMatch {
	if e == nil || (e.textMatcher == nil && e.pinyinMatcher == nil) {
		return nil
	}

	norm := Normalize(text, e.homoglyphExtra)

	// 1. Main keyword library: match against both normalized and stripped.
	if e.textMatcher != nil {
		if m := e.matchAC(norm.Normalized, e.textMatcher, e.textEntries, e.textIndexMap, "keyword"); m != nil {
			return m
		}
		if norm.Stripped != norm.Normalized {
			if m := e.matchAC(norm.Stripped, e.textMatcher, e.textEntries, e.textIndexMap, "keyword"); m != nil {
				return m
			}
		}
	}

	// 2. Pinyin library: convert input to pinyin, then match.
	if e.pinyinEnabled && e.pinyinMatcher != nil {
		pinyinText := textToPinyin(norm.Normalized)
		if pinyinText != "" {
			if m := e.matchAC(pinyinText, e.pinyinMatcher, e.pinyinEntries, e.pinyinIndexMap, "pinyin"); m != nil {
				return m
			}
		}
	}

	return nil
}

// matchAC runs the AC automaton against the given text and checks
// require_with constraints for each hit. Block-level matches take priority
// over suspect-level matches.
func (e *KeywordEngine) matchAC(text string, matcher *ahocorasick.Matcher, entries []compiledEntry, indexMap map[int]int, layer string) *KeywordMatch {
	if text == "" {
		return nil
	}
	indices := matcher.MatchThreadSafe([]byte(text))
	if len(indices) == 0 {
		return nil
	}

	var suspectMatch *KeywordMatch
	for _, idx := range indices {
		entryIdx, ok := indexMap[idx]
		if !ok {
			continue
		}
		ce := &entries[entryIdx]

		// Check require_with co-occurrence.
		if !checkRequireWith(text, ce.requireWithNormalized) {
			continue
		}

		if ce.original.Level == domain.KeywordLevelBlock {
			return &KeywordMatch{Entry: ce.original, HitLayer: layer}
		}
		if suspectMatch == nil && ce.original.Level == domain.KeywordLevelSuspect {
			suspectMatch = &KeywordMatch{Entry: ce.original, HitLayer: layer}
		}
	}
	return suspectMatch
}

// checkRequireWith verifies that all co-occurrence words appear in the
// text. An empty list means no constraint (always passes).
func checkRequireWith(text string, requireWith []string) bool {
	for _, rw := range requireWith {
		if rw == "" {
			continue
		}
		if !strings.Contains(text, rw) {
			return false
		}
	}
	return true
}
