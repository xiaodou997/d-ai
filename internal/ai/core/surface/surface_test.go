package surface

import "testing"

func TestFamilyAndKinds(t *testing.T) {
	tests := []struct {
		id      ID
		family  Family
		isText  bool
		isImage bool
	}{
		{OpenAIChat, FamilyOpenAICompatible, true, false},
		{OpenAIResponses, FamilyOpenAICompatible, true, false},
		{AnthropicMessages, FamilyAnthropic, true, false},
		{GeminiText, FamilyGoogle, true, false},
		{OpenAIImages, FamilyOpenAICompatible, false, true},
		{GeminiImages, FamilyGoogle, false, true},
	}

	for _, tc := range tests {
		if got := tc.id.Family(); got != tc.family {
			t.Fatalf("%s Family() = %s, want %s", tc.id, got, tc.family)
		}
		if got := tc.id.IsText(); got != tc.isText {
			t.Fatalf("%s IsText() = %v, want %v", tc.id, got, tc.isText)
		}
		if got := tc.id.IsImage(); got != tc.isImage {
			t.Fatalf("%s IsImage() = %v, want %v", tc.id, got, tc.isImage)
		}
		if !IsKnown(tc.id) {
			t.Fatalf("%s should be known", tc.id)
		}
	}

	if IsKnown(ID("unknown")) {
		t.Fatal("unknown surface should not be known")
	}
}
