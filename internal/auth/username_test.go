package auth

import "testing"

func TestNormalizeUsername(t *testing.T) {
	tests := map[string]string{
		"alice":     "alice",
		" u_alice ": "u_alice",
		"":          "",
	}
	for input, want := range tests {
		if got := NormalizeUsername(input); got != want {
			t.Errorf("NormalizeUsername(%q) = %q, want %q", input, got, want)
		}
	}
}
