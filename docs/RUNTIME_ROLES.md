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

