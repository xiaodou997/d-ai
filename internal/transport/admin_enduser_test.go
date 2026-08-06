package transport

import "testing"

func TestNormalizedOptionalText(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		set, value := normalizedOptionalText(nil)
		if set || value != "" {
			t.Fatalf("expected omitted value, got set=%v value=%q", set, value)
		}
	})

	t.Run("trimmed", func(t *testing.T) {
		input := "  tenant note  "
		set, value := normalizedOptionalText(&input)
		if !set || value != "tenant note" {
			t.Fatalf("expected trimmed value, got set=%v value=%q", set, value)
		}
	})

	t.Run("explicit empty", func(t *testing.T) {
		input := "   "
		set, value := normalizedOptionalText(&input)
		if !set || value != "" {
			t.Fatalf("expected explicit empty value, got set=%v value=%q", set, value)
		}
	})
}
