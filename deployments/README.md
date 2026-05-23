# Deployments

## Nginx

- `nginx/uni-ai-api.conf` — AI 三个前端域名的生产 nginx 配置。

域名与静态目录：

| 域名 | 前端 | 静态目录 |
|---|---|---|
| `ai.admin.ainm.store` | `ai-admin` | `/opt/html/ai-admin` |
| `ai.tenant.ainm.store` | `ai-tenant` | `/opt/html/ai-tenant` |
| `ai.ainm.store` | `ai-customer` | `/opt/html/ai-customer` |

配置默认后端 upstream：

| upstream | 目标 |
|---|---|
| `uni_ai_api_backend` | `ai-service:13010` |
| `urm_backend` | `urm-service:6900` |

如果线上容器名或端口不同，只需要调整 `nginx/uni-ai-api.conf` 顶部的 upstream。
