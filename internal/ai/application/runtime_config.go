package application

import (
	"strings"
)

// runtime_config.go is the single source of truth for how an app's stored
// runtime configuration (the raw JSONB blob) is interpreted at invocation time.
// Both the public run gateway and the in-console chat/image paths parse config
// through ParseRuntimeConfig so the two entry points can never silently diverge.
//
// The stored blob is a small, closed structure — no open-ended option maps ever
// reach an end user:
//
//	chat  app: {"chat":  {"creativity":"balanced","allow_attachments":true}}
//	image app: {"image": {"resolution":"auto","default_output_count":1,"max_output_count":1,"allow_output_count_override":false}}

// MaxAppAttachments caps how many attachments a chat app accepts per call. It is
// a fixed platform limit, never configured per app and never surfaced to users.
const MaxAppAttachments = 6

// Creativity presets are stored semantically and mapped to an upstream
// temperature at request-build time — we persist intent, not a magic number.
const (
	CreativityPrecise  = "precise"
	CreativityBalanced = "balanced"
	CreativityCreative = "creative"
)

const (
	DefaultImageOutputCount = 1
	MaxImageOutputCount     = 10
)

// RuntimeConfig is the parsed, typed view of an app's stored configuration.
// Exactly one of Chat/Image is populated, matching the app type.
type RuntimeConfig struct {
	Chat  *ChatRuntimeConfig
	Image *ImageRuntimeConfig
}

// ChatRuntimeConfig is the complete behaviour surface of a chat app: a
// creativity preset plus whether callers may attach images/files.
type ChatRuntimeConfig struct {
	Creativity       string
	AllowAttachments bool
}

// Temperature maps the creativity preset to an upstream temperature value.
func (c ChatRuntimeConfig) Temperature() float64 {
	switch c.Creativity {
	case CreativityPrecise:
		return 0.2
	case CreativityCreative:
		return 1.0
	default:
		return 0.6
	}
}

// MaxAttachments returns the per-call attachment limit for this app: the fixed
// platform cap when attachments are allowed, otherwise zero.
func (c ChatRuntimeConfig) MaxAttachments() int {
	if c.AllowAttachments {
		return MaxAppAttachments
	}
	return 0
}

// ImageRuntimeConfig is the complete surface of an image app. The app fixes the
// output resolution and controls whether callers may override its default count.
type ImageRuntimeConfig struct {
	Resolution               string
	AspectRatio              string
	DefaultOutputCount       int
	MaxOutputCount           int
	AllowOutputCountOverride bool
}

// ResolveOutputCount applies the app's count policy. A zero request means the
// caller omitted n and receives the configured default.
func (c ImageRuntimeConfig) ResolveOutputCount(requested int) (int, bool) {
	if requested == 0 {
		return c.DefaultOutputCount, true
	}
	if requested < 1 || requested > c.MaxOutputCount {
		return 0, false
	}
	if requested != c.DefaultOutputCount && !c.AllowOutputCountOverride {
		return 0, false
	}
	return requested, true
}

// ParseRuntimeConfig is the sole interpreter of an app's stored config blob.
func ParseRuntimeConfig(appType AppType, raw map[string]any) RuntimeConfig {
	switch appType {
	case AppTypeChatAgent:
		section := nestedMap(raw, "chat")
		return RuntimeConfig{Chat: &ChatRuntimeConfig{
			Creativity:       NormalizeCreativity(stringFromAny(section["creativity"])),
			AllowAttachments: boolFromAny(section["allow_attachments"]),
		}}
	case AppTypeImageGenerationAgent, AppTypeImageEditAgent:
		section := nestedMap(raw, "image")
		resolution, aspectRatio := NormalizeOpenAIImageConfig(
			stringFromAny(section["resolution"]),
			stringFromAny(section["aspect_ratio"]),
		)
		defaultCount := normalizeImageOutputCount(intFromAny(section["default_output_count"]), DefaultImageOutputCount)
		maxCount := normalizeImageOutputCount(intFromAny(section["max_output_count"]), defaultCount)
		if maxCount < defaultCount {
			maxCount = defaultCount
		}
		return RuntimeConfig{Image: &ImageRuntimeConfig{
			Resolution:               resolution,
			AspectRatio:              aspectRatio,
			DefaultOutputCount:       defaultCount,
			MaxOutputCount:           maxCount,
			AllowOutputCountOverride: boolFromAny(section["allow_output_count_override"]),
		}}
	default:
		return RuntimeConfig{}
	}
}

// ToStored produces the canonical, normalized blob to persist for an app. The
// write path runs the incoming payload through ParseRuntimeConfig then ToStored
// so only well-formed, closed config ever lands in the database.
func (c RuntimeConfig) ToStored() map[string]any {
	switch {
	case c.Chat != nil:
		return map[string]any{"chat": map[string]any{
			"creativity":        c.Chat.Creativity,
			"allow_attachments": c.Chat.AllowAttachments,
		}}
	case c.Image != nil:
		return map[string]any{"image": map[string]any{
			"resolution":                  c.Image.Resolution,
			"aspect_ratio":                c.Image.AspectRatio,
			"default_output_count":        c.Image.DefaultOutputCount,
			"max_output_count":            c.Image.MaxOutputCount,
			"allow_output_count_override": c.Image.AllowOutputCountOverride,
		}}
	default:
		return map[string]any{}
	}
}

// NormalizeCreativity clamps an arbitrary value to a known preset.
func NormalizeCreativity(value string) string {
	switch value {
	case CreativityPrecise, CreativityBalanced, CreativityCreative:
		return value
	default:
		return CreativityBalanced
	}
}

// RenderTemplate substitutes {{var}} / {{ var }} placeholders. It is the single
// shared implementation used by both the run gateway and the console paths.
func RenderTemplate(templateText string, variables map[string]string) string {
	out := templateText
	for key, value := range variables {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
		out = strings.ReplaceAll(out, "{{ "+key+" }}", value)
	}
	return out
}

func nestedMap(source map[string]any, key string) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	if nested, ok := source[key].(map[string]any); ok {
		return nested
	}
	return map[string]any{}
}

func stringFromAny(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func boolFromAny(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	}
	return 0
}

func normalizeImageOutputCount(value, fallback int) int {
	if value >= 1 && value <= MaxImageOutputCount {
		return value
	}
	return fallback
}
