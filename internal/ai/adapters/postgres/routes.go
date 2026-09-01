package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	corebridge "xiaodou/dai/internal/ai/core/bridge"
	"xiaodou/dai/internal/ai/domain"
)

// ── 协议匹配契约（账号级路由 + 协议转换网关）｜排查入口 ─────────────────────────
//
// P-C 起，候选的 c.Protocol 语义是「账号/池**实际**接受的 provider 协议」，而非
// client 协议。client↔provider 的落差就是中继要做的协议转换（internal/formats）：
//
//  1. 候选拉取（listRoutesForGroups）：SQL 直接按 ai_upstream_models 显式绑定
//     拉取本组内 model_code/capability 命中的 active 目标，回带 binding 的
//     api_format，以及池的 fixed_provider_type。
//     Go 端 buildCandidate 仅按显式 binding protocol 做协议匹配；pool 在
//     异常缺失 binding protocol 时，才按 fixed provider 协议兜底。
//
//  2. 协议匹配 + 偏好桶（chooseProviderProtocol）：对每个候选，在其「真实支持的
//     provider 协议集」里挑一个作为 c.Protocol：
//       - 命中 client 协议本身 → 零转换 passthrough（桶 0），与历史 1:1 透传一致。
//       - 否则（需转换）：仅当真实 bridge runtime 支持，且分组开关
//         allow_protocol_conversion 开时，才可作为转换目标；桶由 runtime 的
//         SupportMatrix 给出（0 同格式 >1 同子类型/专用双向桥 >2 同家族 >3
//         跨家族）。全不可服务则丢弃该候选。
//  3. 执行层先遵守分组故障切换层级，协议偏好桶只在当前分组内选择：
//     同层零转换优先、跨家族兜底；组内目标由分组 route_policy 自动择优。
//
// 排查 “某上游协议路由不到” → 看 chooseProviderProtocol（匹配/丢弃）与分组开关；
// 排查 “路由到了但上游 4xx / 路径不对” → 看 c.Protocol（= provider 协议）与
// buildURL/defaultPath；排查 “响应 shape 不对” → 看 serving 的转换分派
// （client≠provider 时走 ConvertRequest/ConvertResponse，否则 passthrough）。

// chooseProviderProtocol 为一个候选挑选要暴露给 clientProtocol 的 provider 协议
// 及其转换偏好桶。supported = 候选真实接受的细粒度协议集；allowConversion = 分组
// 协议转换开关；isStream 收紧流式桥接矩阵。图片/向量能力同样走 capability-aware
// 的真实 bridge runtime。
// ok=false 表示该候选不能服务此 client 协议，调用方应丢弃。
func chooseProviderProtocol(capType domain.CapabilityType, clientProtocol domain.UpstreamProtocol, supported []domain.UpstreamProtocol, allowConversion, isStream bool) (provider domain.UpstreamProtocol, bucket int, ok bool) {
	return chooseProviderProtocolWithSupport(nil, capType, clientProtocol, supported, allowConversion, isStream)
}

// candidateSupportedProtocols 解出一个候选行真实接受的细粒度 provider 协议集。
// 显式 binding api_format 是主事实源；pool 仅在异常缺失 binding protocol 时，
// 才回退到 fixed provider 的固有协议。
func candidateSupportedProtocols(row routeRow) []domain.UpstreamProtocol {
	if protocol := domain.UpstreamProtocol(row.strVal(row.APIFormat)); protocol != "" {
		return []domain.UpstreamProtocol{protocol}
	}
	if row.PoolID != nil {
		return []domain.UpstreamProtocol{
			domain.FixedProviderProtocol(domain.FixedProviderType(row.strVal(row.FixedProviderType))),
		}
	}
	return nil
}

func capabilityTypeFromProtocol(protocol domain.UpstreamProtocol) domain.CapabilityType {
	switch protocol {
	case domain.ProtocolOpenAIEmbeddings, domain.ProtocolGeminiEmbeddings:
		return domain.CapabilityEmbedding
	case domain.ProtocolOpenAIImages:
		return domain.CapabilityImage
	default:
		return domain.CapabilityChat
	}
}

// routeRow holds one resolved candidate (a row of ai_group_targets joined to its
// group, target account/pool, and explicit upstream model binding).
type routeRow struct {
	RouteID string // ai_group_targets.id

	PriceBookID      *string // 租户结算价格表（COALESCE account→pool）
	TenantMultiplier float64
	UpstreamModel    string // 显式 binding 已解析出的上游真实模型名

	// 成本提示（仅供 scorer 自动择优；非计费口径）：
	// (first-tier input + output) × 1000 × tenant_multiplier，无成本 entry 时为 0。
	CostPer1kTokens float64

	// Account fields — nil for pool targets
	AccountID        *string
	AccountName      *string
	BaseURL          *string
	APIKeyCiphertext *string
	ExtraHeaders     []byte
	APIFormat        *string // explicit binding api_format
	ConfigJSON       []byte

	// Pool fields — nil for account targets
	PoolID            *string
	FixedProviderType *string
	OAuthStrategy     *string

	// 分组售价绑定
	GroupID                    *string
	GroupName                  string
	RetailPriceBookID          *string
	GroupDefaultUserMultiplier float64

	// 分组协议转换开关：决定本组候选能否作为跨格式转换目标。
	GroupAllowConversion bool
	GroupRoutePolicy     string
}

// listRoutesForGroups fetches active targets that can serve modelCode within the
// given groups (in failover order), protocol-matched and explicit-binding filtered.
// Results are ordered by group failover order and binding ID.
func (s *RouteInspector) listRoutesForGroups(ctx context.Context, modelCode string, capType domain.CapabilityType, groupIDs []string) ([]routeRow, error) {
	const q = `
			SELECT
			  gt.id::text,
		  COALESCE(a.price_book_id, cp.price_book_id)::text        AS cost_price_book_id,
		  COALESCE(tp.tenant_multiplier_override, a.tenant_multiplier, cp.tenant_multiplier, 1) AS tenant_multiplier,
		  um.upstream_model_name                                    AS upstream_model,
		  COALESCE((
		    SELECT (
		      (e.token_price_tiers->0->>'input_per_token')::numeric
		      + (e.token_price_tiers->0->>'output_per_token')::numeric
		    ) * 1000
		    FROM ai_price_book_entries e
		    WHERE e.price_book_id = COALESCE(a.price_book_id, cp.price_book_id)
		      AND e.model_code    = um.model_code
		      AND e.capability_type = um.capability_type
		  ), 0) * COALESCE(tp.tenant_multiplier_override, a.tenant_multiplier, cp.tenant_multiplier, 1) AS cost_per_1k,
			  a.id::text, a.name, a.base_url, a.api_key_ciphertext, a.extra_headers,
			  um.api_format, um.config_json,
			  cp.id::text, cp.fixed_provider_type, cp.oauth_strategy,
			  g.id::text, g.name, g.retail_price_book_id::text, g.default_user_multiplier, g.allow_protocol_conversion,
			  g.route_policy
		FROM ai_group_targets gt
		JOIN ai_groups g ON g.id = gt.group_id
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.model_code = $1
		 AND um.capability_type = $2
		 AND um.status = 'active'
		LEFT JOIN ai_upstream_accounts a
		  ON gt.target_kind = 'direct_upstream' AND a.id = gt.target_id
		LEFT JOIN ai_credential_pools cp
		  ON gt.target_kind = 'oauth_pool' AND cp.id = gt.target_id
		LEFT JOIN ai_upstream_resource_tenant_policies tp
		  ON tp.resource_kind = gt.target_kind
		 AND tp.resource_id = gt.target_id
		 AND tp.tenant_id = g.tenant_id
		JOIN ai_price_book_entries retail_e
		  ON retail_e.price_book_id = g.retail_price_book_id
		 AND retail_e.model_code = um.model_code
		 AND retail_e.capability_type = um.capability_type
		JOIN ai_price_book_entries account_e
		  ON account_e.price_book_id = COALESCE(a.price_book_id, cp.price_book_id)
		 AND account_e.model_code = um.model_code
		 AND account_e.capability_type = um.capability_type
		WHERE gt.group_id = ANY($3::uuid[])
		  AND gt.status = 'active'
		  AND g.status  = 'active'
		  AND (
		    (gt.target_kind = 'direct_upstream' AND a.status = 'active')
		    OR
		    (gt.target_kind = 'oauth_pool' AND cp.status = 'active')
		  )
		  AND (
		    COALESCE(a.tenant_access_mode, cp.tenant_access_mode) = 'public'
		    OR COALESCE(tp.access_granted, false)
		  )
			ORDER BY array_position($3::uuid[], gt.group_id), gt.id ASC`

	if groupIDs == nil {
		groupIDs = []string{}
	}
	pgRows, err := s.pool.Query(ctx, q, modelCode, string(capType), groupIDs)
	if err != nil {
		return nil, fmt.Errorf("list routes for groups: %w", err)
	}
	defer pgRows.Close()

	var out []routeRow
	for pgRows.Next() {
		var r routeRow
		if err := pgRows.Scan(
			&r.RouteID,
			&r.PriceBookID, &r.TenantMultiplier, &r.UpstreamModel, &r.CostPer1kTokens,
			&r.AccountID, &r.AccountName, &r.BaseURL, &r.APIKeyCiphertext, &r.ExtraHeaders,
			&r.APIFormat, &r.ConfigJSON,
			&r.PoolID, &r.FixedProviderType, &r.OAuthStrategy,
			&r.GroupID, &r.GroupName, &r.RetailPriceBookID, &r.GroupDefaultUserMultiplier, &r.GroupAllowConversion,
			&r.GroupRoutePolicy,
		); err != nil {
			return nil, fmt.Errorf("scan route row: %w", err)
		}
		out = append(out, r)
	}
	return out, pgRows.Err()
}

// ModelsWithProtocolRoute returns the subset of modelCodes that have at least one
// active target (account or pool) reachable over clientProtocol in any active group.
func (s *RouteInspector) ModelsWithProtocolRoute(
	ctx context.Context,
	modelCodes []string,
	clientProtocol domain.UpstreamProtocol,
	_ bool,
) (map[string]bool, error) {
	out := make(map[string]bool, len(modelCodes))
	if len(modelCodes) == 0 {
		return out, nil
	}
	capType := capabilityTypeFromProtocol(clientProtocol)
	// 宽拉每个候选行的协议元数据，Go 端按 chooseProviderProtocol 判可服务（与
	// 路由匹配同口径）。isStream=false：模型可见性反映广义可达（同步路径），不因
	// 流式转换暂未实现的格式对而误判不可用。
	const q = `
		SELECT mc.code,
		       um.api_format,
		       g.allow_protocol_conversion
		FROM unnest($1::text[]) AS mc(code)
		JOIN ai_group_targets gt ON gt.status = 'active'
		JOIN ai_groups g ON g.id = gt.group_id AND g.status = 'active'
		JOIN ai_upstream_models um
		  ON um.upstream_kind = gt.target_kind
		 AND um.upstream_id = gt.target_id
		 AND um.model_code = mc.code
		 AND um.capability_type = $2
		 AND um.status = 'active'
		WHERE gt.target_kind IN ('direct_upstream', 'oauth_pool')`

	rows, err := s.pool.Query(ctx, q, modelCodes, string(capType))
	if err != nil {
		return nil, fmt.Errorf("models with protocol route: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			code            string
			apiFormat       string
			allowConversion bool
		)
		if err := rows.Scan(&code, &apiFormat, &allowConversion); err != nil {
			return nil, fmt.Errorf("scan model code: %w", err)
		}
		if out[code] {
			continue // already reachable
		}
		if _, _, ok := chooseProviderProtocol(capType, clientProtocol, []domain.UpstreamProtocol{domain.UpstreamProtocol(apiFormat)}, allowConversion, false); ok {
			out[code] = true
		}
	}
	return out, rows.Err()
}

func (s *RouteInspector) ModelSupportsClientProtocolInGroups(
	ctx context.Context,
	modelCode string,
	capType domain.CapabilityType,
	groupIDs []string,
	clientProtocol domain.UpstreamProtocol,
	isStream bool,
	forceAllowConversion bool,
) (bool, error) {
	if strings.TrimSpace(modelCode) == "" || len(groupIDs) == 0 {
		return false, nil
	}
	rows, err := s.listRoutesForGroups(ctx, modelCode, capType, groupIDs)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		allowConversion := row.GroupAllowConversion || forceAllowConversion
		if _, _, ok := chooseProviderProtocol(capType, clientProtocol, candidateSupportedProtocols(row), allowConversion, isStream); ok {
			return true, nil
		}
	}
	return false, nil
}

// RouteInspector answers control-plane route capability and preview queries.
// Runtime candidate planning belongs exclusively to core/runtime.Resolver.
type RouteInspector struct {
	pool          *translatingPool
	bridgeSupport corebridge.SupportMatrix
}

func NewRouteInspector(pool *pgxpool.Pool) *RouteInspector {
	return &RouteInspector{pool: newTranslatingPool(pool)}
}

func (s *RouteInspector) WithBridgeSupport(support corebridge.SupportMatrix) *RouteInspector {
	s.bridgeSupport = normalizeBridgeSupport(support)
	return s
}

type imageBindingPolicy struct {
	StreamMode             string
	EditTransport          string
	UpstreamResponseFormat string
	MaxOutputCount         int
	EditMaxOutputCount     int
}

func imagePolicyFromConfig(raw []byte) imageBindingPolicy {
	policy := imageBindingPolicy{
		StreamMode:         domain.ImageStreamModeForceSync,
		EditTransport:      domain.ImageEditTransportMultipart,
		MaxOutputCount:     domain.DefaultImageOutputCount,
		EditMaxOutputCount: domain.DefaultImageOutputCount,
	}
	if len(raw) == 0 {
		return policy
	}
	var cfg struct {
		ImageGeneration struct {
			StreamMode             string `json:"stream_mode"`
			EditTransport          string `json:"edit_transport"`
			UpstreamResponseFormat string `json:"upstream_response_format"`
			MaxOutputCount         int    `json:"max_output_count"`
			EditMaxOutputCount     int    `json:"edit_max_output_count"`
		} `json:"image_generation"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return policy
	}
	switch strings.TrimSpace(cfg.ImageGeneration.StreamMode) {
	case domain.ImageStreamModeAuto, domain.ImageStreamModeForceStream, domain.ImageStreamModeForceSync:
		policy.StreamMode = strings.TrimSpace(cfg.ImageGeneration.StreamMode)
	}
	switch strings.TrimSpace(cfg.ImageGeneration.EditTransport) {
	case domain.ImageEditTransportJSON, domain.ImageEditTransportMultipart:
		policy.EditTransport = strings.TrimSpace(cfg.ImageGeneration.EditTransport)
	case "":
	default:
		policy.EditTransport = strings.TrimSpace(cfg.ImageGeneration.EditTransport)
	}
	switch strings.TrimSpace(cfg.ImageGeneration.UpstreamResponseFormat) {
	case domain.ImageResponseFormatURL, domain.ImageResponseFormatB64:
		policy.UpstreamResponseFormat = strings.TrimSpace(cfg.ImageGeneration.UpstreamResponseFormat)
	}
	if cfg.ImageGeneration.MaxOutputCount >= 1 && cfg.ImageGeneration.MaxOutputCount <= domain.MaxImageOutputCount {
		policy.MaxOutputCount = cfg.ImageGeneration.MaxOutputCount
	}
	if cfg.ImageGeneration.EditMaxOutputCount >= 1 && cfg.ImageGeneration.EditMaxOutputCount <= domain.MaxImageOutputCount {
		policy.EditMaxOutputCount = cfg.ImageGeneration.EditMaxOutputCount
	}
	return policy
}

func (r routeRow) strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
