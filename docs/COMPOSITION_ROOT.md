# Composition root 与生命周期

P1-02 采用可回滚的渐进拆分。当前 `cmd/server` 仍保留单进程 `dai all`，但启动过程已经
开始按职责分层：

```text
config.Load
    ↓
openInfrastructure  ── PostgreSQL / Redis / schema verification
    ↓
platformModules + aiModules
    ↓
transport and HTTP route registration
    ↓
httpServers.Start / Shutdown
```

## 已落地的边界

- `openInfrastructure` 只负责数据库、Redis 连通性和 schema 版本校验，不构造业务模块。
- `run` 返回可描述的启动错误，避免模块构造失败时直接 `logger.Fatal` 终止进程。
- `shutdownStack` 记录已成功构造的资源，按构造逆序关闭，并且重复调用安全；因此部分启动失败也会释放已经拿到的基础设施。
- `httpServers` 独立管理公共业务监听和 loopback 管理监听；公共 AI 流式监听保持 `WriteTimeout=0`，管理监听使用有限超时。
- 异步任务引擎已经登记到生命周期栈；收到退出信号后先取消 worker context，再释放 Redis/PostgreSQL。
- `transport.Deps` 与 AI Core `CoreHTTPDeps` 已按职责收敛；订阅、风控、审计读取、系统、管理仪表盘、管理用量、OAuth 管理、模型绑定、上游诊断、上游账号管理、上游访问、租户目录、平台 API key、租户自助控制、租户自助读取、workspace、用户自助控制和用户自助读取 HTTP 已脱离 Core，由独立模块依赖注册。
- `platformModules` 已集中负责平台身份、计费、运营服务的构造，并统一托管 Ban reconciler 与 scheduler
  的启动/停止；`run` 只保留平台依赖别名和跨域 wiring。
- `aiModules` 已集中负责 AI 控制面、Serving pipeline、Gateway、Console 和异步 worker 的构造；
  `Start/Stop` 统一管理价格同步、风险审查、审计、Token refresh、结算和异步任务。
- Transport 已将平台 `Deps` 与 AI HTTP 模块分离；composition-only `AIHTTPDeps` 分别持有 `Core`、
  `Subscriptions`、`RiskControl`、`AuditLog`、`System`、`Dashboard`、`Usage`、`OAuthManagement`、`ModelBindings`、`UpstreamDiagnostics`、`UpstreamAccounts`、`UpstreamAccess`、`TenantCatalog`、`APIKeyManagement`、`TenantSelfControl`、`TenantGroups`、`TenantSelfRead`、`Workspace`、`UserSelfControl` 和 `UserSelfRead`，通过 `transport.Module` 调用对应独立注册入口，handler 只接收所属模块依赖。

## 尚未清零的装配遗留

- `transport.Deps` 仍有具体 PostgreSQL/Redis/业务 service；AI Core 已收敛为 `CoreHTTPDeps` 窄端口，账号管理、Provider 密钥、模型绑定、价格簿校验等管理依赖已移出，后续转向 Transport 业务逻辑和平台根容器治理。
- AI 系统端点已经由独立 `SystemHTTPDeps` 组合 `ScoreWeightsStore`、`HealthTracker` 和两个 `ComponentHealthProbe`，不再进入 Core `CoreHTTPDeps`；评分权重 PostgreSQL adapter 仍只在 composition root 构造，其他查询、凭证和控制面 adapter 仍待逐项收敛。
- 管理仪表盘已经由独立 `DashboardHTTPDeps` 组合 `DashboardQueryReader`、身份 provider、失败 observer 和 `HTTPAuthDeps`；租户自助和工作区读取由各自模块显式复用查询端口，具体 `DashboardService` 只在 composition root 构造。
- 管理用量已经由独立 `UsageHTTPDeps` 组合 `UsageQueryReader`、身份 provider、失败 observer 和 `HTTPAuthDeps`；租户自助和工作区读取由各自模块显式复用 `UsageQueryReader` / `UserUsageLogReader`，具体 `UsageService` 只在 composition root 构造。
- OAuth pool/credential 管理已经由独立 `OAuthManagementHTTPDeps` 组合池/凭证端口、池健康、手动刷新、模型目录和绑定端口；Serving 和后台刷新器仍由 composition root/运行时持有。
- 账号/凭证池模型绑定管理已经由独立 `ModelBindingHTTPDeps` 组合账号读取、池读取和 `UpstreamModelBindingStore`；Core 不再注册模型绑定管理路径。
- 上游发现与连通性已经由独立 `UpstreamDiagnosticsHTTPDeps` 组合账号读取、模型绑定、密钥解密、HTTP、能力目录和账号健康端口；Core 不再注册诊断路径。
- 上游账号 CRUD 与迁移已经由独立 `UpstreamAccountManagementHTTPDeps` 组合账号目录、管理、密钥、绑定、价格簿和审计端口；Core 不再持有账号管理、Provider 密钥、账号读取、模型绑定和价格簿校验字段。
- 平台租户上游访问策略已经由独立 `UpstreamAccessManagementHTTPDeps` 组合 `UpstreamAccessManager` 和平台管理员认证；Core 不再持有该控制面端口。
- 租户模型、价格表和上游资源目录已经由独立 `TenantCatalogHTTPDeps` 组合模型目录、分组目录、租户价格表和价格同步端口；Core 不再注册这些租户目录路径。
- 平台 API key 管理已经由独立 `APIKeyManagementHTTPDeps` 组合 key 读写、生命周期、密钥、分组和限额端口；Core 不再注册动态租户/用户 API key 路径，避免与租户自助 `/tenants/me` 静态路径冲突。
- 租户自助 API key/限额已经由独立 `TenantSelfControlHTTPDeps` 组合 key、分组、限额和 end-user ownership 端口；Core 不再注册这些租户控制路径。
- 租户分组控制面已经由独立 `TenantGroupManagementHTTPDeps` 组合分组、调度、上游目标、用户绑定、价格表名称和迁移审计端口；Core 不再注册分组与迁移路径。
- 租户自助 dashboard/usage 已由独立 `TenantSelfReadHTTPDeps` 组合 dashboard/usage 查询端口；Core 不再持有租户 dashboard 查询字段。
- Tenant/user workspace 已由独立 `WorkspaceHTTPDeps` 组合 workspace、dashboard 和 usage 端口；Core 不再持有工作台端口，两个认证 scope 在模块内显式注册。
- 终端用户 API key/限额已由独立 `UserSelfControlHTTPDeps` 组合 key、分组和限额端口；Core 不再持有终端用户 key/限额能力。
- 终端用户分组、模型授权和用量读取已由独立 `UserSelfReadHTTPDeps` 组合分组、模型目录、用户日志和 usage 端口；Core 不再注册终端用户自助读取路径。
- 管理 Dashboard 异常扣费告警查询已移入 `internal/system/pg.SystemRepository`；HTTP handler 不再直接执行 `ai_usage_logs` SQL，查询窗口、排序和行映射由 repository 负责。
- 管理租户详情和终端用户归属校验查询已移入 `internal/tenant/pg.TenantRepository`；HTTP handler 不再直接执行对应 `iam_tenants` / `iam_accounts` 读取。
- 租户启停级联事务已移入 `internal/tenant/pg.TenantRepository`，通过 `tenant/ports.AdminTenantStatusWriter` 注入；handler 只负责租户黑名单和恢复用户黑名单同步。
- 租户创建（含可选初始租户用户/激活令牌）、更新和删除已移入 `tenant/ports.AdminTenantWriter`；composition root 注入带 `ActivationService.Store` 的 `TenantRepository`，handler 不再持有租户生命周期事务。
- AI 身份适配器的终端用户归属校验复用 `TenantRepository.GetEndUserTenantID`，不再持有连接池或直接查询 `iam_accounts`。
- 统一认证保护端点通过 composition root 注入 `auth/ports.AccountReader` / `AccountWriter`；`AuthRepository` 负责当前用户快照、密码和资料写入，Transport 不再直接执行账号 SQL。
- 认证审计日志列表通过 `auth/ports.AuthAuditLogReader` 注入 `AuthRepository`；过滤、计数、稳定排序和分页归一化留在 persistence adapter，管理 Transport 只做 DTO 投影。
- 管理充值目标的租户存在与终端用户归属校验复用 `TenantRepository`；`RechargeService.GrantManual` 持有目标锁并提交充值订单、额度包和余额变更事务，Transport 不再直接读取 `iam_tenants` / `iam_accounts` 做前置校验。
- `GrantBalance` 保留为支付结算使用的外部事务原语，并统一校验订单类型、额度来源与 `payment_order_id` 的组合，避免人工充值和在线支付生命周期交叉写入。
- 管理支付订单与统一充值列表/详情查询通过 `PaymentService` application API 暴露，Transport 不再 import `payment/pg` 或传递 adapter 查询参数。
- 管理充值同步、手动额度冲正和在线退款入口统一由 `PaymentService` 校验订单类型/状态并编排动作，Transport 不再先读投影再自行决定状态机分支。
- 反向充值的租户范围与 `order_type` 校验下沉到 `DeductionService.ReverseTenantOrder` 的订单 `FOR UPDATE` 事务；handler 只传入 claims 作用域并映射领域错误，不再先读 `bill_recharge_orders`。
- 管理系统管理员和租户用户列表查询已移入 `internal/user/pg.AdminAccountRepository`；HTTP handler 不再拼接分页 SQL，只保留状态与 DTO 映射。
- 系统管理员与租户用户的创建、更新和启停状态写入已移入同一 `AdminAccountRepository`，通过 `user/ports.AdminAccountWriter` 注入；激活令牌复用 composition root 注入的 `ActivationService.Store`，黑名单同步仍由 handler 编排。
- 三类管理账号密码重置的目标类型校验与凭证结果映射也通过 `AdminAccountWriter.Reset*Password` / `AdminEndUserWriter.ResetEndUserPassword` 注入；`ActivationService.Reset` 仍由用户 adapter 调用，HTTP handler 不再直接查询 `iam_accounts`。
- 系统管理员删除和终端用户删除事务分别由 `AdminAccountWriter.DeleteSystemAdmin` 与 `AdminEndUserWriter.DeleteEndUser` 持有；终端用户删除在余额行锁后执行显式 blacklist guard，失败会回滚数据库状态。
- 终端用户列表查询已移入 `internal/user/pg.AdminEndUserRepository`，通过 `user/ports.AdminEndUserReader` 注入；HTTP handler 不再直接执行跨租户列表 SQL，只负责 claims scope 和 DTO 映射。
- 终端用户资料更新与启停状态写入已移入同一 `AdminEndUserRepository`，通过 `user/ports.AdminEndUserWriter` 注入；HTTP handler 只保留租户归属校验、错误映射和会话封禁副作用。
- 终端用户创建的账号与一次性激活令牌由同一 `AdminEndUserRepository` 事务写入；repository 通过 composition root 注入的 `ActivationService.Store` 复用激活凭证逻辑，HTTP handler 不再持有创建事务。
- AI 认证端点的 Ban 检查也改用 `HumaBanChecker` 端口，统一 Transport 不再暴露具体 Redis `banstate.Checker`。
- OAuth 凭证管理中的手动刷新能力只依赖 `OAuthTokenRefresher.RefreshByID`，后台轮询刷新器的具体实现继续由 composition root 持有。
- AI 上游模型绑定、凭证导入和 pool CRUD 查询统一使用 `OAuthPoolReader`；创建、更新、状态变更和删除使用 `OAuthPoolWriter` 与领域级 `CredentialPoolCreate` / `CredentialPoolUpdate` 命令。
- 凭证池账号列表改用 `OAuthCredentialReader` 和无密文 `domain.OAuthCredentialSummary`；管理摘要只允许已知账户 ID/套餐字符串字段，任意 provider metadata、嵌套结构和密钥材料仍留在运行时边界内，Transport 不再接收 `OAuthCredentialRow`。
- 凭证创建响应及更新/刷新/删除前的 pool 归属校验也复用 `GetSummaryByID`；只有 serving/token refresh 仍保留原始 `GetByID`，因为它们需要在 adapter 内解密。
- 凭证状态、权重更新和删除改用 `OAuthCredentialWriter`；刷新端点只依赖摘要 reader 与 `OAuthTokenRefresher`，不再要求完整 OAuth store。
- 凭证导入改用 `OAuthCredentialCreator`，并接收领域级 `OAuthCredentialCreate` 命令；完整 OAuth store 不再进入该端点的创建调用链。
- OAuth pool 健康汇总改用 `OAuthPoolHealthReader` 与领域级 `OAuthPoolHealthSummary`；AI Transport 依赖容器已不再暴露具体 `OAuthCredentialStore`。
- OAuth pool 管理的模型发现只依赖 `ClientCatalogResolver`，模型缓存和 provider inspection 仍封装在 composition root 的服务中。
- AI Transport 的上游发现、连通性、账号导出和风控配置使用 `ProviderSecretCodec`，不再接收原始 `SecretMasterKey`；`ProviderKeyCodec` 只在 composition root 以主密钥构造。
- AI Transport 的上游模型发现、外部模型目录和账号连通性检测只依赖 `HTTPDoer`；具体 `http.Client` 的连接池、重定向和 transport 超时策略由 composition root 持有。
- models.dev 能力目录由 composition root 创建的 `externalmodels.Service` 持有 Redis、HTTP 和实例级缓存；AI Transport 只接收 `ModelCapabilityResolver`，不再编排外部目录基础设施。
- AI 系统状态端点只接收 `ComponentHealthProbe`；Redis adapter 封装 `PING` 和 go-redis 命令类型，AI Transport 依赖容器不再持有 `*redis.Client`。
- usage identity enrichment 的 fail-open 告警只依赖 `IdentityEnrichmentFailureObserver`；observability adapter 负责 zap 日志消息与字段，AI Transport 不再持有 `*zap.Logger`。
- 上游模型发现、连通性检测与绑定校验只依赖密钥读取端口 `UpstreamAccountReader`；`upstreamcontrol.Service` 返回领域级 `AccountSecret`，PostgreSQL adapter 封装账号 sqlc row 映射。
- 上游账号列表、CRUD 和探测后的状态协调分别通过 `UpstreamAccountCatalog`、`UpstreamAccountManager` 和 `UpstreamAccountHealthWriter` 进入独立 AI HTTP 模块；账号导出显式组合目录、密钥读取和解密端口，导入显式组合目录、管理、绑定、价格簿与审计端口，具体 `upstreamcontrol.Service` 只在 composition root 构造并注入这些能力。
- 终端用户自助 usage 日志只依赖 `UserUsageLogReader`；`UsageRepo` 封装专用 sqlc 查询的参数和 row 映射，Transport 只接收 `domain.UsageLog`。
- 账号与分组迁移审计只依赖 `AdminAuditRecorder` 和 `domain.AdminAuditEvent`；`AuditRepo` 封装 sqlc 写入，AI Transport 与顶层 AI 依赖组已不再持有 `*dbgen.Queries`。
- 上游模型绑定管理、目录导入、账号迁移和连通性测试只依赖 `UpstreamModelBindingStore` 与领域模型；PostgreSQL adapter 封装 scope 隔离、状态优先查询和原子导入事务，Transport 不再直接管理 `ai_upstream_models` 持久化。
- 租户/用户可用模型、分组有效价格和租户上游资源目录只依赖 `ModelCatalogReader`；PostgreSQL adapter 封装跨分组、资源、模型绑定、租户授权和价格表的聚合查询与 JSONB 映射。
- AI 系统状态通过 `ComponentHealthProbe` 检查 PostgreSQL 和 Redis，账号迁移价格簿校验依赖 `PriceBookReader`；AI HTTP 模块不再持有 PostgreSQL pool 或诊断 HTTP client 具体类型，价格簿校验只在上游账号管理模块装配。
- 平台价格表管理、租户可见价格表与文件迁移、LiteLLM 查询和同步分别通过 `PlatformPriceBookManager`、`TenantPriceBookManager`、`PriceBookSyncManager` 进入 AI Transport；具体 `billingcontrol.Service` 只在 composition root 构造并注入这些能力，分组生效价格继续只依赖 `ModelCatalogReader`。
- 商业控制面通过 `CommercialGroupCatalog`、`CommercialGroupManager`、`CommercialDispatchRuleManager`、`CommercialGroupTargetManager`、`CommercialUserBindingManager`、`CommercialLimitPolicyManager` 六组端口进入 AI Transport；具体 `commercial.Service` 只在 composition root 构造并按能力注入，Serving 运行时仍独立持有其解析能力。
- API Key 控制面通过 `APIKeyReader`、`APIKeyWriter`、`APIKeyLifecycleManager`、`APIKeySecretManager` 四组端口进入 AI Transport；具体 `identitycontrol.Service` 只在 composition root 构造并按能力注入，已有密钥的明文回显与轮换被隔离在敏感密钥端口，新建端口只返回本次生成的明文。
- AI 工作台通过 `OverviewReader`、`ChatModelReader`、`ChatSessionReader`、`ChatSessionManager`、`ChatMessageManager`、`ImageJobReader` 六组共享端口进入 Huma Transport 和 Console；具体 `workspace.Service` 只在 composition root 构造并按入口所需能力注入。
- 订阅控制面通过独立 `SubscriptionHTTPDeps` 组合 `SubscriptionPlanCatalog`、`SubscriptionPlanManager`、`SubscriptionPurchaser`、`SubscriptionReader`、`SubscriptionOrderReader`、`SubscriptionGroupNameResolver` 六组端口，并由 `RegisterSubscriptions` 自行注册租户/终端用户认证分组；AI Core `CoreHTTPDeps` 不再接收这些能力。Serving 继续使用独立准入/扣费端口，具体 `subscription.Service` 只在 composition root 构造并按能力注入。
- PostgreSQL adapter 通过统一 DBTX/pool/transaction 包装器将缺失行与已知约束错误翻译为领域错误；AI Transport 的错误映射不再依赖 `pgx.ErrNoRows` 或 `pgconn.PgError`，未知数据库和连接错误仍按内部错误处理。
- AI Transport 的 UUID 输入使用通用值类型校验，nullable/numeric/time 转换已由领域投影或标准值承担；该包已清零 pgx、Redis、sqlc 和 PostgreSQL adapter 的直接 import，对应依赖例外已删除。
- 租户上游访问策略通过 `UpstreamAccessManager` 最小端口进入 AI Transport；具体 `upstreamaccess.Service` 只在 composition root 构造，顶层 Transport 的兼容装配也仅转发该端口。
- 分组配置导出、预检和导入通过 `GroupTransferManager` 最小端口进入 AI Transport；具体 `commercial.GroupTransferService` 只在 composition root 构造，顶层 Transport 也仅转发该端口。
- 管理审计列表通过 `AdminAuditLogReader` 进入 AI Transport，并与 `AdminAuditRecorder` 写端口分离；具体 `observabilitycontrol.AuditService` 只在 composition root 同时装配到两个端口。
- 管理端、租户端和工作区仪表盘查询统一通过 `DashboardQueryReader` 进入 AI Transport；具体 `observabilitycontrol.DashboardService` 只在 composition root 构造并注入该读端口。
- 管理端、租户端、用户端和工作区用量查询统一通过 `UsageQueryReader` 进入 AI Transport，受限用户日志保留独立 `UserUsageLogReader`；具体 `observabilitycontrol.UsageService` 只在 composition root 构造并分别注入两个读端口。
- 风控管理路由分别通过 `RiskControlConfigStore`、`RiskControlDetector`、`RiskControlLogReader` 和 `RiskEventManager` 进入 AI Transport；具体 config/log/event service 与 checker 只在 composition root 构造，serving/worker 继续复用 checker。
- 风控 HTTP 已由独立 `RiskControlHTTPDeps` 组合四组业务端口、`HTTPAuthDeps` 和 `ProviderSecretCodec`，并通过 `RegisterRiskControl` 注册平台管理员认证分组；AI Core `CoreHTTPDeps` 不再接收或注册风控能力。
- 管理审计读取 HTTP 已由独立 `AuditLogHTTPDeps` 组合 `AdminAuditLogReader` 和 `HTTPAuthDeps`，并通过 `RegisterAuditLog` 注册平台管理员认证分组；AI Core `CoreHTTPDeps` 不再接收或注册读取端口，迁移写入继续使用独立模块的 `AdminAuditRecorder`。
- 系统状态与路由权重 HTTP 已由独立 `SystemHTTPDeps` 组合 `HealthTracker`、`ComponentHealthProbe`、`ScoreWeightsStore` 和 `HTTPAuthDeps`，并通过 `RegisterSystem` 注册平台管理员认证分组；AI Core `CoreHTTPDeps` 不再接收或注册系统端点。
- 管理仪表盘 HTTP 已由独立 `DashboardHTTPDeps` 组合 `DashboardQueryReader`、身份补全端口和 `HTTPAuthDeps`，并通过 `RegisterDashboard` 注册平台管理员认证分组；租户自助和工作区端点由各自模块显式复用共享查询端口。
- 管理用量 HTTP 已由独立 `UsageHTTPDeps` 组合 `UsageQueryReader`、身份补全端口和 `HTTPAuthDeps`，并通过 `RegisterUsage` 注册平台管理员认证分组；租户、用户和工作区端点由各自模块显式复用共享查询端口。
- OAuth pool/credential HTTP 已由独立 `OAuthManagementHTTPDeps` 组合池/凭证读写、健康、刷新、目录和绑定端口，并通过 `RegisterOAuthManagement` 注册平台管理员认证分组；Core 不再注册 OAuth pool/credential 管理路径。
- 账号/凭证池模型绑定 HTTP 已由独立 `ModelBindingHTTPDeps` 组合账号/池读取和绑定存储端口，并通过 `RegisterModelBindings` 注册平台管理员认证分组；Core 不再注册模型绑定管理路径。
- 上游诊断 HTTP 已由独立 `UpstreamDiagnosticsHTTPDeps` 组合发现、能力推断和连通性测试端口，并通过 `RegisterUpstreamDiagnostics` 注册平台管理员认证分组；Core 不再注册这些诊断路径。
- 上游账号管理 HTTP 已由独立 `UpstreamAccountManagementHTTPDeps` 组合 CRUD、导入/导出迁移和审计端口，并通过 `RegisterUpstreamAccountManagement` 注册平台管理员认证分组；Core 不再注册 8 条账号管理路径。
- 上游访问策略 HTTP 已由独立 `UpstreamAccessManagementHTTPDeps` 组合租户策略端口，并通过 `RegisterUpstreamAccessManagement` 注册平台管理员认证分组；Core 不再注册 2 条策略路径。
- 租户目录 HTTP 已由独立 `TenantCatalogHTTPDeps` 组合模型、价格和上游资源目录端口，并通过 `RegisterTenantCatalog` 注册租户用户认证分组；Core 不再注册 17 条租户目录路径。
- 平台 API key HTTP 已由独立 `APIKeyManagementHTTPDeps` 组合平台代管 key 端口，并通过 `RegisterAPIKeyManagement` 注册平台管理员认证分组；Core 不再注册平台代管 key 路径。
- 租户自助控制 HTTP 已由独立 `TenantSelfControlHTTPDeps` 组合租户 key/限额端口，并通过 `RegisterTenantSelfControl` 注册租户用户认证分组；Core 不再注册 11 条租户控制路径。
- 租户分组 HTTP 已由独立 `TenantGroupManagementHTTPDeps` 组合商业控制面和迁移端口，并通过 `RegisterTenantGroupManagement` 注册租户用户认证分组；Core 不再注册 25 条分组/迁移路径。
- 租户自助读取 HTTP 已由独立 `TenantSelfReadHTTPDeps` 组合 dashboard/usage 端口，并通过 `RegisterTenantSelfRead` 注册租户用户认证分组；Core 不再注册 5 条租户读取路径。
- Workspace HTTP 已由独立 `WorkspaceHTTPDeps` 组合 tenant/user 工作台端口，并通过 `RegisterWorkspace` 注册两个认证分组；Core 不再注册 14 条工作台路径。
- 用户自助控制 HTTP 已由独立 `UserSelfControlHTTPDeps` 组合 API key、分组、限额和 `HTTPAuthDeps`，并通过 `RegisterUserSelfControl` 注册终端用户认证分组；Core 不再注册 9 条用户 key/限额路径。
- 用户自助读取 HTTP 已由独立 `UserSelfReadHTTPDeps` 组合分组、模型目录、用户日志、usage 和 `HTTPAuthDeps`，并通过 `RegisterUserSelfRead` 注册终端用户认证分组；Core 不再注册 5 条用户读取路径。
- 部分后台组件只有 `Start(ctx)` 或 `Start/Stop`，尚未统一为 `Start/Stop/Health` 接口；未提供 Stop 的组件依赖根 context 取消，后续逐个补齐可观测状态和等待语义。

装配测试位于 `cmd/server/*_test.go`，不启动真实监听，覆盖资源逆序关闭、幂等关闭和公共/管理监听参数隔离。
