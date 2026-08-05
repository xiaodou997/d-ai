package serving

import (
	"os"
	"path/filepath"
	"testing"
)

// upstreamSuccessFixtureDir 指向 formats 包的 fixture 目录 —— 网关层的错误判定和
// 转换层的错误判定必须基于同一批真实响应体，两份数据会各自漂移。
const upstreamSuccessFixtureDir = "../formats/testdata/upstream_success"

// TestPayloadIsErrorOnRealSuccessBodies 是 502 all_routes_failed 故障的回归：
// gpt-5.6-terra 经 /v1/responses 同步调用时，5 条上游全部返回 200 且内容正常，
// 但 payloadIsError 只看顶层是否存在 `error` 键，而 OpenAI Responses 的成功体恒带
// `"error": null` —— 于是每条路由都被判为失败、逐条 failover、最后对客户端报 502，
// 5 次上游调用的 token 全部空烧。
func TestPayloadIsErrorOnRealSuccessBodies(t *testing.T) {
	entries, err := os.ReadDir(upstreamSuccessFixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	seen := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(upstreamSuccessFixtureDir, e.Name()))
		if err != nil {
			t.Fatalf("read fixture %s: %v", e.Name(), err)
		}
		seen++
		t.Run(e.Name(), func(t *testing.T) {
			if payloadIsError(body) {
				t.Errorf("real upstream success body would trigger failover")
			}
		})
	}
	if seen == 0 {
		t.Fatalf("no fixtures found in %s", upstreamSuccessFixtureDir)
	}
}
