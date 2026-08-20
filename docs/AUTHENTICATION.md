# 认证与会话设计

## 凭证模型

- Access Token 是 RS256 JWT，默认有效期 15 分钟。
- Access Token 必须包含 `sid` 和 `credential_version`；不带 `sid` 的旧 JWT 会被拒绝。
- Access Token 验签后还会校验数据库会话、凭证版本、账号和租户状态，撤销不依赖 Redis 可用性。
- Refresh Token 是带 `dai_rt_` 前缀的 256 bit 随机不透明凭证，不是 JWT。
- 数据库只保存 Refresh Token 的 SHA-256 哈希，不保存可直接使用的明文。
- Refresh Token 的绝对有效期由 `jwt.refresh_expiration` / `DAI_JWT_REFRESH_EXPIRATION` 配置，轮换不会延长 session family 的绝对过期时间。

## 生命周期

登录时创建一条 `auth_sessions`，并在 `auth_refresh_tokens` 中保存首个 Refresh Token 哈希。
每次刷新都在单个 PostgreSQL 事务内完成：锁定令牌和会话、重新读取账号及租户状态、
校验凭证版本、消费旧哈希并写入新哈希。并发刷新同一凭证时只能有一个事务成功。

已经消费的 Refresh Token 再次出现会被视为重放，整个 session family 会立即撤销，
包括该 family 最新签发的 Refresh Token。客户端收到刷新失败后必须清空本地凭证并重新登录。

登出撤销 Access Token 的 JTI，并撤销 `sid` 对应的当前 session family。修改或重置密码时，
`iam_accounts.credential_version` 原子递增；数据库触发器会撤销该账号的全部 session。
账号停用、级联停用和软删除同样触发全部会话撤销；硬删除通过外键级联删除会话。
租户状态在每次刷新时重新校验，停用或暂停后不能继续刷新。

## HTTP 契约

`POST /api/auth/login` 和 `POST /api/auth/refresh` 返回：

```json
{
  "accessToken": "eyJ...",
  "refreshToken": "dai_rt_...",
  "expiresIn": 900,
  "refreshExpiresIn": 604800
}
```

`refreshExpiresIn` 是当前 session family 距绝对过期的剩余秒数，刷新后只会减少。
当前版本仍通过 JSON body 传递 Refresh Token；迁移到 HttpOnly Cookie 属于 `P0-04`，
不会混入本次服务端生命周期重构。

## 数据库升级

已有 schema 10 数据库必须在部署新二进制前人工执行
`internal/db/changes/0011_20260820_auth_sessions.sql`。应用要求 schema 11，且不会自动执行 DDL。
