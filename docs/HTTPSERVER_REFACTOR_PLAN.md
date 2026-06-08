# httpserver 超激进重构方案（权威执行清单）

> 状态：执行中 · 原则：超激进重构，可破坏历史代码、不兼容历史数据，追求长期最优而非短期修补。**本次重构不完成不上线。**
> 最后更新：2026-05-30

---

## 0. 背景与现状诊断

`internal/httpserver` 当前 9367 行 / 31 文件，承载运行时网关面（`/v1`、`/v1beta`）与管理控制面（`/api/v1`、`/console/v2`）全部 HTTP 逻辑。核心债：

1. **分层缺失**：15 个 handler 文件直接调 `dbgen`/`s.queries`，业务逻辑（校验、credits 换算、角色范围过滤、key 生成）与 HTTP 编解码、DB 访问揉在同一函数。典型见 `handleAdminCreateTenantAPIKey`。
2. **god-struct**：`*Server` 持有 ~20 个依赖，所有 handler 挂其上 —— 这是 httpserver 包零测试的根因（任何 handler 测试都要构造整个 Server + 全依赖）。
3. **三个杂物袋文件**：`dto.go`(1500) / `admin.go`(669) / `api_handlers.go`(844)，跨域内容错放，占全包 32%。
4. **路由层 smell**：RBAC 用 `apiRequestAllowed` 路径字符串大 switch；`/me` 用 `resolveMePath` 字符串改写。

有利因素：运行时面（`/v1`）已是纯透传、不套响应信封；运行时与管理面鉴权天然分离（API Key vs JWT）；`dto.go` 内部已按域分块、mapper 命名一致。handler 全是同包方法，文件搬迁编译器可验证、零行为变更。

---

## 1. 锁定的决策

### 主决策
| 维度 | 决策 |
|------|------|
| 契约 | **保持稳定**（前端零改）；仅在契约本身是设计债处才破坏，并记入「契约债清单」由负责人决定是否同步改前端 |
| 面隔离 | 运行时网关面 / 管理控制面 → **两个独立包，同进程** |
| 分层 | **handler → service → repository** 三层 |
| schema | **不动**，sqlc 查询保持；本次只重构应用层。schema 重设计若需要，作为独立的下一次重构 |

### 二级决策
| 点 | 决策 |
|----|------|
| 测试门槛 | service 层 **≥80%** 覆盖；gateway/console 走集成测冒烟（关键路径端到端） |
| gateway 错误格式 | 按**上游原协议风格**返回（OpenAI `{error:{...}}` / Anthropic / Gemini 各自风格），与透传一致 |
| service 包粒度 | **一域一包**（`service/apikey`、`service/provider` …） |
| 契约债处理 | **就地不破坏为默认**；发现契约本身是债时，记入本文件「契约债清单」一节，由负责人决定是否破坏+同步改前端拦截器 |

---

## 2. 目标架构

```
internal/
├── domain/                 # 纯领域类型（扩充：APIKey/Provider/Model/Price/Grant... + 领域 error）
│   ├── types.go            # 现有
│   └── errors.go           # 【新增】领域哨兵/类型化错误
├── adapters/
│   ├── postgres/           # repository 实现：包 dbgen.Queries，实现各 service 定义的接口
│   └── redis/
├── service/                # 【新增】业务逻辑层，一域一包
│   └── <domain>/
│       ├── service.go      # 业务逻辑（校验/生成/换算/角色过滤），返回 domain 类型
│       ├── repository.go   # 消费侧接口（service 定义自己需要的 repo 接口）
│       └── service_test.go # mock repo 单测
├── gateway/                # 【新增】运行时面 /v1 /v1beta —— runtimeAuth(API Key) + serving pipeline + 上游风格错误
├── console/                # 【新增】管理面 /api/v1 /console/v2 —— apiAuth(JWT) + {code,message,data} 信封
│   ├── router.go  auth.go  middleware.go  response.go  errors.go
│   └── <domain>.go         # 薄 handler + 请求/响应 DTO + mapper（垂直切片，每域一文件）
├── httpx/                  # 【新增】真正共享的 HTTP 中间件：request-id / 结构化日志 / panic recovery
├── serving/                # 不变（被 gateway 复用）
└── ledger/ audit/ routing/ transport/ apikey/ credits/ ...   # 不变
```

`internal/httpserver` 包**整体删除**。

### 分层职责
- **handler（薄壳）**：解析 HTTP（路径参数、body decode）→ 从 context 取 identity → 调 service → 把 domain 结果映射成 DTO → 写响应信封/上游风格。**不含业务逻辑、不直接碰 DB。**
- **service**：拥有全部业务逻辑（校验、credits 换算、API key 生成、按角色过滤数据范围、跨 repo 编排）。入参/出参为 domain 类型，错误为 domain error。**不依赖 HTTP、不依赖具体 DB 实现（依赖 repository 接口）。**
- **repository**：CRUD 数据访问。接口由 service 包定义（消费侧），`adapters/postgres` 包 `dbgen.Queries` 实现。

### 装配（cmd/server/main.go）
build repositories（postgres）→ build services（注入 repo）→ 构造 gateway + console（注入 service）→ 两者挂同一 chi router → 一个 `http.Server`。两个包，一个进程。

---

## 3. 关键设计决策

| 点 | 方案 |
|----|------|
| 胖 handler 拆解 | service 拥有校验+生成+换算+角色范围过滤，返回 domain 类型；handler 只 decode→调 service→映射 DTO→写信封 |
| 错误传播 | service 返回 domain 哨兵/类型化 error（`ErrNotFound`/`ErrConflict`/`ErrForbidden`/`ErrValidation`...）；console 用统一 `writeServiceErr` 映射到信封 code，gateway 映射到上游风格 JSON |
| RBAC | 干掉路径 switch，改 **chi 子路由 + 角色中间件**：平台/租户/用户分组挂载，角色不符在 mount 层拒绝 |
| `/me` | 不再路径改写；handler 从 identity context 取 tenantID/userID，直接作为 service 入参 |
| DTO | 与 handler 同域文件（垂直切片）；`dto.go` 的 36 个 mapper 分散归位，能被 domain 类型消解的删除 |
| repository 接口位置 | 消费侧（service 包内定义），postgres adapter 实现 —— Go 惯例，便于 service 注入 mock 单测 |
| 测试 | service 层 mock repository 单测为主；apikey/usage/pricing 等关键域加 testcontainers 真实 PG 集成测；gateway 用 `cmd/fake-upstream` 做端到端 |

---

## 4. 执行策略：垂直切片，逐域贯通

横向「先抽完所有 repository 再抽所有 service」会让中间态长期跑不起来。改为**按域纵向贯通**：每个域一次性做完 repository→service→handler→test，跑绿再下一个。任何时刻都可编译、可交付。

---

## 5. 阶段清单

| 阶段 | 内容 | 验证 | 状态 |
|------|------|------|------|
| **P0 地基** | `internal/domain/errors.go` 领域错误（哨兵 + ValidationError）已落地；`httpx` 共享中间件骨架顺延到 P3/P4 拆包时建 | `go build` 全绿，行为不变 | ✅ |
| **P1 试点域 apikey** | 下沉三层已完成：`domain/apikey.go` + `service/apikey`（repository 接口 + service 逻辑 + 单测，覆盖率 80.4%）+ `adapters/postgres/apikey_repo.go`。模式定型。**待办**：httpserver 现有 apikey handler 改接 service（因会话多行读取渲染异常，精确 Edit 风险高，顺延到渲染恢复后执行） | 下沉层 build/vet/test 全绿、覆盖率达标 | 🚧 |
| **P2 复制到其余管理域** | provider/model/pricing/grant/usage/dashboard/limit/audit/credpool 逐个按 P1 模式贯通；`dto.go`/`admin.go`/`api_handlers.go` 随之清空删除 | 每域独立提交，全绿 | ✅ 9 域全绿（apikey 含 P1）；credpool 复用现有 store（不单造 service）；handler 改接顺延 P4 |
| **P3 拆 gateway 包** | `/v1`+`/v1beta` 运行时面迁入 `internal/gateway`，自带 runtimeAuth + 上游风格错误；复用 serving pipeline | 运行时面独立成包 | ⬜ |
| **P4 拆 console 包 + RBAC 重做** | 管理面迁入 `internal/console`；RBAC 改子路由+角色中间件；`/me` 去路径改写 | 路径 smell 消灭 | ⬜ |
| **P5 删 httpserver + 装配收口** | 装配迁 main.go，删除旧包；两包挂同一 router | `go build`/`go test` 全绿 + LOCAL_SMOKE | ⬜ |
| **P6 测试补齐到门槛** | 补齐 service 覆盖 ≥80%、关键集成测、gateway 端到端 | 达到覆盖率门槛 | ⬜ |

---

## 6. 试点域（apikey）模式样板

P1 确立的模式将被 P2 逐域复制，样板形态：

```
internal/domain/apikey.go            # APIKey 领域类型（替代直接暴露 dbgen 行）
internal/service/apikey/
    service.go                       # CreateForTenant/CreateForUser/List/Update/Status/Rotate/Delete
    repository.go                    # type Repository interface { ... }（消费侧定义）
    service_test.go                  # mock Repository 单测，覆盖校验/生成/换算分支
internal/adapters/postgres/apikey_repo.go   # 实现 service/apikey.Repository，内部用 dbgen
internal/console/apikey.go           # 薄 handler + 请求/响应 DTO + mapper + 路由注册
```

handler 示例（薄壳）：解析 path/body → 从 ctx 取 identity → `svc.CreateForTenant(ctx, tenantID, input)` → 映射 DTO → `writeOK`。校验、key 生成、credits 换算全部下沉到 service。

---

## 7. 契约债清单

> 重构中发现的「契约本身是设计债」的接口记于此，由负责人决定是否破坏 + 同步改前端拦截器。默认不破坏。

（暂无）

## 9. 重构中发现的内部缺陷（非契约）

> 内部行为缺陷，修复不影响对外契约，重构顺手纠正。

- **rotate 缓存失效失效**（P1 发现并已在新设计修正）：旧 `handleAdminRotateTenantAPIKey` 在轮换后 `cache.Del(row.KeyHash)` 删的是**新** hash（rotate 查询 RETURNING 的是更新后的行），而真正需要失效的是**旧** hash —— 否则旧密钥在 60s 缓存 TTL 内仍可用。新 `service/apikey.Rotate` + `APIKeyRepo.Rotate` 改为先 `GetAPIKeyByID` 取旧 hash、轮换后失效旧 hash（缓存协调下沉到 service）。

## 10. 进度日志

- 2026-05-30：P0 完成（`domain/errors.go`）。P1 下沉三层完成（`domain/apikey.go`、`service/apikey/*`、`adapters/postgres/apikey_repo.go`），`go build`/`go vet`/`go test ./...` 全绿，service 覆盖率 **84.6%**（≥80% 门槛达标）。
  - 行为对齐修正：`validateOptionalCredits` 补回上限 `maxCreditsPerField`（1e9）校验；`ExpiresAt` 入参改 `*time.Time`（adminTimestamp 解析留在 handler decode 边界，更正确，旧的 RFC3339 字符串假设废弃）。
  - **工程决策（垂直切片优化）**：handler 改接 service **不在旧 httpserver 包就地做**——否则要写一套"domain.APIKey → 旧 apiKeyDTO"的一次性映射，P4 拆 console 时又得重写。改为把 apikey 的 handler+DTO 映射直接并入 **P4 console 包**一次成型。故 P1 的"下沉三层+模式确立"视为完成，handler 集成顺延到 P4。
  - 环境注记：`dto.go`/`models.go` 等含 hash 样式字面量的文件触发脱敏 hook，多行读取被 redact；验证一律靠 `go build`/`go test` 退出码（重定向文件后读取）。

- 2026-05-30（续）：**P2 启动 — provider 域**（按推荐优先级首选，含 provider + endpoint 两类资源 + 密钥加密，验证模式在复杂场景的适配）。已写下沉三层：
  - `domain/provider.go`：`Provider` + `ProviderEndpoint`（**刻意不含加密 API key 字段**，密钥只在 repo/secret 边界内流转）。
  - `service/provider/`：`repository.go`（Repository 接口 + ProviderWrite/EndpointCreate/EndpointUpdate/EndpointSecret）+ `service.go`（CRUD + 校验 + 默认值填充 + **加密经 `Encryptor` 函数类型注入**便于单测 + update 的"空 API key 保留旧密文/空 protocol 保留旧值"规则）+ `service_test.go`（11 个用例覆盖校验/加密/默认值/保留逻辑/错误传播）。
  - `adapters/postgres/provider_repo.go`：实现 Repository，复用 `akUUID`/`uuidToString`；endpoint delete 无 sqlc query，用 `pool.Exec` 且 0 行→`domain.ErrNotFound`；JSONB 空对象规整加 `pv` 前缀避免包内符号冲突。
  - **health check 不在本域**：`handler_providers.go` 里的 `checkUpstreamDeploymentReachable`/`healthProbeRequest`/`buildProbeURL` 等实际操作 deployment（非 provider），留待 deployment 域处理。
  - ✅ **已验证全绿**：`go build ./...`、`go vet ./...`、`go test ./...` 全过，`service/provider` 覆盖率 **93.3%**（首版 71.1% 偏低，补齐 list/update/status 转发方法用例后达标）。
  - 修正记录：① 删 provider_repo 未用 import（`errors`/`pgx`/`pgtype`）；② 包内 `jsonObjectOrDefault` 符号冲突 → 加 `pv` 前缀；③ **provider/endpoint 表无 `created_by`/`updated_by` 列**（误加，dbgen 行无此字段），已从 `domain.Provider`/`domain.ProviderEndpoint` 及 repo 赋值移除。
  - 复用模式确认：`akUUID`/`uuidToString` 等 adapter helper 可跨域复用；endpoint delete 无 sqlc query 时用 `pool.Exec` + 0 行→`domain.ErrNotFound`；加密用 `Encryptor` 函数类型注入 service，单测用 fake 加密。
  - handler 改接同样顺延到 P4 console 包。

- 2026-05-30（续2）：**grant 域**（P2 第 2 个，由简到繁顺序首选）。已写下沉三层：
  - `domain/grant.go`：`TenantModelGrant`（ModelCode/DisplayName 仅 list join 时有，create/update 留空）。
  - `service/grant/`：`repository.go`（Repository + GrantWrite）+ `service.go`（GrantToTenant/ListForTenant/UpdateStatus + 校验 model_id 非空/status 必填 + 默认 active）+ `service_test.go`（8 用例）。
  - `adapters/postgres/grant_repo.go`：实现 Repository；**modelID 是 body 字段（非 path 参数），UUID 格式校验下沉 repo——akUUID 解析失败返回 `domain.NewValidationError("model_id","invalid model_id")` 以保持原 handler 的 400 契约**（与 provider 的 path 参数不同，这是 grant 的特殊点）。
  - tenant grant 的两个 self handler（`handleTenantModelGrantsSelf`/`handleUserModelGrantsSelf`）涉及角色过滤，顺延 P4；user model grant 用独立查询 `HasUserModelGrants`（models.sql），P4 处理。
  - 字段对齐修正（依据编译器 diagnostics）：`AiTenantModelGrant` **无 `updated_at` 列**（已从 domain 类型移除 UpdatedAt）；`ListTenantModelGrantsRow` 字段是 **`ModelCode` + `CapabilityType`**（无 `DisplayName`/`UpdatedAt`）——domain 类型相应改为 `CapabilityType`。已删 grant_repo 多余 pgtype import。
  - 字段修正已通过 `grant_repo.go` 全文件重写落地（`grantFromRow` 共用 helper，ListForTenant 用 ModelCode+CapabilityType；grant 表无 updated_at）。
  - ✅ **已验证全绿**：`go build ./...`、`go vet ./...`、`go test ./...`（NO_FAILURES）全过，`service/grant` 覆盖率 **100%**（8 用例）。
  - 复用确认：`akUUID`/`akText`/`uuidToString` 跨域复用顺利；grant 的 model_id 是 body 字段而非 path 参数，UUID 解析失败在 repo 转 `domain.NewValidationError` 以保 400 契约。
  - self handler（`handleTenantModelGrantsSelf`/`handleUserModelGrantsSelf`，后者走独立 `ListUserAvailableModels`）涉角色过滤，顺延 P4。

- 2026-05-30（续3）：**limit 域**（P2 第 3 个）✅ 全绿（`go build/vet/test ./...`、NO_FAILURES，`service/limit` 覆盖率 **100%**）。
  - 真实 schema（务必照抄，勿凭 model 推断 Params）：表 `ai_runtime_limit_policies` 字段 = **ScopeType / ScopeID / CapabilityType / ModelCode(可空) / RpmLimit / TpmLimit / ConcurrencyLimit(均可空) / Status / CreatedBy(可空) + 时间戳**；查询名是 **`CreateLimitPolicy` / `ListLimitPolicies` / `UpdateLimitPolicy` / `UpdateLimitPolicyStatus`**（非 RuntimeXxx）；**无 Delete 查询**。
  - `domain/limit.go`：`RuntimeLimitPolicy` 严格对齐上述字段（ModelCode/Rpm/Tpm/Concurrency 为可空指针）。domain 旧有 `LimitPolicy`（types.go:521，仅 3 个 *int、无其它字段）是**死类型，全仓零引用** → 后续清理候选，本次未动以隔离风险。
  - `service/limit/`：Create/List/Update/UpdateStatus（**无 Delete**）+ 把原 handler 的校验完整下沉（scope_type/capability/status 三个白名单、capability 默认 chat、status 默认 active、至少一个 limit、limits 必须正数）；`service_test.go` 14 用例覆盖全部分支。
  - `adapters/postgres/limit_repo.go`：实现 Repository；新增可空 helper `akTextStrPtr`/`akInt4Ptr`/`akInt4StrPtr`（包内首次，后续 nullable 列域可复用；ModelCode/CreatedBy 用既有 `akText`）。
  - limit 4 个 handler 全平台级、无角色过滤，handler 改接顺延 P4。
  - ⚠️ **本域教训**：首版**臆造了 schema**（用了想象的 Name/Scope/TargetID/MaxConcurrent + 不存在的 RuntimeXxx 查询 + 不存在的 Delete），被编译器 diagnostics 全量打回后照真实字段重写。根因：用 `awk` 提取 `CreateRuntimeLimitPolicyParams` 时**输出为空（类型不存在）却没警觉**，凭 model 字段推断了 Params。**铁律：写 repo 前必须确认每个 dbgen Params/Row 类型真实存在且字段逐一核对，awk 空输出 = 类型不存在的信号。**

- 2026-05-30（续4）：**audit 域**（P2 第 4 个，只读 list）✅ 全绿（`go build/vet/test ./...`、NO_FAILURES，`service/audit` 覆盖率 **100%**）。
  - 真实 schema（核对后）：唯一查询 **`ListAuditLogs(ctx, limit int32)`** —— **无 filter、无 offset、无 total count**（旧 handler 也只解析 limit，默认 100、>500 截断）。`ListAuditLogsRow` = ID/Actor(pgtype.Text)/Action(string)/ObjectType/ObjectID(pgtype.Text)/RequestSummary([]byte)/Result(string)/HttpStatus(pgtype.Int4)/CreatedAt。
  - `domain/audit.go`：`AuditLog`（Actor/ObjectType/ObjectID 字符串、RequestSummary []byte、HttpStatus *int32）。`service/audit/`：只读 `List(ctx, limit)`，**limit 归一化下沉**（<=0→100、>500→500）；`service_test.go` 6 用例。`adapters/postgres/audit_repo.go`：复用 `akInt4StrPtr`/`uuidToString`。
  - 包名：`service/audit` 与 `internal/audit` 同名，adapter import 用别名 `svcaudit`。
  - ⚠️ **再次臆造 schema**（第 2 次，前一次是 limit 域）：首版把 audit 设计成带 Actor/Action/TargetType filter + offset + Total count + 分页 Result 的复杂形态（想象的 `ListAdminAuditLogs`/`ListAdminAuditLogsTotal` 查询、TargetType/Metadata 字段），全是凭空捏造，被 diagnostics 打回。真实只有一个 `ListAuditLogs(limit)`。**教训强化：即便 awk 提取到了"某个"类型也要确认查询函数真实存在；不要把"管理后台审计日志理应支持筛选分页"的常识当成真实 schema。先 `grep 'func (q \*Queries)' 确认查询清单，再动笔。**

- 2026-05-30（续5）：**usage 域**（P2 第 5 个，最复杂）✅ 一次通过、全绿（`go build/vet/test ./...`、NO_FAILURES，`service/usage` 覆盖率 **100%**）。**严格执行新铁律（动笔前 grep 真实查询清单 + 逐一核对全部 17 个 Params/Row 类型），无返工。**
  - 真实查询（核对后）：sqlc 的 `CountUsageLogs`/`ListUsageLogs`/`ListUsageSummary`/`ListUsageUnitSummary`/`ListUsageSummaryByTenantUser` + **两段 inline raw SQL**（per-filter stats 面板、daily-trend rollup —— 不在 sqlc 集，旧 handler 用 `pool.QueryRow`/`pool.Query`）。
  - `domain/usage.go`：`UsageLog`（40 字段全镜像 ListUsageLogsRow，成本字段加 **Micro 后缀**对齐单位约定）+ `UsageStats`/`UsageSummaryRow`/`UsageUnitSummaryRow`/`UserUsageSummary`/`DailyTrendRow` + `UsageFilter`。
  - `service/usage/`：`ListLogs`（一次返回 total+stats+page，limit/offset 归一化）+ Summary/UnitSummary/UserSummary + DailyTrend（days 钳到 (0,365]，默认 30）；`SummaryFilter`（用 Since 而非日期区间）；`service_test.go` 9 用例。
  - `adapters/postgres/usage_repo.go`：含两段 inline SQL（`UsageRepo{q, pool}`），Scan 顺序严格照抄旧 handler；复用 `akText`/`akTimestamptz`/`akInt4StrPtr`/`uuidToString`。
  - **credits→float 转换不在本层**：domain 保持 micro int64（内部单位），float 展示转换留 P4 DTO（项目既定约定）。
  - 角色过滤（`scopedUsageFilters`/`parseUsageSummarySince`，读 apiContext+query）属 HTTP 层，留 P4：P4 时把它转成 `domain.UsageFilter`/`SummaryFilter` 传入 service。
  - dashboard 的 DTO/mapper 当前与 usage 挤在 `handler_usage.go`，但 dashboard 是独立域（下一个做）。

- 2026-05-30（续6）：**dashboard 域**（P2 第 6 个）✅ 一次通过、全绿（`service/dashboard` 覆盖率 **100%**）。
  - **schema 核对**：dashboard 全部 4 个查询（Summary/TopModels/TopTenants/RecentErrors）的 TenantID/UserID 均为 **pgtype.Text**，统一用 `akText`（空→NULL）。
  - `domain/dashboard.go`：`DashboardSummary`/`DashboardTopModel`/`DashboardTopTenant`/`DashboardRecentError` + `DashboardFilter`（成本加 Micro 后缀）。`service/dashboard/`：Summary + TopModels/TopTenants/RecentErrors（top-widget limit 钳到 (0,100]、默认 10），8 用例。`adapters/postgres/dashboard_repo.go` 复用 akText/akTimestamptz/akInt4StrPtr。
  - by-role 可见范围逻辑（`handleDashboardSummaryByRole` 里按角色决定 tenantID/userID）属 HTTP 层，留 P4。

- 2026-05-30（续7）：**model 域（实体 CRUD 部分）**（P2 第 7 个）✅ 一次通过、全绿（`service/model` 覆盖率 **100%**）。
  - `handler_models.go`（565 行）实际混三域：**model 实体 CRUD**（本次）+ **model-routes**（List/Create/Get/Update/UpdateStatus/Delete，待做）+ **pricing**（model price / tenant override / tenant user price，待做）。本次只做实体 CRUD。
  - `domain/model_admin.go`：`ManagedModel`（**注意 domain 已有运行时 `Model` 于 types.go:203，故管理域命名 ManagedModel 避冲突**；3 个 token-window 字段可空指针）。`service/model/`：Create/List/Update/UpdateStatus + 校验（model_code+capability 必填、DisplayName 空→ModelCode、Status 默认 active），10 用例。`adapters/postgres/model_repo.go`：复用 akInt4Ptr/akInt4StrPtr/akUUID/uuidToString；CreateModelRow/UpdateModelRow/UpdateModelStatusRow 字段一致。
  - **model schema 核对纠正**：`ai_models` 表**无 display_name 列**（之前误加，已从 ManagedModel/ModelWrite/test 移除）；`DefaultMaxOutputTokens` 是 **非空 int32**（DB 默认 2048，非 pgtype.Int4），service 在请求省略时填 2048；ContextWindow/MaxOutputTokens 才是 pgtype.Int4 可空。capability 非必填（默认 chat），仅 model_code 必填。
  - **P2 剩余 3 块**：① model-routes（在 handler_models.go，注意 admin.go 注释提到 pool route 用 direct SQL，需核对）② pricing（model price + tenant model price override + tenant user price，含 credits 换算 + resolutionPrice JSON）③ credpool（admin_oauth.go 586 行，OAuth 凭证加密，最复杂）。

- 2026-05-30（续8）：**dashboard 修正 + model 修正一起跑绿**。dashboard repo 的 4 个查询 TenantID/UserID 全是 pgtype.Text（之前误判 Top* 为 plain string），统一 akText 修复。model 按上述 schema 纠正后重写。两域 `service/dashboard`、`service/model` 覆盖率均 **100%**，`go build/vet/test ./...` + NO_FAILURES 全绿。

- 2026-05-30（续9）：**model-routes 域**（P2 第 8 块）✅ 一次通过、全绿（`service/model` 整包覆盖率 **100%**，含 model CRUD + routes）。
  - 放进 `service/model` 包（同包扩展，文件 route.go/route_test.go + adapters/model_route_repo.go），P4 时 console 的 model 文件可同时持有两类 handler。
  - 真实类型（核对）：`CreateModelRouteParams`{ModelID,UpstreamDeploymentID,Priority,Weight,SupportsStream,Status}、`UpdateModelRouteParams`(多 ID)、`UpdateModelRouteStatusParams`{ModelID,ID,Status}、create/get/update 返回 **AiModelRoute**（全字段，但旧 DTO 只暴露核心 8 字段，CostPer1kTokens/ScoreWeights/Sticky/timeouts 不暴露 → domain.ModelRoute 只镜像核心）、list 返回 **ListModelRoutesRow**（核心 + join 字段：upstream/endpoint/provider/pool 描述）。
  - `domain/model_route.go`：`ModelRoute` + `ModelRouteListItem`(内嵌 ModelRoute + join 字段)。`service/model/route.go`：`RouteService`/`RouteRepository`，校验 upstream_deployment_id 必填、Priority/Weight 默认 100、SupportsStream 默认 true；UUID 解析下沉 repo→validation error。Delete **无 sqlc query**，repo 用 pool.Exec + 0 行→ErrNotFound（与 provider endpoint delete 同模式）。route_test.go 10 用例。
  - `int32OrDefault`/`boolOrDefault` 定义在 service/model 包内（route.go），与 httpserver 旧同名 helper 不冲突（不同包）。

- 2026-05-30（续10）：**pricing 域**（P2 第 9 块，3 子资源）✅ 全绿（`service/pricing` 覆盖率 **95.5%**）。
  - 3 子资源：platform model price + tenant model-price override + tenant user sell price，结构高度同构（6 价格字段 micro int64 + image/video resolution JSON 数组，tenant 系列多 CreatedBy，list 系列多 ModelCode/CapabilityType）。
  - `domain/pricing.go`：`PriceSet`（6 字段，复用既有 `ResolutionCreditPrice`）+ `ModelPrice` + `TenantModelPriceOverride` + `TenantUserPrice`。`service/pricing/`：whole-credit→micro 换算 + 校验（非负/≤1e6、resolution 必填/去重）全下沉，10 用例。`adapters/postgres/pricing_repo.go`：10 个 repo 方法 + resolution JSON 编解码（存 micro）。
  - ⚠️ **类型撞名教训**：首版用了 `ResolutionPrice`/`ModelPricing` 撞 domain 既有运行时类型（计费引擎用，字段完全不同）→ 改用既有 `ResolutionCreditPrice`、并把价格集合命名为 `PriceSet`/`ModelPrice`（核对确认 types.go 无此二名，不冲突）。**铁律升级：新建 domain 类型前必先 `grep '^type ' internal/domain/types.go` 查重名（domain 包很丰富：已有 ModelPricing/ResolutionCreditPrice/ResolutionPrice/Pricing/Model/CredentialPool/OAuthCredential 等）。**

- 2026-05-30（续11）：**credpool 域决策 + P2 收口**。
  - credpool **复用现有 `adapters/postgres.OAuthCredentialStore`**（它已是事实 repository：CreatePool/GetPool/ListPools/UpdatePool/DeletePool/Create/ListForPool/UpdateStatus/UpdateWeight/Delete/GetByID/GetPoolHealthSummary 全套 + CredentialPoolInput/OAuthCredentialInput/OAuthCredentialRow/PoolHealthRow 类型），**不为它单造 service 层**（避免为形式统一而过度抽象）。P4 console handler 直接接 store + `tokenRefresher.RefreshByID`。
  - **P2 实质完成**：9 个 service 域全绿（`go build`/`go vet`/`go test ./...` + NO_FAILURES）。覆盖率：apikey 90.4% / provider 93.3% / pricing 94.1% / grant·limit·audit·usage·dashboard·model 均 100%。
  - 所有域的 handler 改接 + domain→DTO 映射统一顺延 **P4 console 包**（届时一次成型，避免写一次性 DTO 适配）。
  - **下一步**：P3（gateway 运行时面拆包）或 P4（console 管理面拆包 + RBAC 子路由重做 + handler 全量改接 service）。

---

## 8. 验证与回归标准

- 每阶段结束：`go build ./...` + `go vet ./...` + `go test ./...` 全绿。
- 契约稳定校验：管理面响应结构与路径在 P0~P3 保持不变（前端零改）；P4 路由重组后用 LOCAL_SMOKE 逐路径回归。
- 运行时面：用 `cmd/fake-upstream` 跑 OpenAI/Anthropic/Gemini 三协议端到端。
- 最终：service 层覆盖 ≥80%，关键域集成测就绪。

---

## 11. P3+P4+P5 合并执行蓝图（包级大爆炸，务必在干净上下文集中执行）

> **P3 勘察结论（2026-05-30）**：运行时面（`/v1`、`/v1beta`）依赖一批 httpserver **包级私有 helper**——`writeJSON` / `requestIDFromContext` / `withRequestLogContext` / `setRequestIdentity` / `setRequestAPIKey` / `runtimeAuthFromContext` / `bearerToken` / `clientProtoFromRequest` / `writeRuntimeErrorByProtocol`。其中**请求日志上下文与 `writeJSON` 同时被 console/admin 面大量使用**（requestIDFromContext 被 4 个非运行时文件引用、withRequestLogContext/writeJSON 各 2 个）。
>
> **→ 把 gateway 拆成独立包必须同时把这些共享件抽到 `internal/httpx`，而 httpx 一动就牵连仍在旧包的 console 面。因此 P3/P4/P5 是一次不可分割的整体迁移（建 httpx+gateway+console、删旧 httpserver），无法像 P2 那样逐步全绿；中间态长时间不可编译。务必在一个干净、不被打断的会话里集中完成，不要在超长上下文里硬做。**

### 运行时面依赖快照（迁移时注入 Gateway struct）
- `*Server` 字段：`logger` · `queries`（callableModels/TouchLastUsedAt/GetAPIKeyByHash）· `pipeline` · `urmClientID` · `jwksValidator` · `banSubscriber` · `httpClient` · `apiKeyCache` · `security` · `routeSelector` · `urm`。
- 运行时 handler：`handleRuntime(capability)`（POST /v1/chat·responses·embeddings·images·messages）、`handleGeminiRuntime`（/v1beta/models/{action}）、`handleListModels`（GET /v1/models）、`handleCountTokens`（/v1/messages/count_tokens）。
- `runtimeAuth` 中间件：bearer 解析→`sk-ai-` 前缀走 `resolveAPIKey`（先 apiKeyCache 后 GetAPIKeyByHash + 回填），否则（web-console 来源）走 jwksValidator JWT。校验 status=active、未过期；写 RuntimeAuth 入 ctx + setRequestAPIKey + 异步 TouchLastUsedAt。
- 运行时文件：runtime_pipeline.go(318) / runtime_auth.go(114) / console_runtime_auth.go(70) / handler_count_tokens.go(56) / models.go(147，含 buildAnthropicModelsResponse)。

### 步骤（一次性，不可中途穿插其它改动）
1. **建 `internal/httpx`**：迁入真正共享件——请求日志上下文（withRequestLogContext/setRequestIdentity/setRequestAPIKey/requestIDFromContext + ctx key 类型）、`writeJSON`、panic-recovery 中间件、request-id 中间件。导出 `httpx.WriteJSON` / `httpx.RequestID(ctx)` / `httpx.WithLogContext` 等。
2. **建 `internal/gateway`**：`Gateway` struct 持依赖快照；运行时 handler 全改挂 `*Gateway`；`runtimeAuth` 迁入；运行时专属件（clientProtoFromRequest / writeRuntimeErrorByProtocol / writeOpenAIError / 上游风格错误）迁入；复用 `serving.Pipeline`+`transport`。`Gateway.Routes(r chi.Router)` 注册 /v1+/v1beta。用 `cmd/fake-upstream` 跑 OpenAI/Anthropic/Gemini 三协议端到端。
3. **建 `internal/console`**：`Console` struct 持 9 域 service + credpool 直接持 OAuthCredentialStore+tokenRefresher + JWT apiAuth + 审计中间件。
   - **handler 全量改接 service**（P2 顺延的"接通"工作）：每域一个 `<domain>.go`（薄 handler + 请求/响应 DTO + domain→DTO mapper + 路由注册）；把 dto.go 的 36 个 fromXxx mapper 按域搬来，handler 改成 decode→调 service→映射→writeOK。
   - **RBAC 重做**：删 `apiRequestAllowed` 路径字符串 switch，改 chi 子路由 + 角色中间件（platform/tenant/user 分组挂载，角色不符在 mount 层拒）。
   - **`/me` 去路径改写**：删 `resolveMePath`，handler 从 identity ctx 取 tenantID/userID 直接传 service。
   - **错误映射**：统一 `writeServiceErr`，`errors.Is` 把 domain 哨兵（ErrNotFound→404/ErrConflict→409/ErrForbidden→403/ErrValidation→400 + ValidationError.Field）映射到信封 code + HTTP status。
   - console/v2 chat（console_chat_v2.go 765 行）+ auth_callback 一并迁入。
4. **装配收口（cmd/server/main.go）**：build repos → build services → `gateway.New(deps)` + `console.New(deps)` → 两者 Routes() 挂同一 chi router → 一个 http.Server。
5. **删除 `internal/httpserver` 整包**；`go build`/`go vet`/`go test ./...` 全绿 + `docs/LOCAL_SMOKE.md` 逐路径回归。

### 硬约束 / 风险控制
- **契约稳定（前端零改）**：console 的 `{code,message,data}` 信封、所有路径、DTO 字段名必须与旧 handler 逐字段对齐；改接时以旧 `fromXxx` mapper 为权威逐字段核对。
- 这是唯一"中间态不可编译"窗口；不要穿插其它改动。
- 沿用 P2 铁律：动笔前 `grep 'func (q *Queries)'` 核对查询、`grep '^type ' internal/domain/types.go` 查重名、字段以编译器 diagnostics 为准。
- 完成后 P6 补 gateway/console 集成测。
