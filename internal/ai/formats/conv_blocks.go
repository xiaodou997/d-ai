package formats

import (
	"bytes"
	"encoding/json"
	"strings"
)

// 跨格式共享的内容块工具：data-url 拆分/拼装、tool 参数规整、以及把规范化块
// emit 成 OpenAI chat 的 content part（Claude/Gemini 的 emit 在各自文件里）。

// splitDataURL 解析形如 data:<mime>;base64,<data> 的串：命中则返回 (mime, data, "")；
// 否则把它当普通 URL，返回 (fallbackMediaType, "", value)。空值返回 (fallback, "", "")。
func splitDataURL(value, fallbackMediaType string) (mediaType, data, url string) {
	if value == "" {
		return fallbackMediaType, "", ""
	}
	if rest, ok := strings.CutPrefix(value, "data:"); ok {
		if mt, d, ok := strings.Cut(rest, ";base64,"); ok {
			return mt, d, ""
		}
	}
	return fallbackMediaType, "", value
}

// mediaDataOrURL 反向拼装：有 base64 数据则拼成 data-url，否则回退到 url。
func mediaDataOrURL(mediaType, data, url string) string {
	if data != "" {
		mt := mediaType
		if mt == "" {
			mt = "application/octet-stream"
		}
		return "data:" + mt + ";base64," + data
	}
	return url
}

// canonicalizeToolArgs 把 tool 输入规整为字符串（OpenAI tool_calls.function.arguments
// 要求字符串）。已是 JSON 字符串则取其内容，否则用紧凑 JSON 文本。
func canonicalizeToolArgs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	if s, ok := asString(raw); ok {
		return s
	}
	return compactJSON(raw)
}

// parseJSONish 把可能被字符串包裹的 JSON 解开：若是 JSON 字符串且其内容本身是合法
// JSON，则返回解开后的值；否则原样返回；空则返回空对象。
func parseJSONish(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	if s, ok := asString(raw); ok {
		if json.Valid([]byte(s)) {
			return json.RawMessage(s)
		}
		return raw
	}
	return raw
}

// toolOutputText 把工具结果值压成纯文本：字符串取内容，其它用紧凑 JSON。
func toolOutputText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := asString(raw); ok {
		return s
	}
	return compactJSON(raw)
}

// compactJSON 去除 JSON 中的无关空白，得到紧凑表示（确定性输出）。
func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// canonicalBlockToOpenAIPart 把一个规范化块 emit 成 OpenAI chat 的 content part。
// tool_use/tool_result/unknown 在 message 层单独处理，这里返回 ok=false。
func canonicalBlockToOpenAIPart(b ContentBlock) (json.RawMessage, bool) {
	switch b.Kind {
	case BlockText, BlockThinking:
		return mustRaw(map[string]any{"type": "text", "text": b.Text}), true
	case BlockImage:
		image := map[string]any{"url": mediaDataOrURL(b.MediaType, b.Data, b.URL)}
		if b.Detail != "" {
			image["detail"] = b.Detail
		}
		return mustRaw(map[string]any{"type": "image_url", "image_url": image}), true
	case BlockFile:
		file := map[string]any{}
		if b.FileID != "" {
			file["file_id"] = b.FileID
		}
		if b.Data != "" || b.FileURL != "" {
			file["file_data"] = mediaDataOrURL(b.MediaType, b.Data, b.FileURL)
		}
		if b.Filename != "" {
			file["filename"] = b.Filename
		}
		return mustRaw(map[string]any{"type": "file", "file": file}), true
	case BlockAudio:
		format := b.Format
		if format == "" {
			format = "mp3"
		}
		return mustRaw(map[string]any{
			"type":        "input_audio",
			"input_audio": map[string]any{"data": b.Data, "format": format},
		}), true
	default:
		return nil, false
	}
}

// openaiContentValueFromParts 把若干 content part 折叠成 OpenAI content 字段：
// 全是纯文本则合并为单个字符串；toolOnly 且无 part 时返回 null；否则保留数组。
func openaiContentValueFromParts(parts []json.RawMessage, toolOnly bool) json.RawMessage {
	if len(parts) == 0 {
		if toolOnly {
			return json.RawMessage("null")
		}
		return mustRaw([]any{})
	}
	allText := true
	var texts []string
	for _, p := range parts {
		obj, ok := decodeObject(p)
		if !ok {
			allText = false
			break
		}
		t, hasText := asString(field(obj, "text"))
		if !hasText {
			allText = false
			break
		}
		if pt, ok := asString(field(obj, "type")); ok && pt != "text" {
			allText = false
			break
		}
		texts = append(texts, t)
	}
	if allText {
		return rawString(strings.Join(texts, ""))
	}
	return mustRaw(parts)
}

// openaiContentText 抽取 OpenAI content（字符串或 part 数组）里的纯文本，用 "\n" 连接。
func openaiContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := asString(raw); ok {
		return s
	}
	if parts, ok := decodeArray(raw); ok {
		var out []string
		for _, p := range parts {
			obj, ok := decodeObject(p)
			if !ok {
				continue
			}
			if t, ok := asString(field(obj, "text")); ok {
				out = append(out, t)
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}
