package riskcontrol

import (
	"testing"
)

func TestNormalize_NFKC(t *testing.T) {
	// Full-width → half-width.
	got := Normalize("ＡＢＣ１２３", nil)
	if got.Normalized != "abc123" {
		t.Fatalf("expected abc123, got %q", got.Normalized)
	}
}

func TestNormalize_TraditionalToSimplified(t *testing.T) {
	got := Normalize("買個蘋果", nil)
	if got.Normalized != "买个苹果" {
		t.Fatalf("expected 买个苹果, got %q", got.Normalized)
	}
}

func TestNormalize_Lowercase(t *testing.T) {
	got := Normalize("Hello WORLD", nil)
	if got.Normalized != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got.Normalized)
	}
}

func TestNormalize_Homoglyph(t *testing.T) {
	// Zero-width characters should be removed.
	got := Normalize("敏\u200B感\u200C词", nil)
	if got.Normalized != "敏感词" {
		t.Fatalf("expected '敏感词', got %q", got.Normalized)
	}
}

func TestNormalize_StripSymbols(t *testing.T) {
	got := Normalize("敏*感*词", nil)
	if got.Stripped != "敏感词" {
		t.Fatalf("expected stripped '敏感词', got %q", got.Stripped)
	}
	// Normalized should preserve the symbols.
	if got.Normalized != "敏*感*词" {
		t.Fatalf("expected normalized '敏*感*词', got %q", got.Normalized)
	}
}

func TestNormalize_StripSpaces(t *testing.T) {
	got := Normalize("敏 感 词", nil)
	if got.Stripped != "敏感词" {
		t.Fatalf("expected stripped '敏感词', got %q", got.Stripped)
	}
}

func TestNormalize_HomoglyphExtra(t *testing.T) {
	// Site-specific homoglyph: map '★' to 'x'.
	got := Normalize("te★t", map[string]string{"★": "x"})
	if got.Normalized != "text" {
		t.Fatalf("expected 'text', got %q", got.Normalized)
	}
}

func TestNormalize_Mixed(t *testing.T) {
	// Full-width + traditional + lowercase + zero-width + symbols.
	got := Normalize("ＢＵＹ　買\u200B藥", nil)
	// NFKC: "BUY 買藥" → lowercase: "buy 買藥" → t2s: "buy 买药"
	// Stripped should remove the space.
	if got.Stripped != "buy买药" {
		t.Fatalf("expected stripped 'buy买药', got %q", got.Stripped)
	}
}
