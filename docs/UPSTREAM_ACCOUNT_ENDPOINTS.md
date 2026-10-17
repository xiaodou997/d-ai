# 上游账号与请求端点

## 最终模型

- 一个 `ai_upstream_accounts` 表示一个供应商账号和一把 API Key。
- 一个账号通过 `ai_upstream_account_endpoints` 声明唯一端点，包含一种精确 API 格式和一个 Base URL。
- 需要另一个地址或 API 格式时新增账号，可以复用供应商 API key；不同账号独立配置模型、并发和路由。
- `ai_upstream_models` 是账号级模型目录，不按 API 格式复制模型。
- OAuth 授权账号继续由凭证池管理；凭证池的 API 格式由 `fixed_provider_type` 决定。

## 端点格式

当前支持：

- `openai_chat`
- `openai_responses`
- `openai_embeddings`
- `openai_images`
- `anthropic_messages`
- `gemini_generate`
- `gemini_embeddings`

端点保存独立的 Base URL、可选路径覆盖、认证注入方式、附加请求头、启停状态和健康状态。`UNIQUE (account_id)` 是数据库稳定不变量。

## 运行时语义

路由先找到账号级模型绑定，再检查该账号唯一端点是否启用、是否兼容客户端格式和模型能力。分组允许协议转换时，可以通过 bridge runtime 转换客户端请求。多个上游账号可加入同一分组，由分组策略进行选择和故障切换。

- 并发限制按账号 ID 计算。复用同一供应商 key 的多个账号分别计数。
- 健康、熔断、粘性和尝试明细按具体端点 ID 计算。
- 用量日志分别记录 `upstream_account_id` 和 `endpoint_id`。
- 直连端点的 401/403 会失效账号级 API Key，不会把端点 ID 误当账号 ID。

## 管理约束

- 创建账号必须且只能配置一个端点；更新或导入时同样校验。
- 启用账号时，其唯一端点必须启用。
- 不能单独删除账号的唯一端点；通过编辑连接修改配置，或删除整个账号。
- 启用账号不能停用其端点；需要先停用账号。
- 账号状态切换、端点停用和删除统一锁定账号行，在同一事务内校验，不能通过并发请求绕过。
- 启用账号的全部 active 模型能力必须与其端点兼容；协议修改若使已有启用模型不兼容，整次保存回滚。固定 OAuth Provider 使用同一能力校验规则。
- 模型管理界面不提供 API 格式字段，避免形成模型 × 格式的重复配置。
- 管理接口中的敏感附加请求头以 `***REDACTED***` 返回；编辑时该占位值表示保留原密文，删除键才表示删除请求头。
- 修改或停用端点会同步重置/移除运行时熔断状态；连通性测试成功也会立即关闭对应端点的熔断器。

## v32 迁移

迁移优先从旧模型绑定的 `api_format` 生成精确端点。没有模型绑定的账号按旧 `default_protocol` 生成一个兼容默认端点：OpenAI → Responses、Anthropic → Messages、Gemini → Generate Content。

历史用量原先把账号 ID 写在 `endpoint_id`。v32 将其迁入 `upstream_account_id`，并按历史 `provider_format` 尽可能回填新的具体端点 ID；无法确定端点的历史记录保留账号归因，端点 ID 留空。

## v47 迁移

`0047_20261017_single_upstream_account_endpoint.sql` 将唯一约束从 `(account_id, api_format)` 收紧为 `account_id`，保留账号、端点 ID、凭据、模型绑定和历史归因。HTTP 的 `endpoints` 字段继续使用数组，创建和更新时限制长度为 1；更新省略此字段保留配置。旧单独添加端点接口也会拒绝第二个端点。

迁移在事务中锁定账号和端点表，先验证每个已有账号恰好有一个端点。若有零端点或多端点账号，迁移整体失败，不自动截取、删除或拆分；先按实际协议及模型绑定整理成独立账号，再重新升级。导入预览同样逐项拒绝多端点账号。

生产升级前创建数据库备份，迁移完成后启动 schema 47 应用。若回退至 v0.3.26，先停止应用并执行 `internal/db/rollback/0047_rollback.sql` 恢复 schema 46，再启动旧镜像；回退脚本也不修改连接或凭据数据。
