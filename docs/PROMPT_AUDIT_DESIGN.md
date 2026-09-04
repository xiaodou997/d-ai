# Prompt Audit / Qwen3Guard 设计

## 目标

D-AI 在现有关键词与 OpenAI Moderations 内容审核之外，增加独立的提示词安全审计引擎。新引擎通过 OpenAI 兼容 Chat Completions 调用 Qwen3Guard，补充 jailbreak、提示词注入、PII、版权及其他输入风险识别。

本能力默认关闭，不改变现有内容审核的分类、阈值、风险事件或处置语义。

## 安全与隐私契约

1. Prompt Audit 自有的 PostgreSQL 表、Redis、结构化日志和管理 API 均不得保存或返回未脱敏提示词原文。平台既有、独立授权的通用请求审计遵循其自身保留策略。
2. 审计记录只保存 SHA-256、字符数、消息数、严格脱敏的短预览、分类、判定、节点和请求关联信息。
3. `observe` 模式使用有界进程内队列。任务只在内存中持有扫描正文；进程重启允许丢失尚未完成的观察任务。
4. `blocking` 模式仅在请求生命周期内持有正文。
5. Guard API Key 使用 `security.secret_master_key` 加密，公共 DTO 只返回 `has_api_key`。
6. Guard 出站请求默认只允许 HTTPS 公网目标；禁止凭据 URL、重定向、loopback、私网、链路本地、组播、未指定地址和保留地址。
7. DNS 解析与实际拨号绑定，防止 DNS rebinding。将来自多个 DNS 结果中的任一非公网地址视为不安全目标。
8. 如需自托管内网 Guard，只能由服务启动配置提供精确主机/CIDR allowlist；Portal 配置不得关闭 SSRF 防护。

## 运行模式

- `off`：不提取、不调用、不记录。
- `observe`：将请求快照提交到独立有界内存队列；投递失败不影响主请求。
- `blocking`：在配额、余额、路由、计费和上游调用前同步判断。`Block` 返回 403；Guard 不可用或返回非法格式时 fail closed，返回 503。

Pipeline 顺序：

```text
AuthN
  -> PromptAudit
  -> ContentModeration
  -> Quota
  -> Subscription
  -> Balance
  -> Route
  -> Billing
  -> RateLimit
  -> Execute
```

Prompt Audit 与 Content Moderation 是并列能力。提示词命中不得触发现有内容审核的累计违规、风险事件或其他副作用。

## 输入提取

从统一 `serving.Request` 的原始 `Envelope.ClientBody` 提取文本：

- OpenAI Chat Completions：`messages`。
- OpenAI Responses：`instructions` 与 `input`。
- Anthropic Messages：`system` 与 `messages`。
- Gemini：`systemInstruction` 与 `contents`。
- 图片能力：仅提取文本 prompt，忽略图片、base64 和远程媒体内容。

扫描客户端可控的 user、system、developer、assistant、tool 和 model 文本。最新用户输入优先；按 Unicode rune 分片。同步模式可配置为“最新用户轮次 + 最近 assistant/model 输出”，完整会话扫描仍为默认值。

## Qwen3Guard 协议

节点使用 `{base_url}/v1/chat/completions`：

```json
{
  "model": "sileader/qwen3guard:0.6b",
  "messages": [{"role": "user", "content": "..."}],
  "temperature": 0,
  "max_tokens": 64,
  "seed": 42
}
```

严格解析 `Safety: Safe|Controversial|Unsafe` 和 `Categories:`。格式不完整、重复字段、空内容或超大响应均视为非法响应。

风险分类使用 Qwen3Guard 九类规范值。分片结果按最高严重度聚合，任一分片达到 Block 即可提前结束。

## 配置与节点

配置存放在 `ai_settings[prompt_audit_config]`，具有单调递增的 `config_revision`。支持：

- 启用状态和运行模式；
- 是否只扫描最新轮次；
- 是否记录 Pass；
- Worker 数量和队列容量；
- 启用分类；
- Tenant/Group 范围；
- 有序节点列表；
- 节点模型、超时、输入上限和加密凭据。

配置读取采用短 TTL 缓存。blocking 配置无法加载或没有可用节点时必须 fail closed；默认关闭或 observe 配置加载失败不得使整个 AI Gateway 不可用。

## 审计事件

`ai_prompt_audit_events` 保存：

- request、tenant、user、API key、模型和协议关联；
- prompt hash、脱敏预览、字符数和消息数；
- decision、risk level、action、categories；
- scanner backend/version、Guard endpoint、配置版本；
- chunk total、latency、稳定错误码和创建时间。

表中不得出现 `full_prompt`、`raw_prompt` 或可恢复正文列。Pass 事件是否落库由配置决定，风险和错误事件始终记录。

## 管理面

平台管理员可以：

- 读取和更新配置；
- 探测节点；
- 查看运行状态、队列与 Guard 指标；
- 按时间、租户、用户、API Key、判定和分类筛选事件；
- 安全删除事件。

Portal 在现有风控中心增加“提示词审计”页签，使用 `PortalPagePanel` 和 DsUI token/组件，仅 `userType=1/2` 可见。

## 验收要点

- Guard 检查发生在所有配额、余额、路由、计费及上游副作用之前。
- observe 模式在队列满、Guard 超时或事件写入失败时不影响主请求。
- blocking 模式对 Block、不可用和非法响应分别返回稳定错误码。
- 单元测试覆盖协议提取、严格解析、分片聚合、SSRF/DNS rebinding、节点 failover、隐私脱敏和 pipeline 顺序。
- 仓库扫描证明 Prompt Audit 表、日志和 DTO 不包含未脱敏提示词正文。
