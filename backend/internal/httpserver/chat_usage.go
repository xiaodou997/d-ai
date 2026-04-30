package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
	"uni-ai-api/backend/internal/upstream"
)

type upstreamUsage struct {
	PromptTokens     int32
	CompletionTokens int32
	TotalTokens      int32
}

// modelPrice is a unified price view: tenant override takes precedence over model price.
type modelPrice struct {
	InputPricePer1m         int64
	OutputPricePer1m        int64
	ImageSizePrices         []byte
	VideoPricePerSecond     int64
	AudioTtsPricePer1mChars int64
	AudioSttPricePerMinute  int64
}

// getEffectiveModelPrice returns the tenant-specific price override if it exists,
// otherwise falls back to the public model price. Returns pgx.ErrNoRows if no price is configured.
// This is the cost price for the tenant (what the tenant pays to the platform).
func (s *Server) getEffectiveModelPrice(ctx context.Context, auth RuntimeAuth, modelID pgtype.UUID) (modelPrice, error) {
	override, err := s.queries.GetTenantModelPriceOverrideForRuntime(ctx, dbgen.GetTenantModelPriceOverrideForRuntimeParams{
		TenantID: auth.APIKey.TenantID,
		ModelID:  modelID,
	})
	if err == nil {
		return modelPrice{
			InputPricePer1m:         override.InputPricePer1m,
			OutputPricePer1m:        override.OutputPricePer1m,
			ImageSizePrices:         override.ImageSizePrices,
			VideoPricePerSecond:     override.VideoPricePerSecond,
			AudioTtsPricePer1mChars: override.AudioTtsPricePer1mChars,
			AudioSttPricePerMinute:  override.AudioSttPricePerMinute,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return modelPrice{}, err
	}
	base, err := s.queries.GetActiveModelPrice(ctx, modelID)
	if err != nil {
		return modelPrice{}, err
	}
	return modelPrice{
		InputPricePer1m:         base.InputPricePer1m,
		OutputPricePer1m:        base.OutputPricePer1m,
		ImageSizePrices:         base.ImageSizePrices,
		VideoPricePerSecond:     base.VideoPricePerSecond,
		AudioTtsPricePer1mChars: base.AudioTtsPricePer1mChars,
		AudioSttPricePerMinute:  base.AudioSttPricePerMinute,
	}, nil
}

// getTenantUserPrice returns the tenant's sale price to users if it exists,
// otherwise falls back to the public model price. Returns pgx.ErrNoRows if no price is configured.
// This is the sale price for the user (what the user pays to the tenant).
func (s *Server) getTenantUserPrice(ctx context.Context, auth RuntimeAuth, modelID pgtype.UUID) (modelPrice, error) {
	userPrice, err := s.queries.GetTenantUserPriceForRuntime(ctx, dbgen.GetTenantUserPriceForRuntimeParams{
		TenantID: auth.APIKey.TenantID,
		ModelID:  modelID,
	})
	if err == nil {
		return modelPrice{
			InputPricePer1m:         userPrice.InputPricePer1m,
			OutputPricePer1m:        userPrice.OutputPricePer1m,
			ImageSizePrices:         userPrice.ImageSizePrices,
			VideoPricePerSecond:     userPrice.VideoPricePerSecond,
			AudioTtsPricePer1mChars: userPrice.AudioTtsPricePer1mChars,
			AudioSttPricePerMinute:  userPrice.AudioSttPricePerMinute,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return modelPrice{}, err
	}
	// Fall back to public model price (platform price)
	base, err := s.queries.GetActiveModelPrice(ctx, modelID)
	if err != nil {
		return modelPrice{}, err
	}
	return modelPrice{
		InputPricePer1m:         base.InputPricePer1m,
		OutputPricePer1m:        base.OutputPricePer1m,
		ImageSizePrices:         base.ImageSizePrices,
		VideoPricePerSecond:     base.VideoPricePerSecond,
		AudioTtsPricePer1mChars: base.AudioTtsPricePer1mChars,
		AudioSttPricePerMinute:  base.AudioSttPricePerMinute,
	}, nil
}

// imagePriceForSize looks up the per-image price for a given size in the JSON map.
// Returns (price, true) if found; (0, false) if size is missing or map is empty.
func imagePriceForSize(imageSize string, sizePricesJSON []byte) (int64, bool) {
	if len(sizePricesJSON) == 0 {
		return 0, false
	}
	var sizePrices map[string]int64
	if err := json.Unmarshal(sizePricesJSON, &sizePrices); err != nil {
		return 0, false
	}
	price, ok := sizePrices[imageSize]
	return price, ok
}

type usageLogInput struct {
	Auth              RuntimeAuth
	RequestID         string
	TraceID           string
	ExternalUserID    string
	ConversationID    string
	ModelCode         string
	CapabilityType    string
	ModelID           pgtype.UUID
	Route             *dbgen.ListRoutesForModelRow
	Stream            bool
	HTTPStatus        int
	UpstreamStatus    int
	Latency           time.Duration
	RequestStatus     string
	ErrorCode         string
	ErrorMessage      string
	Usage             upstreamUsage
	BillableUnitType  string
	BillableUnits     int64
	UsageEstimated    bool
	UsageSource       string
	Costs             *chatCosts
	URMTransactionID  string
	BillingStatus     string
	FirstTokenLatency time.Duration
	ImageSize         string
}

type chatCosts struct {
	ProviderCost    int64
	TenantCost      int64 // Cost to tenant (what tenant pays to platform)
	UserCost        int64 // Sale price to user (what user pays to tenant)
	APIKeyQuotaCost int64
}

func parseOpenAIUsage(resp *upstream.Response) upstreamUsage {
	if resp == nil || len(resp.Body) == 0 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return upstreamUsage{}
	}

	var body struct {
		Usage struct {
			PromptTokens     int32 `json:"prompt_tokens"`
			CompletionTokens int32 `json:"completion_tokens"`
			TotalTokens      int32 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return upstreamUsage{}
	}

	return upstreamUsage{
		PromptTokens:     body.Usage.PromptTokens,
		CompletionTokens: body.Usage.CompletionTokens,
		TotalTokens:      body.Usage.TotalTokens,
	}
}

func usageHasTokens(usage upstreamUsage) bool {
	return usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0
}

func ensureUsageTotal(usage upstreamUsage) upstreamUsage {
	if usage.TotalTokens == 0 && (usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func estimateNonStreamChatUsage(raw map[string]json.RawMessage, resp *upstream.Response) upstreamUsage {
	usage := upstreamUsage{}
	if raw != nil {
		if messages, ok := raw["messages"]; ok {
			usage.PromptTokens = estimateJSONTokens(messages)
		}
	}
	if resp != nil && len(resp.Body) > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var body struct {
			Choices []struct {
				Message struct {
					Content any `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(resp.Body, &body); err == nil {
			parts := make([]string, 0, len(body.Choices))
			for _, choice := range body.Choices {
				if text := flattenText(choice.Message.Content); text != "" {
					parts = append(parts, text)
				}
			}
			if joined := strings.Join(parts, " "); joined != "" {
				usage.CompletionTokens = estimateTextTokens(joined)
			}
		}
	}
	return ensureUsageTotal(usage)
}

func estimateTextTokens(text string) int32 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	tokens := int32((len([]rune(text)) + 3) / 4)
	if tokens <= 0 {
		return 1
	}
	return tokens
}

func externalUserID(raw map[string]json.RawMessage) string {
	if raw == nil {
		return ""
	}
	value, ok := raw["user"]
	if !ok {
		return ""
	}
	var user string
	if err := json.Unmarshal(value, &user); err != nil {
		return ""
	}
	return user
}

func (s *Server) recordChatUsage(ctx context.Context, input usageLogInput) {
	upstreamDeploymentID := pgtype.UUID{}
	endpointID := pgtype.UUID{}
	providerCode := pgtype.Text{}
	upstreamModel := pgtype.Text{}
	modelRouteID := pgtype.UUID{}
	if input.Route != nil {
		upstreamDeploymentID = input.Route.UpstreamDeploymentID
		endpointID = input.Route.EndpointID
		providerCode = pgtype.Text{String: input.Route.ProviderCode, Valid: true}
		upstreamModel = pgtype.Text{String: input.Route.UpstreamModel, Valid: true}
		modelRouteID = input.Route.RouteID
	}

	costs := chatCosts{}
	if input.Costs != nil {
		costs = *input.Costs
	} else {
		costs = s.calculateChatCosts(ctx, input)
	}
	billingStatus := input.BillingStatus
	if billingStatus == "" {
		billingStatus = "not_billed"
	}
	capabilityType := input.CapabilityType
	if capabilityType == "" {
		capabilityType = "chat"
	}
	usageSource := input.UsageSource
	if usageSource == "" {
		usageSource = "upstream"
	}
	billableUnitType := input.BillableUnitType
	if billableUnitType == "" {
		billableUnitType = "token"
	}
	billableUnits := input.BillableUnits
	if billableUnits == 0 && billableUnitType == "token" {
		billableUnits = int64(input.Usage.TotalTokens)
	}
	_, err := s.queries.CreateUsageLog(ctx, dbgen.CreateUsageLogParams{
		RequestID:            input.RequestID,
		TraceID:              optionalTextString(input.TraceID),
		ApiKeyID:             input.Auth.APIKey.ID,
		KeyOwnerType:         input.Auth.APIKey.OwnerType,
		TenantID:             input.Auth.APIKey.TenantID,
		UserID:               input.Auth.APIKey.UserID,
		ExternalUserID:       optionalTextString(input.ExternalUserID),
		ModelID:              input.ModelID,
		ModelCode:            input.ModelCode,
		ModelRouteID:         modelRouteID,
		UpstreamDeploymentID: upstreamDeploymentID,
		EndpointID:           endpointID,
		ProviderCode:         providerCode,
		UpstreamModel:        upstreamModel,
		ConversationID:       optionalTextString(input.ConversationID),
		Stream:               input.Stream,
		PromptTokens:         input.Usage.PromptTokens,
		CompletionTokens:     input.Usage.CompletionTokens,
		TotalTokens:          input.Usage.TotalTokens,
		BillableUnitType:     billableUnitType,
		BillableUnits:        billableUnits,
		ProviderCost:         costs.ProviderCost,
		PlatformCost:         costs.TenantCost,
		UserCost:             costs.UserCost,
		ApiKeyQuotaCost:      costs.APIKeyQuotaCost,
		UrmTransactionID:     optionalTextString(input.URMTransactionID),
		BillingStatus:        billingStatus,
		RequestStatus:        input.RequestStatus,
		HttpStatus:           optionalInt4Value(int32(input.HTTPStatus)),
		UpstreamStatus:       optionalInt4Value(int32(input.UpstreamStatus)),
		LatencyMs:            optionalInt4Value(int32(input.Latency.Milliseconds())),
		FirstTokenLatencyMs:  optionalInt4Value(int32(input.FirstTokenLatency.Milliseconds())),
		ErrorCode:            optionalTextString(input.ErrorCode),
		ErrorMessage:         optionalTextString(input.ErrorMessage),
		UsageEstimated:       input.UsageEstimated,
		UsageSource:          usageSource,
	})
	if err != nil {
		s.logger.Error("record usage log failed", "error", err, "request_id", input.RequestID)
	}

	if input.RequestStatus == "success" && costs.APIKeyQuotaCost > 0 {
		if err := s.queries.ConfirmAPIKeyQuotaUsage(ctx, dbgen.ConfirmAPIKeyQuotaUsageParams{
			ID:        input.Auth.APIKey.ID,
			QuotaUsed: costs.APIKeyQuotaCost,
		}); err != nil {
			s.logger.Error("confirm api key quota usage failed", "error", err, "request_id", input.RequestID)
		}
	}
}

func (s *Server) calculateChatCosts(ctx context.Context, input usageLogInput) chatCosts {
	if input.RequestStatus != "success" || input.Route == nil {
		return chatCosts{}
	}
	var costs chatCosts

	// Get tenant cost price (what tenant pays to platform)
	tenantCostPrice, err := s.getEffectiveModelPrice(ctx, input.Auth, input.ModelID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("get tenant cost price failed", "error", err, "request_id", input.RequestID)
	}

	// Get user sale price (what user pays to tenant)
	userSalePrice, err := s.getTenantUserPrice(ctx, input.Auth, input.ModelID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("get user sale price failed", "error", err, "request_id", input.RequestID)
	}

	// Calculate tenant cost
	if tenantCostPrice.OutputPricePer1m > 0 {
		costs.TenantCost = tokenCost(input.Usage.PromptTokens, tenantCostPrice.InputPricePer1m) +
			tokenCost(input.Usage.CompletionTokens, tenantCostPrice.OutputPricePer1m)
	}

	// Calculate user cost (only for user-owned API keys)
	if input.Auth.APIKey.OwnerType == "user" && userSalePrice.OutputPricePer1m > 0 {
		costs.UserCost = tokenCost(input.Usage.PromptTokens, userSalePrice.InputPricePer1m) +
			tokenCost(input.Usage.CompletionTokens, userSalePrice.OutputPricePer1m)
	}

	// API Key quota cost: use user sale price for user keys, tenant cost for tenant keys
	if input.Auth.APIKey.OwnerType == "user" {
		costs.APIKeyQuotaCost = costs.UserCost
	} else {
		costs.APIKeyQuotaCost = costs.TenantCost
	}

	// Provider cost (actual upstream cost)
	providerPrice, err := s.queries.GetActiveUpstreamDeploymentCostPrice(ctx, input.Route.UpstreamDeploymentID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("get deployment cost price failed", "error", err, "request_id", input.RequestID)
	}
	if err == nil {
		costs.ProviderCost = providerPrice.RequestCost +
			tokenCost(input.Usage.PromptTokens, providerPrice.InputCostPer1m) +
			tokenCost(input.Usage.CompletionTokens, providerPrice.OutputCostPer1m)
	}

	return costs
}

func calculateImageCost(imageSize string, providerPrice dbgen.GetActiveUpstreamDeploymentCostPriceRow) int64 {
	if imageSize == "" {
		return providerPrice.ImageCost
	}
	// Parse image_size_prices JSONB
	var sizePrices map[string]int64
	if len(providerPrice.ImageSizePrices) > 0 {
		if err := json.Unmarshal(providerPrice.ImageSizePrices, &sizePrices); err == nil {
			if cost, ok := sizePrices[imageSize]; ok {
				return cost
			}
		}
	}
	// Fallback to default image_cost
	return providerPrice.ImageCost
}

func tokenCost(tokens int32, pricePer1M int64) int64 {
	if tokens <= 0 || pricePer1M <= 0 {
		return 0
	}
	const scale = int64(1_000_000)
	raw := int64(tokens) * pricePer1M
	return (raw + scale - 1) / scale
}

func requestTraceID(r *http.Request) string {
	if value := r.Header.Get("X-Request-Id"); value != "" {
		return value
	}
	if value := r.Header.Get("X-Trace-Id"); value != "" {
		return value
	}
	return ""
}

func optionalTextString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func optionalInt4Value(value int32) pgtype.Int4 {
	if value == 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: value, Valid: true}
}
