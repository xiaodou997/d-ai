package formats

import "strings"

// 跨格式的推理力度（reasoning effort）⇄ 思考预算（thinking budget）映射，
// 抄 Aether formats/shared/model_directives.rs 的 ReasoningEffort。

// reasoningEffortToBudget 把 OpenAI reasoning_effort 映射为 Claude/Gemini 的思考预算 token 数。
func reasoningEffortToBudget(effort string) (uint64, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return 1280, true
	case "medium":
		return 2048, true
	case "high":
		return 4096, true
	case "xhigh", "max":
		return 8192, true
	default:
		return 0, false
	}
}

// budgetToReasoningEffort 把思考预算 token 数反映射为 OpenAI reasoning_effort。
func budgetToReasoningEffort(budget uint64) string {
	switch {
	case budget <= 1664:
		return "low"
	case budget <= 3072:
		return "medium"
	case budget <= 6144:
		return "high"
	default:
		return "xhigh"
	}
}

// claudeOutputEffortToOpenAI 规范化 Claude output_config.effort 到 OpenAI reasoning_effort。
func claudeOutputEffortToOpenAI(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "xhigh", "max":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return ""
	}
}

// openaiEffortToClaudeOutput 把 OpenAI reasoning_effort 映射为 Claude output_config.effort。
func openaiEffortToClaudeOutput(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
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
