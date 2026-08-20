# D-AI 重构与优化清单

更新日期：2026-08-20

## 目标与原则

D-AI 当前核心技术选型不需要整体替换。Go、Vue、PostgreSQL、Redis、Huma、
code-first OpenAPI、统一账本和结算 Outbox 继续保留。重构目标是先消除安全与发布风险，
再把当前物理合并后的单体整理为边界清晰、可独立扩缩的模块化单体。

目标运行形态：

```text
Portal / CDN
    |
control-api -------- PostgreSQL（identity / billing / ai_control / operations）
    |
runtime-gateway ---- Redis
    |
workers ------------ settlement / async tasks / audit / cleanup / token refresh
```

- 保留 `dai all`，用于本地开发和轻量单文件部署。
- 增加 `control-api`、`gateway`、`worker` 独立运行角色，用于生产隔离和扩缩容。
- 暂不拆成网络微服务；先用 Go 包边界、数据库权限和运行角色建立隔离。
- 保留单 PostgreSQL 集群和计费强事务，不用消息队列替代资金一致性。
- 每次只实施一个清单项；完成实现、测试、文档和验收后再开始下一项。

## 状态约定

- `[ ]` 未开始
- `[~]` 进行中
- `[x]` 已完成并通过验收
- `[!]` 阻塞，必须在条目下记录原因和解除条件

## P0：安全与生产正确性

### P0-01 重建登录会话与 Refresh Token 生命周期

- [x] 新建服务端 session family 模型，只持久化 Refresh Token 哈希。
- [x] Refresh Token 使用一次即失效，以原子 CAS 完成轮换。
- [x] 检测旧 Refresh Token 重放时撤销整个 session family。
- [x] 刷新时重新校验账号状态、租户状态、凭证版本和会话状态。
- [x] 登出撤销当前 session，而不只是当前 Access Token。
- [x] 修改密码、重置密码、停用账号和删除账号时撤销对应全部 session。
- [x] Access Token 缩短有效期，Refresh Token 保留可配置的绝对过期时间。
- [x] 为登录、刷新、登出、重放、改密和封禁补充数据库集成测试。
- [x] 更新 OpenAPI、Portal API 和认证设计文档。

验收标准：旧 Refresh Token 不能再次换取凭证；账号状态或密码发生变化后，所有旧会话
均不能继续刷新；并发刷新只能有一个请求成功。

### P0-02 删除固定默认密码和弱密码流程

- [x] 删除管理员、租户用户和终端用户的固定 `123456` 默认密码。
- [x] 创建账号改为随机一次性激活令牌或一次性高熵临时凭证。
- [x] 增加 `must_change_password` 或等价凭证状态，禁止临时凭证直接进入业务页面。
- [x] 密码策略统一由后端实现，Portal 只展示同一规则。
- [x] 重置密码不再返回固定明文密码，并强制撤销全部旧会话。
- [x] 清理所有界面中的固定密码提示和确认文案。
- [x] 为创建、激活、过期、重复使用和重置流程增加测试。

验收标准：仓库生产代码不再包含固定默认密码；临时凭证不可重复使用；首次设置正式
密码前不能访问受保护业务。

### P0-03 登录防暴力破解与高权限账号保护

- [x] 按账号标识、来源 IP 和租户组合实施 Redis 限速。
- [x] 失败次数采用渐进退避，不泄露用户名、邮箱或账号状态是否存在。
- [x] 登录限速失败时 fail-closed，并记录结构化安全审计事件。
- [x] 为超级管理员和平台管理员增加 TOTP MFA。
- [x] 为高权限敏感操作增加近期认证或二次验证机制。
- [x] 增加限速、多实例共享状态、失效和并发消费场景测试。

### P0-04 改造浏览器 Token 存储

- [ ] Access Token 只保存在内存，不写入 `localStorage` 或 `sessionStorage`。
- [ ] Refresh Token 改为 `HttpOnly`、`Secure`、`SameSite` Cookie。
- [ ] 明确 Cookie path、domain、过期和清除行为。
- [ ] 对状态变更请求加入 CSRF 防护或严格同源校验。
- [ ] 处理多标签页登录、刷新、登出和 session 失效同步。
- [ ] 修正法律与隐私说明，使其与实际认证机制一致。

### P0-05 建立可信代理和公共 URL 边界

- [ ] 增加明确的 `public_base_url` 配置。
- [ ] 增加可信代理 CIDR 配置，非可信来源不得覆盖 forwarded headers。
- [ ] 注册链接、法律链接和文件 capability URL 使用经过验证的公共 origin。
- [ ] 访问日志和风控使用同一套可信客户端 IP 解析逻辑。
- [ ] 增加伪造 `X-Forwarded-*`、多级代理和直连场景测试。

### P0-06 让审计写入具备持久可靠性

- [ ] 用 PostgreSQL durable inbox/outbox 替代仅存在于内存的审计 channel。
- [ ] 审计入队与核心用量事实按需要处于同一事务。
- [ ] Worker 使用租约或 `FOR UPDATE SKIP LOCKED`，支持多副本和崩溃恢复。
- [ ] 写入失败保留重试状态，不静默丢弃记录。
- [ ] 增加队列深度、最老待处理时长、失败和死信指标与告警。
- [ ] 消除 `audit.Worker.byteUsed` 的并发数据竞争。
- [ ] 增加进程崩溃、数据库故障、超大载荷和队列积压测试。

### P0-07 保护签名密钥和敏感配置

- [ ] JWT 私钥迁移到 KMS、Vault 或 envelope encryption 存储。
- [ ] 数据库中不再保存可直接使用的明文私钥。
- [ ] Provider/OAuth/支付凭证统一带密钥版本，支持在线轮换和重新加密。
- [ ] 删除生产代码中的 OAuth client secret 默认值，改为显式配置。
- [ ] 启动时验证生产环境所需密钥完整性，配置错误直接拒绝启动。
- [ ] 增加密钥轮换、旧密钥宽限、解密失败和恢复测试。

### P0-08 补齐 HTTP 与浏览器安全基线

- [ ] 增加 CSP、HSTS、`X-Content-Type-Options`、frame、referrer 和 permissions 策略。
- [ ] 为静态资源、HTML、私有文件和 API 分别定义缓存策略。
- [ ] `/metrics`、调试端点和管理探针放入独立管理监听地址或受认证保护。
- [ ] 为普通 JSON API 设置统一请求体和 header 大小上限。
- [ ] 保持 AI 流式响应需要的 `WriteTimeout=0`，但增加应用层空闲与总时限。
- [ ] 增加安全头、缓存和超限响应测试。

## P1：领域边界与后端结构

### P1-01 建立模块依赖规则

- [ ] 规划 `identity`、`billing`、`catalog`、`runtime`、`operations` 模块责任。
- [ ] 每个模块包含 `domain`、`application`、`ports`、`adapters` 边界。
- [ ] Transport 只依赖 application command/query，不直接访问数据库。
- [ ] 禁止模块绕过公开端口写入其他模块表。
- [ ] 使用依赖检查脚本或 linter 在 CI 中阻止反向依赖。
- [ ] 记录允许的跨模块事务和所有权例外。

### P1-02 拆分 composition root 和巨型依赖容器

- [ ] 将 `cmd/server/main.go` 拆为配置、基础设施、模块装配和运行生命周期。
- [ ] 删除包含几十个字段的 `transport.Deps` / `AIDeps` service locator。
- [ ] 每个模块提供最小的 Register/Module 接口和显式依赖。
- [ ] 后台组件统一实现 Start/Stop/Health 生命周期。
- [ ] 启动失败时按逆序释放已经创建的资源。
- [ ] 为各运行角色增加装配测试。

### P1-03 收敛 HTTP 层业务逻辑

- [ ] 把权限、事务、状态机和数据库查询移出 `internal/transport`。
- [ ] Handler 只负责认证上下文、DTO 转换、调用 application 和错误映射。
- [ ] 用户、租户、支付、充值、公告和清理逐域迁移。
- [ ] AI 管理 API 按价格、上游、路由、用量、订阅和风控拆分。
- [ ] 将 Transport 层关键路径覆盖率提升到可执行门槛。

### P1-04 拆分超大 Go 文件和清理兼容遗留

- [ ] 拆分 `internal/ai/serving/execute.go` 的候选选择、传输、流式响应和图片响应职责。
- [ ] 拆分大型 PostgreSQL Repository，按聚合或 use case 组织。
- [ ] 清理 staticcheck 报告的死代码、无效赋值和潜在 nil dereference。
- [ ] 删除已不再注册的旧 Console handler 和 legacy bridge helper。
- [ ] 为关键拆分建立行为等价测试，避免顺手改变计费和路由语义。

### P1-05 强化授权模型

- [ ] 将散落的 `userType` 判断集中为后端 capability/policy 授权。
- [ ] Portal 菜单 capability 只用于展示，后端始终执行最终授权。
- [ ] 建立 actor、tenant scope、resource ownership 的统一类型。
- [ ] 为 312 个 OpenAPI operation 生成或维护授权矩阵。
- [ ] 增加跨租户、越权、对象枚举和角色降级测试。

## P1：数据库与资金数据治理

### P1-06 统一迁移链和 schema 真相源

- [ ] 采用 forward-only SQL migration 工具，迁移仍由发布步骤显式执行。
- [ ] 不允许应用服务启动时隐式执行生产迁移。
- [ ] 空库基线由完整迁移链生成或验证，避免同时手工维护两套结构。
- [ ] 每个迁移在空库和前一 schema 版本副本上验证。
- [ ] 为缺少专项测试的 0002、0003、0009 补迁移测试。
- [ ] 校准 `README.md`、`docs/DATABASE.md` 和 `docs/PROJECT_STATUS.md` 的 schema 版本。
- [ ] 发布流程加入备份、迁移校验、兼容窗口和失败恢复步骤。

### P1-07 建立数据库领域所有权

- [ ] 从全 `public` schema 迁移到领域 schema，或用独立数据库角色实现等价隔离。
- [ ] 账本表只允许 billing 模块角色写入。
- [ ] 网关只写运行时事实、用量和可靠投递，不直接修改控制面配置。
- [ ] 跨域读取通过视图、只读端口或显式 query service。
- [ ] CI 检查应用角色的最小权限和越权失败行为。

### P1-08 持续验证资金不变量

- [ ] 将余额、批次、充值、退款、订阅和 AI 结算不变量形成统一测试套件。
- [ ] 增加随机并发和属性测试，覆盖充值、扣费、过期、撤销与退款交错。
- [ ] 增加定期线上对账任务和差异告警。
- [ ] 为 Outbox 积压和 parked row 定义处理手册。
- [ ] 所有资金修复必须保留不可变审计证据和幂等键。

## P2：运行角色与部署拓扑

### P2-01 增加多运行角色

- [ ] 增加 `dai control-api` 运行角色。
- [ ] 增加 `dai gateway` 运行角色。
- [ ] 增加 `dai worker` 运行角色。
- [ ] 保留 `dai all` 兼容单进程部署。
- [ ] 每个角色只初始化所需数据库权限、Redis 能力和后台组件。
- [ ] 每个角色提供独立 readiness 和内部 health 状态。

### P2-02 验证所有后台任务的多副本语义

- [ ] 结算 Outbox 保持多消费者唯一处理。
- [ ] 异步任务和 Webhook 保持租约、心跳、回收与 fencing。
- [ ] 调度任务统一使用 advisory lock、租约或可证明的幂等执行。
- [ ] JWT key retire、支付补偿、数据清理、文件清理和价格同步逐项验证。
- [ ] 禁止依赖进程内内存作为跨副本真相源。

### P2-03 独立交付 Portal

- [ ] Portal 默认作为静态制品交付，可由 CDN 或反向代理托管。
- [ ] 保留 embed 构建作为轻量发行选项。
- [ ] HTML 与带 hash 的静态资源使用不同缓存策略。
- [ ] 构建产物不作为普通源码提交，发布流程生成并校验 checksum。
- [ ] 增加独立 Portal 和 embed Portal 两种 smoke test。

## P2：Portal 架构与设计系统

### P2-04 让 OpenAPI 成为唯一传输契约源

- [ ] 生成带类型的 API client，而不只生成 `paths/components` 类型。
- [ ] 删除与 OpenAPI 重复的手写请求和响应 DTO。
- [ ] 领域 view model 与传输 DTO 显式转换，不混用命名和空值语义。
- [ ] 删除 facade 中的 `any` 返回值和未经校验的 JSON 断言。
- [ ] CI 检查生成物 freshness、破坏性契约变化和未使用 operation。

### P2-05 按 feature 垂直切分 Portal

- [ ] `views` 只保留路由入口，状态、API 和业务组件归入对应 feature。
- [ ] 拆分图片工作台、订阅、上游账号、凭据池和价格管理等超大页面。
- [ ] admin、tenant、customer 共享同一业务 workspace，通过权限和配置形成差异。
- [ ] 为复杂页面状态使用明确的 composable/store 和领域类型。
- [ ] 为关键工作流增加组件集成测试。

### P2-06 强制执行 DsUI 约束

- [ ] 把业务代码中的硬编码颜色迁移为 `var(--ds-*)` token。
- [ ] 禁止业务页面新增 hex、rgb、rgba 和自建阴影、圆角值。
- [ ] 将正则存在性检查升级为源码级 lint 规则。
- [ ] 统一 Element Plus 与 DsUI 的交互、尺寸、状态和可访问性。
- [ ] 增加 admin、tenant、customer 三主题视觉回归测试。

### P2-07 增加浏览器端到端验收

- [ ] 使用 Playwright 覆盖四种 `userType` 登录与菜单授权。
- [ ] 覆盖邀请注册、用户管理、API Key、AI 对话、图片任务和用量查询。
- [ ] 覆盖充值、退款、订阅购买和余额变化关键路径。
- [ ] 覆盖 Token 过期刷新、跨标签登出和权限变更。
- [ ] 在桌面和移动视口执行截图、无障碍和控制台错误检查。

## P3：可观测性、质量门禁与运维

### P3-01 完成统一可观测性

- [ ] 接入已实现但未使用的 HTTP Prometheus middleware。
- [ ] 使用路由模板而不是原始路径作为指标标签。
- [ ] 提取入站 OpenTelemetry trace context，并将 trace ID 写入结构化日志。
- [ ] Sampling 改为可配置 parent-based 策略，不在生产固定 `AlwaysSample`。
- [ ] 监控请求、上游、计费、Outbox、异步任务、审计和数据库池。
- [ ] 为 SLO 定义可操作告警和 runbook。

### P3-02 建立静态分析和格式门禁

- [ ] 修复当前 staticcheck 报告。
- [ ] 升级并配置兼容当前 Go 版本的 golangci-lint。
- [ ] 增加前端 ESLint 或等价规则，禁止未受控 `any`、硬编码颜色和非法依赖。
- [ ] 将格式化、lint、vet、typecheck 和测试加入 CI。
- [ ] 检查不允许通过大范围 ignore 让门禁虚假变绿。

### P3-03 完善供应链和发布验证

- [ ] 固定 CI 使用的 Go、Bun 和关键工具版本。
- [ ] 增加 Go 与前端依赖漏洞扫描，并修复 registry 配置。
- [ ] 生成 SBOM、许可证清单、制品 checksum 和 provenance。
- [ ] 扫描容器镜像并使用非 root、只读文件系统和最小 capability。
- [ ] 增加生产构建后的 `/health`、`/ready`、Portal、API 和流式响应 smoke。
- [ ] 建立可重复的回滚和数据库兼容验证。

### P3-04 修正文档与实际状态漂移

- [ ] 更新项目状态中的测试数量、CI 能力和已完成事项。
- [ ] 统一 schema 版本、迁移规则和生产初始化说明。
- [ ] 为各运行角色补配置、部署、扩缩容和故障恢复文档。
- [ ] 每个清单项完成时同步更新相关文档，不在最后集中补写。

## 当前验证基线

- [x] `go test ./...`
- [x] `go vet ./...`
- [x] 数据库迁移和计费集成测试实际连接 PostgreSQL 运行
- [x] `bun run typecheck`
- [x] `bun run test`：63 个测试文件、211 个测试通过
- [x] `bun run ensure:api`
- [ ] `staticcheck ./...`：当前存在潜在 nil dereference、无效赋值和遗留死代码
- [ ] `golangci-lint`：本机版本与当前 Go 工具链不兼容
- [ ] 前端依赖审计：当前 registry 的 audit API 返回 404
- [ ] 浏览器级端到端验收尚未建立

## 执行记录

后续每次完成一个条目时，在此追加：编号、提交或变更摘要、验证命令、遗留风险和下一候选项。

### P0-01（2026-08-20）

- 状态：完成并通过验收。
- 变更：schema 11 增加服务端 session family、Refresh Token 哈希和凭证版本；刷新采用事务锁与单次轮换，重放撤销整个 family；Access Token 默认 15 分钟并实时校验会话；登出、改密、重置、账号/租户状态和删除均纳入撤销链路。
- 契约：更新 OpenAPI、Portal 生成类型和 `refreshExpiresIn`；新增 `docs/AUTHENTICATION.md`。
- 验证：`go test ./...`、`go vet ./...`、`bun run test`、`bun run typecheck`、`bun run ensure:api`。
- 遗留风险：浏览器仍在 `localStorage` 保存 Token，按 `P0-04` 迁移 HttpOnly Cookie；Access 会话校验当前直接查询 PostgreSQL，后续可在保持撤销正确性的前提下增加短 TTL 缓存。
- 下一候选项：`P0-02 删除固定默认密码和弱密码流程`。

### P0-02（2026-08-20）

- 状态：完成并通过验收。
- 变更：schema 12 增加 `credential_state` 与只保存 SHA-256 哈希的一次性激活凭证；新账号使用随机不可登录占位凭证并保持待激活，激活或重置令牌均为 256 位随机值、单账号单份有效且默认 24 小时过期；重置同步提升凭证版本并撤销全部旧会话。
- 密码策略：后端统一要求至少 12 个字符、bcrypt 72 字节上限、大写/小写/数字/符号至少三类且不能包含用户名；邀请注册、账号激活和主动改密复用同一校验，Portal 从公开策略接口展示相同规则。
- Portal：新增公开激活页，令牌通过 URL fragment 传递并在读取后立即清除；管理员、租户用户和终端用户创建/重置后只展示一次激活链接；列表明确标识待激活账号，固定密码提示和旧编辑接口改密入口已清除。
- 验证：`go test ./...`、`go vet ./...`、`bun run test`、`bun run typecheck`、`bun run build:frontend`、`bun run ensure:api`，另含激活并发、过期、复用、重置替换、会话撤销和迁移集成测试。
- 遗留风险：激活链接交付目前由操作员复制，后续通知模块可接管邮件或短信发送。
- 下一候选项：`P0-03 登录防暴力破解与高权限账号保护`。

### P0-03（2026-08-20）

- 状态：完成并通过验收。
- 限速：登录前使用 Redis Lua 原子检查账号、来源 IP、账号-IP 与租户-IP 维度；失败计数 15 分钟窗口内达到阈值后按 5 秒起步指数退避，所有 key 使用 SHA-256 派生，Redis 故障直接拒绝登录并记录 `login_rate_limiter_unavailable` 审计事件。
- 认证语义：密码错误、账号不存在、停用、待激活和租户不可用统一返回“用户名/邮箱或密码错误”；失败计数、限速和 MFA 挑战均不把账号状态泄露给客户端。
- MFA：schema 13 增加加密 TOTP 密钥和启用状态；管理员个人中心可注册/确认 MFA，启用后密码登录返回 5 分钟、最多 5 次尝试的一次性挑战，验证成功后才创建 session。
- 敏感操作：管理员账号/租户/用户状态变更、密码重置、删除、退款等操作要求 Redis 中存在 10 分钟近期密码或 MFA 认证标记；新增 `POST /api/auth/recent-auth` 用于恢复该标记。
- 验证：新增限速渐进退避、多实例共享 Redis、近期认证过期、Redis 故障 fail-closed、TOTP 时间窗口、并发挑战消费和 schema 13 迁移测试；`go test ./...`、`go vet ./...`、`bun run test`（63 个文件/211 个测试）、`bun run typecheck`、`bun run build:frontend`、`bun run ensure:api` 均通过。
- 遗留风险：可信代理客户端 IP 解析仍按 `P0-05` 独立收紧；恢复码/硬件 WebAuthn 作为后续增强，生产管理员应在启用 TOTP 后保存组织级离线恢复流程。
- 下一候选项：`P0-04 改造浏览器 Token 存储`。
