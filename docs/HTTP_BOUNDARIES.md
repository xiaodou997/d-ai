# HTTP 公共边界

## 公共 origin

生产环境必须设置 `DAI_PUBLIC_BASE_URL`。它必须是没有路径、查询参数或片段的绝对
`https://` origin，例如 `https://portal.example.com`。注册链接、法律链接、文件
capability URL 和平台生成的图片 capability URL 都只使用这个固定 origin，不再从请求
的 `Host` 头拼接公网地址。

开发环境可以留空，此时没有可信公共 origin 的后台调用会保留相对路径，便于本地 HTTP
运行；需要模拟反向代理时可设置同一配置。

## 可信代理和客户端 IP

`DAI_TRUSTED_PROXY_CIDRS` 使用逗号或空格分隔的 CIDR 列表。只有直接 TCP 对端落在这些
网段内时，服务才读取 `X-Forwarded-Host`、`X-Forwarded-Proto`、`X-Forwarded-For`、
`X-Real-IP` 或 RFC 7239 `Forwarded`。直连或非可信来源提交的这些头全部视为用户输入并
忽略。

多级 `X-Forwarded-For` 从最靠近服务端的一侧向外检查，取第一个不在可信网段内的地址。
同一解析结果同时用于访问日志、登录限速、AI 用量记录和风控，避免各链路看到不同的
客户端身份。

反向代理必须覆盖或清理入站 forwarded 头，并把自身网段配置到 `DAI_TRUSTED_PROXY_CIDRS`。
如果代理拓扑变化，应同步更新配置和伪造头回归测试。
