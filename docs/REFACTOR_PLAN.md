# D-AI 重构与优化清单

更新日期：2026-08-25

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
- [x] Transport 只依赖 application command/query，不直接访问数据库；业务 adapter 和基础设施直接依赖已由 `cmd/checkdeps` 门禁清零。
- [x] 禁止模块绕过公开端口写入其他模块表；包级依赖由 `cmd/checkdeps` 门禁，数据库角色、表 owner、grant/revoke 和切换窗口由 P1-07 ownership 契约锁定。
- [x] 使用 `cmd/checkdeps` 依赖检查器并接入 Make/CI，阻止未登记的反向依赖。
- [x] 在 `docs/MODULE_DEPENDENCY_RULES.md` 和例外台账记录允许的跨模块事务及历史例外。

### P1-02 拆分 composition root 和巨型依赖容器

- [~] 将 `cmd/server/main.go` 拆为配置、基础设施、模块装配和运行生命周期；基础设施、HTTP、平台模块和 AI 模块生命周期已抽出，剩余是运行角色拆分与全量模块注册完善。
- [x] 删除包含几十个字段的 `transport.Deps` / AI Core service locator；平台路由改由 `transport.Module` 和模块专属依赖类型注册，AI Core 已收敛为 `CoreHTTPDeps` / `AICoreHTTPDeps`，AI 与 raw 路由均使用窄依赖。
- [~] 每个模块提供最小的 Register/Module 接口和显式依赖；已建立 `transport.Module` 并接入平台/AI 路由，identity 自助/公开、认证/JWT key、operations、payment、AI identity user port 与管理员六类子模块已提取最小依赖，平台根容器治理仍需继续收敛。
- [~] 后台组件统一实现 Start/Stop/Health 生命周期；异步任务引擎、数据库、Redis、平台 worker 和 AI worker 已接入统一关闭路径，异步任务引擎已提供自身 Health 快照，其他组件探针仍待补齐。
- [~] 后台组件统一实现 Start/Stop/Health 生命周期；平台 BanReconciler、AI 风控 worker、审计 inbox worker、OAuth 刷新 worker、结算 outbox consumer、LiteLLM 刷新、异步任务、runtime binding cache、subscription janitor、data cleanup 和小时级清理任务已补齐幂等停止、等待退出和共享生命周期 Health，队列故障级指标仍由各自观测面提供。
- [~] 启动失败时按逆序释放已经创建的资源；基础设施、平台模块、AI worker、LiteLLM 刷新、小时级任务和 HTTP 监听器已登记并具备等待语义，仍需清点少量请求外 goroutine。
- [x] Runtime Gateway 的 API Key telemetry goroutine 由 Gateway owner 登记、fencing 和等待；`aiModules.Start/Stop` 统一启停，数据库池释放前不会遗留 `last_used_at` 写入。
- [x] Console 流式消息持久化 goroutine 由请求级 owner 使用 defer、`sync.Once` 和 `WaitGroup` 管理；正常完成持久化最终路由，panic/异常退出标记中断并等待退出，不再遗留请求外 goroutine。
- [~] 为各运行角色增加装配测试；当前覆盖资源栈、平台/AI 模块生命周期和公共/管理监听参数，完整依赖契约测试仍待补齐。
- [x] 组合根新增全量 Transport surface contract，验证 metadata、identity、admin、billing、operations 和 AI 六组模块在同一注册列表中均有代表路由。
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

- [~] 把权限、事务、状态机和数据库查询移出 `internal/transport`；已完成管理 Dashboard 异常扣费告警查询迁移到 `SystemRepository`，管理员租户用户创建改由账号事务和外键错误映射保证一致性，其余用户/租户/支付域继续按边界逐项迁移。
- [~] Handler 只负责认证上下文、DTO 转换、调用 application 和错误映射；核心查询、事务、终端用户 tenant scope 和状态机已下沉，部分管理副作用和跨域编排仍在 HTTP 层。
- [~] 用户、租户、支付、充值、公告和清理逐域迁移；用户、租户、支付、充值查询/写入和清理租约已迁移，管理员账号创建不再依赖锁外租户预检，公告与少量 legacy 编排仍待收敛。
- [x] 运营账务 command（用量退款、充值撤销和批量退款）统一接收调用方 context；Transport 与支付 application 不再让请求脱离 `context.Background()` 访问账务数据库，批量命令在取消后会停止处理剩余项目。
- [x] 账号/租户状态变更、密码重置、资料更新、登出和改密的黑名单/会话副作用统一通过 `auth/ports.AccountSecurityWriter`；Transport 不再直接编排 Redis token 与 ban 写入，读路径也沿用请求 context。
- [x] 通知发送统一通过 `notification.Service.Send` command，并以 `notification.HTTPService` 最小端口注入 Transport；channel 分发和未知 channel 校验不再由 Handler 决定。
- [x] 管理充值目标归属与存在性统一由 `RechargeService.GrantManual` 在同一账务事务锁内解析/校验；Transport 不再通过 `TenantRepository` 做锁外预检，平台管理员可在用户充值时省略 `tenantId`。
- [x] 用户身份查询与状态更新统一透传调用方 context；`UserService` 不再丢弃请求取消信号，PostgreSQL UserRepository 的读写均使用传入 context。
- [x] 邀请码公开查询、邀请码管理、用户名唯一性检查和邀请注册统一透传调用方 context；InviteService/InviteRepository 不再为请求路径隐式创建 `context.Background()`。
- [x] JWT access token 的 session 校验统一透传请求 context；平台、AI Transport 和 Console middleware 不再让取消的请求继续使用独立 `context.Background()` 查询会话。
- [x] JWT signing key 列表与轮换 command 统一接收请求 context；管理端取消请求会停止密钥查询/事务，轮换前不会继续生成无用 RSA 密钥。
- [x] 图片桥接的远程输入 materialization 统一透传请求 context；Gemini→OpenAI image edit 不再在请求取消后使用隐式 `context.Background()` 下载输入图。
- [x] UsageLogger 的 API key quota cache invalidation 复用财务 completion context；成功记账后的缓存失效仍有独立 2 秒上限，但不再重新脱离调用链创建 Background。
- [x] OAuth pool 模型 catalog 的 singleflight discovery 由 catalog owner 登记、取消和等待；请求取消仍可共享已有加载，但 AI 模块停止前不会遗留 provider/数据库访问 goroutine。
- [x] Fixed-provider client runtime 的 credential refresh singleflight 由 Runtime owner 登记、取消和等待；请求取消仍保留共享刷新语义，AI 模块停止前不会遗留 OAuth 刷新访问。
- [x] AI 管理 API 已按价格、上游、路由、用量、订阅和风控拆分为独立 HTTP 模块与最小端口。
- [x] 将 Transport 层关键路径覆盖率提升到可执行门槛；`scripts/check_transport_coverage.sh`、Make target 和 CI 统一执行 atomic coverage，当前门槛 10.0%，基线 10.4%，支持通过 `TRANSPORT_COVERAGE_MIN` 持续抬高。

### P1-04 拆分超大 Go 文件和清理兼容遗留

- [x] 拆分 `internal/ai/serving/execute.go` 的候选选择、传输、流式响应和图片响应职责；候选、upstream attempt、sync/stream relay 和 image relay 已分别移入独立文件，主文件仅保留执行编排及共享 helper。
- [x] 拆分大型 PostgreSQL Repository，按聚合或 use case 组织；`CommercialRepo` 已按 groups/dispatch/targets/bindings/helpers 拆为同包垂直 adapter 文件，构造和 repository port 保持不变。
- [x] 清理 staticcheck 报告的死代码、无效赋值和潜在 nil dereference；使用与 Go 1.27 匹配的 staticcheck 已通过全仓 `staticcheck ./...`，并清理了 internal、cmd、payment、tenant 与 transport 中的死代码和无效赋值。
- [x] 删除已不再注册的旧 Console handler 和 legacy bridge helper；保留当前 `/runtime/v1` 已注册路由及其运行时兼容桥接。
- [x] 为关键拆分建立行为等价测试，避免顺手改变计费和路由语义；Serving 候选/attempt/stream/image 回归测试与 CommercialRepo 聚合/接口测试覆盖路由、协议、失败转移、图片响应和计费边界。

### P1-05 强化授权模型

- [x] 将散落的 `userType` 判断集中为后端 capability/policy 授权；管理、财务、支付、仪表盘、清理、代理节点、JWT key、公告、通知和自助端点均已由共享 capability/policy 决策，剩余 userType 仅用于持久化/展示/claims 完整性。
- [x] 将散落的 `userType` 判断集中为后端 capability/policy 授权；`auth.Actor` / `Capability` / `requireCapability` 已覆盖主要 HTTP 与应用策略，登录租户状态、管理员 MFA 和审计 principal 分类也已收敛到同一 Actor policy。
- [x] Portal 菜单和路由 capability 只用于展示/导航提示；后端通过 operation middleware 和 capability/policy 矩阵执行最终授权，并有服务端拒绝回归测试。
- [x] 建立统一的 `auth.Actor`、`TenantScope`、`UserID`/`TenantID` 和 `ResourceOwnership` 类型；Transport 从 claims 统一构造 actor，租户、终端用户、租户详情和支付订单 ownership 均通过同一 typed reference 校验。
- [x] 为当前 318 个 OpenAPI operation 生成并维护 capability 授权矩阵；`cmd/checkauthz` 对未归类 operation 失败，并在 CI 校验生成物 freshness。
- [x] 增加跨租户、越权、对象枚举和角色降级测试；授权核心已覆盖四类角色、缺失/未知角色、租户/用户 ownership 组合，Transport middleware、AI identity、终端用户、异步任务、账务和会话集成测试覆盖拒绝与对象不可枚举语义。

## P1：数据库与资金数据治理

### P1-06 统一迁移链和 schema 真相源

- [x] 采用 forward-only SQL migration 工具，迁移仍由发布步骤显式执行；`deploy/production/schema_release.sh` 负责备份、连续脚本执行和逐步版本确认，不引入运行时迁移依赖。
- [x] 不允许应用服务启动时隐式执行生产迁移；启动只调用 `db.VerifySchema`，迁移由发布步骤显式执行。
- [x] 空库基线由完整迁移链生成或验证，避免同时手工维护两套结构；`scripts/replay_schema_chain.sh` 从不可变 v1 Git 基线重放到 v24，并将最终 schema-only dump 与 `init.sql` 比较。
- [x] 每个迁移在空库和前一 schema 版本副本上验证；23 个迁移有真实 PostgreSQL 专项测试，CI 另执行 v1→v24 全链重放并逐步校验版本。
- [x] 为缺少专项测试的 0002、0003、0009 补迁移测试；测试覆盖来源版本、关键结构、数据转换和约束行为。
- [x] 校准 `README.md`、`docs/DATABASE.md` 和 `docs/PROJECT_STATUS.md` 的 schema 版本；`docs/SCHEMA_CHAIN.md` 由门禁生成并在 CI 校验 freshness。
- [x] 发布流程加入备份、迁移校验、兼容窗口和失败恢复步骤；`deploy/production/schema_release.sh` 显式执行并记录备份/哈希/版本，`docs/SCHEMA_RELEASE_RUNBOOK.md` 固化停流量、readiness、兼容窗口和恢复步骤。

### P1-07 建立数据库领域所有权

- [x] 从全 `public` schema 迁移到领域 schema，或用独立数据库角色实现等价隔离；`internal/db/ownership.sql` 固化 runtime/billing 角色契约，composition root 已支持独立 `DAI_BILLING_DATABASE_URL`。
- [x] 账本表只允许 billing 模块角色写入；账务表 owner、runtime DML revoke 和 billing grants 已由契约/探针锁定，角色 provisioning 与维护窗口切换均已脚本化并要求显式确认。
- [x] 网关只写运行时事实、用量和可靠投递，不直接修改控制面配置；runtime 仅保留账务读取和 `bill_charge_outbox` 的 `INSERT`，billing pool 承担结算/支付/订阅扣费写路径。
- [x] 跨域读取通过视图、只读端口或显式 query service；billing/payment、租户管理/分析、管理员终端用户与运营仪表盘核心聚合均已迁移到只读视图，相关查询不再直接跨域联表。
- [x] CI 检查应用角色的最小权限和越权失败行为；`scripts/check_db_ownership.sh` 验证 runtime 账本写入失败、outbox 入队成功和 billing 角色写入成功。

### P1-08 持续验证资金不变量

- [x] 将余额、批次、充值、退款、订阅和 AI 结算不变量形成统一测试套件；只读 `billing/invariants.Check`、真实 PostgreSQL 生命周期测试和每阶段 7 项检查断言已覆盖充值→扣费→过期→撤销→退款→订阅→Outbox。
- [x] 增加固定种子随机并发属性测试与并发幂等测试，覆盖充值、扣费、过期、撤销与退款交错；统一账本写路径为「账户→批次」锁顺序，消除反向锁死。
- [x] 增加定期线上对账任务和差异告警；Scheduler 使用 Repeatable Read 只读快照与跨副本 advisory lock，发布 Prometheus 差异指标，`docs/DATABASE.md` 固化告警阈值与禁止绕过账本的恢复边界。
- [x] 为 Outbox 积压和 parked row 定义处理手册；`docs/BILLING_OUTBOX_RUNBOOK.md` 固化状态语义、只读排查、单行受控 requeue、验收和禁止操作。
- [x] 所有资金修复必须保留不可变审计证据和幂等键；schema v24 的 `bill_repair_audits` 以触发器拒绝修改/删除，退款、充值撤销和 parked requeue 在同一事务写入 before/after 快照并以唯一幂等键防重。

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
- [x] LiteLLM 价格导入与常用模型同步按价格表事务批量执行，重复快照不重复 bump revision，失败回滚可重试且不覆盖手工条目。
- [x] 数据清理运行增加 owner、heartbeat、lease_until 和终态 fencing，过期租约可回收，长任务不会被固定启动扫描误判为 stale。
- [x] FileStore 过期资产采用 claim → 文件删除 → owner fencing finalize，两阶段失败可释放 claim 重试，不提前删除数据库元数据。
- [x] 本地图片 `_tmp`/`ephemeral` 清理增加共享 storage 文件租约与心跳；task orphan 扫描接入 async task 检查器，未确认的任务目录默认不删。
- [x] 支付补偿使用 advisory lock 执行单轮任务，并以订单级持久化退避、失败计数和状态 fencing 处理 provider 长时间故障。
- [x] 增加支付 retry backlog、最老失败时长指标和告警 runbook，避免仅依赖单轮 scheduler 错误或人工查询 `sweep_last_error`。
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

- [x] 修复当前 staticcheck 报告；全仓 `staticcheck ./...` 已通过。
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
- [x] `staticcheck ./...`：使用与 Go 1.27 匹配的 staticcheck 已通过
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
- 依赖边界：平台路由模块与 AI `AICoreHTTPDeps` 分离，新增 `transport.Module` 并由 composition root 显式注册；AI Transport Core 使用 `CoreHTTPDeps`，垂直模块各自持有窄依赖。
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
- 遗留风险：订阅、风控、审计读取、系统、管理仪表盘、管理用量、OAuth 管理、模型绑定、上游诊断、上游账号管理、上游访问、租户目录、平台 API key、租户自助控制、租户分组、租户自助读取、workspace、用户自助控制和用户自助读取路由均已脱离 `CoreHTTPDeps`，AI Core 只保留平台价格/限额端口；平台 concrete service 仍由单进程 composition root 持有，运行角色拆分和少量 worker 生命周期边界仍待继续治理。
- 下一候选项：P1-03 收敛 Transport 层权限、事务、状态机和数据库查询逻辑，并继续治理平台根容器。

### P1-03（进行中，2026-08-23）

- 查询边界闭环：管理 Dashboard 异常扣费告警的 `ai_usage_logs` 查询和行扫描已移入 `internal/system/pg.SystemRepository`；Transport 只负责调用端口、错误映射和 DTO 投影。
- 租户/终端用户归属读取：`admin_tenant.getTenant` 与 `admin_enduser.checkUserBelongsToTenant` 已移入 `internal/tenant/pg.TenantRepository`，权限 handler 不再直接执行这两类身份查询。
- 租户状态事务：租户启停、组织用户/终端用户级联状态变更及恢复用户 ID 收集已移入 `internal/tenant/pg.TenantRepository.UpdateStatus`；Transport 只保留黑名单同步和错误映射。
- 租户生命周期写入：租户创建（含初始用户激活凭证）、更新和删除已移入 `internal/tenant/pg.TenantRepository`，通过 `tenant/ports.AdminTenantWriter` 注入；handler 只负责输入归一化、凭证输出和错误映射。
- AI 身份边界：`aiIdentityAdapter.CheckTenantEndUser` 改用 `TenantRepository.GetEndUserTenantID`，Transport 不再为终端用户归属校验执行内联 SQL。
- 认证账号边界：`/api/auth/me`、密码修改和用户名/邮箱更新通过 `auth/ports.AccountReader` / `AccountWriter` 调用 `AuthRepository`，唯一约束与 deleted 账号保护留在 persistence adapter。
- 认证审计读取边界：`/api/v1/auth-audit-logs` 通过 `auth/ports.AuthAuditLogReader` 调用 `AuthRepository.ListAuthAuditLogs`；过滤、计数、分页和稳定排序不再由 Transport 拼接 SQL。
- 租户门户品牌边界：门户名称、站点名称和 favicon 读写统一通过 `tenant/ports.PortalBrandingReader` / `PortalBrandingWriter`；PostgreSQL 事务、租户名称唯一约束和二进制投影留在 `tenant/pg`，Huma 与原生 favicon Transport 不再依赖 `pgxpool` 或 `tenant/pg`。
- 租户自助边界：当前租户用户、邀请码 CRUD 和三类租户分析统一通过 `tenant/ports.TenantSelfService`；`tenant.SelfService` 负责邀请码生成与唯一冲突重试，Transport 不再构造 `TenantRepo` 或识别数据库错误。
- 账户查询边界：账户余额、充值记录和账户统计统一通过 `billing/ports.AccountQueryReader` 与 `billing/service.AccountQueryService`；余额/充值查询使用请求 context，Transport 不再构造 `billing/pg.AccountRepository`。
- 债务查询边界：管理端账户债务改用同一 `AccountQueryReader` 余额投影，Transport 不再直接读取 `billing/ledger` 或重复计算账户服务状态。
- 租户管理读边界：租户列表/详情、终端用户归属和 AI identity 租户补全统一通过 `tenant/ports.AdminTenantReader`，Transport 不再构造 `tenant/pg.TenantRepository`。
- 充值目标边界：管理充值的租户存在和终端用户归属由 `RechargeService.GrantManual` 通过 `ManualRechargeTargetLocker` 在同一账务事务内锁定/解析；Transport 不再执行锁外目标预检，避免跨连接竞态。
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
- 价格同步边界：LiteLLM 导入与 common-model sync 收集完整快照后交给 `PriceBookRepo.ImportEntries` 单事务执行；价格表行锁 + 条件 upsert 保证多副本串行、相同快照无变化，事务失败整批回滚后下一次可安全重试，`manually_edited` 条目不会被覆盖。
- 数据清理边界：`sys_data_cleanup_runs` 以 owner/heartbeat/lease_until 持有可回收租约；heartbeat 失败或 owner 不匹配会取消工作，`finishRun` 以 owner 条件完成 fencing，schema v15 的 lease 索引支持启动和下次排程回收过期运行。
- FileStore 清理边界：`file_assets` 以 owner/lease_until claim 过期资产，文件系统删除成功后才 finalize 数据库记录；删除失败释放 claim 保留元数据，schema v16 的租约索引支持崩溃回收，多副本通过 `FOR UPDATE SKIP LOCKED` 只处理一次。
- 本地图片清理边界：`imageassets.Service.CleanupExpired(ctx)` 以 `.image-cleanup-lease` 协调共享目录副本，heartbeat 丢失即停止；临时/ephemeral 文件按 mtime 清理，task 目录仅由 `TaskRetained` 检查器确认 orphan 后删除，避免误删仍可访问的任务产物。
- 跨副本锁边界：支付 sweep/cleanup 的 advisory lock 由同一物理 PostgreSQL 连接完成获取、执行和释放；任务上下文有 5 分钟硬超时，解锁不受任务超时影响，避免连接池错配和永久占锁。
- 调度指标边界：Scheduler 暴露任务运行结果、耗时、运行中、连续失败和跳过原因 Prometheus 指标；失败/卡住/持续跨副本跳过可以在管理端 `/metrics` 上做告警，不改变 `/ready` 的基础设施语义。
- 管理账号列表读取：系统管理员与租户用户分页查询已移入 `internal/user/pg.AdminAccountRepository`，Transport 只负责状态展示和分页 DTO 转换。
- 管理账号写入边界：系统管理员与租户用户创建、更新和启停状态变更已移入 `internal/user/pg.AdminAccountRepository`，通过 `user/ports.AdminAccountWriter` 接收命令；Transport 只保留角色/租户前置校验、错误映射和 `AccountSecurityWriter` 调用。
- 密码重置边界：系统管理员、租户用户和终端用户的目标类型/删除状态校验与 `ActivationService.Reset` 调用已移入对应 user repository；Transport 只映射一次性凭证响应并触发会话下线。
- 删除事务边界：系统管理员硬删除和终端用户余额保护/软删除已移入 user repository；终端用户 security ban guard 在持锁事务提交前执行，黑名单不可用或失败时不会提交删除。
- 终端用户列表读取：租户范围、租户名/用户名/关键词/状态过滤，以及余额、最后登录和资料投影已移入 `internal/user/pg.AdminEndUserRepository`；权限范围仍由 claims 在 handler 先收窄。
- 终端用户写入边界：资料更新与启用/停用状态变更已移入 `internal/user/pg.AdminEndUserRepository`，通过 `user/ports.AdminEndUserWriter` 接收显式字段更新命令；Transport 不再直接执行这两类 `iam_accounts` UPDATE，黑名单同步改由 `AccountSecurityWriter` command 负责。
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
- 回归：新增价格表导入并发/回滚测试，确认两个副本仅一个有效更新、重复快照不 bump revision、手工条目不被覆盖，失败批次可完整重试。
- 回归：新增数据清理租约测试，确认 heartbeat 续租、接管后旧 owner 的终态写入被 fencing、过期运行可回收并重新排队；新增 schema v15 migration 测试。
- 回归：新增 FileStore 清理并发/失败测试，确认两个副本只 finalize 一次、文件删除失败时元数据与 claim 保留/释放、下一轮可恢复；新增 schema v16 migration 测试。
- 回归：新增本地图片 orphan/文件租约测试，确认活跃 task 保留、缺失 task 目录可回收、共享租约阻止并发清理且过期租约可接管。
- 下一候选项：P2-02 支付补偿的 provider 故障退避、重试告警与多副本验收。

### P2-02 支付补偿（2026-08-24—2026-08-25）

- 状态：完成本轮 provider 故障退避与重试语义改造。
- 持久化：schema v17 为 `pay_orders` 增加 sweep 失败次数、最后尝试时间、最后错误和下一次尝试时间；schema v18 增加 retry 健康统计部分索引；候选查询跨进程重启继续遵守数据库中的重试窗口。
- 退避：provider 查询、关单和补偿入账失败按 1 分钟起步指数退避，最大 1 小时；`USERPAYING/NOTPAY` 仅延后 5 分钟查单，不虚增失败次数。
- 并发：所有失败记录、延后查单、过期迁移和终态清理均按观察到的订单状态条件更新；回调已经入账时，迟到的 sweep 结果不会覆盖 `paid`。
- 观测：新增 retry 订单数、到期重试数、最老失败时长和统计查询失败指标；告警阈值、只读排查 SQL、恢复确认和禁止手工改状态流程见 `docs/PAYMENT_SWEEP_RUNBOOK.md`。
- 验证：新增 schema v17/v18 migration、PostgreSQL retry state/fencing、service provider failure、backoff 和 retry stats 单测；`go test ./internal/payment/...`、`go test ./internal/db/...`、`go vet ./internal/payment/... ./internal/db/...` 通过。
- 遗留风险：当前指标是全局低基数聚合，不按租户或订单暴露；单笔异常仍需通过管理端同步动作和审计/对账流程处理。
- 下一候选项：P1-03 继续清零 Transport 领域越界，并评估支付按渠道维度的对账指标。

### P1-03（继续，2026-08-25）

- 状态：完成租户门户品牌垂直切片，继续推进 Transport 业务边界收敛。
- 端口：新增 `tenant/ports.PortalBrandingReader` / `PortalBrandingWriter` 与领域投影，品牌读写和错误语义由租户端口承接；PostgreSQL adapter 保留唯一名检查、设置更新事务和 favicon 持久化。
- HTTP：租户品牌 Huma 路由与公开 favicon 路由只接收最小读写端口，Transport 仅处理 claims、DTO/图片输入校验、响应投影和 404/409/503 映射。
- 回归：新增端口注入、租户作用域命令、错误映射和公开 favicon 响应测试；adapter 通过编译期接口断言保持实现完整。
- 租户自助边界：新增 `tenant.SelfService` 应用服务和 `TenantSelfService` 端口，聚合当前用户、邀请码 CRUD、租户概览、应用消耗和用户排行查询；邀请生成/唯一冲突重试从 Handler 移入应用层，持久化 adapter 仅翻译唯一约束。
- HTTP：租户自助路由只依赖 `TenantSelfService`，邀请码注册 URL 作为 Transport DTO 投影生成；认证、claims 租户范围和错误映射保留在 HTTP 层。
- 回归：新增应用服务冲突重试/非冲突失败/能力缺失测试、Transport 端口注入与租户范围命令测试，以及唯一约束翻译测试。
- 账户查询边界：新增 `billing/ports.AccountQueryReader` 与 `billing/service.AccountQueryService`，余额、充值记录和统计查询统一经过应用读端口；充值筛选参数收敛为 `RechargeRecordsQuery`，分页归一化由 application service 负责。
- 基础设施边界：`billing/pg.AccountRepository` 实现账户查询端口，保留账本余额投影和 SQL JOIN；所有查询改为接收请求 context，不再隐式创建 `context.Background()`。
- HTTP：账户余额、充值记录和账户统计路由只依赖 `AccountQueryReader`，Transport 保留用户类型范围、DTO 时间转换和领域错误映射。
- 管理债务边界：`/api/v1/admin/debts/{owner_type}/{id}` 与 Portal 账户余额复用同一余额投影，统一 debt/service-state 语义并清除 Transport 的 ledger 依赖。
- 租户管理读边界：新增 `AdminTenantReader` 查询投影，管理列表/详情、充值目标校验、终端用户越权校验和 AI identity 共享同一端口；PostgreSQL adapter 负责缺失租户/用户和外键引用错误翻译。
- 登录认证边界：登录账号投影、租户限速维度、租户状态检查、最后登录时间更新和认证审计写入统一通过 `auth/ports.LoginReader` / `AuthAuditRecorder`；登录、MFA、刷新和最近认证 handler 不再构造 `auth/pg.AuthRepository`，凭证查询与审计序列化留在 persistence adapter。`auth/pg` 例外仅剩资料/管理账号路径的唯一性错误兼容。
- 唯一性错误边界：用户名/邮箱唯一约束由 `auth/ports.ErrUsernameTaken` / `ErrEmailTaken` 表达，auth、user、tenant、invite PostgreSQL adapter 在边界处统一翻译；资料、管理员账号、终端用户、租户初始用户和公开注册 handler/service 只使用 `errors.Is`，Transport 不再导入 `auth/pg`。
- 公开邀请边界：邀请码缺失/不可用、格式、注册和公开邀请投影统一通过 `invite/ports.PublicService` 与 `invite/ports` 错误契约注入；Transport 不再依赖 `invite` 实现包或 `invite/pg`，邀请查询、注册事务和法律接受记录留在 InviteService/adapter。
- 系统仪表盘查询边界：异常结算告警、全局统计、消费趋势和资源统计统一通过 `system/ports.AdminDashboardReader` 注入；Transport 只负责时间窗口、筛选参数和 DTO 投影，`SystemRepository` 负责 SQL、金额转换和扫描。
- 账务/支付依赖清理：复核确认 Transport 的账户查询、充值、支付订单、回调、退款、提现和租户现金路径均已通过 `billing/service`、`billing/ports` 或 `payment/service` 间接调用，未再直接导入 `billing/pg` / `payment/pg`；删除两条过期依赖例外，后续重点转向基础设施字段从根容器下沉。
- 认证限速基础设施边界：登录、激活、MFA、MFA 确认和最近认证限速器统一由 composition root 通过 `auth.RateLimiters` 构造并注入；Transport 不再导入 `pgxpool`/Redis，也不再暴露 `InfrastructureDeps.Pool/Redis`，未装配时使用 fail-closed 的空客户端限速器。
- 生命周期回归：`BanReconciler` 的 Start/Stop 现在可重复调用且等待后台 reconcile loop 退出，避免 shutdown stack 重复关闭导致 panic。
- AI worker 生命周期：`riskcontrol.Worker` 由 `aiModules` 持有独立 worker context，支持幂等 Start/Stop、停止后不再启动，并在 Stop 时等待 moderation worker 退出；空装配 `Start(nil)` 保持兼容。
- 审计 worker 生命周期：`audit.Worker` 使用独立 worker context，Stop 会取消轮询并等待当前 delivery 完成 Complete 或 Retry，避免 shutdown 时遗留已领取但未处理的 inbox lease。
- OAuth 刷新 worker 生命周期：`tokenrefresh.Refresher` 使用独立 worker context，Stop 会取消刷新请求并等待当前 provider 调用与凭证持久化完成；重复 Start/Stop 和停止后启动均安全。
- 授权模型基础：新增 `auth.Actor`、`auth.Capability` 和 Transport `requireCapability`，统一表达角色与 tenant scope；租户自助/品牌读写已改用 capability，保留 `requireUserType` 兼容未迁移路由。
- 管理授权迁移：平台管理员/超级管理员路由改用 `CapabilityPlatformAdmin` / `CapabilitySuperAdmin`，管理财务的跨角色入口使用 `requireAnyCapability`；原有 userType 允许集合未改变。
- 资源 ownership 迁移：新增 `Actor.CanAccessTenant` / `CanAccessUser`，终端用户管理、租户详情、管理充值目标和用户/租户支付订单访问统一按 actor tenant scope 校验；跨租户和客户越权由领域 helper 测试覆盖。
- 账户授权迁移：余额、充值记录和账户统计路由改用 capability 组合；租户/终端用户范围覆盖通过 `Actor` 的 tenant/user scope 计算，管理员仍保留显式账户查询能力。
- 公告/通知授权迁移：公告收件箱、已读操作和通知列表使用全角色 capability，平台公告管理/通知发送使用 `CapabilityPlatformAdmin`，租户公告管理使用 `CapabilityTenantSelf`；handler 通过 `Actor` 组装领域 principal，保留 service 的收件人和发布者 ownership 校验。
- 自助入口授权迁移：模块状态、租户现金和在线支付入口已改用 `CapabilityPlatformAdmin`、`CapabilityTenantSelf`、`CapabilityCustomerSelf`；支付场景选择也由 actor capability 决定，未授权角色不会进入下单/查单流程。
- 授权遗留清理：Transport 已删除未使用的 `requireUserType` 兼容 middleware；认证资料、MFA 注册/确认和租户状态检查改用 actor capability/tenant-scope helper，JWT userType 数值只保留为 claims 数据完整性校验。
- 授权遗留清理：管理反向充值也改用 `CapabilityTenantSelf` 选择 scoped/unscoped command；Transport 中剩余 userType 读取仅用于持久化投影和 claims 完整性校验，不再承担路由授权判断。
- 授权矩阵：以 `contracts/openapi.yaml` 为唯一 operation 输入，`docs/AUTHORIZATION_MATRIX.md` 逐条列出 318 个 operation 的 policy、capability/auth mode 和 ownership；`go run ./cmd/checkauthz` 负责覆盖率、重复 operationId、未分类规则和文档 freshness。
- 结算 outbox 生命周期：`billing/outbox.Consumer` 使用独立 worker context，Stop 会停止 claim 新批次并等待当前数据库事务/批次完成；重复 Run/Stop 和停止后启动均安全。
- 回归：新增账户查询 service 委托/能力缺失测试、Transport 账户范围与 query command 测试；adapter 增加编译期端口断言。
- 验证：`go test ./internal/billing/... ./internal/transport ./cmd/server`、`go test ./internal/transport -run 'TestAccount'`、`bun run ensure:api` 和 `git diff --check` 通过；完整仓库验证在提交前执行。
- 遗留风险：P1-03 仍有部分 handler 保留黑名单/通知等副作用编排，管理财务与公告/清理路径需要继续抽取 command port；数据库 schema 所有权、统一 capability 授权和浏览器 E2E 尚未建立。
- 下一候选项：同步完成 P1-02 的运行角色/生命周期装配项后，进入 P1-05 capability/policy 授权模型。

### P1-03（基线同步，2026-08-25）

- 已完成边界：租户品牌、自助、管理读写和 AI identity；账户余额/充值/债务查询；支付订单、充值、退款、提现与补偿状态机；用户/终端用户管理 repository；认证登录、审计、唯一性错误；公开邀请；系统仪表盘查询。
- 基础设施收敛：Transport 已移除 `auth/pg`、`invite/pg`、`system/pg`、`billing/pg`、`payment/pg`、`pgxpool`、Redis 直接依赖；认证限速器由 composition root 通过 `auth.RateLimiters` 注入。
- 门禁状态：`go run ./cmd/checkdeps` 报告 dependency direction clean，历史例外台账已清空。
- 回归状态：相关 auth、tenant、user、invite、system、billing、payment、transport 和 server 测试，以及 `go vet ./...`、`go build ./...`、`bun run ensure:api` 已通过。
- 剩余范围：Transport 仍需收敛少量黑名单/通知副作用和 legacy 编排；P1-03 的覆盖率门槛尚未定义并接入 CI。

### P1-05（继续，2026-08-26）

- 会话授权边界：access token 的数据库会话校验现在同时绑定当前账号的 `user_type` 和 `tenant_id`；角色降级、角色变更或租户迁移后，旧 token 立即失效，不会继续携带过期 capability 或 tenant scope。
- 刷新语义：refresh token 仍从数据库读取最新账号角色与租户归属，成功轮换后签发新 scope；旧 access token 失效不影响可控的会话恢复路径。
- 回归：新增 `TestRoleAndTenantScopeChangesInvalidateExistingAccessToken`，覆盖平台管理员降为租户、租户范围迁移、旧 token fail-closed、刷新后的 claims 与新 scope；既有跨租户、越权和对象枚举测试继续作为授权矩阵基础。
- 验证：`go test ./internal/auth -count=1`、定向会话回归和 `git diff --check` 通过。
- 遗留范围：仍需补充覆盖所有 Portal operation 的端到端角色降级/对象枚举矩阵。

### P1-05（授权边界，2026-08-26）

- Portal 边界：`PortalCapability` 仅作为菜单过滤和前端路由提示，不再被描述为安全授权来源；路由守卫只负责阻止错误页面跳转。
- 后端边界：新增 Huma middleware 回归，验证 super admin/platform admin 可通过平台 capability，租户、终端用户、未知角色和无 claims 均在 handler 执行前被拒绝。
- 验证：`go test ./internal/transport -run 'TestCapabilityMiddlewareEnforcesServerSideAuthorization|TestIsUserAccessClaims'`、Portal capability/router 测试、`bun run typecheck` 和 `git diff --check` 通过。

### P1-05（统一作用域类型，2026-08-26）

- 类型边界：`auth.UserID`、`auth.TenantID`、`auth.UserType`、`auth.TenantScope` 和 `auth.ResourceOwnership` 统一承载身份、角色、租户范围及资源归属；`NewActor` / `ActorFromClaims` 是唯一的 Transport actor 装配入口。
- ownership 迁移：租户详情、终端用户管理、管理充值和支付订单访问均改用 `Actor.Owns(ResourceOwnership)`；不再在这些授权点分别拼接裸字符串比较。
- fail-closed：未知角色不会被数值转换环节包裹成特权角色；缺少租户或用户的非法资源引用直接拒绝。
- 回归：新增 typed scope、tenant/user ownership、全局管理员访问和 malformed role 不溢出测试；认证与 Transport 全量回归通过。
- 遗留范围：AI runtime 的 API-key `identity.Scope` 仍描述认证入口语义，不与 Portal actor role 混用；后续端到端授权矩阵继续复用本类型边界。

### P1-05（授权矩阵回归，2026-08-26）

- 矩阵：新增 `TestActorAuthorizationMatrix`，逐一验证 super admin、platform admin、tenant、customer、缺失租户和未知角色对四类 capability 及租户/用户/跨租户/非法 ownership reference 的结果。
- 集成覆盖：保留并串联 Transport middleware 的服务端拒绝、JWT 角色/租户变更失效、AI identity 跨租户拒绝、终端用户对象 scope、异步任务对象不可枚举和账务跨租户撤销测试。
- fail-closed：customer 不能把 tenant control-plane resource 当作自己的 ownership；未知角色和不完整资源引用均拒绝。
- 验证：`go test ./internal/auth ./internal/transport -count=1`、`go vet ./internal/auth ./internal/transport` 和 `git diff --check` 通过。

### P1-05（公告授权迁移，2026-08-26）

- 领域边界：公告 `Actor/Principal` 改用共享 `auth.UserID`、`auth.TenantID`、`auth.UserType`，通过 `AuthorizationActor`/`Has` 调用统一 capability policy；数据库中的 `actor_user_type` 和收件人 user type 仍保留为持久化投影。
- 业务策略：公告创建、编辑、管理列表和收件箱 principal 校验不再直接比较裸数字角色；平台管理员、租户用户和终端用户的 capability/scope 语义集中由 `auth.Actor` 决定。
- 回归：新增公告 capability 矩阵，覆盖管理员、租户、终端用户、缺少 tenant scope 和未知角色；公告 service/pg、Transport 全量测试与依赖门禁通过。
- 遗留范围：少量账户/终端用户 DTO 仍使用数据库兼容的 `userType` 字段，后续只清理授权判断，不改历史数据格式。

### P1-05（通知授权迁移，2026-08-26）

- 查询边界：通知列表改为 `ListForActor(ctx, auth.Actor, limit)`，service 内部统一验证 actor capability、用户身份和 tenant scope，再绑定 recipient/user type/tenant 查询参数。
- 防越权：调用方不再能通过裸三元组查询其他用户或租户通知；无用户、未知角色和错误 scope 在 service 层 fail-closed，Transport 将其映射为 403。
- 回归：新增真实 PostgreSQL 通知 scope 测试，覆盖同租户、跨租户、其他用户、全局通知、错误角色和缺失身份；Transport 全量回归通过。
- 遗留范围：通知投递记录中的 `recipient_user_type` 仍是数据库兼容投影，不作为调用方授权输入。

### P1-05（登录策略清理，2026-08-26）

- 认证策略：登录租户状态门禁、管理员 MFA 分支和认证审计 principal 分类改用 `auth.Actor.RequiresTenantScope` / `CapabilityPlatformAdmin`，移除 Transport 中最后几处基于数值范围的安全判断。
- 回归：新增登录角色策略矩阵，覆盖四类合法角色、未知角色、租户 scope、管理员审计分类和 MFA 分支；Transport 全量测试与 `go vet` 通过。
- 结论：P1-05 capability/policy 授权迁移项完成；剩余 `userType` 仅为数据库兼容投影、响应展示或 claims 数据完整性校验。

### P1-03（Transport 覆盖率门禁，2026-08-26）

- 基线：`go test ./internal/transport -covermode=atomic` 当前覆盖率为 10.4%，未再把“测试命令成功”误当成关键路径覆盖证明。
- 门禁：新增 `scripts/check_transport_coverage.sh`，解析真实 `go test` 覆盖率并以 10.0% 作为可执行的非回退下限；`TRANSPORT_COVERAGE_MIN` 可在后续测试补齐后直接抬高。
- 接入：Makefile 新增 `check-transport-coverage`，GitHub Actions 后端 job 在完整 Go 测试后执行同一脚本，避免本地与 CI 使用不同口径。
- 验证：脚本、Make target、CI YAML 语法和 Transport 测试通过；`go tool cover` 在当前受限环境的缓存读取异常不影响门禁，因为脚本只读取 `go test` 的权威汇总输出。

### P1-04（Serving 候选选择拆分，2026-08-26）

- 文件边界：`execute_candidates.go` 独立承载 sticky affinity、group/priority/conversion 分层、动态 scorer、OAuth pool credential 选择、健康探针和 physical target exhaustion；`execute.go` 保留执行编排与共享契约。
- 行为：未改变候选排序、sticky 首选、credential fallback、健康 lease 或失败耗尽语义。
- 验证：`go test ./internal/ai/serving -count=1` 通过（受限环境需允许既有 `httptest` 本地监听）。
- 遗留范围：上游 attempt/transport、sync/stream relay 和 image response 仍待移出主文件。

### P1-04（Serving upstream attempt 拆分，2026-08-26）

- 文件边界：`execute_attempt.go` 独立承载 client runtime/HTTP transport invocation、deadline controller、concurrency slot、结果分类、credential swap、health/sticky 结果处理和 failover decision；主文件不再持有单次 upstream attempt 实现。
- 行为：保持 timeout cause、2xx precommit/postcommit、credential invalidate/cooldown、direct account invalidation、inflight/latency 统计和 slot release 语义不变。
- 验证：`go test ./internal/ai/serving -count=1` 通过（受限环境需允许既有 `httptest` 本地监听）。
- 遗留范围：sync/stream protocol relay、image response 和低层 SSE helper 仍待独立文件化。

### P1-04（Serving sync/stream relay 拆分，2026-08-26）

- 文件边界：`execute_stream.go` 承载 sync passthrough、stream passthrough、cross-protocol sync/stream conversion、response body reader、stream commit/failure 和 precommit/postcommit 错误处理；主文件保留执行主循环、候选/attempt 连接和图片入口。
- 行为：保持首帧提交、空流 failover、SSE preamble、idle/max-duration timeout、usage/audit 提取、PII restore、client disconnect 和协议错误帧语义不变。
- 验证：`go test ./internal/ai/serving -count=1`、`go vet ./internal/ai/serving` 通过（受限环境需允许既有 `httptest` 本地监听）。
- 遗留范围：图片 response relay、图片 client stream commit 与低层通用 SSE/egress helper 仍待独立文件化。

### P1-04（Serving image relay 拆分，2026-08-26）

- 文件边界：`execute_image.go` 承载图片上游 body 聚合、provider→client 转换、OpenAI 图片响应归一化、同步 JSON 提交和 client-facing SSE 提交；主文件不再承载图片 response 状态机。
- 行为：保持 upstream SSE 聚合、200 error-body failover、图片 usage/audit、PII/egress sanitize、URL/Base64 normalize、stream first-byte 和写失败语义不变。
- 验证：`go test ./internal/ai/serving -count=1`、`go vet ./internal/ai/serving` 通过（受限环境需允许既有 `httptest` 本地监听）。
- 结果：`execute.go` 已从 2,350 行降至 818 行；候选、attempt、stream 和 image 四类职责均有独立文件，后续仅需大型 PostgreSQL Repository 与 staticcheck/legacy 清理。

### P1-04（Commercial PostgreSQL adapter 拆分，2026-08-26）

- 文件边界：将 1,444 行 `commercial_repo.go` 拆为 `commercial_groups.go`、`commercial_targets.go`、`commercial_dispatch.go`、`commercial_bindings.go` 和 `commercial_helpers.go`；每个文件按聚合/use case 承载同一 `CommercialRepo` 的方法，避免引入包装对象或改变注入类型。
- 行为：保留 group/target/dispatch/binding/limit/routing 的 SQL、事务、错误翻译和 `commercial.Repository` 编译期契约；仅调整文件边界与 import 依赖。
- 验证：PostgreSQL adapter 编译、commercial/dispatch/target/price/usage 相关集成测试和 `go vet` 通过；既有数据库测试未改变。
- 结果：主文件降至构造/根契约，最大业务文件约 429 行，后续继续处理 staticcheck 报告和 legacy Console bridge。

### P1-04（PostgreSQL staticcheck 清理，2026-08-26）

- 工具链：安装 staticcheck 2026.2.1（v0.8.1）以匹配当前 Go 1.27 export data；旧版工具的 export-data version 2 不再作为诊断依据。
- 清理：删除 PostgreSQL adapter 中已无调用的转换/helper 函数，修复 `pricebook_repo` 无效赋值、重复 upstream import 和 subscription 参数转换诊断。
- 验证：`staticcheck ./internal/ai/adapters/postgres` 零诊断，adapter 编译、关键回归测试、`go vet` 和 `checkdeps` 通过。
- 遗留范围：其他 internal 包仍有 staticcheck U1000/SA/S1016/ST 诊断；旧 Console handler/legacy bridge 将单独清理，避免把行为变更与工具修复混在一起。

### P1-04（Console legacy cleanup，2026-08-26）

- 清理：删除未在当前路由表注册的 Console chat models/list/create/get/delete handlers、旧会话 DTO，以及未注册的 image job list/legacy DTO bridge helper；移除随之失效的空错误和 DTO helper 文件。
- 边界：保留 `console.go` 当前 `/runtime/v1` 注册的模型、任务、资产和流式消息路由，以及仍被运行时兼容层使用的 bridge helper，未改变对外路由契约。
- 验证：`staticcheck ./internal/ai/console` 零诊断；`go test ./internal/ai/console -count=1` 与 `go vet ./internal/ai/console` 通过。

### P1-04（Serving candidate control-flow cleanup，2026-08-26）

- 修复：删除 `ExecuteStep.pickCandidate` 中每次都会立即返回的无条件循环，消除 staticcheck SA4004，同时保持 sticky、分层候选和动态评分的选择顺序不变。
- 验证：`go test ./internal/ai/serving -count=1` 与 `go vet ./internal/ai/serving` 通过；该模块剩余诊断仅为待后续处理的两个 U1000 legacy helper。

### P1-04（Serving legacy helper cleanup，2026-08-26）

- 清理：删除未被调用的 `runtimeSubjectLegacyOwnerType` 和 `MultiDimScorer.pickMultiDim`；保留当前 `RuntimeSubject`、`PickWithScore` 及其实际调用链，不改变运行时授权或候选评分行为。
- 验证：`staticcheck ./internal/ai/serving` 零诊断，`go vet ./internal/ai/serving` 和 `go test ./internal/ai/serving -count=1` 通过。

### P1-04（Bridge image helper cleanup，2026-08-26）

- 清理：删除未被注册 bridge 调用的 `convertImageResponse`、`encodeJSONSSE` 和三个整数转换 helper；将 Gemini 图片聚合中的机械 append 循环改为切片追加，保持图片顺序与去重逻辑不变。
- 验证：`staticcheck ./internal/ai/adapters/bridgefmt` 零诊断，`go vet ./internal/ai/adapters/bridgefmt` 和 `go test ./internal/ai/adapters/bridgefmt -count=1` 通过。

### P1-04（Billing control legacy validation cleanup，2026-08-26）

- 清理：删除从未接入价格簿写入或同步流程的 `maxMultiplier` 与 `validateMultiplier`；保留实际使用的价格、token tier 和分辨率校验，避免维护一套不会生效的倍率上限。
- 验证：`staticcheck ./internal/ai/billingcontrol` 零诊断，`go vet ./internal/ai/billingcontrol` 和 `go test ./internal/ai/billingcontrol -count=1` 通过。

### P1-04（Client runtime error wording cleanup，2026-08-26）

- 修复：将 Antigravity/Gemini client runtime 中以产品名开头的内部 error 文本改为小写开头，符合 Go error 约定；不改变 provider revision、请求 envelope 或协议字段。
- 验证：`staticcheck ./internal/ai/clientruntime` 零诊断，`go vet ./internal/ai/clientruntime` 和 `go test ./internal/ai/clientruntime -count=1` 通过。

### P1-04（Runtime resolver test import cleanup，2026-08-26）

- 清理：合并 `resolver_test.go` 对同一 `core/upstream` 包的重复导入，统一使用 `coreupstream` 别名；测试行为与 runtime binding stub 保持不变。
- 验证：`staticcheck ./internal/ai/core/runtime` 零诊断，`go vet ./internal/ai/core/runtime` 和 `go test ./internal/ai/core/runtime -count=1` 通过。

### P1-04（Domain bridge helper cleanup，2026-08-26）

- 清理：删除 `core_bridge.go` 中未被 domain 包或其他包调用的 `int32PtrToIntPtr`；Transport 与 PostgreSQL 各自实际使用的同名转换保持不变。
- 验证：`staticcheck ./internal/ai/domain` 零诊断，`go vet ./internal/ai/domain` 和 `go test ./internal/ai/domain -count=1` 通过。

### P1-04（Formats legacy helper cleanup，2026-08-26）

- 清理：删除未被任何格式转换路径调用的 `openaiEffortToGeminiLevel` 与 `sortedKeys`，同时移除随之失效的排序 import；保留当前 OpenAI、Claude、Gemini reasoning effort 映射链。
- 验证：`staticcheck ./internal/ai/formats` 零诊断，`go vet ./internal/ai/formats` 和 `go test ./internal/ai/formats -count=1` 通过。

### P1-04（Gateway image validation assignment cleanup，2026-08-26）

- 修复：在 Gemini 图片数量校验中直接对配置节点做类型断言，避免先读取后立即覆盖的无效 `ok` 赋值；缺失或非对象配置仍按原逻辑跳过。
- 验证：gateway 测试、`go vet` 通过，staticcheck SA4006 已消除；该包剩余两个独立的 U1000/S1016 诊断待后续处理。

### P1-04（Gateway OpenAPI test helper cleanup，2026-08-26）

- 清理：删除 `openapi_test.go` 中没有任何调用方的 `assertSchemaProperties` 测试 helper；保留当前实际使用的 schema enum 断言。
- 验证：gateway 测试、`go vet` 通过，staticcheck U1000 已消除；该包仅剩 `tasks_http.go` 的 S1016 诊断待后续处理。

### P1-04（Gateway task caller conversion cleanup，2026-08-26）

- 修复：将 `RuntimeAuth` 直接转换为同构的 `taskCaller`，消除手工结构体复制和 staticcheck S1016；任务认证上下文及 subject 传递保持不变。
- 验证：`staticcheck ./internal/ai/gateway` 零诊断，`go vet ./internal/ai/gateway` 和 `go test ./internal/ai/gateway -count=1` 通过。

### P1-04（Transport auth legacy cleanup，2026-08-26）

- 清理：删除没有任何路由注册或调用方的 `platformOrTenantUserAuth`；保留平台、租户和终端用户各自实际使用的认证 middleware。
- 验证：Transport 测试、`go vet` 通过，`platformOrTenantUserAuth` 的 U1000 已消除；其余 transport staticcheck 诊断继续按聚合分批处理。

### P1-04（Transport groups legacy DTO cleanup，2026-08-26）

- 清理：删除 `groups.go` 中没有注册入口或调用方的 linked-group 查询 DTO，以及未接入当前 API 的 provider family 双向转换 helper；现有分组、目标和 dispatch DTO/路由保持不变。
- 验证：Transport 测试、`go vet` 通过；该文件的五个 U1000 已消除，剩余 transport 诊断集中在 self limit/subscription/tenant group/usage helper 和一处切片追加建议。

### P1-04（Transport self limit DTO cleanup，2026-08-26）

- 清理：删除未被任何注册 handler 使用的 `selfAPIKeyLimitInput`；保留实际用于租户和终端用户 API key 限额写入的 `selfUpsertAPIKeyLimitInput`。
- 验证：Transport 测试、`go vet` 通过，`selfAPIKeyLimitInput` 的 U1000 已消除；剩余 transport 诊断继续按 subscriptions、tenant groups、usage helper 分批处理。

### P1-04（Transport subscription output cleanup，2026-08-26）

- 清理：删除没有任何 handler 返回或测试引用的 `subscriptionOutput`，保留分页列表和可空订阅响应使用的专用输出类型。
- 验证：Transport 测试、`go vet` 通过，`subscriptionOutput` 的 U1000 已消除；剩余 transport 诊断集中在 tenant group/usage helper 和一处切片追加建议。

### P1-04（Transport tenant group pricing helper cleanup，2026-08-26）

- 清理：删除没有调用方的 `decodeUSDResolutionsInto` 及其唯一依赖的 JSON import；保留当前生效价格 DTO 的排序与倍率计算路径。
- 验证：Transport 测试、`go vet` 通过，`decodeUSDResolutionsInto` 的 U1000 已消除；剩余 transport 诊断集中在 identity enrichment 和一处切片追加建议。

### P1-04（Transport subscription identity helper cleanup，2026-08-26）

- 清理：删除没有任何订阅路由调用的 `buildIdentityIncludedForSubPlans`；保留实际用于订阅列表和订单列表的 identity enrichment，避免维护未注册的套餐路径。
- 验证：Transport 测试、`go vet` 通过，`buildIdentityIncludedForSubPlans` 的 U1000 已消除；当前 transport 仅剩 `user_self.go` 的切片追加建议。

### P1-04（Transport user model list append cleanup，2026-08-26）

- 修复：将用户可用模型列表中逐项追加的机械循环改为一次切片追加，保留空列表为非 nil 空切片的响应语义。
- 验证：`staticcheck ./internal/ai/transport` 零诊断，`go vet ./internal/ai/transport` 和 `go test ./internal/ai/transport -count=1` 通过。

### P1-04（WeChat config SQL index cleanup，2026-08-26）

- 修复：删除微信配置更新 SQL 中最后一个可选字段之后不会再被读取的 `nextIdx++`，保持前序可选字段的参数编号逻辑不变。
- 验证：微信支付模块测试、`go vet` 通过，config.go 的 SA4006 已消除；gateway.go 的独立赋值诊断待后续处理。

### P1-04（WeChat gateway error propagation cleanup，2026-08-26）

- 修复：避免公钥模式下 `core.NewClient` 的错误被同名短变量遮蔽；现在统一检查外层 `err`，初始化失败会正确返回而不是继续缓存无效 client。
- 验证：`staticcheck ./internal/payment/wechat` 零诊断，微信支付模块测试、`go vet` 和差异检查通过。

### P1-04（Tenant repository query index cleanup，2026-08-26）

- 修复：删除租户概览行为查询在最后一个可选 `time_to` 参数之后不会再使用的 `argIdx++`，保持动态 SQL 占位符和参数顺序不变。
- 验证：`staticcheck ./internal/tenant/pg` 零诊断，tenant/pg 测试、`go vet` 和差异检查通过。

### P1-04（Transport auth/log helper cleanup，2026-08-26）

- 清理：删除未被调用的 `isAdminClaims` 与 `claimsLogFields`，保留实际使用的 actor 解析和 principal 日志字段构建路径。
- 验证：`staticcheck ./internal/transport` 零诊断，Transport 测试、`go vet` 和差异检查通过；全仓 `internal/...` staticcheck 已清零。

### P1-04（Schema checker chain validation cleanup，2026-08-26）

- 修复：将迁移链首项的重复 `previous = migration.From` 赋值改为仅从第二项开始校验来源版本，保持链断裂/重叠错误判断和最终版本校验不变。
- 验证：`staticcheck ./cmd/checkschema` 零诊断，checkschema 测试、`go vet` 和差异检查通过；全仓 staticcheck 仅剩 `cmd/server/ai_modules_test.go` 的 4 处 nil context 建议。

### P1-04（Server lifecycle test context cleanup，2026-08-26）

- 修复：生命周期幂等测试使用 `context.Background()` 替代 nil context，明确传入合法上下文，不改变 `Start`/`Stop` 双调用的测试意图。
- 验证：`staticcheck ./cmd/server` 零诊断，cmd/server 测试、`go vet` 和全仓 `staticcheck ./...` 通过。

### P1-07（Database role provisioning，2026-08-27）

- 工具：新增 `deploy/production/provision_db_roles.sh`，支持只读 `preflight` 与显式 `DB_ROLE_PROVISION_CONFIRM=APPLY` 的 `apply`；从 secret-manager 环境读取两组密码，不把密码放入命令行参数或发布附件。
- 安全：创建/轮换 `dai` 与 `dai_billing` LOGIN 角色时固定为 NOINHERIT、NOSUPERUSER、NOCREATEDB、NOCREATEROLE、NOREPLICATION、NOBYPASSRLS，并拒绝占位符、空白字符、少于 32 个字符、相同密码和既有角色成员关系；脚本只授予数据库 CONNECT，不提前授予表权限。
- 接入：生产数据库发布附件、Make 检查目标和 CI 确认门禁均已接入；ownership/revoke 仍由后续维护窗口切换步骤执行。
- 验证：脚本 `bash -n`、帮助入口、Make `check-db-role-provision` 通过。

### P1-07（Database ownership maintenance cutover，2026-08-27）

- 工具：新增 `deploy/production/cutover_db_ownership.sh`，提供 `preflight/apply/verify` 三阶段；`apply` 同时要求 `DB_OWNERSHIP_CUTOVER_CONFIRM=APPLY` 与 `DB_OWNERSHIP_CUTOVER_WINDOW=OPEN`，并在调用 ownership SQL 前再次确认无其他数据库 client session。
- 校验：切换前验证 admin/runtime/billing 三个 DSN 的实际角色和目标数据库一致，角色为 LOGIN/NOINHERIT 最小权限且无成员关系；切换后验证 runtime 投影视图读取、outbox INSERT 和 billing 账本写权限，支持可选 readiness URL。
- 接入：发布附件、Make、CI 和 `docs/DATABASE_OWNERSHIP.md` 已改为优先使用 wrapper；应用停止、单实例启动和 readiness 观察顺序固化在维护窗口手册中。
- 验证：wrapper/ownership 脚本 `bash -n`、帮助入口、确认门禁和差异检查通过。

### P1-08（Unified invariant suite completion，2026-08-27）

- 契约：生命周期测试的每个健康阶段现在都断言 `billing/invariants.Check` 执行完整 7 项检查，避免新增检查函数后测试仍然静默漏跑。
- 覆盖：真实 PostgreSQL 流程继续覆盖余额/批次守恒、批次状态、充值撤销、Outbox/用量链接、退款冲正、订阅订单和订阅额度边界；随机并发、幂等、Scheduler 对账和 repair audit 已由同一阶段的既有测试覆盖。
- 验证：`go test ./internal/billing/invariants -count=1`、`go vet ./internal/billing/invariants`、`staticcheck ./internal/billing/invariants` 通过。

### P1-02（Data cleanup worker lifecycle，2026-08-27）

- 生命周期：`cleanup.Service` 现在使用独立 worker context，`Start`/`Stop` 幂等，Stop 会取消自动清理并等待数据库 worker 退出；Stop 先于 Start 时不会再启动后台任务。
- 装配：`cmd/server/run` 将 data cleanup 注册到 shutdown stack，保证它在 PostgreSQL 连接池释放前退出。
- 验证：新增 cleanup lifecycle 单元测试；`go test ./internal/cleanup -count=1`、`go vet ./internal/cleanup`、`staticcheck ./internal/cleanup` 和 cmd/server 生命周期定向测试通过。

### P1-02（Hourly cleanup worker lifecycle，2026-08-27）

- 生命周期：新增 `periodicWorker`，为图片、文件、认证会话和激活凭证四个小时级任务提供独立可取消 context、幂等 Stop 和退出等待；清理函数接收 worker context，停止时可中断正在进行的数据库/文件操作。
- 装配：main 不再直接启动裸 goroutine，四个 worker 的 Stop 函数均登记到 shutdown stack，且在资源释放前按逆序等待完成。
- 验证：新增首次执行、重复 Stop 和 in-flight cleanup 超时/再次等待测试；cmd/server 测试、`go vet`、`staticcheck` 和差异检查通过。

### P1-02（Unified lifecycle health projection，2026-08-27）

- 投影：新增 composition-root `lifecycleHealth` 注册表，统一记录 PostgreSQL（含 billing 逻辑池）、Redis、平台模块、Ban reconciler、Scheduler、AI 模块、异步任务、data cleanup、四个小时级清理 worker 和公共/管理监听器的 started/stopped 状态。
- 接口：`/health` 新增 `components` 对象并保留原有 `scheduler` 详细快照；组件状态不包含 DSN、Provider 或错误内部细节，`/ready` 继续执行 PostgreSQL/Redis 真实连通性探活。
- 装配：所有资源在成功启动后登记，关闭回调完成后标记 stopped；小时级 worker 通过统一注册函数接入 shutdown stack，超时关闭不会被误报为已停止。
- 回归：新增生命周期快照隔离、幂等标记和 health JSON 兼容性测试；`go test ./cmd/server -count=1`、`go vet ./cmd/server` 和差异检查通过。

### P1-02（LiteLLM refresh lifecycle，2026-08-27）

- 生命周期：`liteLLMPriceSource` 现在拥有独立的可取消刷新上下文、启动/停止状态和 in-flight refresh 等待通道；重复 Stop 安全，停止后不会重新触发远程刷新。
- 装配：`billingcontrol.Service.Stop` 暴露最小关闭入口，`aiModules.Stop` 在释放其他 AI worker 时同步取消并等待 LiteLLM 价格刷新，避免根 context 取消后遗留网络 goroutine。
- 回归：新增刷新取消、停止后禁止重启以及首个 Stop 超时后可用更长上下文再次等待的测试；`go test ./internal/ai/billingcontrol -count=1`、`go test ./cmd/server -count=1`、`go vet` 和差异检查通过。

### P1-02（HTTP listener lifecycle，2026-08-27）

- 生命周期：`httpServers` 增加幂等 Start、关闭状态和公共/管理监听 goroutine 的完成通道；`Shutdown` 在调用标准库关闭后继续等待两个监听循环退出，并支持超时后再次用更长上下文等待。
- 故障：监听启动失败仍触发根 context 取消，监听 goroutine 无论正常关闭还是异常返回都会关闭完成通道，避免启动失败路径遗留不可观测 goroutine。
- 回归：增加双监听器重复 Start、Shutdown 等待和重复 Shutdown 测试；`go test ./cmd/server -count=1`、HTTP 生命周期 race 测试、`go vet` 和差异检查通过。

### P1-02（Platform transport module registration，2026-08-27）

- 装配：新增 `platformModule` 实现 `transport.Module`，平台元数据、认证、身份、账务和运营路由与 AI 纵向模块统一由模块列表注册；保留 `transport.Register` 入口和既有 operation/path 契约。
- 边界：本次只改变注册拓扑，不伪装完成依赖缩减；平台模块暂时接收已经按 Infrastructure/Portal/Identity/Billing/Operations 分组的 `Deps`，后续再逐组提取最小 HTTP 端口。
- 回归：新增平台模块 OpenAPI surface contract，确认认证、租户、支付和运营代表路径均由模块注册；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（Operations transport module split，2026-08-27）

- 依赖：公告、通知、系统模块、数据清理和代理节点路由改为各自接收服务实例与平台认证依赖，不再从完整 `transport.Deps` 读取运营服务。
- 装配：新增 `platformOperationsModule`，由统一 `transport.Module` 列表注册五类运营路由；平台主模块继续承载尚未拆出的管理聚合，既有 operation/path 保持不变。
- 回归：扩展 OpenAPI module surface contract，确认运营代表路径仍被注册；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（Billing payment transport module split，2026-08-27）

- 依赖：在线充值、租户额度、管理支付和微信回调路由改为显式接收支付服务与平台认证/日志依赖，不再通过完整 `transport.Deps` 构造支付 handler。
- 装配：新增 `platformBillingModule`，由统一 `transport.Module` 列表注册支付相关 Huma 路由；原生微信回调继续保留在 `RegisterRaw`，仅复用同一窄支付依赖。
- 回归：OpenAPI module surface contract 增加支付代表路径，确认注册顺序和对外 operation/path 不变；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（Identity self-service transport module split，2026-08-27）

- 依赖：账户余额/充值查询、租户自助、门户品牌和公开邀请路由改为显式接收查询端口、租户自助/品牌端口、公开邀请端口及平台认证/法律配置，不再从完整 `transport.Deps` 读取无关服务。
- 装配：新增 `platformIdentityModule`，与计费/运营模块并列注册；原生公开 favicon 路由复用窄品牌依赖，认证与管理员聚合路由暂留平台主模块。
- 回归：OpenAPI module surface contract 增加身份自助和公开代表路径，确认既有 operation/path 不变；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（Authentication transport module split，2026-08-27）

- 依赖：登录、刷新、激活、MFA、近期认证、当前用户、登出和改密路由改为显式接收认证服务、账号读写/审计端口、限速器、Cookie 策略和日志，不再通过完整 `transport.Deps` 读取身份/运营聚合。
- 装配：`platformIdentityModule` 内新增 `authModule` 注册认证公开与受保护端点；管理员账号、JWT key 管理和其他管理聚合继续留在平台主模块，避免本次切片扩大状态机范围。
- 回归：现有认证 cookie、MFA、限速、账号投影和端口契约测试继续通过；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（JWT key transport module split，2026-08-27）

- 依赖：JWT key 列表与轮换路由改为仅接收 JWT 服务和黑名单认证依赖，不再从完整 `transport.Deps` 读取平台其他服务。
- 装配：`jwtKeysModule` 并入 `platformIdentityModule`，保持超级管理员 capability 和既有 operation/path 不变。
- 回归：OpenAPI module surface contract 增加 JWT key 代表路径；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（Platform admin transport module split，2026-08-27）

- 依赖：管理员租户、系统/租户账号、终端用户、充值/退款、仪表盘和批量用量退款路由改为显式管理员端口、账务 application service、激活凭证、黑名单和近期认证依赖。
- 装配：新增 `platformAdminModule` 并从平台主模块移除六组管理员注册调用；`adminHandlers` 共享的状态/ownership 编排保持不变，后续可继续按 use case 拆成更小 command/query 端口。
- 回归：OpenAPI module surface contract 增加系统管理员代表路径，既有管理员授权和数据库集成测试继续复用；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（AI platform dependency narrowing，2026-08-27）

- 依赖：AI 路由模块与全部 HTTP 依赖构造函数改用 `aiPlatformDeps`，仅接收 JWT/黑名单、租户读取和用户投影；不再把完整 `transport.Deps` 保存到 AI 模块。
- 边界：AI Transport 仍可通过显式构造函数取得所需平台认证与 identity provider，平台账务、运营和管理服务不会进入 AI 路由依赖容器。
- 回归：更新 AI HTTP 依赖 wiring 测试，覆盖全部纵向模块的认证端口投影；`go test ./internal/transport -count=1`、`go test ./cmd/server -count=1`、`go vet` 和差异检查通过。

### P1-02（Minimal metadata transport module，2026-08-27）

- 依赖：服务信息与 JWKS 公钥端点改由 `metaModule` 注册，仅接收版本字符串和 JWT 服务，不再把完整平台 `Deps` 传给元数据路由。
- 装配：移除仅承载元数据的空壳 `platformModule`，平台 identity/admin/billing/operations/AI 模块按职责并列注册。
- 回归：OpenAPI module surface contract 增加服务信息路径并确认 JWKS/平台代表路由仍存在；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（Raw transport dependency narrowing，2026-08-27）

- 依赖：chi 原生微信回调与公开 favicon 路由改用独立 `RawDeps`，分别只接收支付服务/日志和品牌读端口；`RegisterRaw` 不再传递完整平台 `Deps`。
- 契约：保留签名验签所需的原始 request body、微信回调响应和公开 favicon 路径，不将 raw 路由错误地迁回 Huma JSON 注册链。
- 回归：新增 raw route pattern contract，确认两条非 JSON 路由仍被注册；`go test ./internal/transport -count=1`、`go vet` 和差异检查通过。

### P1-02（AI identity user port，2026-08-27）

- 依赖：AI 身份适配器改为接收 `user/ports.IdentityUserReader`，用户域只向跨域补全暴露非敏感身份投影，不再把具体 `UserService` 或 PostgreSQL 用户模型带入 Transport。
- 装配：`UserService` 通过 `BatchGetIdentityUsers` 实现用户身份端口，`transport.IdentityDeps` / `aiPlatformDeps` 仅保存该端口；原有租户归属校验和 AI identity provider 契约保持不变。
- 回归：新增用户投影映射与错误传播测试；`go test ./internal/user ./internal/transport ./cmd/server -count=1`、`go vet` 和差异检查通过。

### P1-02（Platform admin child modules，2026-08-27）

- 依赖：管理员租户、账号、终端用户、财务、用量/账单和仪表盘路由分别接收六组显式模块依赖，不再以一个包含全部管理员能力的 `adminModule` 作为注册契约。
- 装配：`platformAdminModule` 仅编排六个子模块；共享 `adminHandlers` 只由各子模块按需投影字段，认证、近期认证和黑名单能力保持统一但不携带无关业务服务。
- 回归：既有管理员 OpenAPI surface、授权和数据库集成测试继续覆盖原 operation/path；`go test ./internal/transport ./cmd/server -count=1`、`go vet` 和差异检查通过。

### P1-02（Manual cleanup lifecycle fencing，2026-08-27）

- 生命周期：`cleanup.Service.StartManual` 的脱离请求执行现在挂接服务级可取消 context，并登记到 manual run wait group；`Stop` 会同时取消自动 worker 与手动执行，避免关闭数据库后仍有清理 goroutine 访问依赖。
- 并发：手动 run 入队使用可取消 context，停止竞态不会留下未被等待的后台执行；服务停止后拒绝新的手动清理请求。
- 回归：新增手动执行取消、Stop 等待和停止后拒绝测试；`go test ./internal/cleanup -count=1`、`go vet` 和差异检查通过。

### P1-02（Platform transport assembly ownership，2026-08-27）

- 装配：从 `aiModules` 移除平台路由依赖字段及构造逻辑，新增 composition-root `buildPlatformTransportModules`，由平台服务分别投影身份、账务和运营路由模块。
- 边界：`aiModules` 只保留 AI HTTP 依赖和运行时 owner，避免 AI 生命周期容器持有平台路由服务；下一步继续删除 `transport.Register` 的兼容聚合入口。
- 回归：新增平台依赖投影的 production/cfg nil 契约测试；`go test ./cmd/server ./internal/transport -count=1`、`go build ./...`、`go vet` 和差异检查通过。

### P1-02（Explicit transport module registration contract，2026-08-27）

- API：删除 `transport.Deps` 聚合类型，`transport.Register` 改为接收显式 `Module` 列表；平台身份、管理员、账务、运营和 AI 模块通过公开构造函数及模块专属依赖类型创建。
- 装配：`cmd/server` 与 `cmd/openapi` 均按固定顺序注册模块，Transport 不再负责从跨域 service locator 组装平台依赖；`AIHTTPDeps` 继续只承载 AI 控制面 HTTP 能力。
- 回归：扩展 OpenAPI module surface contract 覆盖新注册入口；`go test ./internal/transport ./cmd/server ./cmd/openapi -count=1`、`go build ./...`、`go vet` 和 `checkdeps` 通过。

### P1-02（Async task heartbeat lifecycle fencing，2026-08-27）

- 生命周期：异步任务单次执行的租约心跳 goroutine 现在由 `runClaimed` 持有完成信号，任务退出时先取消并等待心跳结束，避免 Engine worker 计数归零后仍访问存储。
- 超时：心跳数据库调用改用继承任务取消的 5 秒超时 context，不再通过 `WithoutCancel` 绕过关闭信号；租约续期失败/丢失的既有停止语义保持不变。
- 回归：异步任务取消、生命周期和心跳相关测试通过；`go test -race ./internal/ai/asynctask -run '^TestEngine(CancelStopsRunningTask|HealthAndStopBeforeStart)$' -count=1`、`go vet` 和差异检查通过（完整包 httptest 受当前本地监听权限限制）。

### P1-02（Runtime auth telemetry cancellation，2026-08-27）

- 生命周期：Runtime API Key 的 `last_used_at` best-effort 更新不再脱离请求 context，后台写入最多运行 2 秒并会随请求/服务器关闭取消，避免残留数据库 goroutine。
- 行为：鉴权成功与下游请求路径保持不变，telemetry 写入失败仍不影响请求响应。
- 回归：`go test ./internal/ai/gateway -count=1`、`go vet ./internal/ai/gateway` 和差异检查通过。

### P1-02（Runtime binding cache lifecycle，2026-08-27）

- 生命周期：`CachedBindingResolver` 为共享 cache-miss 加载增加 Start/Stop/Health，所有 detached load 由 resolver WaitGroup 登记，并在 Stop 时取消、等待后再释放 AI 运行时依赖。
- 装配：`aiModules` 持有并管理 runtime binder 的启动/停止，生产请求不再触发无主的 `WithoutCancel` 配置读取；请求级等待/并发合并语义保持不变。
- 回归：新增 resolver 停止取消、Stop-before-Start 和健康状态测试；`go test ./internal/ai/core/runtime ./cmd/server -count=1`、race 定向测试、`go build`、`go vet` 和 `checkdeps` 通过。

### P1-02（Subscription janitor lifecycle，2026-08-27）

- 生命周期：`subscription.Service` 增加幂等 `Start/Stop/Health`，janitor 由服务自持有 context 和完成通道，Stop 会等待订单过期/卡单补偿循环退出。
- 装配：`aiModules` 统一启动和停止订阅 janitor，购买热路径仍复用同一服务，未改变订单状态机与幂等重放语义。
- 回归：新增 Start/Stop、Stop-before-Start 和重复停止测试；`go test ./internal/ai/subscription ./cmd/server -count=1`、race 定向测试、`go vet` 和差异检查通过。

### P1-02（Async task engine lifecycle，2026-08-27）

- 生命周期：`asynctask.Engine` 现在自持有 worker context，Start/Stop 幂等且 Stop-before-Start 会阻止后续启动；停止时先取消 worker、webhook 和 reaper 循环，再执行租约释放。
- 等待：Stop 支持超时返回并可由后续更长上下文继续等待，成功释放任务/投递后不会重复执行释放 SQL；调用方会记录关闭不完整告警。
- 观测：新增 `HealthSnapshot`，暴露 started/stopped 生命周期状态，不将队列内容或内部错误细节混入健康响应。
- 回归：新增 Stop-before-Start 与 Health 测试；`go test ./internal/ai/asynctask -run '^TestEngineHealthAndStopBeforeStart$'`、`go vet` 和差异检查通过（完整包测试受限于当前环境 httptest IPv6 监听权限）。

### P1-02（Shared worker Health contract，2026-08-27）

- 合约：新增无基础设施依赖的 `internal/lifecycle.Component` / `HealthSnapshot`，Ban reconciler、cleanup、风险审查、审计 inbox、OAuth refresh、结算 outbox、LiteLLM refresh、异步任务和小时级 worker 均提供锁安全的生命周期 Health。
- 语义：Health 只表达 started/stopped，不把队列载荷、DSN、Provider 密钥或内部错误细节暴露给管理探针；真实连通性与失败/积压指标继续由 `/ready` 和各领域指标负责。
- 回归：扩展各 worker 生命周期测试与编译期接口断言，并补充根 health 投影的 async task 状态；相关包测试、`go vet` 和差异检查通过。

### P1-01（Database cross-module write boundary completion，2026-08-27）

- 边界：`cmd/checkdeps` 报告 dependency direction clean；runtime/billing 数据库角色与 ownership/revoke 契约已禁止 runtime 直接写账本，并由 outbox 仅保留结算意图 INSERT 例外。
- 验证：依赖门禁、ownership probe、角色 provisioning 和维护窗口 cutover wrapper 均已接入 Make/CI，P1-01 的数据库写入边界条件完成闭环。

### P1-04（Split behavior regression coverage，2026-08-27）

- 覆盖：Serving 既有候选分层、sticky、attempt failover、stream pre/post-commit、跨协议转换和 image relay 测试继续锁定拆分前的路由与响应语义；新增 `TestCandidateSplitExhaustsAllRoutesForOnePhysicalTarget` 锁定同一物理目标的耗尽规则。
- 覆盖：CommercialRepo 保留 compile-time repository contract，并由 group/target/dispatch/binding、pricebook、group transfer 集成测试覆盖聚合拆分后的 SQL、计费和配置行为。
- 验证：`go test ./internal/ai/serving -count=1`、`go vet ./internal/ai/serving`、`staticcheck ./internal/ai/serving` 通过；相关 CommercialRepo 测试已在拆分提交中验证。

### P1-03（Billing command context propagation，2026-08-27）

- 边界：`DeductionService` 的用量退款、充值撤销和批量退款 command 统一接收调用方 context；Transport 与 PaymentService 不再让账务操作隐式创建 `context.Background()`。
- 取消语义：数据库查询、锁、账本写入、审计和提交沿用同一 context；批量退款检测取消并停止处理剩余请求，查询取消不会再被误映射为“记录不存在”。
- 回归：新增取消 context 的退款、撤销和批量命令测试；定向 billing/payment/transport 编译测试、gofmt 与差异检查通过。

### P1-03（Account security side-effect command，2026-08-27）

- 边界：新增 `auth/ports.AccountSecurityWriter` 与 `AccountSecurityService`，统一承接账号/租户状态同步、token 撤销和用户会话失效；管理账号、终端用户、认证路由不再直接写 Redis 黑名单。
- 上下文：BlacklistService 的读写 API 全部接收调用方 context，HTTP middleware、AI middleware 和账号 security command 不再隐式创建 `context.Background()`。
- 回归：新增 miniredis 覆盖用户/租户 ban 同步、token 撤销与取消语义；auth、AI transport、platform transport/server 定向编译测试通过。

### P1-03（Notification send command，2026-08-27）

- 边界：新增 `notification.Service.Send` application command，统一 `in_app`/`webhook` 分发与 channel 校验；Transport 只组装输入、调用 command 和映射错误。
- 依赖：通知 HTTP 模块改为注入 `notification.HTTPService`，不再暴露具体通知 service 实现。
- 回归：未知 channel 在进入数据库前被拒绝；`notification`、`transport`、`cmd/server` 定向测试、`go vet`、`go build` 和 `checkdeps` 通过。

### P1-03（Manual recharge target transaction boundary，2026-08-27）

- 边界：`ManualRechargeTargetLocker` 改为在账务事务内返回锁定后的 tenant ID；平台管理员的用户充值可省略 `tenantId`，服务在锁定用户/租户后生成最终充值目标。
- HTTP：删除管理充值对 `TenantRepository.GetTenantDetails` / `GetEndUserTenantID` 的锁外预检；租户用户仍由 claims tenant scope 约束，最终目标状态由同一事务再次校验。
- 回归：新增平台用户充值目标解析测试；billing、tenant、transport、server 定向测试、`go vet`、`go build` 和 `checkdeps` 通过。

### P1-02（Runtime Gateway telemetry lifecycle，2026-08-27）

- 生命周期：Runtime Gateway 新增 owner-managed auth telemetry，`Start` 允许请求触发 `last_used_at` 写入，`Stop` 先 fencing、取消 in-flight context，再等待所有写入退出。
- 装配：`aiModules` 将 Gateway 纳入统一 Start/Stop；HTTP listener 关闭后才释放 AI/数据库依赖，重复 Stop 和 Stop 超时后再次等待均安全。
- 回归：新增 telemetry Stop 等待、超时、取消和禁止新写入 race 测试，并将 Runtime Gateway 状态接入 `/health.components`；`go test ./internal/ai/gateway ./cmd/server`、`go test -race`、`go vet`、`go build` 和 `checkdeps` 通过。

### P1-02（Composition transport assembly contract，2026-08-27）

- 装配：新增组合根级 OpenAPI surface contract，直接验证 `buildPlatformTransportModules` 生成的六个显式模块都注册了 metadata、identity、admin、billing、operations 和 AI 路由代表。
- 防漂移：模块数量与单个模块测试之外，契约现在能发现组合根遗漏整组模块或错误注册顺序导致的 surface 缺失。
- 回归：`go test ./cmd/server ./internal/transport`、`go vet`、`go build`、`checkdeps` 和 `git diff --check` 通过。

### P1-02（Console stream persistence lifecycle，2026-08-27）

- 生命周期：Console 流式 assistant message persistence 现在由请求级 owner 的 defer 统一关闭，`sync.Once` 防止正常与异常路径重复收尾；panic、请求取消或提前退出会标记 `interrupted`，正常返回才标记 `completed`。
- 等待：关闭流程先发出 done 信号、等待持久化 goroutine，再使用独立短超时上下文完成最终会话路由更新；数据库池释放前不会遗留该请求的消息写入。
- 回归：新增关闭幂等与中断状态测试；Console/Gateway/server 定向测试、race 测试、全仓 `go test ./...`、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-03（User repository context propagation，2026-08-27）

- 上下文：`UserService` 的用户查询、批量身份投影、资料更新和封禁状态写入现在将调用方 context 传递到 PostgreSQL adapter；取消的 HTTP/AI 请求不会继续使用无界 `context.Background()` 查询。
- 边界：`UserService` 通过最小 repository 接口依赖持久化能力，保持身份投影端口不泄漏 PostgreSQL 实现，同时让取消语义可被单元测试锁定。
- 回归：新增 service context 透传测试；`go test ./internal/user ./internal/user/pg ./internal/transport ./cmd/server -count=1`、race、`go vet` 和差异检查通过。

### P1-03（Invitation repository context propagation，2026-08-27）

- 上下文：邀请码查询、租户品牌读取、列表统计、创建/更新/删除、用户名唯一性检查和邀请注册均接收并使用调用方 context；公开邀请 HTTP 请求取消后不会继续使用无界数据库操作。
- 边界：`InviteService` 的 repository contract 显式携带 context，注册事务继续使用同一 context，未改变邀请码状态校验、密码策略和法律文件记录语义。
- 回归：新增邀请服务读写/注册 context 透传测试；`go test ./internal/invite ./internal/invite/pg ./internal/transport ./cmd/server -count=1`、race、`go vet` 和差异检查通过。

### P1-03（JWT session validation context propagation，2026-08-27）

- 上下文：`JWTService.ParseToken` 接收调用方 context，access token 的实时 session 查询使用同一取消/超时链；平台 Huma、AI Huma 和 Console 原生入口均传递请求 context。
- 契约：更新 `TokenVerifier` 接口及实现，保持 token 签名、claims、黑名单和租户/账号状态校验语义不变，仅改变数据库查询的取消边界。
- 回归：新增真实 PostgreSQL 取消查询测试；auth、transport、AI transport/Console 定向测试与 race、全仓 `go test ./...`、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-03（JWT key management context propagation，2026-08-27）

- 上下文：JWT key 列表查询与轮换事务改为接收管理请求 context；轮换在 context 已取消时先返回，不生成无用 RSA 密钥，提交后的 key reload 继续沿用调用链。
- 边界：`/api/v1/jwt-keys` 的 Transport 只转发 context，未改变超级管理员授权、RS256 key 状态转换和 24 小时 grace 语义。
- 回归：新增真实 PostgreSQL 下 `ListKeys`/`RotateKey` 取消测试；auth/transport 定向测试、race、全仓 `go test ./...`、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-03（Image bridge context propagation，2026-08-27）

- 上下文：`corebridge.RequestEnvelope` 携带请求 context，bridgefmt 在 Gemini→OpenAI image edit 的 multipart materialization 中将其传递给 `imageedit.Encode` 和远程图片下载。
- 行为：请求取消会中止输入图下载并返回取消错误；无请求 envelope 的离线转换仍使用受控默认 context，不改变已有协议、SSRF 和图片格式校验。
- 回归：新增取消请求的远程输入图测试；core bridge、bridgefmt、serving、gateway 定向测试、race、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-03（Usage cache invalidation context propagation，2026-08-27）

- 上下文：usage 事务完成后，API key quota cache invalidation 改为接收 `UsageLogger.Log` 的 completion context；该 context 已由 finalizer 提供独立完成超时，客户端断开语义保持不变。
- 边界：invalidation 仍只对 API key subject 执行，最多运行 2 秒并保留失败告警；JWT/匿名请求不触发缓存失效。
- 回归：新增 completion context 值、取消和 subject 过滤测试；adapter 定向测试、`go vet` 和差异检查通过。

### P1-02（Client catalog discovery lifecycle，2026-08-27）

- 生命周期：`clientcatalog.Service` 为 `singleflight.DoChan` 背后的 provider discovery 增加 owner-managed active load、可取消 context、WaitGroup 和幂等 `Start/Stop/Health`；Stop 会 fencing 新加载并等待既有 selector/inspector 调用退出。
- 语义：请求取消仍返回 stale/fallback 且不破坏共享 discovery；进程停止会取消脱离请求的加载，`aiModules` 在释放 OAuth/数据库依赖前完成 catalog 收尾。
- 回归：新增 Stop 等待、Stop-before-Start、owner context 取消和 catalog 装配生命周期测试；clientcatalog/server 定向测试、race、`go vet`、`go build`、`checkdeps` 和差异检查通过。一次全仓回归仅复现既有 `migration_0019` OID 与支付 sweep 指标测试的环境/时序 flake，均与本改动无关。

### P1-02（Client runtime credential refresh lifecycle，2026-08-27）

- 生命周期：`clientruntime.Runtime` 为 401 触发的 shared credential refresh 增加 owner-managed active refresh、可取消 context、WaitGroup 和幂等 `Start/Stop/Health`；Stop 会 fencing 新刷新并等待既有 refresher 调用退出。
- 语义：同一 credential 的并发刷新仍由 singleflight 合并，请求取消只结束当前 caller，不取消共享刷新；owner context 或 AI 模块 Stop 会取消脱离请求的刷新。
- 回归：新增 refresh Stop 等待和取消测试，并验证 `aiModules` 的 Runtime/ Catalog 停止顺序；clientruntime/clientcatalog/server 定向测试、race、全仓 `go test ./...`、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-03（Admin account tenant transaction boundary，2026-08-27）

- 边界：管理员创建租户用户不再先调用 `TenantReader.GetTenantDetails` 做锁外预检；账号与激活凭证继续由 `AdminAccountRepository` 在同一事务内写入。
- 错误：`iam_accounts.tenant_id` 外键冲突翻译为 `user/ports.ErrTenantNotFound`，系统管理员/终端用户创建入口分别映射为明确的 400 错误；缺失租户不会留下账号或激活令牌。
- 装配：`AdminUsersModuleDeps` 和 `adminUsersModule` 删除不再需要的 `TenantReader` 依赖，Transport 只接收账号读写端口。
- 回归：新增缺失租户原子回滚集成测试；用户/Transport 定向测试、全仓 `go test ./...`（支付 sweep 指标测试出现既有时序 flake）、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-02（AI modules shutdown retry fencing，2026-08-27）

- 生命周期：`aiModules` 移除不可重试的 `stopOnce`，由 owner 状态与停止互斥锁管理 Start/Stop；短超时后再次 Stop 会继续等待所有已登记组件，Stop-before-Start 会阻止后续启动。
- 组件：风险控制 worker 的重复 Stop 现在也会继续等待在途任务，和 catalog/runtime、gateway、订阅、异步任务等组件的可重试等待语义保持一致。
- 回归：新增真实阻塞 credential refresh 的聚合停止竞态测试，并执行 race 定向测试；`cmd/server` 与风险控制 worker 生命周期测试通过。风险控制完整包测试受当前环境 IPv6 `httptest` 监听权限限制。

### P1-02（Scheduler shutdown context propagation，2026-08-27）

- 生命周期：Scheduler 由 composition root 注入 worker context，五类后台循环同时响应父 context 与 Stop；任务的 5 分钟操作超时不再从独立 `context.Background()` 起步。
- 关闭：Scheduler/平台模块的 Stop 支持调用方 deadline，超时后可再次等待；平台 shutdown stack 不再吞掉 scheduler 的关闭错误，Stop-before-Start 会封存平台后台 worker。
- 回归：新增 scheduler Stop-before-Start、短 deadline 重试等待测试；scheduler/cmd-server 定向测试、race、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-02（Ban reconciler shutdown context propagation，2026-08-27）

- 生命周期：BanReconciler 的周期校准循环由平台 worker context 驱动，Redis SCAN/写入和 PostgreSQL 真相查询会在关闭时收到取消信号。
- 关闭：`Stop(ctx)` 支持调用方 deadline 与超时后重试等待；平台模块继续在 scheduler 后停止 reconciler，并汇总关闭错误，避免数据库/Redis 释放前遗留校准 goroutine。
- 回归：新增 Stop-before-Start 和短 deadline 重试测试；auth/cmd-server 定向测试、race、`go vet`、`go build`、`checkdeps` 和差异检查通过。

### P1-03（End-user mutation ownership boundary，2026-08-27）

- 边界：终端用户资料、状态、密码重置和删除 command 均携带 actor 的 tenant scope；非空 scope 在 `AdminEndUserRepository` 的 UPDATE/SELECT/FOR UPDATE 谓词内执行，跨租户目标统一表现为不存在。
- HTTP：移除 `TenantRepository.GetEndUserTenantID` 的锁外归属预检，删除 `AdminEndUsersModuleDeps` 的无关 `TenantReader` 依赖；Transport 只提取 claims scope、调用 writer 和映射结果。
- 一致性：状态/重置/删除与最终写入共享同一 scope 条件，避免检查后租户归属变化或删除竞态导致越权操作；删除仍在余额锁和安全 guard 之后提交。
- 回归：新增 repository 跨租户状态、重置和删除拒绝测试，以及 scope 投影测试；user/pg、Transport、server 定向测试和差异检查通过。
