# 运行角色

同一个 `dai` 二进制支持四种运行角色。未提供参数时默认使用 `all`，兼容本地开发和单文件部署。

```text
dai all          # 控制面、Gateway、后台任务和 Portal（默认）
dai control-api  # 控制面 API、Portal 与管理探针
dai gateway      # AI runtime/file/console 路由与管理探针
dai worker       # 后台任务与管理探针，不监听公共业务端口
```

角色通过同一份配置和数据库 schema 启动，但只开启对应的路由和生命周期组件：

| 角色 | 控制面 HTTP | Gateway HTTP | 后台任务 | 公共监听 |
| --- | --- | --- | --- | --- |
| `all` | 是 | 是 | 是 | 是 |
| `control-api` | 是 | 否 | 否 | 是 |
| `gateway` | 否 | 是 | 否 | 是 |
| `worker` | 否 | 否 | 是 | 否 |

所有角色都提供独立管理监听上的 `/health`、`/healthz` 和 `/ready`。`/ready` 只表示 PostgreSQL、账务 PostgreSQL（如配置）和 Redis 可达；`/health` 额外返回该角色已启动的进程组件。生产环境应为每个角色配置独立的管理地址或通过私有网络暴露。

`control-api` 与 `gateway` 可以分别扩容；`worker` 可运行多个副本，后台任务的租约、advisory lock 和幂等约束负责跨副本协调。进程内状态不是跨副本真相源。

## 生产运行约定

- 所有角色使用同一份 schema v30；生产启动只校验版本，不执行迁移。迁移、备份和恢复按 [`SCHEMA_RELEASE_RUNBOOK.md`](SCHEMA_RELEASE_RUNBOOK.md) 执行。
- 每个角色都应配置独立的 `DAI_SERVER_MANAGEMENT_ADDR`，或通过私有管理网络汇聚 `/health`、`/ready` 和 `/metrics`；业务监听器不承载 Prometheus 抓取。
- `control-api` 主要受数据库读写和身份/计费流量约束；`gateway` 主要受 Redis、上游连接池和 AI 请求并发约束；`worker` 主要受 Outbox、审计 inbox、异步任务和支付扫单积压约束。对应指标和阈值见 [`OBSERVABILITY_RUNBOOK.md`](OBSERVABILITY_RUNBOOK.md)。
- 生产容器以非 root 用户运行、根文件系统只读，只有 `/data` 挂载卷可写；临时文件使用 `/tmp` tmpfs。扩容或恢复时不能依赖进程内缓存作为业务真相。
- 发布制品必须来自同一构建目录，包含 Portal checksum、SPDX SBOM、provenance 和数据库 SQL 附件；回滚优先恢复迁移前备份，放量后禁止执行历史回滚 SQL。
