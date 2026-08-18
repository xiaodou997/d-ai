-- AI 订阅制套餐查询（docs/ai-subscription-design.md）
-- 本文件在批次 B（领域层）继续扩充 gate 点查、Purchase、list 等查询。
-- 当前先落热路径核心：单行原子窗口记账 DebitSubscription（§5.3）。

-- 单行原子窗口记账：行锁天然串行同一订阅的并发请求；
-- 所有 CASE 引用的都是更新前旧值（Postgres 语义），窗口翻转与累加原子完成。
-- 该查询只用于已经在上游调用前提交为 subscription 的请求，因此完成时即使
-- 套餐到期/停用/并发耗尽也必须继续记入原套餐。$1=订阅 id，$2=本次加权消耗。
-- name: DebitSubscription :one
UPDATE ai_sub_subscriptions SET
  win5h_start = CASE
      WHEN window_5h_limit_micro IS NULL THEN win5h_start
      WHEN win5h_start IS NULL OR now() >= win5h_start + interval '5 hours' THEN now()
      ELSE win5h_start END,
  win5h_used_micro = CASE
      WHEN window_5h_limit_micro IS NULL THEN win5h_used_micro
      WHEN win5h_start IS NULL OR now() >= win5h_start + interval '5 hours' THEN $2
      ELSE win5h_used_micro + $2 END,
  win7d_start = CASE
      WHEN window_7d_limit_micro IS NULL THEN win7d_start
      WHEN win7d_start IS NULL OR now() >= win7d_start + interval '7 days' THEN now()
      ELSE win7d_start END,
  win7d_used_micro = CASE
      WHEN window_7d_limit_micro IS NULL THEN win7d_used_micro
      WHEN win7d_start IS NULL OR now() >= win7d_start + interval '7 days' THEN $2
      ELSE win7d_used_micro + $2 END,
  total_used_micro = total_used_micro + $2,
  updated_at = now()
WHERE id = $1
RETURNING total_used_micro;

-- ============================================================================
-- 套餐 ai_sub_plans
-- ============================================================================

-- name: CreatePlan :one
INSERT INTO ai_sub_plans (
  tenant_id, name, description, price_micro_usd, duration_days,
  total_limit_micro, window_5h_limit_micro, window_7d_limit_micro,
  status, sort_order, sale_limit, created_by
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING *;

-- name: NextPlanSortOrder :one
SELECT COALESCE(max(sort_order), 0)::integer + 10
FROM ai_sub_plans
WHERE tenant_id = $1;

-- name: GetPlan :one
SELECT * FROM ai_sub_plans WHERE id = $1;

-- ============================================================================
-- 套餐购买政策 ai_sub_plan_purchase_policies
-- ============================================================================

-- name: CreatePlanPurchasePolicy :one
INSERT INTO ai_sub_plan_purchase_policies (
  plan_id, lifetime_max_purchases, period_type, period_max_purchases,
  rolling_window_hours, calendar_unit, calendar_timezone,
  allow_advance_purchase, version
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1)
RETURNING *;

-- name: GetPlanPurchasePolicy :one
SELECT * FROM ai_sub_plan_purchase_policies WHERE plan_id = $1;

-- name: ListPlanPurchasePolicies :many
SELECT * FROM ai_sub_plan_purchase_policies
WHERE plan_id = ANY($1::uuid[])
ORDER BY plan_id;

-- Only policy changes increment the version; unrelated plan edits do not.
-- name: UpdatePlanPurchasePolicy :one
UPDATE ai_sub_plan_purchase_policies SET
  lifetime_max_purchases=$2,
  period_type=$3,
  period_max_purchases=$4,
  rolling_window_hours=$5,
  calendar_unit=$6,
  calendar_timezone=$7,
  allow_advance_purchase=$8,
  version=version+1,
  updated_at=now()
WHERE plan_id=$1
  AND ROW(
    lifetime_max_purchases, period_type, period_max_purchases,
    rolling_window_hours, calendar_unit, calendar_timezone,
    allow_advance_purchase
  ) IS DISTINCT FROM ROW($2,$3,$4,$5,$6,$7,$8)
RETURNING *;

-- name: InsertPlanPurchasePolicyRevision :exec
INSERT INTO ai_sub_plan_purchase_policy_revisions (
  plan_id, version, policy_snapshot, changed_by
) VALUES ($1,$2,$3,$4)
ON CONFLICT (plan_id, version) DO NOTHING;

-- name: ListPlanPurchasePolicyRevisions :many
SELECT * FROM ai_sub_plan_purchase_policy_revisions
WHERE plan_id=$1
ORDER BY version DESC;

-- 租户改套餐。改动只影响新购（已售有快照）。
-- name: UpdatePlanByTenant :execrows
UPDATE ai_sub_plans SET
  name=$3, description=$4, price_micro_usd=$5, duration_days=$6,
  total_limit_micro=$7, window_5h_limit_micro=$8, window_7d_limit_micro=$9,
  sort_order=$10, sale_limit=$11, updated_at=now()
WHERE id=$1 AND tenant_id=$2;

-- name: ReorderPlansByTenant :execrows
WITH requested AS (
  SELECT plan_id, ordinal
  FROM unnest($2::uuid[]) WITH ORDINALITY AS item(plan_id, ordinal)
), eligible AS (
  SELECT requested.plan_id, requested.ordinal
  FROM requested
  JOIN ai_sub_plans p ON p.id = requested.plan_id AND p.tenant_id = $1
), valid AS (
  SELECT count(*) = cardinality($2::uuid[]) AS ok FROM eligible
)
UPDATE ai_sub_plans p
SET sort_order = eligible.ordinal::integer * 10,
    updated_at = now()
FROM eligible, valid
WHERE valid.ok AND p.id = eligible.plan_id;

-- 租户上下架：只在 draft/on_sale/off_sale 间流转（目标态由 Go 侧校验）。
-- name: SetPlanStatusByTenant :execrows
UPDATE ai_sub_plans SET status=$3, updated_at=now()
WHERE id=$1 AND tenant_id=$2;

-- ============================================================================
-- 套餐-分组绑定 ai_sub_plan_groups（套餐扣额倍率）
-- ============================================================================

-- 读某批套餐的分组权重（join 分组名供展示）；单查传单元素数组即可。
-- name: ListPlanGroupsForPlans :many
SELECT pg.plan_id, pg.group_id, pg.quota_debit_multiplier, pg.sort_order, g.name AS group_name
FROM ai_sub_plan_groups pg
JOIN ai_groups g ON g.id = pg.group_id
WHERE pg.plan_id = ANY($1::uuid[])
ORDER BY pg.plan_id, pg.sort_order ASC, g.name ASC;

-- 覆盖写套餐分组前先清空（update 走 delete+insert）。
-- name: DeletePlanGroups :exec
DELETE FROM ai_sub_plan_groups WHERE plan_id = $1;

-- name: InsertPlanGroup :exec
INSERT INTO ai_sub_plan_groups (plan_id, group_id, quota_debit_multiplier, sort_order)
VALUES ($1, $2, $3, $4);

-- 校验入参分组中「active 且归属该租户」的子集。
-- 返回集与入参集比对，缺失即非法分组。
-- name: ValidateGroupsForTenant :many
SELECT g.id::text
FROM ai_groups g
WHERE g.id = ANY($2::uuid[])
  AND g.status = 'active'
  AND g.tenant_id = $1;

-- 批量取分组名（订阅快照只存 group_id，展示层据此补名）。
-- name: ListGroupNames :many
SELECT id::text, name FROM ai_groups WHERE id = ANY($1::uuid[]);

-- 购买校验：套餐分组 ∩ 用户可见分组 的数量（>0 才可购买）。用户可见 = active 且
-- 归属租户且（分组默认对用户可见 ∪ ai_user_groups 例外）。
-- name: CountUserAccessiblePlanGroups :one
SELECT count(*)
FROM ai_sub_plan_groups pg
JOIN ai_groups g ON g.id = pg.group_id AND g.status = 'active'
LEFT JOIN ai_user_groups ug ON ug.group_id = g.id AND ug.tenant_id = $2 AND ug.user_id = $3
WHERE pg.plan_id = $1
  AND g.tenant_id = $2
  AND (g.user_default_visible OR ug.id IS NOT NULL);

-- ============================================================================
-- 购买订单 ai_sub_orders
-- ============================================================================

-- name: CreateOrder :one
INSERT INTO ai_sub_orders (
  order_no, tenant_id, user_id, plan_id, plan_name_snapshot, price_micro_usd,
  duration_days_snapshot, total_limit_micro_snapshot,
  window_5h_limit_micro_snapshot, window_7d_limit_micro_snapshot,
  group_quota_debit_multipliers_snapshot,
  purchase_policy_version, purchase_policy_snapshot, inventory_reserved, status
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'created')
RETURNING *;

-- name: ReservePlanInventory :execrows
UPDATE ai_sub_plans
SET reserved_count = reserved_count + 1,
    updated_at = now()
WHERE id = $1
  AND (sale_limit IS NULL OR sold_count + reserved_count < sale_limit);

-- name: ReleasePlanInventory :execrows
UPDATE ai_sub_plans
SET reserved_count = reserved_count - 1,
    updated_at = now()
WHERE id = $1 AND reserved_count > 0;

-- name: CommitPlanInventorySale :execrows
UPDATE ai_sub_plans
SET reserved_count = reserved_count - 1,
    sold_count = sold_count + 1,
    updated_at = now()
WHERE id = $1 AND reserved_count > 0;

-- name: GetOrderByID :one
SELECT * FROM ai_sub_orders WHERE id = $1;

-- name: GetOrderByOrderNo :one
SELECT * FROM ai_sub_orders WHERE order_no = $1;

-- name: MarkOrderDeducting :execrows
UPDATE ai_sub_orders SET status='deducting', updated_at=now()
WHERE id=$1 AND status='created';

-- name: MarkOrderPaid :execrows
UPDATE ai_sub_orders SET status='paid', debit_reference=$2, debited_at=COALESCE(debited_at, now()), subscription_id=$3,
  paid_at=now(), updated_at=now()
WHERE id=$1 AND status IN ('created','deducting');

-- name: MarkOrderFailed :execrows
UPDATE ai_sub_orders SET status='failed', fail_reason=$2, updated_at=now()
WHERE id=$1 AND status IN ('created','deducting');

-- janitor 卡单补偿：created/deducting 超过 cutoff 未推进的订单。
-- name: ListReconcileOrders :many
SELECT * FROM ai_sub_orders
WHERE status IN ('created','deducting') AND updated_at < $1
ORDER BY updated_at ASC
LIMIT $2;

-- ============================================================================
-- 订阅实例 ai_sub_subscriptions
-- ============================================================================

-- 首购 status='active' 时用 DB now() 立即激活并算到期（贯彻统一 DB 时钟）；
-- 排队 status='pending' 时 activated/expires 留 NULL，首个请求或激活时再计。
-- group_quota_debit_multipliers($11) 为套餐分组套餐扣额倍率快照 {group_id: quota_debit_multiplier}。
-- name: CreateSubscription :one
INSERT INTO ai_sub_subscriptions (
  tenant_id, user_id, plan_id, order_id, plan_name_snapshot, duration_days,
  total_limit_micro, window_5h_limit_micro, window_7d_limit_micro,
  status, group_quota_debit_multipliers, activated_at, expires_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
  CASE WHEN $10 = 'active' THEN now() ELSE NULL END,
  CASE WHEN $10 = 'active' THEN now() + make_interval(days => $6) ELSE NULL END
)
RETURNING *;

-- name: GetSubscriptionByID :one
SELECT * FROM ai_sub_subscriptions WHERE id = $1;

-- 购买 finalize 事务开头的按用户串行锁：即使用户当前无任何 live 行也能串行化
-- 首购并发（纯 FOR UPDATE 锁不到不存在的行）。$1 传 tenant||':'||user。
-- name: LockUserPurchaseSerial :exec
SELECT pg_advisory_xact_lock(hashtext($1));

-- 购买事务内锁该用户全部 live 订阅行，序列化并发双购 + 判定是否已有 active。
-- name: LockLiveSubsForUser :many
SELECT * FROM ai_sub_subscriptions
WHERE tenant_id=$1 AND user_id=$2 AND status IN ('pending','active')
ORDER BY created_at ASC
FOR UPDATE;

-- gate 热路径点查（不加锁）：懒惰过期/激活 + 余量判定的输入。
-- name: GetLiveSubsForUser :many
SELECT * FROM ai_sub_subscriptions
WHERE tenant_id=$1 AND user_id=$2 AND status IN ('pending','active')
ORDER BY created_at ASC;

-- 懒惰过期单行（gate）：仅当仍 active 且已过期。
-- name: ExpireSubscriptionIfDue :execrows
UPDATE ai_sub_subscriptions SET status='expired', updated_at=now()
WHERE id=$1 AND status='active' AND expires_at < now();

-- 激活指定 pending 行（懒惰触发 now() 激活；expires=now()+时长）。
-- name: ActivateSubscription :execrows
UPDATE ai_sub_subscriptions SET
  status='active', activated_at=now(),
  expires_at = now() + make_interval(days => duration_days),
  updated_at=now()
WHERE id=$1 AND status='pending';

-- janitor 批量过期（幂等，多实例安全）。
-- name: ExpireDueSubscriptions :execrows
UPDATE ai_sub_subscriptions SET status='expired', updated_at=now()
WHERE status='active' AND expires_at < now();

-- ============================================================================
-- 列表 + 计数（narg 可选筛选：tenant_id 为空=跨租户 admin；三管理面复用）
-- ============================================================================

-- name: ListPlansPage :many
SELECT * FROM ai_sub_plans
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY sort_order ASC, created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountPlansPage :one
SELECT count(*) FROM ai_sub_plans
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));

-- 用户商城在数据库分页前排除分组暂不可售的套餐；利润评估只作租户定价提示。
-- name: ListAvailableOnSalePlansPage :many
SELECT p.*
FROM ai_sub_plans p
JOIN ai_sub_available_on_sale_plans available ON available.id = p.id
WHERE p.tenant_id = $1
ORDER BY p.sort_order ASC, p.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountAvailableOnSalePlansPage :one
SELECT count(*)
FROM ai_sub_available_on_sale_plans
WHERE tenant_id = $1;

-- name: ListSubsPage :many
SELECT * FROM ai_sub_subscriptions
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountSubsPage :one
SELECT count(*) FROM ai_sub_subscriptions
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));

-- name: ListOrdersPage :many
SELECT * FROM ai_sub_orders
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountOrdersPage :one
SELECT count(*) FROM ai_sub_orders
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'));
