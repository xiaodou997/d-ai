package riskcontrol

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NormalizedText holds the result of the normalization pipeline. Both
// fields are used by the AC matcher: Normalized preserves word boundaries
// (some keywords legitimately contain spaces/punctuation), while Stripped
// removes them so that separator-bypass attempts like "敏*感*词" still hit.
type NormalizedText struct {
	Normalized string // NFKC + lowercase + t2s + homoglyph
	Stripped   string // Normalized with punctuation/whitespace removed
}

// Normalize runs the full normalization pipeline on the input text:
//  1. Unicode NFKC (full-width → half-width, compatibility decomposition)
//  2. Lowercase (Latin letters)
//  3. Traditional → Simplified Chinese
//  4. Homoglyph substitution (builtin + site-specific overrides)
//  5. Stripped view (punctuation/whitespace/zero-width removed)
//
// homoglyphExtra is merged on top of the builtin table; keys and values
// are single-character strings. Pass nil to use only the builtin table.
func Normalize(text string, homoglyphExtra map[string]string) NormalizedText {
	// 1. NFKC normalization
	s := norm.NFKC.String(text)

	// 2. Lowercase
	s = strings.ToLower(s)

	// 3. Traditional → Simplified
	s = tradToSimp(s)

	// 4. Homoglyph substitution
	s = applyHomoglyphs(s, homoglyphExtra)

	// 5. Stripped view (remove punctuation, spaces, zero-width chars)
	stripped := stripSymbols(s)

	return NormalizedText{Normalized: s, Stripped: stripped}
}

// applyHomoglyphs replaces characters according to the builtin homoglyph
// table, overlaid with any site-specific overrides. Characters mapped to
// rune(0) are removed entirely (used for zero-width characters).
func applyHomoglyphs(s string, extra map[string]string) string {
	if len(s) == 0 {
		return s
	}

	// Merge extra into a lookup; extra takes precedence over builtin.
	out := make([]rune, 0, len(s))
	for _, r := range s {
		replacement, found := lookupHomoglyph(r, extra)
		if !found {
			out = append(out, r)
			continue
		}
		if replacement != 0 {
			out = append(out, replacement)
		}
		// replacement == 0 → drop the character (zero-width removal)
	}
	return string(out)
}

func lookupHomoglyph(r rune, extra map[string]string) (rune, bool) {
	// Check site-specific overrides first (single-char key → single-char value).
	for k, v := range extra {
		if len([]rune(k)) == 1 && []rune(k)[0] == r {
			runes := []rune(v)
			if len(runes) >= 1 {
				return runes[0], true
			}
			return 0, true // empty value → drop
		}
	}
	// Fall back to builtin table.
	if rep, ok := builtinHomoglyphs[r]; ok {
		return rep, true
	}
	return 0, false
}

// stripSymbols removes punctuation, whitespace, and other non-word
// characters from the normalized text, producing a "stripped view" where
// separator-bypass attempts like "敏*感*词" or "敏 感 词" collapse to
// "敏感词".
func stripSymbols(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if isStripChar(r) {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// isStripChar returns true for characters that should be removed in the
// stripped view: whitespace, punctuation, symbols, and control characters.
// Letters, digits, and CJK ideographs are preserved.
func isStripChar(r rune) bool {
	if r == 0 {
		return true
	}
	if unicode.IsSpace(r) {
		return true
	}
	if unicode.IsPunct(r) {
		return true
	}
	if unicode.IsSymbol(r) {
		return true
	}
	if unicode.IsControl(r) {
		return true
	}
	return false
}
