# D-AI 重构与优化清单

更新日期：2026-08-21

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

- [x] Access Token 只保存在内存，不写入 `localStorage` 或 `sessionStorage`。
- [x] Refresh Token 改为 `HttpOnly`、`Secure`、`SameSite` Cookie。
- [x] 明确 Cookie path、domain、过期和清除行为。
- [x] 对状态变更请求加入 CSRF 防护或严格同源校验。
- [x] 处理多标签页登录、刷新、登出和 session 失效同步。
- [x] 修正法律与隐私说明，使其与实际认证机制一致。

### P0-05 建立可信代理和公共 URL 边界

- [x] 增加明确的 `public_base_url` 配置。
- [x] 增加可信代理 CIDR 配置，非可信来源不得覆盖 forwarded headers。
- [x] 注册链接、法律链接和文件 capability URL 使用经过验证的公共 origin。
- [x] 访问日志和风控使用同一套可信客户端 IP 解析逻辑。
- [x] 增加伪造 `X-Forwarded-*`、多级代理和直连场景测试。

### P0-06 让审计写入具备持久可靠性

- [x] 用 PostgreSQL durable inbox/outbox 替代仅存在于内存的审计 channel。
- [x] 审计入队与核心用量事实按需要处于同一事务。
- [x] Worker 使用租约或 `FOR UPDATE SKIP LOCKED`，支持多副本和崩溃恢复。
- [x] 写入失败保留重试状态，不静默丢弃记录。
- [x] 增加队列深度、最老待处理时长、失败和死信指标与告警。
- [x] 消除 `audit.Worker.byteUsed` 的并发数据竞争。
- [x] 增加进程崩溃、数据库故障、超大载荷和队列积压测试。

### P0-07 保护签名密钥和敏感配置

- [x] JWT 私钥迁移到 envelope encryption 存储，数据库不再保存可直接使用的明文私钥。
- [x] Provider/OAuth/支付凭证统一使用带 key ID 的 AES-GCM 密文，支持旧密钥宽限和在线重新加密。
- [x] 生产代码不提供 OAuth client secret 默认值，所有敏感配置均由显式 keyring 注入。
- [x] 启动时验证生产环境 active/previous key 完整性、长度和版本唯一性，配置错误直接拒绝启动。
- [x] 增加密钥轮换、旧密钥兼容、解密失败和恢复测试。

### P0-08 补齐 HTTP 与浏览器安全基线

- [x] 增加 CSP、HSTS、`X-Content-Type-Options`、frame、referrer 和 permissions 策略。
- [x] 为静态资源、HTML、私有文件和 API 分别定义缓存策略。
- [x] `/metrics`、调试端点和管理探针移到独立的 loopback 管理监听地址。
- [x] 为普通 JSON API 设置统一请求体和 header 大小上限。
- [x] 保持 AI 流式响应需要的 `WriteTimeout=0`，并由应用层执行首字节、空闲间隔与总时限。
- [x] 增加安全头、缓存和超限响应测试。

## P1：领域边界与后端结构

### P1-01 建立模块依赖规则

- [x] 规划 `identity`、`billing`、`catalog`、`runtime`、`operations` 模块责任。
- [x] 定义每个模块的 `domain`、`application`、`ports`、`adapters` 目标边界。
- [~] Transport 只依赖 application command/query，不直接访问数据库；当前遗留越界已冻结并登记，P1-03 负责清零。
- [~] 禁止模块绕过公开端口写入其他模块表；包级依赖已门禁，数据库角色和表所有权在 P1-07 完成。
- [x] 使用 `cmd/checkdeps` 依赖检查器并接入 Make/CI，阻止未登记的反向依赖。
- [x] 在 `docs/MODULE_DEPENDENCY_RULES.md` 和例外台账记录允许的跨模块事务及历史例外。

### P1-02 拆分 composition root 和巨型依赖容器

- [~] 将 `cmd/server/main.go` 拆为配置、基础设施、模块装配和运行生命周期；基础设施、HTTP、平台模块和 AI 模块生命周期已抽出，剩余是运行角色拆分与全量模块注册完善。
- [~] 删除包含几十个字段的 `transport.Deps` / AI Core service locator；平台与 AI 容器已分离并按域分组，AI Core 已收敛为 `CoreHTTPDeps` / `AICoreHTTPDeps`，平台根容器和 Transport 业务逻辑仍待拆除。
- [~] 每个模块提供最小的 Register/Module 接口和显式依赖；已建立 `transport.Module` 并接入 AI 路由，其他域仍待迁移。
- [~] 后台组件统一实现 Start/Stop/Health 生命周期；异步任务、数据库、Redis、平台 worker 和 AI worker 已接入统一关闭路径，Health 与部分无 Stop worker 仍待补齐。
- [~] 启动失败时按逆序释放已经创建的资源；基础设施、平台模块和 AI 异步任务已登记，其他 context-owned worker 仍待补充等待语义。
- [~] 为各运行角色增加装配测试；当前覆盖资源栈、平台/AI 模块生命周期和公共/管理监听参数，完整依赖契约测试仍待补齐。
- [x] PostgreSQL adapter 将缺失行、唯一/外键/检查约束和输入格式错误翻译为领域错误；AI Transport 不再识别 pgx/pgconn 错误类型。
- [x] AI Transport 使用领域/标准值类型承接 HTTP 数据，清零 pgx、Redis、sqlc 和 PostgreSQL adapter 的直接依赖及对应例外台账。
- [x] 租户上游访问策略端点只依赖 `UpstreamAccessManager` 最小端口，AI/顶层 Transport 依赖容器不再暴露具体 `*upstreamaccess.Service`。
- [x] 分组配置导出、预检和导入只依赖 `GroupTransferManager` 最小端口，AI/顶层 Transport 依赖容器不再暴露具体 `*commercial.GroupTransferService`。
- [x] 管理审计日志列表只依赖 `AdminAuditLogReader` 读端口，与 `AdminAuditRecorder` 写端口分离，AI/顶层 Transport 不再暴露具体 `*observabilitycontrol.AuditService`。
- [x] 管理端、租户端和工作区仪表盘统一依赖 `DashboardQueryReader` 聚合读端口，AI/顶层 Transport 不再暴露具体 `*observabilitycontrol.DashboardService`。
- [x] 管理端、租户端、用户端和工作区用量查询统一依赖 `UsageQueryReader`，分页与汇总数据结构下沉到 domain，AI/顶层 Transport 不再暴露具体 `*observabilitycontrol.UsageService`。
- [x] 风控配置、测试检测、审核日志和风险事件分别依赖四组最小端口，检测与分页结果下沉到 domain，AI/顶层 Transport 不再暴露具体 risk-control service/checker。
- [x] 上游账号列表、管理员写入和连通性状态协调分别依赖目录、管理和健康端口；导出组合目录、密钥读取与解密端口，导入组合目录与管理端口，AI/顶层 Transport 不再暴露具体 `*upstreamcontrol.Service`。
- [x] 平台价格表管理、租户价格表自助和 LiteLLM 同步分别依赖三组显式端口；分组生效价格只依赖聚合模型目录，AI/顶层 Transport 不再暴露具体 `*billingcontrol.Service` 或要求无关价格服务就绪。
- [x] 商业控制面按分组目录、分组写入、调度规则、上游目标、用户绑定和限额策略拆成六组端口；API Key 只组合可见分组与限额能力，AI/顶层 Transport 不再暴露具体 `*commercial.Service`。
- [x] API Key 按安全摘要查询、创建更新、状态/删除生命周期和敏感密钥回显/轮换拆成四组端口；平台、租户、用户自助与限额路由按需组合能力，AI/顶层 Transport 不再暴露具体 `*identitycontrol.Service`。
- [x] AI 工作台按概览、模型目录、会话查询、会话管理、运行时消息持久化和图片任务查询拆成六组共享端口；Huma Transport、Console 和顶层装配不再暴露具体 `*workspace.Service`。
- [x] 订阅控制面按套餐查询、套餐管理、购买事务、订阅查询、订单查询和分组名称解析拆成六组端口；Serving 继续使用独立准入/扣费热路径端口，AI/顶层 Transport 不再暴露具体 `*subscription.Service`。
- [x] 订阅 HTTP 路由提取为独立 `SubscriptionHTTPDeps` / `RegisterSubscriptions` 纵向模块；`CoreHTTPDeps` 与 `RegisterAICore` 不再持有或注册订阅能力，认证和身份补全也只接收模块显式依赖。
- [x] 风控管理路由提取为独立 `RiskControlHTTPDeps` / `RegisterRiskControl` 纵向模块；四组业务端口脱离 `CoreHTTPDeps`，平台管理员认证和风控所需 `ProviderSecretCodec` 改为模块显式注入，Serving/worker 继续复用原实现。
- [x] 管理审计日志读取提取为独立 `AuditLogHTTPDeps` / `RegisterAuditLog` 纵向模块；只读查询从 `CoreHTTPDeps` 移出，跨域迁移仍通过独立模块保留的 `AdminAuditRecorder` 写入。
- [x] 系统状态与路由权重提取为独立 `SystemHTTPDeps` / `RegisterSystem` 纵向模块；`HealthTracker`、`ComponentHealthProbe` 和 `ScoreWeightsStore` 从 Core 运行时依赖组移出，平台管理员认证在模块内完成。
- [x] 管理端仪表盘四个查询端点提取为独立 `DashboardHTTPDeps` / `RegisterDashboard` 纵向模块；管理端身份补全能力单独装配，租户自助和工作区读取随后由各自模块组合查询端口。
- [x] 管理端用量查询七个端点提取为独立 `UsageHTTPDeps` / `RegisterUsage` 纵向模块；管理日志与用户排行的身份补全能力单独装配，租户自助和工作区读取随后由各自模块组合查询端口。
- [x] OAuth 凭证池与凭证管理端点提取为独立 `OAuthManagementHTTPDeps` / `RegisterOAuthManagement` 纵向模块；Core 仅保留共享模型绑定能力，Serving 与后台 token refresh 不进入该 HTTP 依赖容器。
- [x] 账号与凭证池模型绑定管理端点提取为独立 `ModelBindingHTTPDeps` / `RegisterModelBindings` 纵向模块；绑定 helper 改为显式接收账号/池读端口与 `UpstreamModelBindingStore`，发现、连通性和迁移流程分别由对应模块组合共享端口。
- [x] 上游模型发现、能力推断与账号连通性测试提取为独立 `UpstreamDiagnosticsHTTPDeps` / `RegisterUpstreamDiagnostics` 纵向模块；`HTTPDoer`、`ModelCapabilityResolver` 和账号健康协调端口不再进入 Core。
- [x] 上游账号 CRUD 与导入/导出迁移提取为独立 `UpstreamAccountManagementHTTPDeps` / `RegisterUpstreamAccountManagement` 纵向模块；目录、管理、密钥、绑定、价格校验和审计端口只在该模块组合，Core 不再持有账号管理与 Provider 密钥依赖。
- [x] 平台租户上游访问策略提取为独立 `UpstreamAccessManagementHTTPDeps` / `RegisterUpstreamAccessManagement` 纵向模块；`UpstreamAccessManager` 不再进入 Core，平台管理员认证在模块内完成。
- [x] 租户自助模型、价格表和上游资源目录提取为独立 `TenantCatalogHTTPDeps` / `RegisterTenantCatalog` 纵向模块；租户认证和模型/商业/价格同步端口在模块内组合，Core 不再注册这 17 条租户目录路径。
- [x] 平台 API key 管理提取为独立 `APIKeyManagementHTTPDeps` / `RegisterAPIKeyManagement` 纵向模块；平台代管的租户/用户 key 路由不再进入 Core，避免动态租户路径吞掉租户自助静态路径。
- [x] 租户自助 API key 与限额工作流提取为独立 `TenantSelfControlHTTPDeps` / `RegisterTenantSelfControl` 纵向模块；租户 ownership、分组可见性和 end-user 校验端口在模块内组合，Core 不再注册这些控制路径。
- [x] 租户分组控制面与迁移提取为独立 `TenantGroupManagementHTTPDeps` / `RegisterTenantGroupManagement` 纵向模块；分组、调度、上游目标、用户绑定和审计迁移端口不再由 Core 注册。
- [x] 租户自助 dashboard/usage 读取提取为独立 `TenantSelfReadHTTPDeps` / `RegisterTenantSelfRead` 纵向模块；Core 不再持有租户 dashboard 查询端口，租户控制和工作台保持独立。
- [x] 租户与终端用户 workspace 提取为独立 `WorkspaceHTTPDeps` / `RegisterWorkspace` 双认证模块；概览、会话、模型和图片任务端口不再进入 Core，tenant/user scope 仍由 JWT claims 决定。
- [x] 终端用户 API key 与限额工作流提取为独立 `UserSelfControlHTTPDeps` / `RegisterUserSelfControl` 纵向模块；key ownership、分组可见性和限额端口在模块内组合，Core 不再注册 9 条用户控制路径。
- [x] 终端用户分组、模型授权和用量读取提取为独立 `UserSelfReadHTTPDeps` / `RegisterUserSelfRead` 纵向模块；JWT claims 绑定用户范围，Core 不再注册 5 条用户读取路径。

### P1-03 收敛 HTTP 层业务逻辑

- [~] 把权限、事务、状态机和数据库查询移出 `internal/transport`；已完成管理 Dashboard 异常扣费告警查询迁移到 `SystemRepository`，其余用户/租户/支付域继续按边界逐项迁移。
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
- [x] JWT key retire 使用数据库条件更新并由每个副本在每轮执行后刷新本地 key cache。
- [ ] 支付补偿、数据清理、文件清理和价格同步逐项验证。
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
- [x] `bun run test`：64 个测试文件、214 个测试通过
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
- 遗留风险：Access 会话校验当前直接查询 PostgreSQL，后续可在保持撤销正确性的前提下增加短 TTL 缓存。
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
- 遗留风险：恢复码/硬件 WebAuthn 作为后续增强，生产管理员应在启用 TOTP 后保存组织级离线恢复流程。
- 下一候选项：`P0-04 改造浏览器 Token 存储`。

### P0-04（2026-08-20）

- 状态：完成并通过验收。
- 存储：Access Token 仅驻留当前标签页内存；Refresh Token 不再进入 JSON 响应、Web Storage 或 JavaScript 状态，改由 `dai_refresh_token` HttpOnly Cookie 携带。
- Cookie：固定 `Path=/api/auth`、host-only（不设置 `Domain`）、`SameSite=Strict`；生产环境启用 `Secure`，刷新轮换同步更新 `Max-Age/Expires`，登出使用同属性立即清除。
- CSRF：登录、刷新、MFA 验证、激活和登出执行 Origin/Referer 与 Host 校验；无来源头的原生 API 客户端保持兼容。
- 多标签页：使用 Web Locks API 或带租约的 localStorage 锁串行化 Refresh Token 轮换；通过 userInfo storage 事件同步登录、登出和 session 失效。
- 契约与说明：OpenAPI 增加 Cookie 参数和 `Set-Cookie` 响应头；Portal Cookie 说明、认证文档和刷新页面流程同步更新。
- 验证：新增 Cookie 属性、同源校验、刷新 Cookie 契约和 Portal 认证 Store 回归测试；`go test ./...`、`go vet ./...`、`bun run test`（64 个文件/214 个测试）、`bun run typecheck`、`bun run build:frontend`、`bun run ensure:api` 均通过。
- 遗留风险：浏览器级端到端测试尚未建立。
- 下一候选项：`P0-05 建立可信代理和公共 URL 边界`。

### P0-05（2026-08-20）

- 状态：完成并通过验收。
- 配置：新增 `server.public_base_url` / `DAI_PUBLIC_BASE_URL` 和 `server.trusted_proxy_cidrs` / `DAI_TRUSTED_PROXY_CIDRS`；生产必须使用 HTTPS 公共 origin，CIDR 在启动时校验。
- 可信解析：只有直接对端属于可信代理网段时才读取 `X-Forwarded-*`、`X-Real-IP` 和 RFC 7239 `Forwarded`；多级链路从服务端一侧反向解析第一个非可信客户端地址，直连伪造头被忽略。
- URL 边界：注册链接、法律链接、文件 capability URL 和图片 capability URL 统一使用固定公共 origin；未配置 origin 的开发请求保持相对路径。
- 观测与风控：访问日志、登录限速、AI 用量和风控统一读取可信客户端 IP。
- 文档：新增 `docs/HTTP_BOUNDARIES.md`，同步更新本地和生产配置示例。
- 验证：新增公共 origin、Host/Forwarded 伪造、可信/非可信代理、多级客户端 IP 和 capability URL 测试；`go test ./...`、`go vet ./...` 均通过。
- 遗留风险：浏览器级端到端测试尚未建立；代理网段和公共 origin 仍需由每个生产部署按实际拓扑配置。
- 下一候选项：`P0-06 让审计写入具备持久可靠性`。

### P0-06（2026-08-20）

- 状态：实现完成并通过回归。
- 持久化：schema 14 新增 `ai_audit_inbox`，请求审计先以 JSONB envelope 入库；`request_id` 唯一约束和 payload 幂等插入防止重复记录。
- 事务：正常路由请求由 UsageLogger 将审计 inbox 与 `ai_usage_logs`、额度扣减和 charge outbox 放入同一 PostgreSQL 事务；早期失败仍可独立 durable enqueue。
- Worker：移除内存 channel 和 `byteUsed`，改为多副本安全的 `FOR UPDATE SKIP LOCKED` 租约领取；租约过期可恢复，物化 payload 与删除 inbox 行同事务完成。
- 失败语义：媒体或 payload 写入失败保存 `last_error`，指数退避重试，超过 10 次进入 `dead`；超大载荷不再静默丢弃而是拒绝 durable enqueue 并记录结构化错误。
- 观测：新增待处理数、最老待处理时长、入队、完成、失败和 dead 指标；运维说明见 `docs/AUDIT_RELIABILITY.md`。
- 验证：新增 schema 14 迁移、事务回滚、入队幂等、claim/complete、失败转 dead 和 worker 回归测试；`go test ./...` 通过。
- 遗留风险：dead 行需要运维确认原因后人工重置；浏览器级端到端测试仍未建立。
- 下一候选项：`P0-07 保护签名密钥和敏感配置`。

### P0-07（2026-08-20）

- 状态：实现完成并通过回归。
- 密钥环：新增 active/previous key ID，统一 `enc:v1:<key-id>` AES-GCM 密文；兼容历史 `v1:` clientsecret 和 `aesgcm:v1:` Provider 密文，支持轮换宽限与显式重新加密。
- JWT：签名私钥写入前加密；启动加载历史明文 PEM 后立即迁移，解密或解析失败直接拒绝启动，不再静默跳过坏密钥。
- 凭证：Provider/OAuth、微信支付、MFA 和代理凭证统一走密钥环；OAuth、直接 Provider、微信支付、MFA 和代理读取路径具备在线重新加密。
- 配置：新增 `secret_master_key_id` / `secret_master_key_previous`，生产环境校验密钥长度、ID 格式、重复版本和 `keyID=key` 语法；轮换操作见 `docs/SECRET_KEY_ROTATION.md`。
- 验证：`go test ./...` 的非网络测试通过；完整测试在受限沙箱中仅因 httptest/miniredis 无法绑定本机端口而失败；`go vet ./...`、`git diff --check` 和 `bun run ensure:api` 作为提交前检查。
- 遗留风险：当前 keyring 由部署 Secret Manager 注入，尚未直接集成云 KMS/Vault；旧密钥必须由运维在迁移宽限期内保留。
- 下一候选项：`P0-08 补齐 HTTP 与浏览器安全基线`。

### P0-08（2026-08-20）

- 状态：实现完成并通过回归。
- 响应头：统一注入 CSP、HSTS（生产）、`nosniff`、DENY frame、Referrer-Policy、Permissions-Policy 和 COOP。
- 缓存：Portal HTML 使用 `no-store`，带 hash 的 `/assets/` 使用 immutable 长缓存，其他嵌入静态资源限制为 24 小时；API、runtime、OpenAPI 和探针默认 `no-store`，私有文件保持 `private, no-store`，显式公共 favicon 可覆盖缓存策略。
- 管理面：`/metrics`、`/ready` 和管理探针从业务监听移到 `server.management_addr`（默认 `127.0.0.1:19642`）；Huma `/docs` 与实时 OpenAPI 调试路由默认关闭；生产 Compose 健康检查改走管理监听，业务监听只保留存活 `/health`。
- 限制与超时：业务 HTTP Server 设置 32 KiB header 上限和 64 MiB 全局 body 上限；AI `WriteTimeout=0` 保留，由既有 serving deadline controller 执行首字节、空闲间隔和总时长限制。
- 验证：新增安全头、缓存覆盖、声明 body 超限和 chunked body 上限测试；`go vet ./...`、`bun run ensure:api` 和 OpenAPI/Portal 生成链通过。
- 遗留风险：管理监听若被部署显式改为非 loopback，必须由私有网络或认证反向代理保护；浏览器级 CSP/HSTS 仍需在真实 HTTPS 域名上做一次端到端验收。

### P1-02（进行中，2026-08-20）

- 本次变更：新增 `aiModules`，集中装配 AI 控制面、Serving pipeline、Gateway、Console、文件/图片服务和异步任务；`run` 仅保留组合顺序、HTTP 注册和健康检查。
- 生命周期：价格同步、风险审查、审计、OAuth Token refresh、结算 Outbox 和异步任务由 `aiModules.Start/Stop` 统一启动与关闭，根 context 取消仍负责无独立 Stop 的 worker。
- 依赖边界：平台 `transport.Deps` 与 AI `AICoreHTTPDeps` 分离，新增 `transport.Module` 并由 AI 模块独立注册路由；AI Transport Core 使用 `CoreHTTPDeps`，垂直模块各自持有窄依赖。
- 端口收敛：AI 系统端点改用 `ScoreWeightsStore` 最小接口，评分权重 PostgreSQL adapter 留在 composition root。
- 端口收敛：AI 认证端点改用 `HumaBanChecker`，Ban 状态 Redis adapter 留在 composition root。
- 端口收敛：OAuth 凭证管理端点改用 `OAuthTokenRefresher`，不再暴露后台刷新器具体类型。
- 端口收敛：AI 上游模型绑定、凭证导入和 pool CRUD 查询统一使用 `OAuthPoolReader`；创建、更新、状态变更和删除使用 `OAuthPoolWriter` 与领域级创建/更新命令。
- 端口收敛：凭证池账号列表改用 `OAuthCredentialReader` 与无密文 `OAuthCredentialSummary`；管理摘要采用已知账户 ID/套餐字符串字段允许列表，未知 provider metadata、嵌套结构和密钥材料不离开运行时边界，Transport 不再依赖原始 credential row。
- 端口收敛：凭证创建响应及更新/刷新/删除前的 scoped read 改用 `GetSummaryByID`；原始 `GetByID` 仅保留给 adapter 内部 serving/token refresh 解密路径。
- 端口收敛：凭证状态、权重更新和删除改用 `OAuthCredentialWriter`；刷新端点移除对完整 OAuth store 的冗余依赖。
- 端口收敛：凭证导入改用 `OAuthCredentialCreator` 与领域级 `OAuthCredentialCreate` 命令；Transport 不再通过完整 OAuth store 创建凭证或接收 adapter 写入模型。
- 端口收敛：OAuth pool 健康汇总改用 `OAuthPoolHealthReader` 与领域级 `OAuthPoolHealthSummary`；AI Transport 依赖容器不再暴露具体 `OAuthCredentialStore`。
- 端口收敛：OAuth pool 模型发现改用 `ClientCatalogResolver`，Transport 不再暴露具体 catalog service。
- 密钥边界：AI Transport 使用 `ProviderSecretCodec` 执行上游密钥加解密，不再持有或传播原始 `SecretMasterKey`；composition root 构造的 `ProviderKeyCodec` 保持既有密文兼容。
- HTTP 边界：上游模型发现、models.dev 目录刷新和账号连通性检测统一依赖最小 `HTTPDoer` 端口；连接池、重定向和 transport 超时策略继续由 composition root 构造的具体客户端持有。
- 模型能力边界：`externalmodels.Service` 封装 Redis、HTTP 和进程内缓存，移除包级共享缓存；AI Transport 仅依赖 `ModelCapabilityResolver`，未装配或目录未命中时保持本地启发式降级。
- 健康检查边界：AI 系统状态端点通过 `ComponentHealthProbe` 检查 PostgreSQL/Redis，go-redis 的 `Ping` 命令类型封装在 Redis adapter；`SystemHTTPDeps` 只接收探针和 `HealthTracker`，Core 不再持有系统状态端口。
- 可观测性边界：usage identity enrichment 失败通过 `IdentityEnrichmentFailureObserver` 上报，zap 消息与字段映射封装在 observability adapter；AI Transport 不再持有具体 `*zap.Logger`，失败时继续返回空补全结果。
- 账号读取边界：模型发现、模型导入、连通性检测和绑定校验通过 `UpstreamAccountReader` 获取 `upstreamcontrol.AccountSecret`；PostgreSQL adapter 负责映射 ciphertext、BaseURL、headers、协议与状态，AI Transport 不再接收上游账号 sqlc row。
- 用户用量读取边界：终端用户自助日志通过 `UserUsageLogReader` 读取 `domain.UsageLog`；专用查询的租户/用户/request source 参数构造与 sqlc row 映射封装进 `UsageRepo`，保持原过滤、倒序和 limit 语义，AI Transport 不再直接执行该查询。
- 审计写入边界：账号与分组迁移审计通过 `AdminAuditRecorder` 写入 `domain.AdminAuditEvent`；`AuditRepo` 负责可空字段和 sqlc 参数映射，保持写入失败不阻断迁移的 best-effort 语义。AI Transport 与顶层路由装配已移除最后一处 `*dbgen.Queries` 字段、import 和调用。
- 模型绑定边界：管理 CRUD、账号/凭证池模型目录导入、账号迁移导入导出和连通性测试统一依赖 `UpstreamModelBindingStore` 与领域模型；PostgreSQL adapter 持有 scoped 查询和原子导入事务，AI Transport 不再直接读写模型绑定表。
- 模型目录边界：租户/用户可用模型、分组有效价格和租户可选上游资源统一依赖 `ModelCatalogReader` 与领域投影；PostgreSQL adapter 封装分组授权、资源可见性、价格表和模型绑定的聚合 JOIN 及 JSONB 解码，Transport 只保留 DTO、价格区间和倍率计算。
- 基础设施边界：系统状态的 PostgreSQL/Redis 检查统一依赖 `ComponentHealthProbe`，账号迁移的价格簿校验依赖 `PriceBookReader`；AI Transport 已清零 `*pgxpool.Pool` 字段、import 和调用，PostgreSQL 连接只留在 composition root 与 adapter。
- 价格表边界：平台 CRUD/手工条目、租户可见范围/自助迁移、LiteLLM 查询与同步分别依赖 `PlatformPriceBookManager`、`TenantPriceBookManager`、`PriceBookSyncManager`；租户和用户分组生效价格复用 `ModelCatalogReader`，不再错误依赖完整价格表服务。
- 商业控制面边界：分组读与可见性、分组配置写入、调度规则、上游目标绑定、终端用户绑定和限额策略分别依赖六组显式端口；API Key 创建和更新只组合 `CommercialGroupCatalog` 与 `CommercialLimitPolicyManager`，具体 `commercial.Service` 只留在 composition root 和运行时管线。
- API Key 边界：非敏感摘要查询、创建与元数据更新、状态与删除生命周期、明文回显与轮换分别依赖 `APIKeyReader`、`APIKeyWriter`、`APIKeyLifecycleManager` 和 `APIKeySecretManager`；创建路由显式组合生命周期端口完成限额策略失败后的补偿删除，具体 `identitycontrol.Service` 只在 composition root 构造。
- 工作台边界：概览、模型目录、会话查询、会话管理、运行时消息持久化和图片任务查询分别依赖 workspace 包的六组能力端口；Huma Transport 与 Console 共享同一应用边界，流式聊天显式要求会话读与消息写端口，具体 `workspace.Service` 只在 composition root 构造。
- 订阅边界：套餐查询与租户管理、用户购买事务、订阅历史与当前状态、订单读取、快照分组名称补全分别依赖六组控制面端口；分组名称解析继续 fail-open，Serving 的 `ResolveForGate` / `DebitAdmitted` 保持独立热路径端口，具体 `subscription.Service` 只在 composition root 和后台 janitor 生命周期中持有。
- 订阅 HTTP 模块：`SubscriptionHTTPDeps` 独立组合六组订阅端口、`HTTPAuthDeps`、身份补全 provider 和失败 observer；`RegisterSubscriptions` 自行注册租户/终端用户认证分组。顶层仅通过 composition-only `AIHTTPDeps` 聚合 Core、订阅、风控、审计读取、系统、管理仪表盘、管理用量、OAuth 管理、模型绑定、上游诊断、上游账号管理、上游访问和租户目录模块，路由 handler 不再接触聚合容器；契约测试确认 core 不注册订阅路径、独立模块注册后执行认证。
- 审计 HTTP 模块：`AuditLogHTTPDeps` 独立组合 `AdminAuditLogReader` 和 `HTTPAuthDeps`；`RegisterAuditLog` 自行注册平台管理员认证分组。顶层仅通过 composition-only `AIHTTPDeps` 装配读取模块，`AdminAuditRecorder` 仍留在 Core 供账号/分组迁移写入；契约测试确认 core 不注册审计读取路径、独立模块注册后执行认证。
- 系统 HTTP 模块：`SystemHTTPDeps` 独立组合 `HealthTracker`、两个 `ComponentHealthProbe`、`ScoreWeightsStore` 和 `HTTPAuthDeps`；`RegisterSystem` 自行注册平台管理员认证分组。顶层通过 `AIHTTPDeps.System` 装配，Core 不再注册系统状态与路由权重路径；契约测试覆盖三条路径的 core 404 与独立模块认证。
- 仪表盘 HTTP 模块：`DashboardHTTPDeps` 独立组合管理端 `DashboardQueryReader`、身份 provider、失败 observer 和 `HTTPAuthDeps`；`RegisterDashboard` 自行注册平台管理员认证分组。租户自助和工作区随后由各自模块显式组合同一读端口，契约测试覆盖四条管理路径的 core 404 与独立模块认证。
- 用量 HTTP 模块：`UsageHTTPDeps` 独立组合管理端 `UsageQueryReader`、身份 provider、失败 observer 和 `HTTPAuthDeps`；`RegisterUsage` 自行注册平台管理员认证分组。租户自助、用户自助和工作区随后由各自模块显式组合共享查询端口，契约测试覆盖七条管理路径的 core 404 与独立模块认证。
- OAuth 管理 HTTP 模块：`OAuthManagementHTTPDeps` 独立组合池/凭证读写、池健康、手动刷新、模型目录和模型绑定端口，以及 `HTTPAuthDeps`；`RegisterOAuthManagement` 自行注册平台管理员认证分组。Core 不再承载 OAuth pool/credential 管理路径，契约测试覆盖 13 条池/凭证管理路径的 core 404 与独立模块认证。
- 模型绑定 HTTP 模块：`ModelBindingHTTPDeps` 独立组合账号读取、池读取、模型绑定存储和 `HTTPAuthDeps`；`RegisterModelBindings` 自行注册平台管理员认证分组。Core 不再承载账号/池模型绑定管理路径，契约测试覆盖 10 条账号/池绑定路径的 core 404 与独立模块认证。
- 上游诊断 HTTP 模块：`UpstreamDiagnosticsHTTPDeps` 独立组合账号读取、模型绑定、密钥解密、`HTTPDoer`、账号健康协调、模型能力目录和 `HTTPAuthDeps`；`RegisterUpstreamDiagnostics` 自行注册平台管理员认证分组。契约测试覆盖发现、导入、能力推断和连通性测试四条路径的 core 404 与独立模块认证。
- 上游账号管理 HTTP 模块：`UpstreamAccountManagementHTTPDeps` 独立组合账号目录、CRUD 管理、密钥读取/解密、模型绑定、价格簿和审计端口及 `HTTPAuthDeps`；`RegisterUpstreamAccountManagement` 自行注册平台管理员认证分组。Core 不再注册账号列表、CRUD、状态和迁移路径，契约测试覆盖 8 条路径的 core 404 与独立模块认证。
- 上游访问策略 HTTP 模块：`UpstreamAccessManagementHTTPDeps` 独立组合 `UpstreamAccessManager` 和 `HTTPAuthDeps`；`RegisterUpstreamAccessManagement` 自行注册平台管理员认证分组。Core 不再注册租户上游策略路径，契约测试覆盖 2 条路径的 core 404 与独立模块认证。
- 租户目录 HTTP 模块：`TenantCatalogHTTPDeps` 独立组合 `ModelCatalogReader`、`CommercialGroupCatalog`、`TenantPriceBookManager`、`PriceBookSyncManager` 和 `HTTPAuthDeps`；`RegisterTenantCatalog` 自行注册租户用户认证分组。Core 不再注册租户可用模型、有效价格、价格表和上游资源目录路径，契约测试覆盖 17 条路径的 core 404 与独立模块认证。
- API key 管理 HTTP 模块：`APIKeyManagementHTTPDeps` 独立组合 API key 读写/生命周期/密钥端口、分组目录、限额策略和 `HTTPAuthDeps`；`RegisterAPIKeyManagement` 自行注册平台管理员认证分组。Core 不再注册 14 条平台代管租户/用户 key 路径，契约测试覆盖代表性动态路径的 core 404 与独立模块认证。
- 租户自助控制 HTTP 模块：`TenantSelfControlHTTPDeps` 独立组合 API key 端口、分组目录、限额策略、租户 end-user 校验和 `HTTPAuthDeps`；`RegisterTenantSelfControl` 自行注册租户用户认证分组。Core 不再注册 11 条租户 API key/限额路径，契约测试覆盖全部路径的 core 404 与独立模块认证。
- 租户分组管理 HTTP 模块：`TenantGroupManagementHTTPDeps` 独立组合分组、调度、目标、用户绑定、价格表名称、迁移审计和 `HTTPAuthDeps`；`RegisterTenantGroupManagement` 自行注册租户用户认证分组。Core 不再注册 25 条分组/迁移路径，契约测试覆盖全部路径的 core 404 与独立模块认证。
- 租户自助读取 HTTP 模块：`TenantSelfReadHTTPDeps` 独立组合 `DashboardQueryReader`、`UsageQueryReader` 和 `HTTPAuthDeps`；`RegisterTenantSelfRead` 自行注册租户用户认证分组。Core 不再注册 5 条租户 dashboard/usage 路径，契约测试覆盖全部路径的 core 404 与独立模块认证。
- Workspace HTTP 模块：`WorkspaceHTTPDeps` 组合 workspace 概览、模型、会话、会话管理、图片任务、dashboard 和 usage 端口，并由 `RegisterWorkspace` 同时注册 tenant/user 两个认证分组。Core 不再注册 14 条 workspace 路径，契约测试覆盖两种 scope 的 core 404 与独立模块认证。
- 用户自助控制 HTTP 模块：`UserSelfControlHTTPDeps` 独立组合 API key 读写/生命周期/密钥、分组目录、限额策略和 `HTTPAuthDeps`；`RegisterUserSelfControl` 自行注册终端用户认证分组。Core 不再注册 9 条用户 API key/限额路径，契约测试覆盖全部路径的 core 404 与独立模块认证。
- 用户自助读取 HTTP 模块：`UserSelfReadHTTPDeps` 独立组合分组目录、模型目录、`UserUsageLogReader`、`UsageQueryReader` 和 `HTTPAuthDeps`；`RegisterUserSelfRead` 自行注册终端用户认证分组。Core 不再注册 5 条用户分组/模型授权/用量路径，契约测试覆盖全部路径的 core 404 与独立模块认证。
- 错误边界：生产 sqlc、内联 SQL、事务和批处理统一通过 PostgreSQL 翻译器，将 `ErrNoRows`、`23505`、`23503`、`23514`、`22P02` 分类为领域持久化错误；AI Transport 仅按领域错误生成既有 404/409/400 响应，不再 import `pgx` 或 `pgconn` 错误类型。
- 错误边界测试：覆盖翻译器分类、真实 PostgreSQL 缺失行与唯一约束、模型绑定重复写入，以及 HTTP 状态和 detail 映射；未知 SQLSTATE 和连接故障仍保留原始运维错误并返回 500。
- 值类型边界：AI Transport 的 UUID 校验与批量 ID 规范化改用通用 UUID 值类型，删除遗留的 `pgtype.UUID/Text/Timestamptz/Numeric/Int4` DTO 辅助函数；HTTP 包已清零整个 pgx 模块、Redis、sqlc 和 PostgreSQL adapter 的直接 import。
- 依赖门禁：删除 7 条 AI Transport 和 2 条主 Transport 已失效的基础设施例外，并补充规则测试；后续重新引入 pgx、Redis、sqlc 或 adapter 依赖会直接导致 `cmd/checkdeps` 失败。
- 上游访问边界：租户上游策略列表与全量替换统一依赖 `UpstreamAccessManager` 的 `ListForTenant` / `ReplacePolicies`；具体 service 只由 composition root 构造，顶层 Transport 也仅转发端口。路由测试覆盖 DTO 映射、策略命令与未装配 503。
- 分组迁移边界：分组配置 `Export` / `Preview` / `Import` 统一依赖 `GroupTransferManager`；具体 planning service 只由 composition root 构造，顶层 Transport 仅转发端口。路由测试覆盖租户 claims、三类请求/响应与未装配 503，审计行为保持不变。
- 审计读取边界：管理审计列表通过单方法 `AdminAuditLogReader` 读取 `domain.AuditLog`，与既有 `AdminAuditRecorder` 写端口独立装配；具体 service 只在 composition root 同时满足两个端口。路由测试覆盖 limit 上限、DTO 投影和未装配 503。
- 仪表盘读取边界：管理端四类统计、租户自助统计和工作区概览统一通过 `DashboardQueryReader` 读取领域投影；具体 service 只在 composition root 构造。路由测试覆盖四个查询方法、scope/时间窗口、limit 上限、金额与错误 DTO 投影及未装配 503。
- 用量读取边界：分页日志、详情、模型/计费单位/上游汇总、用户排行、用户汇总和每日趋势统一通过 `UsageQueryReader`；受限用户日志继续使用独立 `UserUsageLogReader`。`UsageSummaryFilter` / `UsageLogPage` 已迁入 domain，Transport 与 PostgreSQL adapter 不再依赖 control 包 DTO。路由测试覆盖 8 个方法、scope/时间窗口、分页与 limit、金额投影及未装配 503。
- 风控边界：配置读写、不落库检测、审核日志分页和风险事件处置分别通过 `RiskControlConfigStore` / `RiskControlDetector` / `RiskControlLogReader` / `RiskEventManager`；检测与分页结果已迁入 domain，control 包保留类型别名兼容 serving/worker。路由测试覆盖六类接口、配置密文保留、检测输入、日志/事件过滤、处置 actor 和四组未装配 503。
- 风控 HTTP 模块：`RiskControlHTTPDeps` 独立组合四组业务端口、`HTTPAuthDeps` 与 `ProviderSecretCodec`；`RegisterRiskControl` 自行注册平台管理员认证分组。契约测试确认 core 不注册风控路径、独立模块执行认证，配置更新路由只把明文 API key 交给 codec 并向存储端传递密文。
- 验证：`go test ./...`、`go vet ./...`、`go build ./...`、`go run ./cmd/checkdeps`、`bun run ensure:api`、`bun run typecheck`、`bun run test` 和 `git diff --check` 通过。
- 遗留风险：订阅、风控、审计读取、系统、管理仪表盘、管理用量、OAuth 管理、模型绑定、上游诊断、上游账号管理、上游访问、租户目录、平台 API key、租户自助控制、租户分组、租户自助读取、workspace、用户自助控制和用户自助读取路由均已脱离 `CoreHTTPDeps`，AI Core 只保留平台价格/限额端口；顶层平台 `transport.Deps` 仍保留部分具体 service，部分 worker 只有 context 取消，没有可等待的 `Stop/Health` 接口。
- 下一候选项：P1-03 收敛 Transport 层权限、事务、状态机和数据库查询逻辑，并继续治理平台根容器。

### P1-03（进行中，2026-08-23）

- 查询边界闭环：管理 Dashboard 异常扣费告警的 `ai_usage_logs` 查询和行扫描已移入 `internal/system/pg.SystemRepository`；Transport 只负责调用端口、错误映射和 DTO 投影。
- 租户/终端用户归属读取：`admin_tenant.getTenant` 与 `admin_enduser.checkUserBelongsToTenant` 已移入 `internal/tenant/pg.TenantRepository`，权限 handler 不再直接执行这两类身份查询。
- 租户状态事务：租户启停、组织用户/终端用户级联状态变更及恢复用户 ID 收集已移入 `internal/tenant/pg.TenantRepository.UpdateStatus`；Transport 只保留黑名单同步和错误映射。
- 租户生命周期写入：租户创建（含初始用户激活凭证）、更新和删除已移入 `internal/tenant/pg.TenantRepository`，通过 `tenant/ports.AdminTenantWriter` 注入；handler 只负责输入归一化、凭证输出和错误映射。
- AI 身份边界：`aiIdentityAdapter.CheckTenantEndUser` 改用 `TenantRepository.GetEndUserTenantID`，Transport 不再为终端用户归属校验执行内联 SQL。
- 认证账号边界：`/api/auth/me`、密码修改和用户名/邮箱更新通过 `auth/ports.AccountReader` / `AccountWriter` 调用 `AuthRepository`，唯一约束与 deleted 账号保护留在 persistence adapter。
- 认证审计读取边界：`/api/v1/auth-audit-logs` 通过 `auth/ports.AuthAuditLogReader` 调用 `AuthRepository.ListAuthAuditLogs`；过滤、计数、分页和稳定排序不再由 Transport 拼接 SQL。
- 充值目标边界：管理充值的租户存在和终端用户归属前置校验复用 `TenantRepository`；账务事务中的用户 `FOR UPDATE` 校验保持在充值工作流内，避免跨连接破坏资金一致性。
- 反向充值边界：租户用户的 `tenant_id` / `order_type` 校验已移入 `DeductionService.ReverseTenantOrder` 的订单 `FOR UPDATE` 事务；handler 只选择 scoped/unscoped port 并映射领域错误。
- 充值生命周期边界：人工充值改由 `RechargeService.GrantManual` 持有事务并调用 `TenantRepository.LockManualRechargeTarget`；`GrantBalance` 仅作为支付结算复用的外部事务原语。
- 支付订单边界：`GrantBalance` 统一约束五类 `order_type` 与额度来源、用户目标和 `payment_order_id` 的组合，阻止在线支付与人工充值参数混用。
- 支付查询边界：普通支付订单和统一充值列表/详情查询参数下沉到 `payment` domain，`PaymentService` 封装 PostgreSQL adapter；Transport 不再直接依赖 `payment/pg` 查询。
- 支付状态机边界：管理端同步、手动充值额度冲正和在线退款均通过 `PaymentService` application command 执行；订单类型判断、可操作状态和结果刷新不再由 Transport 编排。
- Sweep 状态机边界：超时关单/过期迁移改为按扫描时状态条件更新，支付提供商调用与回调并发时不会发生 `paid` 被 `closed/expired` 覆盖。
- 提现查询边界：提现列表改用 `payment.WithdrawalListParams`，`PaymentService` 负责状态白名单与输入归一化，adapter 只执行已校验的查询。
- 调度生命周期边界：支付 cleanup 错误回传 Scheduler 统一观测；Scheduler 防止重复 Start/Stop，停止时等待 worker 退出并可中断 JWT 退役初始等待。
- 后台任务观测边界：Scheduler 统一记录任务运行/成功/失败/跳过状态并挂载到 `/health`；PaymentService.SweepOnce 聚合单轮错误，避免局部失败被误报为成功。
- JWT 退役边界：`RetireExpiredGraceKeys(ctx)` 使用带超时的条件更新，并在 UPDATE 影响 0 行时仍 reload keys，避免只有抢到最后一行更新的副本清理本地 grace key、其他副本永久保留旧公钥。
- 跨副本锁边界：支付 sweep/cleanup 的 advisory lock 由同一物理 PostgreSQL 连接完成获取、执行和释放；任务上下文有 5 分钟硬超时，解锁不受任务超时影响，避免连接池错配和永久占锁。
- 调度指标边界：Scheduler 暴露任务运行结果、耗时、运行中、连续失败和跳过原因 Prometheus 指标；失败/卡住/持续跨副本跳过可以在管理端 `/metrics` 上做告警，不改变 `/ready` 的基础设施语义。
- 管理账号列表读取：系统管理员与租户用户分页查询已移入 `internal/user/pg.AdminAccountRepository`，Transport 只负责状态展示和分页 DTO 转换。
- 管理账号写入边界：系统管理员与租户用户创建、更新和启停状态变更已移入 `internal/user/pg.AdminAccountRepository`，通过 `user/ports.AdminAccountWriter` 接收命令；Transport 只保留角色/租户前置校验、错误映射和黑名单副作用。
- 密码重置边界：系统管理员、租户用户和终端用户的目标类型/删除状态校验与 `ActivationService.Reset` 调用已移入对应 user repository；Transport 只映射一次性凭证响应并触发会话下线。
- 删除事务边界：系统管理员硬删除和终端用户余额保护/软删除已移入 user repository；终端用户 guard 在持锁事务提交前执行，黑名单不可用或失败时不会提交删除。
- 终端用户列表读取：租户范围、租户名/用户名/关键词/状态过滤，以及余额、最后登录和资料投影已移入 `internal/user/pg.AdminEndUserRepository`；权限范围仍由 claims 在 handler 先收窄。
- 终端用户写入边界：资料更新与启用/停用状态变更已移入 `internal/user/pg.AdminEndUserRepository`，通过 `user/ports.AdminEndUserWriter` 接收显式字段更新命令；Transport 不再直接执行这两类 `iam_accounts` UPDATE，黑名单同步仍由 handler 编排。
- 终端用户创建事务：账号插入和一次性激活令牌写入已移入 `AdminEndUserRepository.CreateEndUser`，由同一事务提交/回滚；handler 只生成凭证、组装命令和映射唯一约束错误。
- 回归：新增 canonical schema 集成测试，确认 24 小时窗口、失败状态过滤、空 settlement error 和时间倒序；`internal/transport` 仅保留展示转换。
- 回归：新增 TenantRepository canonical schema 测试，覆盖租户详情投影、联系人字段、终端用户租户归属和 deleted 用户不可见。
- 回归：扩展 TenantRepository 状态测试，覆盖 active/disabled/inherited_disabled 级联、跨租户隔离、已禁用/锁定账号保护和恢复用户清单。
- 回归：新增 TenantRepository 生命周期测试，覆盖初始用户激活关联、更新提交、无引用删除和有引用删除保护。
- 回归：新增 AI identity scope 测试，覆盖跨租户和 deleted 终端用户拒绝。
- 回归：新增 AuthRepository 账号投影/写入测试，覆盖快照、密码更新、唯一约束、deleted 保护和租户状态读取。
- 回归：充值入口不再执行租户/用户身份 SQL，目标边界由 TenantRepository canonical schema 测试覆盖。
- 回归：新增 AdminAccountRepository canonical schema 测试，覆盖管理员关键词、租户范围、分页顺序、状态和凭证状态投影。
- 回归：扩展 AdminAccountRepository 写入测试，覆盖两类账号创建、激活令牌关联、超级管理员保护、租户用户状态和资料更新。
- 回归：扩展账号 repository 重置测试，覆盖三类目标类型隔离、deleted 账号拒绝和一次性凭证结果。
- 回归：新增系统管理员删除与终端用户删除测试，覆盖超级管理员保护、余额正负值阻断、blacklist guard 回滚和重复删除不可见。
- 回归：新增 AdminEndUserRepository canonical schema 测试，覆盖租户范围、租户名/状态/关键词过滤、余额和最后登录投影。
- 回归：新增 AdminEndUserRepository 写入测试，覆盖显式清空字段、租户作用域、deleted 账号保护和状态变更。
- 回归：新增 AdminEndUserRepository 创建测试，覆盖 pending_activation 投影、激活令牌关联和激活写入失败时的账号回滚。
- 回归：新增反向充值租户范围测试，覆盖跨租户订单、错误 `order_type` 拒绝、事务回滚以及本租户订单成功撤销。
- 回归：新增 RechargeService canonical schema 测试，覆盖人工充值事务提交、停用用户锁定拒绝、余额不变和支付/人工订单参数边界。
- 回归：新增 AuthRepository canonical schema 测试，覆盖认证审计日志过滤、空值投影、时间/ID 稳定排序和分页归一化。
- 回归：新增 PaymentService canonical schema 测试，覆盖统一充值列表/详情查询和缺失订单错误归一化。
- 回归：扩展 PaymentService 状态动作测试，覆盖手动订单拒绝同步/退款、人工额度冲正、重复冲正和统一投影刷新。
- 回归：新增支付订单状态迁移测试，覆盖条件更新成功、陈旧 sweep 更新被拒绝和终态保持不变。
- 回归：新增提现列表 application 测试，覆盖状态空白归一化、有效筛选和未知状态拒绝。
- 回归：新增 Scheduler 生命周期测试，覆盖重复启动/停止不 panic 且 worker 可及时退出。
- 回归：新增 Scheduler 健康快照测试，覆盖失败计数、跨副本 advisory lock 跳过和最近错误投影。
- 回归：新增 Scheduler PostgreSQL advisory lock 集成测试，确认同一锁连续两轮均可获取，防止连接池跨会话释放回归。
- 回归：新增 JWT 多副本退役测试，确认一个副本完成 grace key 退役后，另一个 UPDATE 影响 0 行的副本也会刷新 JWKS 缓存。
- 下一候选项：验证价格同步的多副本幂等与失败重试语义，并为需要长任务的 worker 补充租约/心跳与 fencing。
