package application

import "testing"

func TestResolveOpenAIImageSize(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		aspect     string
		want       string
	}{
		{name: "preserves auto", resolution: ImageResolutionAuto, aspect: "16:9", want: "auto"},
		{name: "one k square", resolution: ImageResolution1K, aspect: "1:1", want: "1024x1024"},
		{name: "one k portrait", resolution: ImageResolution1K, aspect: "9:16", want: "768x1360"},
		{name: "two k landscape", resolution: ImageResolution2K, aspect: "16:9", want: "2720x1536"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveOpenAIImageSize(tt.resolution, tt.aspect); got != tt.want {
				t.Fatalf("ResolveOpenAIImageSize(%q, %q) = %q, want %q", tt.resolution, tt.aspect, got, tt.want)
			}
		})
	}
}

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

func TestNormalizeOpenAIImageConfigMigratesLegacySizes(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		aspect     string
		wantTier   string
		wantAspect string
	}{
		{name: "semantic", resolution: "2K", aspect: "16:9", wantTier: ImageResolution2K, wantAspect: "16:9"},
		{name: "legacy 1k", resolution: "1024x1024", wantTier: ImageResolution1K, wantAspect: "1:1"},
		{name: "legacy 2k", resolution: "1536x1024", wantTier: ImageResolution2K, wantAspect: "3:2"},
		{name: "legacy 4k", resolution: "3840x2160", wantTier: ImageResolutionAuto, wantAspect: DefaultImageAspectRatio},
		{name: "invalid legacy", resolution: "9x9", wantTier: ImageResolutionAuto, wantAspect: DefaultImageAspectRatio},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tier, aspect := NormalizeOpenAIImageConfig(tc.resolution, tc.aspect)
			if tier != tc.wantTier || aspect != tc.wantAspect {
				t.Fatalf("NormalizeOpenAIImageConfig(%q, %q) = (%q, %q), want (%q, %q)", tc.resolution, tc.aspect, tier, aspect, tc.wantTier, tc.wantAspect)
			}
		})
	}
}

func TestNormalizeImageAspectRatio(t *testing.T) {
	if got := NormalizeImageAspectRatio("32:18"); got != "16:9" {
		t.Fatalf("normalized ratio = %q, want 16:9", got)
	}
	if got := NormalizeImageAspectRatio("5:1"); got != DefaultImageAspectRatio {
		t.Fatalf("invalid ratio = %q, want %q", got, DefaultImageAspectRatio)
	}
}
