package billing

import (
	"strconv"
	"strings"
)

const (
	ImageResolution1K = "1k"
	ImageResolution2K = "2k"
	ImageResolution4K = "4k"

	openAIImage1KPixels = 1024 * 1024
	openAIImage2KPixels = 2048 * 2048
	openAIImage4KPixels = 3840 * 2160
)

// ClassifyOpenAIImagePriceTier maps a requested OpenAI image size to its
// billing tier. Auto uses the highest tier because upstream selects the size.
func ClassifyOpenAIImagePriceTier(size string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(size), "auto") {
		return ImageResolution4K, true
	}
	widthText, heightText, ok := strings.Cut(strings.ToLower(strings.TrimSpace(size)), "x")
	if !ok || strings.Contains(heightText, "x") {
		return "", false
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(widthText))
	height, heightErr := strconv.Atoi(strings.TrimSpace(heightText))
	if widthErr != nil || heightErr != nil || width < 1 || height < 1 || width > openAIImage4KPixels/height {
		return "", false
	}
	pixels := width * height
	switch {
	case pixels <= openAIImage1KPixels:
		return ImageResolution1K, true
	case pixels <= openAIImage2KPixels:
		return ImageResolution2K, true
	case pixels <= openAIImage4KPixels:
		return ImageResolution4K, true
	default:
		return "", false
	}
}
