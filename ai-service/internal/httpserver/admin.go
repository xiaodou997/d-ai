package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultProtocol       = "openai_chat"
	defaultCapability     = "chat"
	defaultStatus         = "active"
	defaultEndpointWeight = int32(100)
	defaultTimeoutMs      = int32(30000)
)

type adminActorContextKey struct{}
type adminContextKey struct{}

type adminRole string

const (
	adminRolePlatform adminRole = "platform"
	adminRoleTenant   adminRole = "tenant"
	adminRoleUser     adminRole = "user"
)

type adminContext struct {
	Actor    string    `json:"actor"`
	Role     adminRole `json:"role"`
	TenantID string    `json:"tenant_id"`
	UserID   string    `json:"user_id"`
	UserType int       `json:"user_type"`
}

type createProviderRequest struct {
	Code   string          `json:"code"`
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
	Status string          `json:"status"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

type createEndpointRequest struct {
	Name            string          `json:"name"`
	BaseURL         string          `json:"base_url"`
	APIKey          string          `json:"api_key"`
	ExtraHeaders    json.RawMessage `json:"extra_headers"`
	Weight          *int32          `json:"weight"`
	TimeoutMs       *int32          `json:"timeout_ms"`
	DefaultProtocol string          `json:"default_protocol"`
	Status          string          `json:"status"`
}

type createModelRequest struct {
	ModelCode              string `json:"model_code"`
	DisplayName            string `json:"display_name"`
	CapabilityType         string `json:"capability_type"`
	ContextWindow          *int32 `json:"context_window"`
	DefaultMaxOutputTokens *int32 `json:"default_max_output_tokens"`
	MaxOutputTokens        *int32 `json:"max_output_tokens"`
	Status                 string `json:"status"`
}

type modelPriceRequest struct {
	InputPricePer1MCredits         float64              `json:"input_price_per_1m_credits"`  // 积分
	OutputPricePer1MCredits        float64              `json:"output_price_per_1m_credits"` // 积分
	ImagePrices                    []resolutionPriceDTO `json:"image_prices"`
	VideoPrices                    []resolutionPriceDTO `json:"video_prices"`
	AudioTTSPricePer1MCharsCredits float64              `json:"audio_tts_price_per_1m_chars_credits"` // 积分
	AudioSTTPricePerMinuteCredits  float64              `json:"audio_stt_price_per_minute_credits"`   // 积分
}

type createUpstreamDeploymentRequest struct {
	EndpointID         string          `json:"endpoint_id"`
	UpstreamModel      string          `json:"upstream_model"`
	CapabilityType     string          `json:"capability_type"`
	UpstreamProtocol   string          `json:"upstream_protocol"`
	RequestPath        *string         `json:"request_path"`
	UpstreamParameters json.RawMessage `json:"upstream_parameters"`
	Pricing            json.RawMessage `json:"pricing"`
	CredentialPoolID   string          `json:"credential_pool_id"`
	Status             string          `json:"status"`
}

type createModelRouteRequest struct {
	// Deployment route (XOR with pool fields below)
	UpstreamDeploymentID string `json:"upstream_deployment_id"`
	// Pool route
	CredentialPoolID  string `json:"credential_pool_id"`
	PoolUpstreamModel string `json:"pool_upstream_model"`
	Priority          *int32 `json:"priority"`
	Weight            *int32 `json:"weight"`
	SupportsStream    *bool  `json:"supports_stream"`
	Status            string `json:"status"`
}

type grantModelToTenantRequest struct {
	ModelID   string `json:"model_id"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by"`
}

type createTenantAPIKeyRequest struct {
	Name              string          `json:"name"`
	QuotaLimitCredits *float64        `json:"quota_limit_credits"` // 积分 (nil=无限制)
	AllowedModels     []string        `json:"allowed_models"`
	Status            string          `json:"status"`
	ExpiresAt         *adminTimestamp `json:"expires_at"`
	CreatedBy         string          `json:"created_by"`
}

type createTenantAPIKeyResponse struct {
	APIKey string    `json:"api_key"`
	Key    apiKeyDTO `json:"key"`
}

type createUserAPIKeyResponse struct {
	APIKey string    `json:"api_key"`
	Key    apiKeyDTO `json:"key"`
}

func (s *Server) validURMAdminToken(r *http.Request) (adminContext, bool) {
	if s.jwksValidator == nil {
		return adminContext{}, false
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return adminContext{}, false
	}

	claims, err := s.jwksValidator.ValidateToken(r.Context(), token)
	if err != nil {
		s.logger.Warn("validate urm admin token failed",
			zap.Error(err),
			zap.String("request_id", requestIDFromContext(r.Context())),
		)
		return adminContext{}, false
	}

	role, ok := adminRoleForUserType(claims.UserType)
	if !ok {
		return adminContext{}, false
	}
	actor := claims.UserID
	if actor == "" {
		actor = claims.Username
	}
	if actor == "" {
		actor = "urm_admin"
	}
	return adminContext{
		Actor:    actor,
		Role:     role,
		TenantID: claims.TenantID,
		UserID:   claims.UserID,
		UserType: claims.UserType,
	}, true
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func adminRoleForUserType(userType int) (adminRole, bool) {
	switch userType {
	case 1: // 超管
		return adminRolePlatform, true
	case 2: // 平台管理员
		return adminRolePlatform, true
	default:
		return "", false
	}
}

func withAdminContext(ctx context.Context, admin adminContext) context.Context {
	ctx = context.WithValue(ctx, adminContextKey{}, admin)
	ctx = context.WithValue(ctx, adminActorContextKey{}, admin.Actor)
	return ctx
}

func adminFromContext(ctx context.Context) (adminContext, bool) {
	admin, ok := ctx.Value(adminContextKey{}).(adminContext)
	return admin, ok
}

type usageFilters struct {
	tenantID      string
	userID        string
	modelCode     string
	requestStatus string
	requestSource string
	dateFrom      pgtype.Timestamptz
	dateTo        pgtype.Timestamptz
}

type dashboardParams struct {
	tenantID string
	userID   string
	since    pgtype.Timestamptz
}

func scopedDashboardParams(w http.ResponseWriter, r *http.Request) (dashboardParams, bool) {
	filters, ok := scopedUsageFilters(w, r)
	if !ok {
		return dashboardParams{}, false
	}
	since, ok := parseDashboardSince(w, r)
	if !ok {
		return dashboardParams{}, false
	}
	return dashboardParams{
		tenantID: filters.tenantID,
		userID:   filters.userID,
		since:    since,
	}, true
}

func parseDashboardSince(w http.ResponseWriter, r *http.Request) (pgtype.Timestamptz, bool) {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		raw = "1"
	}
	days, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || days < 0 || days > 3650 {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid days")
		return pgtype.Timestamptz{}, false
	}
	if days == 0 {
		return pgtype.Timestamptz{}, true
	}
	return pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -int(days)), Valid: true}, true
}

func parseUsageSummarySince(w http.ResponseWriter, r *http.Request) (pgtype.Timestamptz, bool) {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		return pgtype.Timestamptz{}, true
	}
	days, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || days < 0 || days > 3650 {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid days")
		return pgtype.Timestamptz{}, false
	}
	if days == 0 {
		return pgtype.Timestamptz{}, true
	}
	return pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, -int(days)), Valid: true}, true
}

func parseAdminLimit(w http.ResponseWriter, r *http.Request, defaultLimit int32, maxLimit int32) (int32, bool) {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid limit")
			return 0, false
		}
		if parsed > int64(maxLimit) {
			parsed = int64(maxLimit)
		}
		limit = int32(parsed)
	}
	return limit, true
}

func parseOptionalTimestamptz(w http.ResponseWriter, r *http.Request, param string) (pgtype.Timestamptz, bool) {
	raw := r.URL.Query().Get(param)
	if raw == "" {
		return pgtype.Timestamptz{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid "+param+": expected RFC3339")
		return pgtype.Timestamptz{}, false
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, true
}

func scopedUsageFilters(w http.ResponseWriter, r *http.Request) (usageFilters, bool) {
	dateFrom, ok := parseOptionalTimestamptz(w, r, "date_from")
	if !ok {
		return usageFilters{}, false
	}
	dateTo, ok := parseOptionalTimestamptz(w, r, "date_to")
	if !ok {
		return usageFilters{}, false
	}
	filters := usageFilters{
		tenantID:      r.URL.Query().Get("tenant_id"),
		userID:        r.URL.Query().Get("user_id"),
		modelCode:     r.URL.Query().Get("model_code"),
		requestStatus: r.URL.Query().Get("request_status"),
		requestSource: r.URL.Query().Get("request_source"),
		dateFrom:      dateFrom,
		dateTo:        dateTo,
	}
	admin, ok := adminFromContext(r.Context())
	if !ok {
		return filters, true
	}
	switch admin.Role {
	case adminRolePlatform, "":
		return filters, true
	case adminRoleTenant:
		if admin.TenantID == "" {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "tenant scope is required")
			return usageFilters{}, false
		}
		if filters.tenantID != "" && filters.tenantID != admin.TenantID {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
			return usageFilters{}, false
		}
		filters.tenantID = admin.TenantID
	case adminRoleUser:
		if admin.TenantID == "" || admin.UserID == "" {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "user scope is required")
			return usageFilters{}, false
		}
		if filters.tenantID != "" && filters.tenantID != admin.TenantID {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
			return usageFilters{}, false
		}
		if filters.userID != "" && filters.userID != admin.UserID {
			writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
			return usageFilters{}, false
		}
		filters.tenantID = admin.TenantID
		filters.userID = admin.UserID
	default:
		writeErr(w, http.StatusForbidden, BizErrForbidden, "forbidden")
		return usageFilters{}, false
	}
	return filters, true
}

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		zap.L().Warn("decode admin request body failed",
			zap.Error(err),
			zap.String("path", r.URL.Path),
			zap.String("request_id", requestIDFromContext(r.Context())),
		)
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// decodeAdminJSONLenient decodes JSON without rejecting unknown fields.
// Used for import endpoints where the payload may contain extra provider-specific fields.
func decodeAdminJSONLenient(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		zap.L().Warn("decode admin request body failed",
			zap.Error(err),
			zap.String("path", r.URL.Path),
			zap.String("request_id", requestIDFromContext(r.Context())),
		)
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

func jsonArrayOrDefault(v json.RawMessage) []byte {
	if len(v) == 0 {
		return []byte("[]")
	}
	return []byte(v)
}

func decodeStatusUpdate(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req updateStatusRequest
	if !decodeAdminJSON(w, r, &req) {
		return "", false
	}
	status := strings.TrimSpace(req.Status)
	switch status {
	case "active", "inactive", "disabled":
		return status, true
	default:
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "status must be active, inactive or disabled")
		return "", false
	}
}

func tenantUserParams(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "tenantID is required")
		return "", "", false
	}
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "userID is required")
		return "", "", false
	}
	return tenantID, userID, true
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	value := chi.URLParam(r, name)
	id, err := parseUUID(value)
	if err != nil {
		writeErr(w, http.StatusBadRequest, BizErrBadRequest, "invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if value == "" {
		return id, errors.New("uuid is required")
	}
	if err := id.Scan(value); err != nil {
		return id, err
	}
	return id, nil
}

func optionalUUIDString(value string) (pgtype.UUID, error) {
	if value == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(value)
}

func jsonObjectOrDefault(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func optionalTextParam(value *string) pgtype.Text {
	if value == nil || *value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalTextValue(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func optionalInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func optionalText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func optionalTextString(value string) pgtype.Text {
	return optionalTextValue(value)
}

func optionalInt4Value(value int32) pgtype.Int4 {
	if value == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: value, Valid: true}
}

type adminTimestamp struct {
	Time  time.Time
	Valid bool
}

func (t *adminTimestamp) UnmarshalJSON(b []byte) error {
	raw := strings.TrimSpace(string(b))
	if raw == "" || raw == "null" || raw == `""` {
		t.Valid = false
		t.Time = time.Time{}
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" {
			t.Valid = false
			t.Time = time.Time{}
			return nil
		}
		if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
			t.Time = time.UnixMilli(millis).UTC()
			t.Valid = true
			return nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return err
		}
		t.Time = parsed.UTC()
		t.Valid = true
		return nil
	}

	var millis int64
	if err := json.Unmarshal(b, &millis); err != nil {
		return err
	}
	if millis == 0 {
		t.Valid = false
		t.Time = time.Time{}
		return nil
	}
	t.Time = time.UnixMilli(millis).UTC()
	t.Valid = true
	return nil
}

func optionalTime(value *adminTimestamp) (pgtype.Timestamptz, error) {
	if value == nil || !value.Valid {
		return pgtype.Timestamptz{}, nil
	}
	return pgtype.Timestamptz{Time: value.Time, Valid: true}, nil
}

func parseEffectiveFrom(value adminTimestamp) (pgtype.Timestamptz, error) {
	if !value.Valid {
		return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, nil
	}
	return pgtype.Timestamptz{Time: value.Time, Valid: true}, nil
}

func int32OrDefault(value *int32, defaultValue int32) int32 {
	if value == nil {
		return defaultValue
	}
	return *value
}

func boolOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func (s *Server) writeAdminServerError(w http.ResponseWriter, r *http.Request, message string, err error) {
	s.logger.Error(message, zap.Error(err), zap.String("request_id", requestIDFromContext(r.Context())))
	writeErr(w, http.StatusInternalServerError, BizErrInternal, "server error")
}

// ============================================================================
// Pool ModelRoute helpers (direct SQL — avoids regenerating sqlc for new columns)
// ============================================================================

type modelRouteResponse struct {
	ID                   string  `json:"id"`
	ModelID              string  `json:"model_id"`
	UpstreamDeploymentID *string `json:"upstream_deployment_id,omitempty"`
	CredentialPoolID     *string `json:"credential_pool_id,omitempty"`
	PoolUpstreamModel    *string `json:"pool_upstream_model,omitempty"`
	Priority             int32   `json:"priority"`
	Weight               int32   `json:"weight"`
	SupportsStream       bool    `json:"supports_stream"`
	Status               string  `json:"status"`
	CreatedAt            int64   `json:"created_at"`
	UpdatedAt            int64   `json:"updated_at"`
}

func createPoolModelRoute(ctx context.Context, db *pgxpool.Pool, modelIDBytes []byte, req createModelRouteRequest) (*modelRouteResponse, error) {
	modelID := pgtype.UUID{Bytes: [16]byte(modelIDBytes), Valid: true}
	poolID, err := parseUUID(req.CredentialPoolID)
	if err != nil {
		return nil, fmt.Errorf("invalid credential_pool_id: %w", err)
	}
	var row modelRouteResponse
	var createdAt, updatedAt pgtype.Timestamptz
	err = db.QueryRow(ctx, `
		INSERT INTO ai_model_routes
		  (model_id, credential_pool_id, pool_upstream_model, priority, weight, supports_stream, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, model_id::text, credential_pool_id::text, pool_upstream_model,
		          priority, weight, supports_stream, status, created_at, updated_at`,
		modelID, poolID, req.PoolUpstreamModel,
		int32OrDefault(req.Priority, 100),
		int32OrDefault(req.Weight, 100),
		boolOrDefault(req.SupportsStream, true),
		req.Status,
	).Scan(
		&row.ID, &row.ModelID, &row.CredentialPoolID, &row.PoolUpstreamModel,
		&row.Priority, &row.Weight, &row.SupportsStream, &row.Status,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	row.CreatedAt = createdAt.Time.UnixMilli()
	row.UpdatedAt = updatedAt.Time.UnixMilli()
	return &row, nil
}

func updatePoolModelRoute(ctx context.Context, db *pgxpool.Pool, modelIDBytes, routeIDBytes []byte, req createModelRouteRequest) (*modelRouteResponse, error) {
	modelID := pgtype.UUID{Bytes: [16]byte(modelIDBytes), Valid: true}
	routeID := pgtype.UUID{Bytes: [16]byte(routeIDBytes), Valid: true}
	poolID, err := parseUUID(req.CredentialPoolID)
	if err != nil {
		return nil, fmt.Errorf("invalid credential_pool_id: %w", err)
	}
	var row modelRouteResponse
	var createdAt, updatedAt pgtype.Timestamptz
	err = db.QueryRow(ctx, `
		UPDATE ai_model_routes
		SET upstream_deployment_id = NULL,
		    credential_pool_id     = $3,
		    pool_upstream_model    = $4,
		    priority               = $5,
		    weight                 = $6,
		    supports_stream        = $7,
		    status                 = $8,
		    updated_at             = now()
		WHERE model_id = $1 AND id = $2
		RETURNING id::text, model_id::text, credential_pool_id::text, pool_upstream_model,
		          priority, weight, supports_stream, status, created_at, updated_at`,
		modelID, routeID, poolID, req.PoolUpstreamModel,
		int32OrDefault(req.Priority, 100),
		int32OrDefault(req.Weight, 100),
		boolOrDefault(req.SupportsStream, true),
		req.Status,
	).Scan(
		&row.ID, &row.ModelID, &row.CredentialPoolID, &row.PoolUpstreamModel,
		&row.Priority, &row.Weight, &row.SupportsStream, &row.Status,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	row.CreatedAt = createdAt.Time.UnixMilli()
	row.UpdatedAt = updatedAt.Time.UnixMilli()
	return &row, nil
}
