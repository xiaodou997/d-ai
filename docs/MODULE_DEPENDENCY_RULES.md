# 模块依赖规则

这份规则是 D-AI 模块化单体的边界契约。当前代码仍处于物理合并状态，因此规则采用“先冻结新增越界、再逐项消除历史例外”的迁移方式。`go run ./cmd/checkdeps` 会读取
[`module-dependency-exceptions.txt`](module-dependency-exceptions.txt)，对未登记的反向依赖直接失败。

## 逻辑模块职责

| 模块 | 负责的事实和用例 | 不负责的事情 |
| --- | --- | --- |
| identity | 账号、租户、会话、凭证、MFA、API Key、身份范围和资源归属 | 余额变更、上游调用、运营报表 |
| billing | 余额、不可变账本、充值/退款/提现、扣费、结算 Outbox 和资金不变量 | 认证、模型路由和 HTTP DTO |
| catalog | Provider、模型、价格簿、路由规则、租户可见性和商业目录 | 请求执行、账本写入 |
| runtime | AI 请求执行、上游协议、代理、流式响应、用量事实和运行时缓存 | 控制面配置写入、直接修改余额 |
| operations | 审计、通知、清理、调度、系统设置、观测和风控运营 | 认证主数据、账本核心写入、上游密钥明文 |

现有包到逻辑模块的迁移映射见下表。映射不是允许跨域直接导入的理由，只是 composition root
和逐域迁移时的归属记录。

| 逻辑模块 | 当前主要包 |
| --- | --- |
| identity | `auth`、`user`、`tenant`、`invite`、`clientsecret`、`ai/identitycontrol`、`ai/apikey`、`ai/workspace` |
| billing | `billing`、`payment`、`ai/billingcontrol`、`ai/billingledger`、`ai/credits`、`ai/subscription` |
| catalog | `ai/clientcatalog`、`ai/commercial`、`ai/core/catalog`、`ai/upstreamcontrol`、`ai/upstreamaccess` |
| runtime | `ai/gateway`、`ai/serving`、`ai/proxy`、`ai/routing`、`ai/core/runtime`、`ai/clientruntime` |
| operations | `announcement`、`notification`、`system`、`cleanup`、`scheduler`、`ai/audit`、`ai/observabilitycontrol`、`ai/riskcontrol` |

## 目标包形状和依赖方向

新代码按以下形状组织；迁移期间旧包可以作为兼容外壳，但不能新增对外暴露的基础设施字段：

```text
internal/<module>/
  domain/       纯业务实体、不知道数据库和 HTTP
  application/  command/query、事务编排、权限用例
  ports/        application 需要的仓储、消息、时钟和外部能力接口
  adapters/     PostgreSQL、Redis、HTTP、文件等端口实现
```

允许的方向：

```text
transport -> application/ports
application -> domain/ports
adapters -> application/domain + external infrastructure
domain/ports -> shared value types and standard library
cmd/server (composition root) -> all modules and adapters
```

禁止事项：

- `transport` 直接导入 `pgxpool`、Redis client、sqlc 生成包、`*/pg` 仓储、账本内部函数或其他 adapter。
- `domain`、`ports`、`application` 和 `ai/core` 反向导入数据库、缓存和 adapter。
- 领域包反向导入 HTTP transport；路由注册只能发生在外层。
- 任何模块绕过目标模块的 application/ports 直接写入其他模块的表。只读跨域数据也应优先通过 query service 或只读视图。

## 当前历史例外

`internal/ai/transport` 已清零 PostgreSQL、Redis、sqlc 和 adapter 包的直接依赖；
`internal/transport` 的租户自助、账户查询和管理债务路径已清零对应 adapter 依赖，仍有其他平台数据库/Redis 依赖和遗留 SQL。剩余包级越界都在例外台账登记，
并由门禁冻结；新增边缘不会因为“暂时方便”自动获得许可。例外的删除顺序是：

1. P1-02 已将 AI Core 收敛为 `CoreHTTPDeps` / `AICoreHTTPDeps` 最小端口；后续治理 `transport.Deps` 中仍残留的具体业务 service。
2. P1-03 将权限、事务和 SQL 迁移到各域 application service/query service。
3. P1-07 用数据库角色、领域 schema 或等价权限隔离验证表所有权。

## 允许的跨模块事务

以下事务是业务不变量要求的明确例外，必须由拥有事务的 application port 编排，并保留幂等键和审计证据：

1. AI 用量事实与 `bill_charge_outbox` 在同一事务落库；结算 worker 随后独立更新 billing 账本。
2. 订阅购买在接受购买决定时与 billing ledger 同事务扣款，重复请求返回同一扣款引用。
3. 充值、退款和提现在 payment 状态、充值批次、退款冲正和 cash ledger 之间保持同一事务。
4. 租户和运营报表可以读取 billing 投影，但不得修改余额、账本或结算状态。

这些例外只说明事务边界，不授权 transport 直接执行 SQL。跨域写入在 P1-03/P1-07 完成后应
收敛为 billing 暴露的 command port；跨域读取应收敛为 query port 或只读视图。

## 验证方式

本地和 CI 使用：

```sh
GOCACHE="$PWD/.cache/go-build" go run ./cmd/checkdeps
```

检查器会先运行 `go list` 获取真实直接 import 边，再按规则匹配。历史例外只允许精确的
`from -> to` 边，未登记的新边或反向 transport 依赖会使命令返回非零状态。
