package formats

import (
	"encoding/json"
	"strings"
)

// JSON 访问 helper：转换层统一用 map[string]json.RawMessage 表示对象、
// []json.RawMessage 表示数组，按需把字段解码成标量，避免 round-trip 时丢失未知字段。

// normLower 去空白并转小写，用于角色/类型等枚举串的规范化匹配。
func normLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// decodeObject 把原始 JSON 解码为对象（字段名→原始值）；非对象返回 false。
func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, false
	}
	return obj, true
}

// decodeArray 把原始 JSON 解码为数组；非数组返回 false。
func decodeArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil || arr == nil {
		return nil, false
	}
	return arr, true
}

// asString 解码字符串标量；非字符串返回 false。
func asString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// asBool 解码布尔标量。
func asBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

// asFloat 解码数字为 float64。
func asFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	return f, true
}

// asUint 解码非负整数为 uint64（截断小数）。
func asUint(raw json.RawMessage) (uint64, bool) {
	f, ok := asFloat(raw)
	if !ok || f < 0 {
		return 0, false
	}
	return uint64(f), true
}

// asInt 解码整数为 int64（截断小数）。
func asInt(raw json.RawMessage) (int64, bool) {
	f, ok := asFloat(raw)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

// field 取对象字段（不存在返回 nil）。
func field(obj map[string]json.RawMessage, key string) json.RawMessage {
	if obj == nil {
		return nil
	}
	return obj[key]
}

// fieldOr 依次尝试多个键名，返回首个存在的字段（兼容 camelCase/snake_case）。
func fieldOr(obj map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, k := range keys {
		if v := field(obj, k); v != nil {
			return v
		}
	}
	return nil
}

// getStr 取对象字段并解码为字符串（缺失/非串返回 ""）。
func getStr(obj map[string]json.RawMessage, key string) string {
	s, _ := asString(field(obj, key))
	return s
}

// mustRaw 把任意 Go 值序列化为 json.RawMessage（仅用于已知可序列化的内部构造）。
func mustRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// rawString 把字符串包成 JSON 字符串原文。
func rawString(s string) json.RawMessage {
	return mustRaw(s)
}

// objHasKey 报告对象是否含某键。
func objHasKey(obj map[string]json.RawMessage, key string) bool {
	_, ok := obj[key]
	return ok
}

// ErrorFieldIsPresent 报告一个 JSON 对象的 `error` 字段是否真的携带了错误。
//
// 键存在本身说明不了任何事：OpenAI Responses 的成功响应体恒带 `"error": null`，
// 部分中转上游还会在成功时补 `"error": {}` 或 `"error": ""`。只有非空值才算失败。
// 这条规则一度在网关的每个判定点各写一遍，只有一处写对；现在全部收敛到这里。
func ErrorFieldIsPresent(raw json.RawMessage) bool {
	v := json.RawMessage(strings.TrimSpace(string(raw)))
	if len(v) == 0 {
		return false
	}
	switch string(v) {
	case "null", `""`, "{}", "[]", "0", "false":
		return false
	}
	return true
}

// objErrorFieldIsPresent 是 ErrorFieldIsPresent 的对象版便捷形式。
func objErrorFieldIsPresent(obj map[string]json.RawMessage) bool {
	return ErrorFieldIsPresent(field(obj, "error"))
}
