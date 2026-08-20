# Composition root 与生命周期

P1-02 采用可回滚的渐进拆分。当前 `cmd/server` 仍保留单进程 `dai all`，但启动过程已经
开始按职责分层：

```text
config.Load
    ↓
openInfrastructure  ── PostgreSQL / Redis / schema verification
    ↓
platform + AI module assembly（当前仍在 run 内，下一步继续拆）
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

## 尚未清零的装配遗留

- 平台模块和 AI 模块仍在 `run` 中顺序构造，下一步按 identity、billing、catalog、runtime、operations 拆成显式模块装配函数。
- `transport.Deps` 与 `ai/transport.AIDeps` 仍是兼容型依赖容器，组内仍有具体 PostgreSQL/Redis/adapter 类型；P1-02 后续会按端点组替换为最小端口集合和 Module 接口。
- 部分后台组件只有 `Start(ctx)` 或 `Start/Stop`，尚未统一为 `Start/Stop/Health` 接口；未提供 Stop 的组件依赖根 context 取消，后续逐个补齐可观测状态和等待语义。

装配测试位于 `cmd/server/*_test.go`，不启动真实监听，覆盖资源逆序关闭、幂等关闭和公共/管理监听参数隔离。
