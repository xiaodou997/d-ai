package application

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	ImageResolutionAuto = "auto"
	ImageResolution1K   = "1k"
	ImageResolution2K   = "2k"
	ImageResolution4K   = "4k"

	DefaultImageResolution  = ImageResolutionAuto
	DefaultImageAspectRatio = "1:1"

	openAIImage1KPixels = 1024 * 1024
	openAIImage2KPixels = 2048 * 2048
	openAIImage4KPixels = 3840 * 2160
	openAIImageMaxEdge  = 3840
	openAIImageGrid     = 16
)

// ImageResolutions are the app-configurable output modes. 4K remains a billing
// tier and a compatible API input, but is intentionally not exposed to apps.
var ImageResolutions = []string{
	ImageResolutionAuto,
	ImageResolution1K,
	ImageResolution2K,
}

// NormalizeResolution clamps app configuration to a supported output mode.
func NormalizeResolution(value string) string {
	if IsValidResolution(value) {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return DefaultImageResolution
}

// IsValidResolution reports whether value is an app-configurable output mode.
func IsValidResolution(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, resolution := range ImageResolutions {
		if value == resolution {
			return true
		}
	}
	return false
}

// NormalizeImageAspectRatio returns a reduced OpenAI Image 2-compatible ratio.
func NormalizeImageAspectRatio(value string) string {
	width, height, ok := parseImageAspectRatio(value)
	if !ok {
		return DefaultImageAspectRatio
	}
	divisor := greatestCommonDivisor(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

// NormalizeOpenAIImageConfig turns a stored image setting into the current
// semantic configuration. It also accepts legacy WxH values so existing apps
// retain their intended scale and shape after the tier-based UI rollout.
func NormalizeOpenAIImageConfig(resolution, aspectRatio string) (string, string) {
	if IsValidResolution(resolution) {
		return NormalizeResolution(resolution), NormalizeImageAspectRatio(aspectRatio)
	}

	width, height, ok := parseOpenAIImageSize(resolution)
	if !ok || !isOpenAIImageSize(width, height) {
		return DefaultImageResolution, NormalizeImageAspectRatio(aspectRatio)
	}
	tier, tierOK := ClassifyOpenAIImagePriceTier(resolution)
	if !tierOK || tier == ImageResolution4K {
		return DefaultImageResolution, NormalizeImageAspectRatio(aspectRatio)
	}
	legacyAspectRatio := fmt.Sprintf("%d:%d", width, height)
	if strings.TrimSpace(aspectRatio) != "" {
		legacyAspectRatio = aspectRatio
	}
	return tier, NormalizeImageAspectRatio(legacyAspectRatio)
}

// ResolveOpenAIImageSize converts an app's output mode and aspect ratio into an
// OpenAI Image 2 size. Auto is preserved exactly so the upstream can decide.
func ResolveOpenAIImageSize(resolution, aspectRatio string) string {
	resolution = NormalizeResolution(resolution)
	if resolution == ImageResolutionAuto {
		return ImageResolutionAuto
	}

	pixels := openAIImage1KPixels
	if resolution == ImageResolution2K {
		pixels = openAIImage2KPixels
	}
	width, height, ok := parseImageAspectRatio(aspectRatio)
	if !ok {
		width, height = 1, 1
	}
	ratio := float64(width) / float64(height)
	resolvedWidth := floorToImageGrid(math.Sqrt(float64(pixels) * ratio))
	resolvedHeight := floorToImageGrid(math.Sqrt(float64(pixels) / ratio))

	if resolvedWidth > openAIImageMaxEdge || resolvedHeight > openAIImageMaxEdge {
		scale := math.Min(
			float64(openAIImageMaxEdge)/float64(resolvedWidth),
			float64(openAIImageMaxEdge)/float64(resolvedHeight),
		)
		resolvedWidth = floorToImageGrid(float64(resolvedWidth) * scale)
		resolvedHeight = floorToImageGrid(float64(resolvedHeight) * scale)
	}
	for resolvedWidth*resolvedHeight > pixels {
		if resolvedWidth >= resolvedHeight {
			resolvedWidth -= openAIImageGrid
		} else {
			resolvedHeight -= openAIImageGrid
		}
	}
	return fmt.Sprintf("%dx%d", resolvedWidth, resolvedHeight)
}

// ClassifyOpenAIImagePriceTier maps a requested OpenAI size to the billing
// tier. Auto deliberately uses the highest tier because its output dimensions
// are selected by the upstream model.
func ClassifyOpenAIImagePriceTier(size string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(size), ImageResolutionAuto) {
		return ImageResolution4K, true
	}
	width, height, ok := parseOpenAIImageSize(size)
	if !ok {
		return "", false
	}
	if width > openAIImage4KPixels/height {
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

func parseImageAspectRatio(value string) (int, int, bool) {
	widthText, heightText, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || strings.Contains(heightText, ":") {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(widthText))
	height, heightErr := strconv.Atoi(strings.TrimSpace(heightText))
	if widthErr != nil || heightErr != nil || width < 1 || height < 1 || width > 10_000 || height > 10_000 {
		return 0, 0, false
	}
	ratio := float64(width) / float64(height)
	if ratio < 1.0/3.0 || ratio > 3.0 {
		return 0, 0, false
	}
	return width, height, true
}

func parseOpenAIImageSize(value string) (int, int, bool) {
	widthText, heightText, ok := strings.Cut(strings.ToLower(strings.TrimSpace(value)), "x")
	if !ok || strings.Contains(heightText, "x") {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(widthText))
	height, heightErr := strconv.Atoi(strings.TrimSpace(heightText))
	if widthErr != nil || heightErr != nil || width < 1 || height < 1 {
		return 0, 0, false
	}
	return width, height, true
}

func isOpenAIImageSize(width, height int) bool {
	if width%openAIImageGrid != 0 || height%openAIImageGrid != 0 {
		return false
	}
	if width > openAIImageMaxEdge || height > openAIImageMaxEdge {
		return false
	}
	pixels := width * height
	if pixels < 655_360 || pixels > openAIImage4KPixels {
		return false
	}
	ratio := float64(width) / float64(height)
	return ratio >= 1.0/3.0 && ratio <= 3.0
}

func floorToImageGrid(value float64) int {
	return int(math.Floor(value/float64(openAIImageGrid))) * openAIImageGrid
}

func greatestCommonDivisor(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
