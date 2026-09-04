# 上游账号与请求端点

## 最终模型

- 一个 `ai_upstream_accounts` 表示一个供应商账号和一把 API Key。
- 一个账号通过 `ai_upstream_account_endpoints` 声明一种或多种精确 API 格式。
- 同一账号的 `api_format` 唯一，不能为同一种格式配置多个 Base URL。
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

端点保存独立的 Base URL、可选路径覆盖、认证注入方式、附加请求头、启停状态和健康状态。`UNIQUE (account_id, api_format)` 是数据库稳定不变量。

## 运行时语义

路由先找到账号级模型绑定，再从账号的启用端点中选择与客户端格式和模型能力兼容的端点。无需转换时优先精确格式；分组允许协议转换时，才由 bridge runtime 选择可转换格式。

- 并发限制按账号 ID 计算，多个端点共享账号并发额度。
- 健康、熔断、粘性和尝试明细按具体端点 ID 计算。
- 用量日志分别记录 `upstream_account_id` 和 `endpoint_id`。
- 直连端点的 401/403 会失效账号级 API Key，不会把端点 ID 误当账号 ID。

## 管理约束

- 创建账号时至少需要一个端点。
- 启用账号时至少需要一个启用端点。
- 不能删除账号的最后一个端点。
- 启用账号不能停用其最后一个启用端点；需要先停用账号。
- 账号状态切换、端点停用和删除统一锁定账号行，在同一事务内校验，不能通过并发请求绕过。
- 启用账号的每种 active 模型能力都必须至少有一个兼容的 active 请求端点；固定 OAuth Provider 使用同一规则。
- 模型管理界面不提供 API 格式字段，避免形成模型 × 格式的重复配置。
- 管理接口中的敏感附加请求头以 `***REDACTED***` 返回；编辑时该占位值表示保留原密文，删除键才表示删除请求头。
- 修改或停用端点会同步重置/移除运行时熔断状态；连通性测试成功也会立即关闭对应端点的熔断器。

## v32 迁移

迁移优先从旧模型绑定的 `api_format` 生成精确端点。没有模型绑定的账号按旧 `default_protocol` 生成一个兼容默认端点：OpenAI → Responses、Anthropic → Messages、Gemini → Generate Content。

历史用量原先把账号 ID 写在 `endpoint_id`。v32 将其迁入 `upstream_account_id`，并按历史 `provider_format` 尽可能回填新的具体端点 ID；无法确定端点的历史记录保留账号归因，端点 ID 留空。
