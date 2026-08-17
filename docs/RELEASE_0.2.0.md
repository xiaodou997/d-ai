# 0.2.0 上线手册（schema 2 → 3）

本次发布把账户余额从「2 张表 3 个非负列」收敛为 `bill_accounts.balance_micro`
一个有符号整数，并把 AI 扣费改为经 `bill_charge_outbox` 异步结算。

**这是一次会改结构、动余额的发布，必须在维护窗口内执行。**

- 迁移脚本：`internal/db/changes/0003_20260817_signed_balance_ledger.sql`
  （发布附件位于 `release/sql/changes/`）
- 应用要求 schema 版本 `3`，版本不符会拒绝启动。
- 0.1.x 应用**不能**运行在 schema 3 上；0.2.0 也不能运行在 schema 2 上。
  所以数据库和应用必须在同一个窗口内一起切换。

---

## 一、上线前准备

- [ ] **完整备份数据库**，并确认备份可恢复（这是唯一无条件可靠的回滚手段）
- [ ] 记录当前应用版本与 schema 版本
      `SELECT version FROM dai_schema_metadata WHERE singleton = TRUE;` → 应为 `2`
- [ ] 准备好 0.2.0 二进制与 `release/sql/`
- [ ] 确认 Redis 可用（准入限流依赖它；不可用时准入会 fail-closed 拒绝请求）
- [ ] **保存迁移前对账基线**，执行下面的 SQL 并把结果存档：

```sql
SELECT t.tenant_id AS account_id, 1 AS kind,
       COALESCE((SELECT SUM(p.remaining_credits) FROM bill_credit_packages p
                 WHERE p.package_type = 'tenant' AND p.tenant_id = t.tenant_id
                   AND p.status = 'available'
                   AND (p.expires_at IS NULL OR p.expires_at > now())), 0)
       - COALESCE(t.current_overdraft, 0) AS balance_micro
FROM iam_tenants t
UNION ALL
SELECT a.user_id, 2,
       COALESCE((SELECT SUM(p.remaining_credits) FROM bill_credit_packages p
                 WHERE p.package_type = 'user' AND p.user_id = a.user_id
                   AND p.status = 'available'
                   AND (p.expires_at IS NULL OR p.expires_at > now())), 0)
       - COALESCE(a.current_overdraft, 0)
FROM iam_accounts a WHERE a.user_type = 4
ORDER BY 2, 1;
```

---

## 二、执行

- [ ] **停止全部 0.1.x 应用实例**（必须先停应用再迁移：旧代码会写已被删除的列）
- [ ] 确认没有在途写入：`SELECT count(*) FROM pg_stat_activity WHERE datname = 'dai' AND state = 'active';`
- [ ] 执行迁移

  ```bash
  psql -v ON_ERROR_STOP=1 -U <user> -d dai -f release/sql/changes/0003_20260817_signed_balance_ledger.sql
  ```

  脚本会在整个过程中持有 advisory lock 并在单个事务内完成，失败自动整体回滚。
  若输出 `NOTICE: N 个账户在旧模型下同时持有可用余额和欠费`，记下 N，
  这些账户在旧模型下处于不可能状态，已按**净额**迁移，建议事后人工复核。

- [ ] **对账**：执行下面的 SQL，与第一步存档的基线**逐行比对，必须完全一致**

  ```sql
  SELECT account_id, account_kind AS kind, balance_micro
  FROM bill_accounts ORDER BY account_kind, account_id;
  ```

  ```sql
  -- 不变量：可花余额 == 活跃批次未消耗之和
  SELECT b.account_id, b.balance_micro,
         COALESCE(SUM(l.granted_micro - l.consumed_micro), 0) AS lots
  FROM bill_accounts b
  LEFT JOIN bill_credit_lots l
    ON l.account_id = b.account_id AND l.expired_at IS NULL AND l.revoked_at IS NULL
  GROUP BY b.account_id, b.balance_micro
  HAVING GREATEST(b.balance_micro, 0) <> COALESCE(SUM(l.granted_micro - l.consumed_micro), 0);
  -- 期望返回 0 行
  ```

- [ ] **对账不一致就地停止，走「窗口内回滚」，不要放流量进来**
- [ ] 启动 0.2.0，确认日志出现 `database schema verified {"version": 3}`
- [ ] `curl /health` 与 `curl /ready` 均为 `ok`

---

## 三、放量后观察

结算改为异步后，**余额前进完全依赖后台 outbox 消费者**。它停摆时用量照常记录、
请求照常放行，而余额不再变化——所以这几项必须确认：

- [ ] `curl /metrics | grep dai_billing_outbox`

  | 指标 | 期望 |
  | --- | --- |
  | `dai_billing_outbox_applied_total` | 随请求增长 |
  | `dai_billing_outbox_pending` | 接近 0，短暂尖峰正常 |
  | `dai_billing_outbox_failed` | **恒为 0**；非 0 表示有扣费永久失败，是收不到的钱 |
  | `dai_billing_outbox_oldest_pending_seconds` | 通常 < 2；持续增长表示结算停摆 |

- [ ] 建议告警：`failed > 0`，或 `oldest_pending_seconds > 120` 持续 5 分钟
      （后者应用侧也会打 error 日志：`billing settlement is falling behind`）
- [ ] 冒烟一次真实计费请求，确认余额确实下降：

  ```sql
  SELECT request_id, status, attempts FROM bill_charge_outbox ORDER BY id DESC LIMIT 5;
  SELECT account_id, balance_micro FROM bill_accounts WHERE account_id = '<被测账户>';
  ```

- [ ] 确认负余额账户被拒绝：对余额 `<= 0` 的账户发起请求应返回
      `402 insufficient_balance`
- [ ] 核对平台大盘、租户视图、用户账户页三处余额数字一致（此前它们互相矛盾）

### 需要知道的行为变化

| | 变化 |
| --- | --- |
| 结算时机 | 由同步变为**秒级异步**；`ai_usage_logs.billing_status` 先 `pending` 后 `confirmed` |
| 并发上限 | 新增默认单账户在途上限 **32**（`runtime.default_in_flight_per_account`）。已在 `ai_runtime_limit_policies` 显式配置的 scope 用配置值。若有高并发 API 客户被 429，调高该值而不是设为 0 |
| 停用账号 | 停用租户/用户不再冻结其额度；访问由 BanChecker 拦截，欠费仍会被正常结算 |
| 额度过期 | 由查询时过滤改为 scheduler 每 5 分钟一次真实扣减 |
| 信用额度 | `overdraft_limit` 概念取消，管理端调额接口已移除 |

---

## 四、回滚

### 情况 A：仍在维护窗口内，流量尚未恢复

使用下面的脚本，**已验证可无损往返**（余额逐账户一致，列结构与 schema 2 基线完全相同）。
脚本自带保护：一旦检测到 `bill_charge_outbox` 有记录（说明流量已恢复），会直接报错拒绝执行。

```bash
psql -v ON_ERROR_STOP=1 -U <user> -d dai -f release/sql/rollback/0003_rollback.sql
# 随后启动 0.1.3
```

脚本随发布附件一起分发（源文件 `internal/db/rollback/0003_rollback.sql`），
不要从文档复制粘贴。

### 情况 B：流量已恢复

**不要使用上面的脚本。** 迁移后产生的余额变动无法映射回旧的两列表示，
强行回滚会丢账或算错。此时唯一正确的做法是：

1. 停止应用
2. 从上线前的备份恢复数据库
3. 启动 0.1.3
4. 用 `bill_events` 与上游账单人工核对窗口内产生的消费，必要时补记

因为情况 B 代价高得多，**第二步的对账必须在放量之前做完**——那是成本最低的
决策点。

---
