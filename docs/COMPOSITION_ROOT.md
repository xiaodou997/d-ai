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
- `transport.Deps` 和 `ai/transport.AIDeps` 已按 identity、billing、catalog、runtime、operations
  责任拆成嵌入式依赖组；handler 的 `d.Field` 访问保持兼容，但 composition root 必须显式写出所属组。
- `platformModules` 已集中负责平台身份、计费、运营服务的构造，并统一托管 Ban reconciler 与 scheduler
  的启动/停止；`run` 只保留平台依赖别名和跨域 wiring。
- `aiModules` 已集中负责 AI 控制面、Serving pipeline、Gateway、Console 和异步 worker 的构造；
  `Start/Stop` 统一管理价格同步、风险审查、审计、Token refresh、结算和异步任务。
- Transport 已将平台 `Deps` 与 AI `AIDeps` 分离，并通过 `transport.Module` 注册 AI 路由；后续运行角色可以
  只注册所需模块，不再必须接收整套跨域依赖。

## 尚未清零的装配遗留

- `transport.Deps` 与 `transport.AIDeps` 组内仍有具体 PostgreSQL/Redis/adapter 类型；P1-02 后续会按端点组继续替换为最小端口集合。
- AI 系统端点已经只依赖 `ScoreWeightsStore` 评分权重端口，不再暴露 PostgreSQL `RouteWeightsStore` 具体类型；其他查询、凭证和控制面 adapter 仍待逐项收敛。
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
- 部分后台组件只有 `Start(ctx)` 或 `Start/Stop`，尚未统一为 `Start/Stop/Health` 接口；未提供 Stop 的组件依赖根 context 取消，后续逐个补齐可观测状态和等待语义。

装配测试位于 `cmd/server/*_test.go`，不启动真实监听，覆盖资源逆序关闭、幂等关闭和公共/管理监听参数隔离。
