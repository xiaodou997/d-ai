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
- `transport.Deps` 和 `ai/transport.AIDeps` 已按 identity、catalog、runtime、operations
  责任拆成嵌入式依赖组；订阅、风控、审计读取、系统和管理仪表盘 HTTP 已脱离该容器，由独立模块依赖注册。
- `platformModules` 已集中负责平台身份、计费、运营服务的构造，并统一托管 Ban reconciler 与 scheduler
  的启动/停止；`run` 只保留平台依赖别名和跨域 wiring。
- `aiModules` 已集中负责 AI 控制面、Serving pipeline、Gateway、Console 和异步 worker 的构造；
  `Start/Stop` 统一管理价格同步、风险审查、审计、Token refresh、结算和异步任务。
- Transport 已将平台 `Deps` 与 AI HTTP 模块分离；composition-only `AIHTTPDeps` 分别持有 `Core`、
  `Subscriptions`、`RiskControl`、`AuditLog`、`System`、`Dashboard` 和 `Usage`，通过 `transport.Module` 调用对应独立注册入口，handler 只接收所属模块依赖。

## 尚未清零的装配遗留

- `transport.Deps` 仍有具体 PostgreSQL/Redis/业务 service，AI core `AIDeps` 虽已接口化但仍是 service locator；P1-02 后续会按订阅模块模式继续拆分端点组。
- AI 系统端点已经由独立 `SystemHTTPDeps` 组合 `ScoreWeightsStore`、`HealthTracker` 和两个 `ComponentHealthProbe`，不再进入 Core `AIDeps`；评分权重 PostgreSQL adapter 仍只在 composition root 构造，其他查询、凭证和控制面 adapter 仍待逐项收敛。
- 管理仪表盘已经由独立 `DashboardHTTPDeps` 组合 `DashboardQueryReader`、身份 provider、失败 observer 和 `HTTPAuthDeps`；租户自助与工作区仍从 Core 复用同一读端口，具体 `DashboardService` 只在 composition root 构造。
- 管理用量已经由独立 `UsageHTTPDeps` 组合 `UsageQueryReader`、身份 provider、失败 observer 和 `HTTPAuthDeps`；租户、用户和工作区仍从 Core 复用 `UsageQueryReader` / `UserUsageLogReader`，具体 `UsageService` 只在 composition root 构造。
- AI 认证端点的 Ban 检查也改用 `HumaBanChecker` 端口，统一 Transport 不再暴露具体 Redis `banstate.Checker`。
- OAuth 凭证管理端点只依赖 `OAuthTokenRefresher.RefreshByID`，后台轮询刷新器的具体实现继续由 composition root 持有。
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
- 上游账号列表、CRUD 和探测后的状态协调分别通过 `UpstreamAccountCatalog`、`UpstreamAccountManager` 和 `UpstreamAccountHealthWriter` 进入 AI Transport；账号导出显式组合目录、密钥读取和解密端口，导入显式组合目录与管理端口，具体 `upstreamcontrol.Service` 只在 composition root 构造并注入这些能力。
- 终端用户自助 usage 日志只依赖 `UserUsageLogReader`；`UsageRepo` 封装专用 sqlc 查询的参数和 row 映射，Transport 只接收 `domain.UsageLog`。
- 账号与分组迁移审计只依赖 `AdminAuditRecorder` 和 `domain.AdminAuditEvent`；`AuditRepo` 封装 sqlc 写入，AI Transport 与顶层 AI 依赖组已不再持有 `*dbgen.Queries`。
- 上游模型绑定管理、目录导入、账号迁移和连通性测试只依赖 `UpstreamModelBindingStore` 与领域模型；PostgreSQL adapter 封装 scope 隔离、状态优先查询和原子导入事务，Transport 不再直接管理 `ai_upstream_models` 持久化。
- 租户/用户可用模型、分组有效价格和租户上游资源目录只依赖 `ModelCatalogReader`；PostgreSQL adapter 封装跨分组、资源、模型绑定、租户授权和价格表的聚合查询与 JSONB 映射。
- AI 系统状态通过 `ComponentHealthProbe` 检查 PostgreSQL 和 Redis，账号迁移价格簿校验依赖 `PriceBookReader`；`ai/transport.InfrastructureDeps` 已不再持有 `*pgxpool.Pool`。
- 平台价格表管理、租户可见价格表与文件迁移、LiteLLM 查询和同步分别通过 `PlatformPriceBookManager`、`TenantPriceBookManager`、`PriceBookSyncManager` 进入 AI Transport；具体 `billingcontrol.Service` 只在 composition root 构造并注入这些能力，分组生效价格继续只依赖 `ModelCatalogReader`。
- 商业控制面通过 `CommercialGroupCatalog`、`CommercialGroupManager`、`CommercialDispatchRuleManager`、`CommercialGroupTargetManager`、`CommercialUserBindingManager`、`CommercialLimitPolicyManager` 六组端口进入 AI Transport；具体 `commercial.Service` 只在 composition root 构造并按能力注入，Serving 运行时仍独立持有其解析能力。
- API Key 控制面通过 `APIKeyReader`、`APIKeyWriter`、`APIKeyLifecycleManager`、`APIKeySecretManager` 四组端口进入 AI Transport；具体 `identitycontrol.Service` 只在 composition root 构造并按能力注入，已有密钥的明文回显与轮换被隔离在敏感密钥端口，新建端口只返回本次生成的明文。
- AI 工作台通过 `OverviewReader`、`ChatModelReader`、`ChatSessionReader`、`ChatSessionManager`、`ChatMessageManager`、`ImageJobReader` 六组共享端口进入 Huma Transport 和 Console；具体 `workspace.Service` 只在 composition root 构造并按入口所需能力注入。
- 订阅控制面通过独立 `SubscriptionHTTPDeps` 组合 `SubscriptionPlanCatalog`、`SubscriptionPlanManager`、`SubscriptionPurchaser`、`SubscriptionReader`、`SubscriptionOrderReader`、`SubscriptionGroupNameResolver` 六组端口，并由 `RegisterSubscriptions` 自行注册租户/终端用户认证分组；AI core `AIDeps` 不再接收这些能力。Serving 继续使用独立准入/扣费端口，具体 `subscription.Service` 只在 composition root 构造并按能力注入。
- PostgreSQL adapter 通过统一 DBTX/pool/transaction 包装器将缺失行与已知约束错误翻译为领域错误；AI Transport 的错误映射不再依赖 `pgx.ErrNoRows` 或 `pgconn.PgError`，未知数据库和连接错误仍按内部错误处理。
- AI Transport 的 UUID 输入使用通用值类型校验，nullable/numeric/time 转换已由领域投影或标准值承担；该包已清零 pgx、Redis、sqlc 和 PostgreSQL adapter 的直接 import，对应依赖例外已删除。
- 租户上游访问策略通过 `UpstreamAccessManager` 最小端口进入 AI Transport；具体 `upstreamaccess.Service` 只在 composition root 构造，顶层 Transport 的兼容装配也仅转发该端口。
- 分组配置导出、预检和导入通过 `GroupTransferManager` 最小端口进入 AI Transport；具体 `commercial.GroupTransferService` 只在 composition root 构造，顶层 Transport 也仅转发该端口。
- 管理审计列表通过 `AdminAuditLogReader` 进入 AI Transport，并与 `AdminAuditRecorder` 写端口分离；具体 `observabilitycontrol.AuditService` 只在 composition root 同时装配到两个端口。
- 管理端、租户端和工作区仪表盘查询统一通过 `DashboardQueryReader` 进入 AI Transport；具体 `observabilitycontrol.DashboardService` 只在 composition root 构造并注入该读端口。
- 管理端、租户端、用户端和工作区用量查询统一通过 `UsageQueryReader` 进入 AI Transport，受限用户日志保留独立 `UserUsageLogReader`；具体 `observabilitycontrol.UsageService` 只在 composition root 构造并分别注入两个读端口。
- 风控管理路由分别通过 `RiskControlConfigStore`、`RiskControlDetector`、`RiskControlLogReader` 和 `RiskEventManager` 进入 AI Transport；具体 config/log/event service 与 checker 只在 composition root 构造，serving/worker 继续复用 checker。
- 风控 HTTP 已由独立 `RiskControlHTTPDeps` 组合四组业务端口、`HTTPAuthDeps` 和 `ProviderSecretCodec`，并通过 `RegisterRiskControl` 注册平台管理员认证分组；AI core `AIDeps` 不再接收或注册风控能力。
- 管理审计读取 HTTP 已由独立 `AuditLogHTTPDeps` 组合 `AdminAuditLogReader` 和 `HTTPAuthDeps`，并通过 `RegisterAuditLog` 注册平台管理员认证分组；AI core `AIDeps` 不再接收或注册读取端口，迁移写入继续使用核心的 `AdminAuditRecorder`。
- 系统状态与路由权重 HTTP 已由独立 `SystemHTTPDeps` 组合 `HealthTracker`、`ComponentHealthProbe`、`ScoreWeightsStore` 和 `HTTPAuthDeps`，并通过 `RegisterSystem` 注册平台管理员认证分组；AI core `AIDeps` 不再接收或注册系统端点。
- 管理仪表盘 HTTP 已由独立 `DashboardHTTPDeps` 组合 `DashboardQueryReader`、身份补全端口和 `HTTPAuthDeps`，并通过 `RegisterDashboard` 注册平台管理员认证分组；租户自助和工作区端点继续由 Core 使用共享查询端口。
- 管理用量 HTTP 已由独立 `UsageHTTPDeps` 组合 `UsageQueryReader`、身份补全端口和 `HTTPAuthDeps`，并通过 `RegisterUsage` 注册平台管理员认证分组；租户、用户和工作区端点继续由 Core 使用共享查询端口。
- 部分后台组件只有 `Start(ctx)` 或 `Start/Stop`，尚未统一为 `Start/Stop/Health` 接口；未提供 Stop 的组件依赖根 context 取消，后续逐个补齐可观测状态和等待语义。

装配测试位于 `cmd/server/*_test.go`，不启动真实监听，覆盖资源逆序关闭、幂等关闭和公共/管理监听参数隔离。
