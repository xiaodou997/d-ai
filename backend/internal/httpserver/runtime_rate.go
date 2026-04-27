package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

const runtimeLimitWindowTTL = 2 * time.Minute
const runtimeConcurrencyTTL = 5 * time.Minute

type runtimeLimitLease struct {
	rateKeys        []runtimeRateDebit
	concurrencyKeys []string
}

type runtimeRateDebit struct {
	key    string
	amount int64
}

type runtimePolicySnapshot struct {
	ID               string
	ScopeType        string
	ScopeID          string
	CapabilityType   string
	ModelCode        string
	RPMLimit         pgtype.Int4
	TPMLimit         pgtype.Int4
	ConcurrencyLimit pgtype.Int4
}

func (s *Server) acquireRuntimeLimits(w http.ResponseWriter, r *http.Request, auth RuntimeAuth, modelCode string, capabilityType string, tokenEstimate int32, deployment dbgen.ListDeploymentsForModelRow) (*runtimeLimitLease, bool) {
	if s.redis == nil {
		return nil, true
	}

	policies, err := s.runtimePolicies(r.Context(), auth, modelCode, capabilityType, deployment)
	if err != nil {
		s.logger.Error("list runtime limit policies failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		writeOpenAIError(w, http.StatusInternalServerError, "Rate limit check failed.", "server_error", "rate_limit_check_failed")
		return nil, false
	}
	if len(policies) == 0 {
		return nil, true
	}

	lease := &runtimeLimitLease{}
	window := time.Now().UTC().Unix() / 60
	for _, policy := range policies {
		if policy.RPMLimit.Valid && policy.RPMLimit.Int32 > 0 {
			key := runtimeLimitRedisKey(policy, "rpm", window)
			ok, err := s.tryIncrementWindow(r.Context(), key, 1, int64(policy.RPMLimit.Int32))
			if err != nil {
				s.rollbackRuntimeLimits(r.Context(), lease)
				s.logger.Error("runtime rpm check failed", "error", err, "request_id", requestIDFromContext(r.Context()))
				writeOpenAIError(w, http.StatusInternalServerError, "Rate limit check failed.", "server_error", "rate_limit_check_failed")
				return nil, false
			}
			if !ok {
				s.rollbackRuntimeLimits(r.Context(), lease)
				writeOpenAIError(w, http.StatusTooManyRequests, "Rate limit exceeded.", "rate_limit_exceeded", "rate_limit_exceeded")
				return nil, false
			}
			lease.rateKeys = append(lease.rateKeys, runtimeRateDebit{key: key, amount: 1})
		}

		if policy.TPMLimit.Valid && policy.TPMLimit.Int32 > 0 && tokenEstimate > 0 {
			key := runtimeLimitRedisKey(policy, "tpm", window)
			ok, err := s.tryIncrementWindow(r.Context(), key, int64(tokenEstimate), int64(policy.TPMLimit.Int32))
			if err != nil {
				s.rollbackRuntimeLimits(r.Context(), lease)
				s.logger.Error("runtime tpm check failed", "error", err, "request_id", requestIDFromContext(r.Context()))
				writeOpenAIError(w, http.StatusInternalServerError, "Token rate limit check failed.", "server_error", "rate_limit_check_failed")
				return nil, false
			}
			if !ok {
				s.rollbackRuntimeLimits(r.Context(), lease)
				writeOpenAIError(w, http.StatusTooManyRequests, "Token rate limit exceeded.", "rate_limit_exceeded", "rate_limit_exceeded")
				return nil, false
			}
			lease.rateKeys = append(lease.rateKeys, runtimeRateDebit{key: key, amount: int64(tokenEstimate)})
		}

		if policy.ConcurrencyLimit.Valid && policy.ConcurrencyLimit.Int32 > 0 {
			key := runtimeLimitRedisKey(policy, "concurrency", 0)
			ok, err := s.tryAcquireConcurrency(r.Context(), key, int64(policy.ConcurrencyLimit.Int32))
			if err != nil {
				s.rollbackRuntimeLimits(r.Context(), lease)
				s.logger.Error("runtime concurrency check failed", "error", err, "request_id", requestIDFromContext(r.Context()))
				writeOpenAIError(w, http.StatusInternalServerError, "Concurrency limit check failed.", "server_error", "rate_limit_check_failed")
				return nil, false
			}
			if !ok {
				s.rollbackRuntimeLimits(r.Context(), lease)
				writeOpenAIError(w, http.StatusTooManyRequests, "Concurrency limit exceeded.", "rate_limit_exceeded", "rate_limit_exceeded")
				return nil, false
			}
			lease.concurrencyKeys = append(lease.concurrencyKeys, key)
		}
	}

	return lease, true
}

func (s *Server) releaseRuntimeLimits(ctx context.Context, lease *runtimeLimitLease) {
	if s.redis == nil || lease == nil {
		return
	}
	for _, key := range lease.concurrencyKeys {
		if key == "" {
			continue
		}
		if err := s.redis.Decr(ctx, key).Err(); err != nil {
			s.logger.Error("release runtime concurrency failed", "error", err, "key", key)
		}
	}
}

func (s *Server) rollbackRuntimeLimits(ctx context.Context, lease *runtimeLimitLease) {
	if s.redis == nil || lease == nil {
		return
	}
	for _, item := range lease.rateKeys {
		if item.key == "" || item.amount <= 0 {
			continue
		}
		if err := s.redis.DecrBy(ctx, item.key, item.amount).Err(); err != nil {
			s.logger.Error("rollback runtime rate debit failed", "error", err, "key", item.key)
		}
	}
	s.releaseRuntimeLimits(ctx, lease)
}

func (s *Server) tryIncrementWindow(ctx context.Context, key string, amount int64, limit int64) (bool, error) {
	const script = `
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local amount = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
if current + amount > limit then
  return 0
end
current = redis.call("INCRBY", KEYS[1], amount)
if current == amount then
  redis.call("EXPIRE", KEYS[1], tonumber(ARGV[3]))
end
return 1
`
	value, err := s.redis.Eval(ctx, script, []string{key}, amount, limit, int(runtimeLimitWindowTTL.Seconds())).Int()
	if err != nil {
		return false, err
	}
	return value == 1, nil
}

func (s *Server) tryAcquireConcurrency(ctx context.Context, key string, limit int64) (bool, error) {
	const script = `
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local limit = tonumber(ARGV[1])
if current + 1 > limit then
  return 0
end
current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], tonumber(ARGV[2]))
end
return 1
`
	value, err := s.redis.Eval(ctx, script, []string{key}, limit, int(runtimeConcurrencyTTL.Seconds())).Int()
	if err != nil {
		return false, err
	}
	return value == 1, nil
}

func (s *Server) runtimePolicies(ctx context.Context, auth RuntimeAuth, modelCode string, capabilityType string, deployment dbgen.ListDeploymentsForModelRow) ([]runtimePolicySnapshot, error) {
	userID := ""
	if auth.APIKey.OwnerType == "user" && auth.APIKey.UserID.Valid {
		userID = auth.APIKey.UserID.String
	}
	rows, err := s.queries.ListActiveRuntimeLimitPolicies(ctx, dbgen.ListActiveRuntimeLimitPoliciesParams{
		CapabilityType: capabilityType,
		ModelCode:      pgtype.Text{String: modelCode, Valid: modelCode != ""},
		ScopeID:        auth.APIKey.TenantID,
		ScopeID_2:      userID,
		ScopeID_3:      auth.APIKey.ID.String(),
		ScopeID_4:      deployment.ProviderID.String(),
		ScopeID_5:      deployment.EndpointID.String(),
	})
	if err != nil {
		return nil, err
	}

	policies := make([]runtimePolicySnapshot, 0, len(rows)+2)
	for _, row := range rows {
		model := ""
		if row.ModelCode.Valid {
			model = row.ModelCode.String
		}
		policies = append(policies, runtimePolicySnapshot{
			ID:               row.ID.String(),
			ScopeType:        row.ScopeType,
			ScopeID:          row.ScopeID,
			CapabilityType:   row.CapabilityType,
			ModelCode:        model,
			RPMLimit:         row.RpmLimit,
			TPMLimit:         row.TpmLimit,
			ConcurrencyLimit: row.ConcurrencyLimit,
		})
	}

	return policies, nil
}

func runtimeLimitRedisKey(policy runtimePolicySnapshot, metric string, window int64) string {
	parts := []string{
		"uni_ai_api", "rate", policy.ScopeType, policy.ScopeID, policy.CapabilityType,
	}
	if policy.ModelCode != "" {
		parts = append(parts, "model", policy.ModelCode)
	}
	parts = append(parts, metric)
	if window > 0 {
		parts = append(parts, strconv.FormatInt(window, 10))
	}
	return strings.Join(parts, ":")
}

func estimateChatRateTokens(raw map[string]json.RawMessage, defaultMaxOutputTokens int32) int32 {
	promptTokens := int32(0)
	if messages, ok := raw["messages"]; ok {
		promptTokens = estimateJSONTokens(messages)
	}
	outputTokens := requestedOutputTokens(raw, defaultMaxOutputTokens)
	if outputTokens < 0 {
		outputTokens = 0
	}
	return promptTokens + outputTokens
}

func estimateImageRateTokens(raw map[string]json.RawMessage) int32 {
	if raw == nil {
		return 0
	}
	if prompt, ok := raw["prompt"]; ok {
		return estimateJSONTokens(prompt)
	}
	return 0
}

func estimateJSONTokens(raw json.RawMessage) int32 {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0
	}
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		text = flattenText(value)
	}
	if text == "" {
		text = string(raw)
	}
	tokens := int32((len([]rune(text)) + 3) / 4)
	if tokens <= 0 {
		return 1
	}
	return tokens
}

func flattenText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := flattenText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := flattenText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		if typed == nil {
			return ""
		}
		return fmt.Sprint(typed)
	}
}
