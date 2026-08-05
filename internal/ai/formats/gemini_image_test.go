package formats

import (
	"testing"
)

func TestGeminiRequestImageIntent(t *testing.T) {
	t.Parallel()

	generate := []byte(`{"generationConfig":{"responseModalities":["TEXT","IMAGE"]},"contents":[{"role":"user","parts":[{"text":"draw a kite"}]}]}`)
	if isImage, isEdit := GeminiRequestImageIntent(generate); !isImage || isEdit {
		t.Fatalf("generate intent = %v/%v, want true/false", isImage, isEdit)
	}

	edit := []byte(`{"generationConfig":{"responseModalities":["IMAGE"]},"contents":[{"role":"user","parts":[{"text":"make it brighter"},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}]}`)
	if isImage, isEdit := GeminiRequestImageIntent(edit); !isImage || !isEdit {
		t.Fatalf("edit intent = %v/%v, want true/true", isImage, isEdit)
	}

	text := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
	if isImage, isEdit := GeminiRequestImageIntent(text); isImage || isEdit {
		t.Fatalf("text intent = %v/%v, want false/false", isImage, isEdit)
	}
}
