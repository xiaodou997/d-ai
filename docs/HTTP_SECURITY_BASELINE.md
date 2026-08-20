# HTTP 安全基线

D-AI 的业务监听和管理监听分离。业务监听默认 `:19641`，只提供 Portal、API、runtime 和公开存活检查 `/health`；管理监听默认 `127.0.0.1:19642`，提供 `/ready`、`/metrics` 和其他运维探针，不应直接暴露到公网。

业务响应统一包含 CSP、`X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、严格 Referrer-Policy、Permissions-Policy 和 COOP。生产环境额外发送一年期 HSTS；本地 HTTP 开发不发送 HSTS。

缓存策略按内容边界处理：

- Portal `index.html` 和 API/runtime/OpenAPI 响应使用 `Cache-Control: no-store`。
- Vite hash 资源 `/assets/*` 使用 `public, max-age=31536000, immutable`。
- 其他嵌入静态资源使用有限的公共缓存时间。
- capability 文件、运行时图片和其他私有内容使用 `private, no-store`；明确版本化的公共 favicon 可以覆盖为公共缓存。

业务服务端默认限制单请求 body 为 64 MiB、请求 header 为 32 KiB。上传和支付回调仍有更严格的路由级限制。AI 流式响应保持 `WriteTimeout=0`，由 serving 层的 response-header、first-byte、idle-gap 和 max-duration deadline 控制，避免慢客户端或上游无限占用连接。

Huma 交互式 `/docs` 和实时 `/openapi.json` 调试路由默认不在运行时监听器注册；契约通过仓库中的 OpenAPI 导出流程发布，避免把完整管理面暴露给公网。

如果必须让 Prometheus 或探针从容器外访问管理监听，应通过内网、网络策略或认证反向代理暴露，并保持 `DAI_SERVER_MANAGEMENT_ADDR` 不直接绑定公网网卡。
