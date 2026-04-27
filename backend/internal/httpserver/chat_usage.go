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

type usageLogInput struct {
	Auth              RuntimeAuth
	RequestID         string
	TraceID           string
	ExternalUserID    string
	ConversationID    string
	ModelCode         string
	CapabilityType    string
	ModelID           pgtype.UUID
	Deployment        *dbgen.ListDeploymentsForModelRow
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
}

type chatCosts struct {
	ProviderCost    int64
	PlatformCost    int64
	UserCost        int64
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
	deploymentID := pgtype.UUID{}
	endpointID := pgtype.UUID{}
	providerCode := pgtype.Text{}
	upstreamModel := pgtype.Text{}
	if input.Deployment != nil {
		deploymentID = input.Deployment.DeploymentID
		endpointID = input.Deployment.EndpointID
		providerCode = pgtype.Text{String: input.Deployment.ProviderCode, Valid: true}
		upstreamModel = pgtype.Text{String: input.Deployment.UpstreamModel, Valid: true}
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
		RequestID:           input.RequestID,
		TraceID:             optionalTextString(input.TraceID),
		ApiKeyID:            input.Auth.APIKey.ID,
		KeyOwnerType:        input.Auth.APIKey.OwnerType,
		TenantID:            input.Auth.APIKey.TenantID,
		UserID:              input.Auth.APIKey.UserID,
		ExternalUserID:      optionalTextString(input.ExternalUserID),
		ModelCode:           input.ModelCode,
		CapabilityType:      capabilityType,
		DeploymentID:        deploymentID,
		EndpointID:          endpointID,
		ProviderCode:        providerCode,
		UpstreamModel:       upstreamModel,
		ConversationID:      optionalTextString(input.ConversationID),
		Stream:              input.Stream,
		PromptTokens:        input.Usage.PromptTokens,
		CompletionTokens:    input.Usage.CompletionTokens,
		TotalTokens:         input.Usage.TotalTokens,
		BillableUnitType:    billableUnitType,
		BillableUnits:       billableUnits,
		ProviderCost:        costs.ProviderCost,
		PlatformCost:        costs.PlatformCost,
		UserCost:            costs.UserCost,
		ApiKeyQuotaCost:     costs.APIKeyQuotaCost,
		UrmTransactionID:    optionalTextString(input.URMTransactionID),
		BillingStatus:       billingStatus,
		RequestStatus:       input.RequestStatus,
		HttpStatus:          optionalInt4Value(int32(input.HTTPStatus)),
		UpstreamStatus:      optionalInt4Value(int32(input.UpstreamStatus)),
		LatencyMs:           optionalInt4Value(int32(input.Latency.Milliseconds())),
		FirstTokenLatencyMs: optionalInt4Value(int32(input.FirstTokenLatency.Milliseconds())),
		ErrorCode:           optionalTextString(input.ErrorCode),
		ErrorMessage:        optionalTextString(input.ErrorMessage),
		UsageEstimated:      input.UsageEstimated,
		UsageSource:         usageSource,
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
	if input.RequestStatus != "success" || input.Deployment == nil {
		return chatCosts{}
	}
	capabilityType := input.CapabilityType
	if capabilityType == "" {
		capabilityType = "chat"
	}

	var costs chatCosts
	modelPrice, err := s.queries.GetActiveModelPrice(ctx, input.ModelID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("get model price failed", "error", err, "request_id", input.RequestID)
	}
	if err == nil {
		platformCost := tokenCost(input.Usage.PromptTokens, modelPrice.PlatformInputPricePer1m) +
			tokenCost(input.Usage.CompletionTokens, modelPrice.PlatformOutputPricePer1m)
		tenantSaleCost := tokenCost(input.Usage.PromptTokens, modelPrice.TenantInputPricePer1m) +
			tokenCost(input.Usage.CompletionTokens, modelPrice.TenantOutputPricePer1m)
		costs.PlatformCost = platformCost
		costs.APIKeyQuotaCost = tenantSaleCost
		if input.Auth.APIKey.OwnerType == "user" {
			costs.UserCost = tenantSaleCost
		}
	}

	providerPrice, err := s.queries.GetActiveProviderModelPrice(ctx, dbgen.GetActiveProviderModelPriceParams{
		ProviderID:     input.Deployment.ProviderID,
		EndpointID:     input.Deployment.EndpointID,
		UpstreamModel:  input.Deployment.UpstreamModel,
		CapabilityType: capabilityType,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Error("get provider model price failed", "error", err, "request_id", input.RequestID)
	}
	if err == nil {
		costs.ProviderCost = providerPrice.RequestCost +
			tokenCost(input.Usage.PromptTokens, providerPrice.InputCostPer1m) +
			tokenCost(input.Usage.CompletionTokens, providerPrice.OutputCostPer1m)
	}

	return costs
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
