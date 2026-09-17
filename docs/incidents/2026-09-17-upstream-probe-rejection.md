# 2026-09-17 Responses 纯文本响应与上游防探测拦截

## 本次确认的原因

本地现有账号配置指向 `https://sub2.congmingai.com/v1/responses`，模型 `gpt-5.6-sol`。短请求实际收到：

```text
HTTP/2 200
content-type: text/plain; charset=utf-8
x-sub2api-probe-blocked: true
x-sub2api-estimated-input-tokens: 34

Hi! What can I help you with?
```

这是上游明确标记的探测拦截，不能当作模型成功，也不能仅解释为端点不支持 Responses。独立 curl 同样复现，关联上游 request ID 为 `7f8ec776-dd11-4d26-86d1-c073ed4f4c78`。此前 9 月 16 日报告只检查了状态、路径、Content-Type 与正文，没有识别这个响应头；此前的诊断提示修复并未解决这一原因。

## 真实对照

以下请求使用同一本地保存的账号密钥与端点，未修改账号配置、数据库或 Redis。

| 对照 | 结果 |
| --- | --- |
| 原诊断、数组 input、Codex 请求头、普通浏览器 UA | HTTP 200，固定 29 字节纯文本 |
| HTTP/2、独立 curl、小型编码任务 | 同样返回拦截；curl 明确记录 probe-blocked 响应头 |
| 无 Authorization | HTTP 401；不同于携带账号密钥的探测拦截 |
| GET /v1/models | HTTP 200，合法模型列表，包含 gpt-5.6-sol |
| 带 URL/header builder 源码上下文的诊断 | OK=true，上游用量 total_tokens=5988 |
| 同样代码上下文，经真实 bridgefmt → ExecuteStep → transport → 上游 → 业务流式转发 | error=nil、status=success、terminal=completed；下游 29093 字节，包含 response.completed，上游用量证据存在 |

长请求使用正常代码审查内容，没有自动填充短请求或修改用户提示词。上述对照说明此账号能处理 Responses，短请求被上游策略拒绝。没有测定拦截阈值，也不能推断仅由 token 数决定。临时验证程序保存在本机私有目录 `/tmp/dai-incident-20260917/live-validation.go`，不进入自动测试，不含明文密钥。

## 与参考项目和版本的关系

读取了本地 sub2api 的 OpenAI 请求构建、请求头白名单、账号覆盖、Codex 身份与 HTTP/2 传输实现，以及 Aether 的标准请求头透传与 Responses 构建实现。两者的请求头转发比 D-AI 更丰富，但补头和切 HTTP/2 没有解决本例；不据此引入无证据的大范围协议改造。本地参考源码没有匹配到 `x-sub2api-probe-blocked` 的实现，无法假定运行中上游与这份参考代码完全一致。

v0.3.17 到当前 HEAD 的通用 URL/header builder 和 bridgefmt serving adapter 没有改动。v18 的调度/可用性改动与此前 Redis 故障是另一条问题线；本次证据不支持通过整体回滚解决上游探测策略。

## 本次代码修复

- 诊断识别显式拦截头，显示防探测原因和经过数值验证的估算 token 数；继续返回失败，不把纯文本伪装成模型成功。
- 此类诊断不再把端点判为 unhealthy，而是 unknown，并保存具体拦截说明；诊断无法验证模型结果。
- 业务请求在读取/转发正文前识别拦截，记录 `upstream_probe_blocked` 和 `policy_rejected`，关闭响应并按原有预算切换其他候选。
- 拦截不计入模型故障、认证失败或冷却，不产生模型输出或 token 用量。所有实际候选均被拦截且候选已耗尽时，客户端收到明确的 HTTP 502 `upstream_probe_blocked`。
- 没有信号的普通纯文本、401、503 等仍走原有判断；不根据问候语猜测拦截原因。

## 验证及边界

新增回归覆盖同步/流式拒绝、连续 6 次拒绝不冷却、后续健康端点成功、拒绝不转发/不计用量、错误原因保留、诊断端点健康状态、无信号与非 2xx 边界、恶意/非法 token 头。

`CGO_ENABLED=0 go test` 覆盖 serving、transport、upstreamcompat、bridgefmt、gateway、cmd/server，全部通过；依赖方向检查和 `git diff --check` 通过。本机默认 CGO 链接受 macOS SDK/链接器不兼容影响，纯 Go 模式完成验证。

真实业务验证覆盖请求准备、上游调用和业务回包，不是部署后的客户端认证、分组选择、计费落库全链路验收。未部署生产，未修改生产策略，也未整体回滚。

用户附件主要是管理端 GET 查询日志，未包含 Codex 失败请求的上游调用。尚不能证明用户当前 Codex 无响应与本次账号测试是同一原因。下一步应按实际失败的 request_id、账号名称、上游 attempt 错误记录追踪；若是 `upstream_probe_blocked`，处理上游策略，若为其他错误继续按该请求定位。

## 按参考项目统一默认请求（后续用户确认的最终方案）

对照 sub2api `account_test_service.go:createOpenAITestPayload`：它的输入虽然是 hi，但还有嵌入的完整 Codex instructions、结构化 input 和 stream；没有固定 256 token 的小上限。Aether `model_test.rs` 按协议组装测试，支持替换请求内容。用户明确不需要自定义完整 JSON，因此最终移除了该入口及 API 字段，没有把这项功能留在产品里。

最终实现：

- `upstreamcompat.BuildChatRequest` 统一网页版对话与账号测试，按 OpenAI Responses / Chat、Anthropic、Gemini 生成各自合法的系统指令和消息字段。默认使用 D-AI 自有的通用助手 instructions；已有 system 内容保留。不会把代码审查测试任务注入用户对话，也不会对外部 API 客户端注入默认 instructions。
- Responses 补齐 instructions、结构化 input、store=false、stream；历史 assistant 内容使用 output_text，用户内容使用 input_text。网页自身输出预算保留，诊断不施加 256 token 小上限。
- Gemini 使用 systemInstruction；网页明确传递模型和流式元数据，避免依赖 Gemini JSON 中不存在的 stream 字段；出站移除只属于 URL 的 model/stream 元数据。
- `BuildImageRequest` 统一网页版图片生成与账号生图测试的请求结构，保留用户 prompt、图片数量、尺寸、流式策略和响应格式，补上网页此前遗漏的 output_compression、moderation、user 参数。不会向图片协议塞入聊天 instructions。图片编辑继续共用既有 imageedit 编解码器。
- 保留成功/错误协议验证、探测拦截识别，以及响应 Content-Type、上游 request ID 展示。达到输出上限的截断不能当作成功；完整返回的工具调用可作为模型结果展示，但测试不会执行工具。

同一故障账号再次真实验证最终公共默认请求：

| 请求 | 结果 |
| --- | --- |
| 默认代码审查测试 + 公共 instructions | HTTP 200、完整 SSE、OK=true、2038 tokens；上游 ID `0c48ffae-b028-48c8-b1fd-ba14d99ee458` |
| 短问候 + 同一公共 instructions | HTTP 200、完整 SSE、OK=true、854 tokens；上游 ID `cad6d91b-dc9e-4d0b-9708-3b6c092af50c` |

第二项验证的是网页共用的请求结构，并非生产网页认证、分组、计费的全链路验收；本次没有发送真实付费生图。新增回归覆盖多轮消息角色、系统指令映射、网页与测试使用公共构建器、图片参数保留、Gemini 图片内容保留、自定义 JSON 入口移除和诊断信息展示。
