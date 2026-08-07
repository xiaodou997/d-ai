package billing

import "testing"

func TestClassifyOpenAIImagePriceTier(t *testing.T) {
	tests := []struct {
		size string
		want string
		ok   bool
	}{
		{size: "auto", want: ImageResolution4K, ok: true},
		{size: "1024x1024", want: ImageResolution1K, ok: true},
		{size: "1536x1024", want: ImageResolution2K, ok: true},
		{size: "2048x2048", want: ImageResolution2K, ok: true},
		{size: "3840x2160", want: ImageResolution4K, ok: true},
		{size: "4096x4096", ok: false},
		{size: "100000000x100000000", ok: false},
		{size: "not-a-size", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			got, ok := ClassifyOpenAIImagePriceTier(tt.size)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ClassifyOpenAIImagePriceTier(%q) = %q, %v; want %q, %v", tt.size, got, ok, tt.want, tt.ok)
			}
		})
	}
}
