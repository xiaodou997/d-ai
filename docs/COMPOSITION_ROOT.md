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
- `httpServers` 独立管理公共业务监听和 loopback 管理监听；公共 AI 流式监听保持 `WriteTimeout=0`，管理监听使用有限超时，Start/Shutdown 具备幂等保护并等待监听 goroutine 退出。
- 异步任务引擎已经登记到生命周期栈；Engine 自持有 worker context，Stop 会取消并等待 worker/reaper/webhook 循环，再释放 Redis/PostgreSQL，并提供最小 Health 快照。
- 异步任务执行中的租约心跳 goroutine 现在使用可取消的限时上下文，并在单次任务返回前等待退出；Engine Stop 不会在心跳仍访问存储时释放数据库依赖。
- Runtime API Key 的 `last_used_at` best-effort 写入由 Gateway 自持有并继承请求取消，单次最多 2 秒；Gateway Stop 会 fencing 并等待 in-flight 写入，不再遗留访问已释放数据库池的 goroutine。
- Runtime binding cache miss 加载现在由 `CachedBindingResolver` 自持有的 context/WaitGroup 管理，并由 `aiModules` 启动和停止；关闭时不会遗留脱离请求的配置读取。
- OAuth pool 模型 catalog 的 singleflight discovery 现在由 `clientcatalog.Service` 自持有的 active-load registry 管理；请求取消不破坏共享加载，`aiModules.Stop` 会取消并等待 provider inspection 后再释放 OAuth/数据库依赖。
- Fixed-provider client runtime 的 401 credential refresh 现在由 `clientruntime.Runtime` 自持有的 active-refresh registry 管理；请求取消不破坏共享刷新，`aiModules.Stop` 先停止 catalog、再停止 runtime，最后才释放 OAuth/数据库依赖。
- `aiModules.Stop` 现在由 owner 串行执行并允许在短超时后再次等待；Stop-before-Start 会永久封存模块，风险控制 worker 的重复 Stop 也会继续等待在途任务，避免关闭超时后留下无法收敛的后台 goroutine。
- Scheduler 现在继承 composition root 的 worker context；五类调度任务的数据库/账务操作会随关闭取消，`platformModules.Stop(ctx)` 将 scheduler 的超时错误返回 shutdown stack，并支持后续使用更长 deadline 重试等待。
- 图片桥接的请求 envelope 携带原始请求 context，远程输入图 materialization 遵循请求取消；离线/合成 bridge 调用才回退到有限默认 context。
- 订阅 janitor 现在由 `subscription.Service` 提供幂等 `Start/Stop/Health`，并纳入 `aiModules` 生命周期；订阅订单补偿不再是未登记的阻塞循环。
- 平台 Transport 依赖已按模块显式投影，AI Core 使用 `CoreHTTPDeps`；订阅、风控、审计读取、系统、管理仪表盘、管理用量、OAuth 管理、模型绑定、上游诊断、上游账号管理、上游访问、租户目录、平台 API key、租户自助控制、租户自助读取、workspace、用户自助控制和用户自助读取 HTTP 均由独立模块注册。
- `platformModules` 已集中负责平台身份、计费、运营服务的构造，并统一托管 Ban reconciler 与 scheduler
  的启动/停止；`run` 只保留平台依赖别名和跨域 wiring。
- `openInfrastructure` 在生产按 `DAI_BILLING_DATABASE_URL` 打开独立 billing pool；账务、支付、订阅扣费和
  outbox 结算使用 billing pool，未配置时仅在非生产环境回退到主 pool，避免把权限切换误带入开发环境。
- `aiModules` 已集中负责 AI 控制面、Serving pipeline、Gateway、Console 和异步 worker 的构造；
  `Start/Stop` 统一管理价格同步、风险审查、审计、Token refresh、结算和异步任务，LiteLLM 远程刷新在 Stop 时会取消并等待退出。
- 平台 Transport 依赖投影已移出 `aiModules`，由 composition root 的 `buildPlatformTransportModules` 从具体平台服务组装；AI 模块只暴露 AI HTTP 依赖和运行时路由 owner。
- 平台 HTTP 路由现在与 AI 路由一样通过 `transport.Module` 注册；identity、billing、operations 和管理员模块已逐步改用显式依赖，剩余聚合依赖继续收敛。
- `cmd/server` 的组合根测试现在把全量 Transport 模块注册到空依赖 OpenAPI，并断言 metadata、identity、admin、billing、operations 和 AI 六组代表 surface，避免只验证模块数量而漏注册整组路由。
- 元数据与 JWKS 已由最小 `metaModule` 注册，仅接收版本字符串和 JWT 服务；平台主模块不再作为完整依赖容器参与路由注册。
- chi 原生路由现在接收独立 `transport.RawDeps`，微信回调只依赖支付服务/日志，公开 favicon 只依赖品牌读端口；`RegisterRaw` 不再接收平台聚合容器。
- AI 身份补全现在只接收 `user/ports.IdentityUserReader` 的非敏感用户投影；具体 `UserService` 与 PostgreSQL 用户模型留在用户域和 composition root，Transport 不再依赖它们。
- 运营 HTTP 路由已从平台主模块拆成显式 `platformOperationsModule`，公告、通知、系统模块、数据清理和代理节点只接收各自服务及统一平台认证依赖；通知路由进一步只接收 `notification.HTTPService`，其余管理聚合路由仍待继续拆分。
- 计费 HTTP 路由已从平台主模块拆成显式 `platformBillingModule`，在线充值、租户额度、管理支付和微信回调共享仅支付服务与平台认证/日志依赖；管理充值目标解析由 `RechargeService.GrantManual` 在账务事务内完成。
- 身份自助与公开路由已拆成显式 `platformIdentityModule`，账户查询、租户自助、门户品牌和公开邀请只接收各自端口及平台认证/法律配置；管理员账号业务编排和认证状态写入仍待继续收敛。
- 认证路由已在身份模块内使用独立 `authModule` 依赖，登录/刷新/激活、MFA、近期认证、`/me`、登出和改密不再接收平台路由聚合容器；管理员账号与 JWT key 管理仍待后续提取。
- JWT key 列表与轮换已加入身份模块的 `jwtKeysModule`，只接收 JWT/黑名单认证依赖；管理员账号、财务和仪表盘的 handler 业务编排仍待继续收敛。
- 管理员租户、账号、终端用户、财务、仪表盘和用量路由已集中到 `platformAdminModule` 下的六个显式子模块，各自只接收所属端口与认证依赖；平台主模块不再直接注册这组管理路由。
- AI 纵向路由模块不再持有平台路由聚合容器，统一改用仅含 JWT/黑名单、租户读取和用户投影的 `aiPlatformDeps`；平台身份、账务和运营服务不会随 AI Transport 容器传递。
- `cleanup.Service` 已补齐幂等 `Start/Stop` 和 worker 等待语义，并由 `run` 注册到 shutdown stack；自动清理与 HTTP 触发的手动清理共享可取消 context 和等待计数，停止时先取消并等待所有清理执行，再释放数据库依赖。
- 四个小时级清理任务（图片、文件、会话、激活凭证）现在由 `periodicWorker` 统一持有子 context，首次执行、每小时执行和 Stop 等待都登记在 shutdown stack 中。
- composition root 维护统一的 `lifecycleHealth` 投影；`/health.components` 暴露基础设施、平台/AI 模块、Runtime Gateway telemetry、异步任务、后台清理任务和公共/管理监听器的 started/stopped 状态，同时保留 Scheduler 任务快照。真实 PostgreSQL/Redis 连通性仍只由 `/ready` 判定。
- Transport 已将平台路由模块与 AI HTTP 模块分离；composition-only `AIHTTPDeps` 分别持有 `Core`、
  `Subscriptions`、`RiskControl`、`AuditLog`、`System`、`Dashboard`、`Usage`、`OAuthManagement`、`ModelBindings`、`UpstreamDiagnostics`、`UpstreamAccounts`、`UpstreamAccess`、`TenantCatalog`、`APIKeyManagement`、`TenantSelfControl`、`TenantGroups`、`TenantSelfRead`、`Workspace`、`UserSelfControl` 和 `UserSelfRead`，通过 `transport.Module` 调用对应独立注册入口，handler 只接收所属模块依赖。

## 尚未清零的装配遗留

- 平台路由不再暴露 `transport.Deps` service locator，改由 `transport.Module` 与模块专属依赖类型注册；平台 concrete service 仍只由 composition root 持有，后续转向 Transport 业务逻辑和运行角色治理。
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
- 支付 sweep 的 `closed/expired` 迁移使用 `UpdateStatusIfCurrent` 乐观状态条件，外部支付调用返回后不会覆盖并发回调已提交的 `paid` 状态。
- 支付 sweep 的 provider/入账失败写入 `pay_orders.sweep_attempts`、`sweep_last_error` 和 `sweep_next_attempt_at`；指数退避上限 1 小时，重启和多副本不会恢复成每分钟重试。
- 支付 sweep 对 `USERPAYING/NOTPAY` 结果只更新下次查单时间，不虚增失败次数；入账成功或关单会清空重试状态，过期订单仍由状态条件更新保护。
- 提现列表通过 `PaymentService.ListWithdrawals` 接收 `WithdrawalListParams`，由 application 层归一化租户/状态/分页并拒绝未知状态，Transport 不再传递裸查询参数。
- 支付 cleanup 失败通过 `error` 返回 Scheduler 统一记录；Scheduler 的后台任务启动/停止具备幂等保护、等待语义，JWT 退役初始延迟可被停止信号中断。
- Scheduler 通过 `/health` 暴露五类后台任务的运行中、最近成功/失败、连续失败和跨副本锁跳过快照；支付 sweep 将单轮错误回传，订单级退避状态负责跨周期/重启重试，`sweep_last_error` 可用于故障告警与人工对账。
- JWT 密钥退役使用带超时的数据库更新；即使某个副本 UPDATE 影响 0 行，也会重新加载 active/grace 集合，及时清除其他副本已退役的本地公钥缓存。
- LiteLLM 价格导入与常用模型同步按价格表单事务批量执行，价格表行锁串行化同一价格表的副本写入；相同快照只在实际变化时 bump revision，失败整批回滚后可安全重试，手工条目仍受保护。
- 数据清理运行记录持有 owner、heartbeat 和 lease_until；清理 worker 续租失败会主动取消，终态写入必须匹配 owner，过期租约可被其他副本回收，避免长任务误完成或永久占用活动槽位。
- FileStore 过期清理先 claim 数据库资产、再删除本地文件、最后按 owner fencing 删除元数据；文件删除失败会释放 claim 保留记录，下一轮可重试，不再出现元数据先删导致的不可恢复孤儿文件。
- 本地图片清理使用共享 storage 下的可回收文件租约；`_tmp`/`ephemeral` 按文件生命周期清理，task 目录只有在异步任务检查器确认任务已不存在/过期后才作为 orphan 删除，未配置检查器时默认保留。
- 支付 sweep/cleanup 的 PostgreSQL advisory lock 在同一物理连接上获取、执行和释放，避免 `pgxpool` 跨连接释放造成锁泄漏；支付与额度结算任务统一设有 5 分钟操作超时，解锁使用独立短超时上下文。
- Scheduler 发布 `dai_scheduler_task_runs_total`、`dai_scheduler_task_duration_seconds`、`dai_scheduler_task_running`、`dai_scheduler_task_consecutive_failures` 和 `dai_scheduler_task_skips_total`，可直接用于失败、卡住和跨副本跳过告警。
- Scheduler 每 15 分钟执行一次 billing 不变量对账：使用 Repeatable Read 只读快照和事务级 advisory lock，发布 `dai_billing_reconciliation_violations`、按不变量分类的差异数以及最近运行/健康时间戳。
- Outbox 积压与 parked row 的排查、单行 requeue 和禁止操作见 `docs/BILLING_OUTBOX_RUNBOOK.md`；恢复必须保留原 `request_id` 并先修复根因。
- 支付 sweep 额外发布 retry 总量、到期重试量、最老失败时长和统计读取失败指标；排查顺序与 PromQL 告警阈值记录在 `docs/PAYMENT_SWEEP_RUNBOOK.md`。
- 反向充值的租户范围与 `order_type` 校验下沉到 `DeductionService.ReverseTenantOrder` 的订单 `FOR UPDATE` 事务；handler 只传入 claims 作用域并映射领域错误，不再先读 `bill_recharge_orders`。
- 运营账务 command 的 context 由 HTTP/payment application 一路传入 `DeductionService`；退款、充值撤销和批量退款不再创建脱离请求的 `context.Background()` 数据库操作，取消时批量处理会停止剩余项目。
- 账号和租户状态、密码重置、资料变更及登出的 Redis token/ban 副作用统一经 `auth/ports.AccountSecurityWriter`；Transport 只传递 claims、状态结果和请求 context，具体黑名单写入由 auth application service 负责。
- 管理系统管理员和租户用户列表查询已移入 `internal/user/pg.AdminAccountRepository`；HTTP handler 不再拼接分页 SQL，只保留状态与 DTO 映射。
- 系统管理员与租户用户的创建、更新和启停状态写入已移入同一 `AdminAccountRepository`，通过 `user/ports.AdminAccountWriter` 注入；激活令牌复用 composition root 注入的 `ActivationService.Store`，黑名单同步改由 `auth/ports.AccountSecurityWriter` command 负责。
- 管理员租户用户创建不再依赖 Transport 的锁外租户预检；`AdminAccountRepository` 在账号事务内依赖 `iam_accounts.tenant_id` 外键，并将缺失租户翻译为 `user/ports.ErrTenantNotFound`，避免租户删除/创建竞态留下错误的半创建流程。
- 三类管理账号密码重置的目标类型校验与凭证结果映射也通过 `AdminAccountWriter.Reset*Password` / `AdminEndUserWriter.ResetEndUserPassword` 注入；`ActivationService.Reset` 仍由用户 adapter 调用，HTTP handler 不再直接查询 `iam_accounts`，会话失效通过 `AccountSecurityWriter` command 完成。
- 系统管理员删除和终端用户删除事务分别由 `AdminAccountWriter.DeleteSystemAdmin` 与 `AdminEndUserWriter.DeleteEndUser` 持有；终端用户删除在余额行锁后执行 `AccountSecurityWriter` ban guard，失败会回滚数据库状态。
- 终端用户列表查询已移入 `internal/user/pg.AdminEndUserRepository`，通过 `user/ports.AdminEndUserReader` 注入；HTTP handler 不再直接执行跨租户列表 SQL，只负责 claims scope 和 DTO 映射。
- 终端用户资料更新与启停状态写入已移入同一 `AdminEndUserRepository`，通过 `user/ports.AdminEndUserWriter` 注入；HTTP handler 只保留租户归属校验、错误映射和 `AccountSecurityWriter` 调用。
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
- 已纳入进程生命周期的后台组件现在共享 `internal/lifecycle.Component` / `HealthSnapshot` 合约；Health 目前表达 started/stopped 生命周期，队列深度、失败和延迟仍由各自 Prometheus/领域探针提供。请求级短生命周期 goroutine 不纳入该合约，且不再有未登记的小时级清理 goroutine。
- Console 流式消息持久化由请求级 owner 的 defer、`sync.Once` 和 `WaitGroup` 管理；正常完成与 panic/取消路径都等待 goroutine 收尾，异常路径记录 `interrupted`，不会在关闭数据库后继续写入。

装配测试位于 `cmd/server/*_test.go`，不启动真实监听，覆盖资源逆序关闭、幂等关闭和公共/管理监听参数隔离。
