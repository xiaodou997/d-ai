# 认证与会话设计

## 凭证模型

- Access Token 是 RS256 JWT，默认有效期 15 分钟。
- Access Token 必须包含 `sid` 和 `credential_version`；不带 `sid` 的旧 JWT 会被拒绝。
- Access Token 验签后还会校验数据库会话、凭证版本、账号和租户状态，撤销不依赖 Redis 可用性。
- Refresh Token 是带 `dai_rt_` 前缀的 256 bit 随机不透明凭证，不是 JWT。
- 数据库只保存 Refresh Token 的 SHA-256 哈希，不保存可直接使用的明文。
- Refresh Token 的绝对有效期由 `jwt.refresh_expiration` / `DAI_JWT_REFRESH_EXPIRATION` 配置，轮换不会延长 session family 的绝对过期时间。
- 新建账号和密码重置后的账号使用 `pending_activation` 凭证状态，不能创建、刷新或继续使用登录会话。
- 激活令牌是带 `dai_act_` 前缀的 256 bit 随机不透明凭证；数据库只保存 SHA-256 哈希，默认有效期由 `auth.activation_expiration` / `DAI_AUTH_ACTIVATION_EXPIRATION` 配置。
- 登录按账号标识、来源 IP、账号-IP 组合和租户-IP 组合使用 Redis 原子计数；失败达到阈值后采用指数退避。Redis 不可用时登录直接 fail-closed，并写入结构化认证审计事件。
- 平台管理员可在个人中心注册 TOTP MFA。启用后密码登录只返回短时一次性 MFA 挑战，验证码验证成功后才签发 session；敏感管理操作还要求 10 分钟内完成过密码或 MFA 近期认证。
- 修改密码、注册/确认 MFA 等高影响账号操作同样要求近期认证；近期认证标记在 Redis 不可用时拒绝放行。

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

账号创建和重置会生成单次激活令牌，同一账号始终只有最新令牌可用。激活在事务中
消费令牌、写入正式密码、切换为 `active` 凭证状态并提升凭证版本。账号或租户即使
处于停用状态也可以完成凭证设置，但登录和会话检查仍会按业务状态拒绝访问。

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

`GET /api/auth/password-policy` 返回 Portal 展示和校验使用的统一密码策略；
`POST /api/auth/activate` 消费一次性令牌并设置正式密码。Portal 激活链接把令牌放在
URL fragment 中，避免令牌发送给 HTTP 服务或反向代理，读取后立即清空地址栏。

管理员 MFA 使用 `POST /api/auth/mfa/enroll`、`POST /api/auth/mfa/confirm` 和
`POST /api/auth/mfa/verify`；TOTP 密钥以应用主密钥加密后写入账号表，数据库不保存可直接
使用的明文密钥。敏感操作在近期认证过期后返回重新认证提示，可通过
`POST /api/auth/recent-auth` 使用当前密码（已启用 MFA 的管理员还需验证码）恢复权限。

## 数据库升级

已有 schema 10 数据库必须在部署新二进制前依次人工执行：

1. `internal/db/changes/0011_20260820_auth_sessions.sql`
2. `internal/db/changes/0012_20260820_account_activation.sql`
3. `internal/db/changes/0013_20260820_admin_mfa.sql`

已有 schema 11 数据库执行第二、三项；已有 schema 12 数据库只执行第三项。应用要求
schema 13，且不会自动执行 DDL。
