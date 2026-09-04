package promptaudit

import "testing"

func TestParseQwen3GuardStrictAndPolicy(t *testing.T) {
	tests := []struct {
		name, input string
		wantAction  string
		wantErr     bool
	}{
		{"safe", "Safety: Safe\nCategories: None", "Allow", false},
		{"unsafe jailbreak", "Safety: Unsafe\nCategories: Jailbreak", "Block", false},
		{"controversial pii escalates", "Safety: Controversial\nCategories: PII", "Block", false},
		{"disabled known category warns", "Safety: Unsafe\nCategories: Violent", "Warn", false},
		{"unknown unsafe blocks", "Safety: Unsafe\nCategories: Future Risk", "Block", false},
		{"missing categories", "Safety: Safe", "", true},
		{"duplicate safety", "Safety: Safe\nSafety: Unsafe\nCategories: None", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQwen3Guard(tt.input, []string{"jailbreak", "pii"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v", err)
			}
			if !tt.wantErr && got.Action != tt.wantAction {
				t.Fatalf("action=%q want %q", got.Action, tt.wantAction)
			}
		})
	}
}

func TestNormalizeCategory(t *testing.T) {
	if got := NormalizeCategory("Suicide & Self-Harm"); got != "suicide_and_self_harm" {
		t.Fatalf("got %q", got)
	}
}
