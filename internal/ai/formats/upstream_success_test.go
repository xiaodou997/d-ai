package formats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testdata/upstream_success 收录**真实上游成功响应体**，每个协议一份，外加中转
// 站变体。它们存在的理由：网关曾把 OpenAI Responses 的成功体判成错误体（成功体
// 恒带 `"error": null`，而判据只看键是否存在），5 条路由全部 failover 后对客户端
// 报 502 all_routes_failed。手写的最小 JSON 兜不住这类 bug —— 只有形状真实的
// body 才能。新增上游/协议时往这个目录丢一份真实响应即可。
const upstreamSuccessDir = "testdata/upstream_success"

// loadUpstreamSuccessFixtures 返回 fixture 文件名→body。serving 包的回归测试读同
// 一份数据（走相对路径），确保网关层与转换层对「什么算错误体」的判断永远基于同样
// 的真实样本。
func loadUpstreamSuccessFixtures(t *testing.T, dir string) map[string]json.RawMessage {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixture dir %s: %v", dir, err)
	}
	out := make(map[string]json.RawMessage, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read fixture %s: %v", e.Name(), err)
		}
		out[e.Name()] = body
	}
	if len(out) == 0 {
		t.Fatalf("no fixtures found in %s", dir)
	}
	return out
}

// fixtureFormats 把 fixture 映射到它所属的协议，供 ParseResponse 断言。
var fixtureFormats = map[string]FormatID{
	"openai_responses.json":             FormatOpenAIResponses,
	"openai_chat.json":                  FormatOpenAIChat,
	"openai_chat_relay_null_error.json": FormatOpenAIChat,
	"anthropic_messages.json":           FormatClaudeMessages,
	"gemini_generate_content.json":      FormatGeminiGenerate,
}

// TestUpstreamSuccessFixturesAreNotErrors 是这次故障的核心回归：真实成功体一律
// 不得被当成错误。
func TestUpstreamSuccessFixturesAreNotErrors(t *testing.T) {
	fixtures := loadUpstreamSuccessFixtures(t, upstreamSuccessDir)
	for name, body := range fixtures {
		t.Run(name, func(t *testing.T) {
			obj, ok := decodeObject(body)
			if !ok {
				t.Fatalf("fixture is not a JSON object")
			}
			if objErrorFieldIsPresent(obj) {
				t.Errorf("success body was judged an error body")
			}
		})
	}
}

// TestUpstreamSuccessFixturesParse 确认成功体不仅没被判成错误，还能被对应协议的
// 转换层正常解析出内容 —— 否则「不算错误」也没有意义。
func TestUpstreamSuccessFixturesParse(t *testing.T) {
	fixtures := loadUpstreamSuccessFixtures(t, upstreamSuccessDir)
	for name, body := range fixtures {
		format, mapped := fixtureFormats[name]
		if !mapped {
			t.Errorf("fixture %s has no entry in fixtureFormats; add one", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			resp, err := ParseResponse(format, body)
			if err != nil {
				t.Fatalf("ParseResponse(%s) failed: %v", format, err)
			}
			if len(resp.Content) == 0 {
				t.Errorf("parsed response has no content blocks")
			}
		})
	}
}

func TestErrorFieldIsPresent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"missing", ``, false},
		{"null", `null`, false},
		{"empty object", `{}`, false},
		{"empty array", `[]`, false},
		{"empty string", `""`, false},
		{"zero", `0`, false},
		{"false", `false`, false},
		{"padded null", "  null\n", false},
		{"error object", `{"message":"boom","type":"server_error"}`, true},
		{"error string", `"upstream exploded"`, true},
		{"error array", `[{"message":"boom"}]`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ErrorFieldIsPresent(json.RawMessage(c.in)); got != c.want {
				t.Errorf("ErrorFieldIsPresent(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestErrorBodiesAreStillDetected 保证收敛没有把真实错误体一起放过去。
func TestErrorBodiesAreStillDetected(t *testing.T) {
	bodies := map[string]string{
		"openai":            `{"error":{"message":"model not found","type":"invalid_request_error","code":"model_not_found"}}`,
		"gemini":            `{"error":{"code":429,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`,
		"anthropic":         `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
		"relay plain error": `{"error":"upstream account is out of quota"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			obj, ok := decodeObject(json.RawMessage(body))
			if !ok {
				t.Fatalf("not a JSON object")
			}
			if !objErrorFieldIsPresent(obj) && getStr(obj, "type") != "error" {
				t.Errorf("error body was not detected")
			}
		})
	}
}
