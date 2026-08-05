package formats

import (
	"encoding/json"
	"strings"
)

// ExtractReasoningEffort 从原始请求体中提取客户端声明的推理强度（协议转换前口径），
// 供 usage log 记录。budget→effort 反映射有档位损失（如 budget=3000 记 medium）。
// 仅做单层 JSON 探测，不走 canonical 全量解析；任何解析失败一律返回空串。
func ExtractReasoningEffort(src FormatID, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return ""
	}
	switch src {
	case FormatOpenAIChat:
		if effort, ok := asString(field(obj, "reasoning_effort")); ok {
			return normalizeReasoningEffort(effort)
		}
	case FormatOpenAIResponses:
		if reasoning, ok := decodeObject(field(obj, "reasoning")); ok {
			if effort := normalizeReasoningEffort(getStr(reasoning, "effort")); effort != "" {
				return effort
			}
			if b, ok := asUint(field(reasoning, "budget_tokens")); ok && b > 0 {
				return budgetToReasoningEffort(b)
			}
		}
	case FormatClaudeMessages:
		if oc, ok := decodeObject(field(obj, "output_config")); ok {
			if effort := claudeOutputEffortToOpenAI(getStr(oc, "effort")); effort != "" {
				return effort
			}
		}
		if thinking, ok := decodeObject(field(obj, "thinking")); ok {
			if t, ok := asString(field(thinking, "type")); ok && t != "enabled" {
				return ""
			}
			if b, ok := asUint(field(thinking, "budget_tokens")); ok && b > 0 {
				return budgetToReasoningEffort(b)
			}
		}
	case FormatGeminiGenerate:
		if gc, ok := decodeObject(fieldOr(obj, "generationConfig", "generation_config")); ok {
			if tc, ok := decodeObject(fieldOr(gc, "thinkingConfig", "thinking_config")); ok {
				if b, ok := asUint(fieldOr(tc, "thinkingBudget", "thinking_budget")); ok && b > 0 {
					return budgetToReasoningEffort(b)
				}
			}
		}
	}
	return ""
}

// normalizeReasoningEffort 把客户端传入的 effort 归一化到落库 allowlist：
// minimal 并入 low，非法值返回空串（DB 有 CHECK 约束，Go 侧必须兜底）。
func normalizeReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	case "max":
		return "max"
	default:
		return ""
	}
}
