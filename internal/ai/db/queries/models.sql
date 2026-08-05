-- ============================================================================
-- Runtime Model Queries（对外模型目录 = 分组目标上游显式模型绑定）
-- 模型字典已删除；model_code 是否“存在/可见”及其 capability 由 active
-- group targets + ai_upstream_models 决定，价格表只负责价格门槛和计费。
-- ============================================================================

-- name: GetModelByCode :one
-- 解析对外 model_code 是否上架（= 至少一个 active 分组的 active 上游目标显式绑定可服务）及其 capability。
-- 不含可见性校验（可见性由分组成员关系单独校验）。
SELECT DISTINCT
  um.model_code,
  um.capability_type
FROM ai_group_targets gt
JOIN ai_groups g ON g.id = gt.group_id AND g.status = 'active'
JOIN ai_upstream_models um
  ON um.upstream_kind = gt.target_kind
 AND um.upstream_id = gt.target_id
 AND um.model_code = $1
 AND um.capability_type = $2
 AND um.status = 'active'
JOIN ai_price_book_entries e
  ON e.price_book_id = g.retail_price_book_id
 AND e.model_code = um.model_code
 AND e.capability_type = um.capability_type
LEFT JOIN ai_upstream_accounts a
  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
LEFT JOIN ai_credential_pools cp
  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
WHERE gt.status = 'active'
  AND (
    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
    OR
    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
  )
LIMIT 1;

-- name: ListModelsForTenant :many
-- 租户可见模型 = 其自有 active 分组中，active 上游目标显式绑定且零售表有价格的模型。
SELECT DISTINCT
  um.model_code,
  um.capability_type
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
  AND (
    COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
    OR EXISTS (
      SELECT 1 FROM ai_upstream_resource_tenant_policies rg
      WHERE rg.resource_kind = gt.target_kind AND rg.resource_id = gt.target_id AND rg.tenant_id = g.tenant_id
        AND rg.access_granted
    )
  )
ORDER BY um.model_code ASC;
