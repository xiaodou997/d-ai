package auth

import "testing"

func TestNormalizeEndUsername(t *testing.T) {
	tests := map[string]string{
		"alice":     "u_alice",
		" u_alice ": "u_alice",
		"":          "u_",
	}
	for input, want := range tests {
		if got := NormalizeEndUsername(input); got != want {
			t.Errorf("NormalizeEndUsername(%q) = %q, want %q", input, got, want)
		}
	}
}
