# 定价体系超激进重构方案（Price Book 统一定价）

> 状态：设计已定（2026-06-08），待执行。授权：超激进、不兼容历史数据/代码、全删重建。
> 决策来源：与用户逐项确认，全部采用推荐方案。

## 0. 目标与核心思想

把"每个 deployment 各填一份价、对外售价散落三张表"改成 **一套 USD 价格表（Price Book）+ 各处引用时只设一个倍率**：

- 价格集中维护，改一处到处生效。
- 上游成本与对外售价 **共用同一套价格表目录**（成本=价×成本倍率，售价=价×售价倍率→换算积分）。
- 价格表 USD/token，与 LiteLLM / sub2api 1:1，可从 LiteLLM JSON 自动填充、手改覆盖。
- 对外计费用积分；USD→积分汇率为 DB 全局设置（v1 默认 7），可动态调。
- 缓存单独计价用开关控制；关=按 input 价，开=用 cache 价。
- 订阅套餐为二期，本期不做。

## 1. 关键决策（已确认）

| # | 决策 | 选择 |
|---|------|------|
| 1 | 成本与售价目录 | **共用一套 Price Book** |
| 2 | USD→积分汇率 | **DB 全局设置表**，仅在对外计费换算；上游成本保持 USD（核算时换算微积分） |
| 3 | 分层定价 | **v1 扁平单价**，schema 预留区间能力 |
| 4 | 订阅套餐 | **二期**，本期只做 token 定价 |
| 5 | 售价倍率粒度 | **一个绑定一个倍率**（覆盖该表所有模型），需要时再加每模型覆盖 |
| 6 | 租户→用户基价 | **级联**：基准=平台给该租户的售价，租户再×自己倍率 |
| 7 | 缓存价开关 | **每个售价绑定一个开关**（平台→租户、租户→用户各自独立） |
| 8 | LiteLLM 导入 | **手动按钮 + 仅填空**（不覆盖手改，`manually_edited` 标记） |
| 9 | 缺价行为 | **售价缺失→拒绝请求（fail-closed）；成本缺失→记 0 + 告警** |
| 10 | 重构范围 | **全删重建**（无历史兼容） |
| 11 | 前端范围 | ai-admin（核心）+ ai-tenant（租户售价）+ ai-customer（只读展示） |
| 12 | 成本利润记录 | **继续记录**上游 USD 成本，写入 usage 用于利润报表 |

## 2. 数据模型

### 2.1 价格表（USD 目录）

```sql
CREATE TABLE ai_price_books (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL UNIQUE,        -- "标准价" / "中转便宜价"
  description TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 单价单位：USD per token（与 LiteLLM 原生一致，NUMERIC(20,12)）。
-- image/video JSON 内 price 为 USD per image / per second。
-- audio_tts 为 USD per char，audio_stt 为 USD per minute。
CREATE TABLE ai_price_book_entries (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  price_book_id         UUID NOT NULL REFERENCES ai_price_books(id) ON DELETE CASCADE,
  model_code            TEXT NOT NULL,         -- 不外键 ai_models：成本表可含未转售的模型
  capability_type       TEXT NOT NULL DEFAULT 'chat',
  input_per_token       NUMERIC(20,12) NOT NULL DEFAULT 0,
  output_per_token      NUMERIC(20,12) NOT NULL DEFAULT 0,
  cache_write_per_token NUMERIC(20,12) NOT NULL DEFAULT 0,  -- 0 = 同 input
  cache_read_per_token  NUMERIC(20,12) NOT NULL DEFAULT 0,  -- 0 = 同 input
  reasoning_per_token   NUMERIC(20,12) NOT NULL DEFAULT 0,  -- 0 = 同 output
  image_prices          JSONB NOT NULL DEFAULT '[]',        -- [{resolution, price_usd}]
  video_prices          JSONB NOT NULL DEFAULT '[]',
  audio_tts_per_char    NUMERIC(20,12) NOT NULL DEFAULT 0,
  audio_stt_per_minute  NUMERIC(20,12) NOT NULL DEFAULT 0,
  source                TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','litellm')),
  manually_edited       BOOLEAN NOT NULL DEFAULT false,     -- true 时 LiteLLM 导入跳过
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (price_book_id, model_code)
);
```

### 2.2 全局设置（USD→积分汇率）

```sql
CREATE TABLE ai_settings (
  key        TEXT PRIMARY KEY,
  value      JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 种子：('credits_per_usd', '7')
```

### 2.3 上游成本绑定（deployment）

```sql
ALTER TABLE ai_upstream_deployments DROP COLUMN pricing;            -- 删 CNY JSONB
ALTER TABLE ai_upstream_deployments ADD COLUMN price_book_id   UUID REFERENCES ai_price_books(id);
ALTER TABLE ai_upstream_deployments ADD COLUMN cost_multiplier NUMERIC(10,4) NOT NULL DEFAULT 1;
-- 成本(USD) = entry(price_book, upstream_model) 各分项 × cost_multiplier
-- 缺 price_book 或缺条目 → 成本记 0 + 告警（不阻断）
```

### 2.4 平台→租户售价绑定（一租户一倍率）

```sql
CREATE TABLE ai_tenant_sell_bindings (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id             TEXT NOT NULL UNIQUE,
  price_book_id         UUID NOT NULL REFERENCES ai_price_books(id),
  sell_multiplier       NUMERIC(10,4) NOT NULL DEFAULT 1,
  cache_billing_enabled BOOLEAN NOT NULL DEFAULT false,   -- 默认关：缓存按 input 计
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 租户售价(积分/token) = entry × sell_multiplier × credits_per_usd
-- 缺绑定或缺条目 → 拒绝请求（fail-closed）
-- 预留：ai_tenant_sell_overrides(tenant_id, model_code, multiplier) 每模型特例
```

### 2.5 租户→用户售价绑定（级联，租户不选价格表）

```sql
CREATE TABLE ai_user_sell_bindings (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id             TEXT NOT NULL UNIQUE,
  user_multiplier       NUMERIC(10,4) NOT NULL DEFAULT 1,
  cache_billing_enabled BOOLEAN NOT NULL DEFAULT false,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 用户售价(积分/token) = 平台给该租户的售价 × user_multiplier
--                      = entry × sell_multiplier × user_multiplier × credits_per_usd
-- 改平台价/平台倍率会自动向下传导。
```

### 2.6 删除清单（全删重建）

- 表：`ai_model_prices`、`ai_tenant_model_price_overrides`、`ai_tenant_user_prices`
- 列：`ai_upstream_deployments.pricing`
- 代码：`internal/billing/pricing.go`（CNY `CostForUsage` 死代码）、`domain.Pricing/PricingTier/ResolutionPrice`、`domain.ModelPrice/TenantModelPriceOverride/TenantUserPrice/PriceSet`、旧 `service/pricing` 全部、`PriceResolver` 三级查询、相关 sqlc 查询

## 3. 计费引擎重写

替换 `ResolvePricing` + `CalculateBilling`。新解析器一次请求解析出三条线：

```
输入：req（tenant_id, user_id, model_code, deployment, usage）
全局：creditsPerUSD（缓存 ai_settings，TTL 刷新）

1. 上游成本（USD→微积分，仅核算）
   costEntry = entry(deployment.price_book, deployment.upstream_model)
   缺 → ProviderCostMicro=0 + 告警
   有 → ProviderCostUSD = applyTokens(usage, costEntry, cacheToggle=true) × cost_multiplier
        ProviderCostMicro = round(ProviderCostUSD × creditsPerUSD × 10000)

2. 租户售价（积分）
   sell = ai_tenant_sell_bindings[tenant]；缺 → 拒绝请求
   sellEntry = entry(sell.price_book, model_code)；缺 → 拒绝请求
   PlatformCostMicro = round(applyTokens(usage, sellEntry, sell.cache_enabled)
                             × sell_multiplier × creditsPerUSD × 10000)

3. 用户售价（积分，级联）
   若请求归属 user：
     ub = ai_user_sell_bindings[tenant]（缺则 user_multiplier=1, cache=false）
     UserCostMicro = round(PlatformCostBase × ub.user_multiplier ...)
       —— 注意级联用「平台售价的单价」做基准，cache 开关取 user 侧
   若归属 tenant（无 user）：UserCostMicro = PlatformCostMicro

applyTokens(usage, entry, cacheEnabled)：
  inputPrice = entry.input_per_token
  cacheWrite = cacheEnabled ? (entry.cache_write||input) : input
  cacheRead  = cacheEnabled ? (entry.cache_read ||input) : input
  outputPrice= entry.output_per_token
  reasoning  = cacheEnabled ? (entry.reasoning||output) : output   // 复用 cache 开关
  nonCachedIn = prompt - cacheWriteTok - cacheReadTok (≥0)
  nonReasonOut= completion - reasoningTok (≥0)
  USD = nonCachedIn×input + cacheWriteTok×cacheWrite + cacheReadTok×cacheRead
      + nonReasonOut×output + reasoningTok×reasoning
  image/video/audio：按 capability 用对应分辨率/单位价
```

**精度**：单价 NUMERIC(20,12)/token，计算用 `*big.Rat` 或高精度累加，仅在写入微积分（int64）时 floor，避免逐项 round 误差。利润 = (Platform 或 User) − Provider，单位统一微积分。

**生效快照**：计费结果写入 `usage_log` 既有 cost 列，历史账单不随价格表后续改动而变。

## 4. LiteLLM 导入

- 来源：`model_prices_and_context_window.json`（URL 入 config，可覆盖）。
- 字段映射：`input_cost_per_token→input_per_token`、`output_cost_per_token→output`、`cache_creation_input_token_cost→cache_write`、`cache_read_input_token_cost→cache_read`。
- 行为：admin 点"导入/刷新"，对目标价格表 **仅填充新增或 `manually_edited=false` 的条目**，置 `source='litellm'`；手改条目跳过。拉取失败 → 返回错误，用户手填。

## 5. 前端

- **ai-admin**：①价格表 CRUD + 条目表格（输入/显示 per-1M，存储 per-token，×1e6 换算）②LiteLLM 导入按钮 ③`credits_per_usd` 全局设置 ④deployment 编辑加"价格表 + 成本倍率" ⑤租户管理加"售价绑定（价格表+倍率+缓存开关）" ⑥用量页展示毛利=收入−成本。
- **ai-tenant**：用户售价绑定（显示"平台给我的售价"为基准 + 设倍率 + 缓存开关）。
- **ai-customer**：只读展示当前生效积分单价。

## 6. 分阶段任务（本期，订阅除外）

- **P0 schema + domain + sqlc**：删旧表/列/死代码；建新表；domain 新类型（PriceBook/Entry/各绑定/Settings）；sqlc 查询。
- **P1 价格表服务**：repo + service + ai-admin API（CRUD、条目批量 upsert）+ LiteLLM 导入。
- **P2 全局设置**：`ai_settings` repo/service/API（credits_per_usd 读写 + 进程内缓存）。
- **P3 计费引擎重写**：新解析器（三线 + 缓存开关 + USD→积分 + fail-closed）替换 PriceResolver/CalculateBilling；接 ledger。
- **P4 绑定接线**：deployment 成本绑定 + provider 成本写入 usage；租户/用户售价绑定 API。
- **P5 ai-admin 前端**。
- **P6 ai-tenant + ai-customer 前端**。
- **P7（二期）订阅套餐**：参考 sub2api（group + subscription_plans + 日/周/月额度 + 有效期 + 逐级转售）。

## 7. 二期：订阅套餐（占位）

参考 sub2api：`groups(rate_multiplier, daily/weekly/monthly_limit, validity_days)` + `subscription_plans(price, validity)` + `user_subscriptions`。需定：额度单位（积分）、重置语义（日/周/月滚动 or 自然周期）、与 token 计费的关系（预付额度桶 or 限速上限）、admin→租户→用户三级转售。本期不实现。
