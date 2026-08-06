-- ============================================================================
-- Upstream Account CRUD (上游账号；原 Provider + Endpoint 合并为顶级实体)
-- ============================================================================

-- name: CreateUpstreamAccount :one
INSERT INTO ai_upstream_accounts (
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  api_key_ciphertext,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING
  id,
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status,
  invalid_reason,
  invalid_at,
  created_at,
  updated_at;

-- name: ListUpstreamAccounts :many
SELECT
  id,
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status,
  invalid_reason,
  invalid_at,
  created_at,
  updated_at
FROM ai_upstream_accounts
ORDER BY name ASC;

-- name: GetUpstreamAccount :one
SELECT
  id,
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  api_key_ciphertext,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status,
  invalid_reason,
  invalid_at,
  created_at,
  updated_at
FROM ai_upstream_accounts
WHERE id = $1;

-- name: UpdateUpstreamAccount :one
UPDATE ai_upstream_accounts
SET name = $2,
    tenant_display_name = $3,
    tenant_access_mode = $4,
    base_url = $5,
    api_key_ciphertext = $6,
    extra_headers = $7,
    default_protocol = $8,
    concurrency_limit = $9,
    price_book_id = $10,
    tenant_multiplier = $11,
    status = $12,
    invalid_reason = CASE WHEN $12 = 'invalid' THEN invalid_reason ELSE '' END,
    invalid_at = CASE WHEN $12 = 'invalid' THEN invalid_at ELSE NULL END,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status,
  invalid_reason,
  invalid_at,
  created_at,
  updated_at;

-- name: UpdateUpstreamAccountStatus :one
UPDATE ai_upstream_accounts
SET status = $2,
    invalid_reason = '',
    invalid_at = NULL,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status,
  invalid_reason,
  invalid_at,
  created_at,
  updated_at;

-- name: MarkUpstreamAccountInvalid :one
UPDATE ai_upstream_accounts
SET status = CASE WHEN status = 'disabled' THEN status ELSE 'invalid' END,
    invalid_reason = CASE WHEN status = 'disabled' THEN invalid_reason ELSE $2 END,
    invalid_at = CASE WHEN status = 'disabled' THEN invalid_at ELSE now() END,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  name,
  tenant_display_name,
  tenant_access_mode,
  base_url,
  extra_headers,
  default_protocol,
  concurrency_limit,
  price_book_id,
  tenant_multiplier,
  status,
  invalid_reason,
  invalid_at,
  created_at,
  updated_at;

-- name: DeleteUpstreamAccount :exec
DELETE FROM ai_upstream_accounts WHERE id = $1;

-- ============================================================================
-- Group Target CRUD (分组 → 上游目标 直连关联；替代 Group Route)
-- 目标多态：target_kind + target_id。绑定即上架；候选/调度见运行时。
-- ============================================================================

-- name: AddGroupTarget :one
INSERT INTO ai_group_targets (
  group_id,
  target_kind,
  target_id,
  priority,
  status
) SELECT
  sqlc.arg(group_id), sqlc.arg(target_kind), sqlc.arg(target_id),
  sqlc.arg(priority), sqlc.arg(status)
FROM ai_groups g
WHERE g.id = sqlc.arg(group_id) AND g.tenant_id = sqlc.arg(tenant_id)
  AND EXISTS (
    SELECT 1
    FROM ai_upstream_resources r
    WHERE r.resource_kind = sqlc.arg(target_kind)
      AND r.id = sqlc.arg(target_id)
      AND (
        r.tenant_access_mode = 'public'
        OR EXISTS (
          SELECT 1 FROM ai_upstream_resource_tenant_policies rg
          WHERE rg.resource_kind = r.resource_kind
            AND rg.resource_id = r.id
            AND rg.tenant_id = g.tenant_id
            AND rg.access_granted
        )
      )
  )
RETURNING
  id,
  group_id,
  target_kind,
  target_id,
  priority,
  status,
  created_at,
  updated_at;

-- name: UpdateGroupTarget :one
UPDATE ai_group_targets AS gt
SET priority = $2,
    status = $3,
    updated_at = now()
WHERE gt.id = $1
  AND EXISTS (
    SELECT 1 FROM ai_groups g
    WHERE g.id = gt.group_id AND g.tenant_id = $4
  )
RETURNING
  id,
  group_id,
  target_kind,
  target_id,
  priority,
  status,
  created_at,
  updated_at;

-- name: DeleteGroupTarget :exec
DELETE FROM ai_group_targets gt
WHERE gt.id = $1
  AND EXISTS (SELECT 1 FROM ai_groups g WHERE g.id = gt.group_id AND g.tenant_id = $2);

-- name: ListGroupTargets :many
-- 某分组关联的全部上游目标（账号或池），附目标展示信息。
SELECT
  gt.id,
  gt.group_id,
  gt.target_kind,
  gt.target_id,
  gt.priority,
  gt.status,
  gt.created_at,
  gt.updated_at,
  COALESCE(a.tenant_display_name, '')  AS account_name,
  COALESCE(a.default_protocol, '')     AS default_protocol,
  COALESCE(cp.tenant_display_name, '') AS pool_name,
  COALESCE(cp.fixed_provider_type, '') AS fixed_provider_type
FROM ai_group_targets gt
LEFT JOIN ai_upstream_accounts a
  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
LEFT JOIN ai_credential_pools cp
  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
WHERE gt.group_id = $2
  AND EXISTS (SELECT 1 FROM ai_groups g WHERE g.id = gt.group_id AND g.tenant_id = $1)
ORDER BY gt.priority ASC, account_name ASC, pool_name ASC;

-- name: ListGroupModels :many
-- 分组对外可售模型 = 该组售价价格表 entries 去重 by model_code。
SELECT DISTINCT
  e.model_code,
  e.capability_type
FROM ai_groups g
JOIN ai_price_book_entries e ON e.price_book_id = g.retail_price_book_id
WHERE g.id = $2 AND g.tenant_id = $1
ORDER BY e.model_code ASC;

-- ============================================================================
-- Group CRUD (分组)
-- ============================================================================

-- name: CreateGroup :one
INSERT INTO ai_groups (
  tenant_id,
  name,
  description,
  retail_price_book_id,
  default_user_multiplier,
  user_default_visible,
  allow_protocol_conversion,
  sort_order,
  status
) VALUES (
  sqlc.arg('tenant_id')::text,
  sqlc.arg('name')::text,
  sqlc.arg('description')::text,
  (
    SELECT pb.id
    FROM ai_price_books pb
    WHERE pb.id = sqlc.arg('retail_price_book_id')::uuid
      AND pb.status = 'active'
      AND (
        pb.owner_type = 'platform'
        OR (pb.owner_type = 'tenant' AND pb.owner_tenant_id = sqlc.arg('tenant_id')::text)
      )
  ),
  sqlc.arg('default_user_multiplier')::numeric,
  sqlc.arg('user_default_visible')::boolean,
  sqlc.arg('allow_protocol_conversion')::boolean,
  sqlc.arg('sort_order')::integer,
  sqlc.arg('status')::text
)
RETURNING
  id, tenant_id, name, description, retail_price_book_id, default_user_multiplier,
  user_default_visible, allow_protocol_conversion,
  sort_order, status, created_at, updated_at;

-- name: ListGroups :many
SELECT
  g.id, g.tenant_id, g.name, g.description, g.retail_price_book_id, g.default_user_multiplier,
  g.user_default_visible, g.allow_protocol_conversion, g.sort_order, g.status,
  g.created_at, g.updated_at,
  COALESCE(pb.name, '') AS retail_price_book_name
FROM ai_groups g
LEFT JOIN ai_price_books pb ON pb.id = g.retail_price_book_id
WHERE g.tenant_id = $1
ORDER BY g.sort_order ASC, g.name ASC;

-- name: GetGroup :one
SELECT
  id, tenant_id, name, description, retail_price_book_id, default_user_multiplier,
  user_default_visible, allow_protocol_conversion,
  sort_order, status, created_at, updated_at
FROM ai_groups
WHERE id = $2 AND tenant_id = $1;

-- name: UpdateGroup :one
UPDATE ai_groups
SET name = $2,
    description = $3,
    retail_price_book_id = $4,
    default_user_multiplier = $5,
    user_default_visible = $6,
    allow_protocol_conversion = $7,
    sort_order = $8,
    status = $9,
    updated_at = now()
FROM ai_price_books pb
WHERE ai_groups.id = $1
  AND ai_groups.tenant_id = $10
  AND pb.id = $4
  AND pb.status = 'active'
  AND (
    pb.owner_type = 'platform'
    OR (pb.owner_type = 'tenant' AND pb.owner_tenant_id = $10)
  )
RETURNING
  ai_groups.id, ai_groups.tenant_id, ai_groups.name, ai_groups.description,
  ai_groups.retail_price_book_id, ai_groups.default_user_multiplier,
  ai_groups.user_default_visible, ai_groups.allow_protocol_conversion,
  ai_groups.sort_order, ai_groups.status, ai_groups.created_at, ai_groups.updated_at;

-- name: UpdateGroupStatus :one
UPDATE ai_groups
SET status = $2,
    updated_at = now()
WHERE id = $1 AND tenant_id = $3
RETURNING
  id, tenant_id, name, description, retail_price_book_id, default_user_multiplier,
  user_default_visible, allow_protocol_conversion,
  sort_order, status, created_at, updated_at;

-- name: DeleteGroup :exec
DELETE FROM ai_groups WHERE id = $2 AND tenant_id = $1;

-- ============================================================================
-- Group Model Dispatch Rules CRUD (分组级请求模型调度策略)
-- ============================================================================

-- name: AddGroupDispatchRule :one
INSERT INTO ai_group_model_dispatch_rules (
  group_id,
  client_surface,
  match_type,
  match_value,
  target_model_code,
  priority,
  status,
  notes
) VALUES (
  sqlc.arg(group_id),
  sqlc.arg(client_surface),
  sqlc.arg(match_type),
  sqlc.arg(match_value),
  sqlc.arg(target_model_code),
  sqlc.arg(priority),
  sqlc.arg(status),
  sqlc.arg(notes)
)
RETURNING
  id, group_id, client_surface, match_type, match_value, target_model_code,
  priority, status, notes, created_at, updated_at;

-- name: ListGroupDispatchRules :many
SELECT
  id, group_id, client_surface, match_type, match_value, target_model_code,
  priority, status, notes, created_at, updated_at
FROM ai_group_model_dispatch_rules
WHERE group_id = $1
ORDER BY priority ASC, created_at ASC;

-- name: UpdateGroupDispatchRule :one
UPDATE ai_group_model_dispatch_rules
SET client_surface = sqlc.arg(client_surface),
    match_type = sqlc.arg(match_type),
    match_value = sqlc.arg(match_value),
    target_model_code = sqlc.arg(target_model_code),
    priority = sqlc.arg(priority),
    status = sqlc.arg(status),
    notes = sqlc.arg(notes),
    updated_at = now()
WHERE id = sqlc.arg(id) AND group_id = sqlc.arg(group_id)
RETURNING
  id, group_id, client_surface, match_type, match_value, target_model_code,
  priority, status, notes, created_at, updated_at;

-- name: DeleteGroupDispatchRule :exec
DELETE FROM ai_group_model_dispatch_rules WHERE id = $1 AND group_id = $2;

-- name: ListVisibleGroupsForTenant :many
-- 租户自有 active 分组。
SELECT
  g.id, g.tenant_id, g.name, g.description, g.retail_price_book_id,
  g.default_user_multiplier, g.user_default_visible, g.sort_order, g.status,
  g.created_at, g.updated_at,
  g.default_user_multiplier::numeric(10,4) AS effective_user_multiplier
FROM ai_groups g
WHERE g.tenant_id = $1 AND g.status = 'active'
ORDER BY g.name ASC;

-- ============================================================================
-- User Group CRUD (租户→用户：分组限制 + 覆盖倍率)
-- ============================================================================

-- name: UpsertUserGroup :one
INSERT INTO ai_user_groups (
  tenant_id, user_id, group_id, user_multiplier_override, created_by
) SELECT $1, $2, $3, $4, $5
FROM ai_groups g
WHERE g.id = $3 AND g.tenant_id = $1
ON CONFLICT (tenant_id, user_id, group_id) DO UPDATE SET
  user_multiplier_override = EXCLUDED.user_multiplier_override,
  updated_at          = now()
RETURNING id, tenant_id, user_id, group_id, user_multiplier_override, created_by, created_at, updated_at;

-- name: ListUserGroups :many
SELECT
  ug.id, ug.tenant_id, ug.user_id, ug.group_id, ug.user_multiplier_override,
  ug.created_by, ug.created_at, ug.updated_at,
  g.name AS group_name, g.default_user_multiplier AS group_default_user_multiplier
FROM ai_user_groups ug
JOIN ai_groups g ON g.id = ug.group_id AND g.tenant_id = ug.tenant_id
WHERE ug.tenant_id = $1 AND ug.user_id = $2
ORDER BY g.name ASC;

-- name: HasUserGroups :one
SELECT EXISTS(
  SELECT 1 FROM ai_user_groups
  WHERE tenant_id = $1 AND user_id = $2
) AS has_groups;

-- name: GetUserGroup :one
SELECT id, tenant_id, user_id, group_id, user_multiplier_override, created_by, created_at, updated_at
FROM ai_user_groups
WHERE tenant_id = $1 AND user_id = $2 AND group_id = $3;

-- name: DeleteUserGroup :exec
DELETE FROM ai_user_groups WHERE tenant_id = $1 AND user_id = $2 AND group_id = $3;

-- ============================================================================
-- Usage Logs
-- ============================================================================

-- name: ListUsageLogs :many
SELECT
  id,
  request_id,
  trace_id,
  api_key_id,
  key_owner_type,
  auth_method,
  request_source,
  tenant_id,
  user_id,
  client_user_agent,
  external_user_id,
  group_id,
  group_name_snapshot,
  group_default_user_multiplier_snapshot,
    user_multiplier_override_snapshot,
  effective_user_multiplier_snapshot,
  billing_group_label_snapshot,
  model_code,
  requested_model,
  matched_dispatch_rule_id,
  matched_dispatch_rule_summary,
  resolved_logical_model,
  resolved_provider_family,
  capability_type,
  group_target_id,
  endpoint_id,
  credential_pool_id,
  provider_code,
  upstream_model,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  cache_write_tokens,
  cache_read_tokens,
  reasoning_tokens,
  reasoning_effort,
  total_tokens,
  billable_unit_type,
  billable_units,
  resolution,
  catalog_base,
  tenant_payable,
  retail_base,
  user_payable,
  user_charged,
  api_key_quota_cost,
  service_tier,
  billing_breakdown,
  billing_event_id,
  billing_status,
  request_status,
  http_status,
  upstream_status,
  latency_ms,
  first_token_latency_ms,
  request_total_ms,
  request_setup_ms,
  first_response_byte_ms,
  response_tail_ms,
  final_attempt_header_ms,
  final_attempt_total_ms,
  error_code,
  error_message,
  attempts_count,
  protocol_conversion_enabled,
  upstream_model_mapping_applied,
  public_response_model,
  usage_estimated,
  token_usage_source,
  billing_source,
  app_id,
  app_name_snapshot,
  COALESCE(NULLIF(app_owner_type_snapshot, ''), (SELECT a.owner_type FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_type,
  COALESCE(NULLIF(app_owner_tenant_id_snapshot, ''), (SELECT a.owner_tenant_id FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_tenant_id,
  COALESCE(NULLIF(app_owner_user_id_snapshot, ''), (SELECT a.owner_user_id FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_user_id,
  created_at
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code')::text)
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status')::text)
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountUsageLogs :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id')::text)
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id')::text)
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code')::text)
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status')::text)
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source')::text)
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz);

-- name: GetUsageLogByRequestID :one
SELECT
  id,
  request_id,
  trace_id,
  api_key_id,
  key_owner_type,
  auth_method,
  request_source,
  tenant_id,
  user_id,
  client_user_agent,
  external_user_id,
  group_id,
  group_name_snapshot,
  group_default_user_multiplier_snapshot,
    user_multiplier_override_snapshot,
  effective_user_multiplier_snapshot,
  billing_group_label_snapshot,
  model_code,
  requested_model,
  matched_dispatch_rule_id,
  matched_dispatch_rule_summary,
  resolved_logical_model,
  resolved_provider_family,
  capability_type,
  group_target_id,
  endpoint_id,
  credential_pool_id,
  provider_code,
  upstream_model,
  conversation_id,
  stream,
  prompt_tokens,
  completion_tokens,
  cache_write_tokens,
  cache_read_tokens,
  reasoning_tokens,
  reasoning_effort,
  total_tokens,
  billable_unit_type,
  billable_units,
  resolution,
  catalog_base,
  tenant_payable,
  retail_base,
  user_payable,
  user_charged,
  api_key_quota_cost,
  service_tier,
  billing_breakdown,
  billing_event_id,
  billing_status,
  request_status,
  http_status,
  upstream_status,
  latency_ms,
  first_token_latency_ms,
  request_total_ms,
  request_setup_ms,
  first_response_byte_ms,
  response_tail_ms,
  final_attempt_header_ms,
  final_attempt_total_ms,
  error_code,
  error_message,
  attempts_count,
  protocol_conversion_enabled,
  upstream_model_mapping_applied,
  public_response_model,
  usage_estimated,
  token_usage_source,
  app_id,
  app_name_snapshot,
  COALESCE(NULLIF(app_owner_type_snapshot, ''), (SELECT a.owner_type FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_type,
  COALESCE(NULLIF(app_owner_tenant_id_snapshot, ''), (SELECT a.owner_tenant_id FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_tenant_id,
  COALESCE(NULLIF(app_owner_user_id_snapshot, ''), (SELECT a.owner_user_id FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_user_id,
  created_at
FROM ai_usage_logs
WHERE request_id = $1;

-- ============================================================================
-- Limit Policies CRUD
-- ============================================================================

-- name: CreateLimitPolicy :one
INSERT INTO ai_runtime_limit_policies (
  scope_type,
  scope_id,
  concurrency_limit,
  status,
  created_by
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING
  id,
  scope_type,
  scope_id,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at;

-- name: ListLimitPolicies :many
SELECT
  id,
  scope_type,
  scope_id,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at
FROM ai_runtime_limit_policies
ORDER BY scope_type ASC, scope_id ASC;

-- name: ListActiveRuntimeLimitPolicies :many
-- Get all active limit policies for the caller's tenant/user/api_key scopes.
SELECT
  id,
  scope_type,
  scope_id,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at
FROM ai_runtime_limit_policies
WHERE status = 'active'
  AND (
    (scope_type = 'tenant' AND scope_id = $1)
    OR (scope_type = 'user' AND scope_id = $2)
    OR (scope_type = 'api_key' AND scope_id = $3)
  )
ORDER BY scope_type ASC;

-- name: UpdateLimitPolicy :one
UPDATE ai_runtime_limit_policies
SET scope_type = $2,
    scope_id = $3,
    concurrency_limit = $4,
    status = $5,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  scope_type,
  scope_id,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at;

-- name: UpdateLimitPolicyStatus :one
UPDATE ai_runtime_limit_policies
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING
  id,
  scope_type,
  scope_id,
  concurrency_limit,
  status,
  created_by,
  created_at,
  updated_at;

-- name: DeleteLimitPoliciesByScope :exec
DELETE FROM ai_runtime_limit_policies
WHERE scope_type = $1
  AND scope_id = $2;

-- ============================================================================
-- Conversation Bindings
-- ============================================================================

-- name: GetConversationBinding :one
SELECT
  conversation_id,
  tenant_id,
  identity_id,
  model_id,
  upstream_deployment_id,
  account_id,
  expires_at,
  created_at,
  updated_at
FROM ai_conversation_bindings
WHERE conversation_id = $1
  AND tenant_id = $2
  AND identity_id = $3
  AND model_id = $4;

-- name: CreateConversationBinding :one
INSERT INTO ai_conversation_bindings (
  conversation_id,
  tenant_id,
  identity_id,
  model_id,
  upstream_deployment_id,
  account_id,
  expires_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING
  conversation_id,
  tenant_id,
  identity_id,
  model_id,
  upstream_deployment_id,
  account_id,
  expires_at,
  created_at,
  updated_at;

-- ============================================================================
-- Audit Logs
-- ============================================================================

-- name: CreateAuditLog :one
INSERT INTO ai_admin_audit_logs (
  actor,
  action,
  object_type,
  object_id,
  request_summary,
  result,
  http_status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING
  id,
  actor,
  action,
  object_type,
  object_id,
  request_summary,
  result,
  http_status,
  created_at;

-- ============================================================================
-- Usage Summary Queries
-- ============================================================================

-- name: ListUsageSummary :many
SELECT
  model_code,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(prompt_tokens), 0)::bigint AS total_prompt_tokens,
  COALESCE(SUM(completion_tokens), 0)::bigint AS total_completion_tokens,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(catalog_base), 0)::bigint AS total_catalog_base,
  COALESCE(SUM(tenant_payable), 0)::bigint AS total_tenant_payable,
  COALESCE(SUM(retail_base), 0)::bigint AS total_retail_base,
	COALESCE(SUM(user_payable), 0)::bigint AS total_user_payable,
  COALESCE(SUM(user_charged), 0)::bigint AS total_user_charged,
  COALESCE(SUM(api_key_quota_cost), 0)::bigint AS total_quota_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
GROUP BY model_code
ORDER BY request_count DESC;

-- name: ListUsageUnitSummary :many
SELECT
  billable_unit_type,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(billable_units), 0)::bigint AS total_billable_units,
  COALESCE(SUM(catalog_base), 0)::bigint AS total_catalog_base,
  COALESCE(SUM(tenant_payable), 0)::bigint AS total_tenant_payable,
  COALESCE(SUM(retail_base), 0)::bigint AS total_retail_base,
	COALESCE(SUM(user_payable), 0)::bigint AS total_user_payable,
  COALESCE(SUM(user_charged), 0)::bigint AS total_user_charged
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('model_code')::text IS NULL OR model_code = sqlc.narg('model_code'))
  AND (sqlc.narg('request_status')::text IS NULL OR request_status = sqlc.narg('request_status'))
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
GROUP BY billable_unit_type
ORDER BY request_count DESC;

-- ============================================================================
-- Dashboard Queries
-- ============================================================================

-- name: GetDashboardSummary :one
SELECT
  COUNT(*)::bigint AS total_requests,
  COUNT(*) FILTER (WHERE request_status = 'success')::bigint AS successful_requests,
  COUNT(*) FILTER (WHERE request_status != 'success')::bigint AS failed_requests,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(prompt_tokens), 0)::bigint AS total_prompt_tokens,
  COALESCE(SUM(completion_tokens), 0)::bigint AS total_completion_tokens,
  COALESCE(SUM(catalog_base), 0)::bigint AS total_catalog_base,
  COALESCE(SUM(tenant_payable), 0)::bigint AS total_tenant_payable,
  COALESCE(SUM(retail_base), 0)::bigint AS total_retail_base,
  COALESCE(SUM(user_payable), 0)::bigint AS total_user_payable,
  COALESCE(SUM(user_charged), 0)::bigint AS total_user_charged,
  COALESCE(AVG(latency_ms) FILTER (WHERE request_status = 'success' AND latency_ms IS NOT NULL), 0)::double precision AS avg_latency_ms,
  COALESCE(AVG(request_total_ms) FILTER (WHERE request_status = 'success' AND request_total_ms IS NOT NULL), 0)::double precision AS avg_request_total_ms,
  COALESCE(AVG(first_response_byte_ms) FILTER (WHERE request_status = 'success' AND first_response_byte_ms IS NOT NULL), 0)::double precision AS avg_first_response_byte_ms
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz);

-- name: ListDashboardTopModels :many
SELECT
  model_code,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(tenant_payable), 0)::bigint AS total_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
GROUP BY model_code
ORDER BY request_count DESC
LIMIT sqlc.arg('limit');

-- name: ListDashboardTopTenants :many
SELECT
  tenant_id,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(tenant_payable), 0)::bigint AS total_cost
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
GROUP BY tenant_id
ORDER BY request_count DESC
LIMIT sqlc.arg('limit');

-- name: ListDashboardRecentErrors :many
SELECT
  request_id,
  model_code,
  requested_model,
  matched_dispatch_rule_summary,
  resolved_logical_model,
  resolved_provider_family,
  client_protocol,
  provider_format,
  upstream_model,
  protocol_conversion_enabled,
  request_status,
  error_code,
  error_message,
  http_status,
  created_at
FROM ai_usage_logs
WHERE (sqlc.narg('tenant_id')::text IS NULL OR tenant_id = sqlc.narg('tenant_id'))
  AND (sqlc.narg('user_id')::text IS NULL OR user_id = sqlc.narg('user_id'))
  AND (sqlc.narg('date_from')::timestamptz IS NULL OR created_at >= sqlc.narg('date_from')::timestamptz)
  AND (sqlc.narg('date_to')::timestamptz IS NULL OR created_at < sqlc.narg('date_to')::timestamptz)
  AND request_status = 'failed'
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- ============================================================================
-- List Audit Logs
-- ============================================================================

-- name: ListAuditLogs :many
SELECT
  id,
  actor,
  action,
  object_type,
  object_id,
  request_summary,
  result,
  http_status,
  created_at
FROM ai_admin_audit_logs
ORDER BY created_at DESC
LIMIT $1;

-- ============================================================================
-- API Queries for /api/v1/* routes (role-based filtering)
-- ============================================================================

-- name: ListUserAvailableModels :many
-- 用户可用的模型 = 其可见分组（public ∪ 租户绑定）中，active 上游目标显式绑定且售价表有价格的模型。
-- 价格表只提供计价，不再单独扩大模型可用范围。
SELECT DISTINCT
  um.model_code,
  um.capability_type,
  MIN(tier.input_per_token * 1000000)::numeric(20,6) AS input_per_1m_usd_min,
  MAX(tier.input_per_token * 1000000)::numeric(20,6) AS input_per_1m_usd_max,
  MIN(tier.output_per_token * 1000000)::numeric(20,6) AS output_per_1m_usd_min,
  MAX(tier.output_per_token * 1000000)::numeric(20,6) AS output_per_1m_usd_max,
  MIN(tier.cache_write_per_token * 1000000)::numeric(20,6) AS cache_write_per_1m_usd_min,
  MAX(tier.cache_write_per_token * 1000000)::numeric(20,6) AS cache_write_per_1m_usd_max,
  MIN(tier.cache_read_per_token * 1000000)::numeric(20,6) AS cache_read_per_1m_usd_min,
  MAX(tier.cache_read_per_token * 1000000)::numeric(20,6) AS cache_read_per_1m_usd_max,
  BOOL_OR(jsonb_array_length(e.token_price_tiers) > 1) AS has_context_tiers
FROM ai_groups g
JOIN ai_group_targets gt
  ON gt.group_id = g.id AND gt.status = 'active'
JOIN ai_upstream_models um
  ON um.upstream_kind = gt.target_kind
 AND um.upstream_id = gt.target_id
 AND um.status = 'active'
JOIN ai_price_book_entries e
  ON e.price_book_id = g.retail_price_book_id
 AND e.model_code = um.model_code
 AND e.capability_type = um.capability_type
CROSS JOIN LATERAL jsonb_to_recordset(e.token_price_tiers) AS tier(
  up_to_input_tokens int,
  input_per_token numeric,
  output_per_token numeric,
  cache_write_per_token numeric,
  cache_read_per_token numeric
)
LEFT JOIN ai_upstream_accounts a
  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
LEFT JOIN ai_credential_pools cp
  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
WHERE g.status = 'active'
  AND g.tenant_id = $1
  AND (
    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
    OR
    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
  )
GROUP BY um.model_code, um.capability_type
ORDER BY um.model_code ASC;

-- name: ListEndUserAvailableModels :many
-- 终端用户可用模型 = 用户可见分组（租户默认公开 ∪ 用户例外打开）中，active 上游目标显式绑定且售价表有价格的模型。
SELECT DISTINCT
  um.model_code,
  um.capability_type,
  MIN(tier.input_per_token * 1000000)::numeric(20,6) AS input_per_1m_usd_min,
  MAX(tier.input_per_token * 1000000)::numeric(20,6) AS input_per_1m_usd_max,
  MIN(tier.output_per_token * 1000000)::numeric(20,6) AS output_per_1m_usd_min,
  MAX(tier.output_per_token * 1000000)::numeric(20,6) AS output_per_1m_usd_max,
  MIN(tier.cache_write_per_token * 1000000)::numeric(20,6) AS cache_write_per_1m_usd_min,
  MAX(tier.cache_write_per_token * 1000000)::numeric(20,6) AS cache_write_per_1m_usd_max,
  MIN(tier.cache_read_per_token * 1000000)::numeric(20,6) AS cache_read_per_1m_usd_min,
  MAX(tier.cache_read_per_token * 1000000)::numeric(20,6) AS cache_read_per_1m_usd_max,
  BOOL_OR(jsonb_array_length(e.token_price_tiers) > 1) AS has_context_tiers
FROM ai_groups g
JOIN ai_group_targets gt
  ON gt.group_id = g.id AND gt.status = 'active'
JOIN ai_upstream_models um
  ON um.upstream_kind = gt.target_kind
 AND um.upstream_id = gt.target_id
 AND um.status = 'active'
JOIN ai_price_book_entries e
  ON e.price_book_id = g.retail_price_book_id
 AND e.model_code = um.model_code
 AND e.capability_type = um.capability_type
CROSS JOIN LATERAL jsonb_to_recordset(e.token_price_tiers) AS tier(
  up_to_input_tokens int,
  input_per_token numeric,
  output_per_token numeric,
  cache_write_per_token numeric,
  cache_read_per_token numeric
)
LEFT JOIN ai_user_groups ug
  ON ug.group_id = g.id AND ug.tenant_id = $1 AND ug.user_id = $2
LEFT JOIN ai_upstream_accounts a
  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
LEFT JOIN ai_credential_pools cp
  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
WHERE g.status = 'active'
  AND g.tenant_id = $1
  AND (g.user_default_visible OR ug.id IS NOT NULL)
  AND (
    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
    OR
    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
  )
GROUP BY um.model_code, um.capability_type
ORDER BY um.model_code ASC;

-- name: ListUsageLogsByTenantUser :many
SELECT
  id,
  request_id,
  trace_id,
  tenant_id,
  user_id,
  request_source,
  group_id,
  group_name_snapshot,
  effective_user_multiplier_snapshot,
  billing_group_label_snapshot,
  model_code,
  stream,
  prompt_tokens,
  completion_tokens,
  cache_write_tokens,
  cache_read_tokens,
  reasoning_tokens,
  reasoning_effort,
  total_tokens,
  billable_unit_type,
  billable_units,
  service_tier,
  user_payable,
  user_charged,
  billing_source,
  request_status,
  http_status,
  latency_ms,
  first_token_latency_ms,
  error_code,
  error_message,
  app_id,
  app_name_snapshot,
  COALESCE(NULLIF(app_owner_type_snapshot, ''), (SELECT a.owner_type FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_type,
  COALESCE(NULLIF(app_owner_tenant_id_snapshot, ''), (SELECT a.owner_tenant_id FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_tenant_id,
  COALESCE(NULLIF(app_owner_user_id_snapshot, ''), (SELECT a.owner_user_id FROM ai_apps a WHERE a.id = ai_usage_logs.app_id), '')::text AS app_owner_user_id,
  created_at
FROM ai_usage_logs
WHERE tenant_id = $1
  AND user_id = $2
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source'))
ORDER BY created_at DESC
LIMIT $3;

-- name: CountUsageLogsByTenantUser :one
SELECT COUNT(*) AS count
FROM ai_usage_logs
WHERE tenant_id = $1
  AND user_id = $2
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source'));

-- name: ListUsageSummaryByTenantUser :one
SELECT
  COALESCE(SUM(request_count), 0)::bigint AS request_count,
  COALESCE(SUM(success_count), 0)::bigint AS success_requests,
  COALESCE(SUM(failed_count), 0)::bigint AS failed_requests,
  COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
  COALESCE(SUM(prompt_tokens), 0)::bigint AS total_prompt_tokens,
  COALESCE(SUM(completion_tokens), 0)::bigint AS total_completion_tokens,
	COALESCE(SUM(user_charged), 0)::bigint AS total_user_charged,
  COALESCE(SUM(latency_success_sum_ms)::double precision / NULLIF(SUM(latency_success_count), 0), 0)::double precision AS avg_latency_ms
FROM ai_usage_rollups_hourly
WHERE tenant_id = $1
  AND user_id = $2
  AND (sqlc.narg('request_source')::text IS NULL OR request_source = sqlc.narg('request_source'));
