package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

var errInsufficientQuotaReservation = errors.New("insufficient quota reservation")

type quotaReservation struct {
	key    string
	amount int64
}

func (s *Server) reserveAPIKeyQuota(ctx context.Context, auth RuntimeAuth, amount int64) (*quotaReservation, error) {
	if amount <= 0 || !auth.APIKey.QuotaLimit.Valid {
		return nil, nil
	}
	if s.redis == nil {
		if auth.APIKey.QuotaUsed+auth.APIKey.QuotaReserved+amount > auth.APIKey.QuotaLimit.Int64 {
			return nil, errInsufficientQuotaReservation
		}
		return nil, nil
	}

	redisKey := "uni_ai_api:quota:key:" + auth.APIKey.ID.String() + ":reserved"
	reserved, err := s.redis.IncrBy(ctx, redisKey, amount).Result()
	if err != nil {
		return nil, err
	}
	_ = s.redis.Expire(ctx, redisKey, 10*time.Minute).Err()

	if auth.APIKey.QuotaUsed+auth.APIKey.QuotaReserved+reserved > auth.APIKey.QuotaLimit.Int64 {
		_ = s.redis.DecrBy(ctx, redisKey, amount).Err()
		return nil, errInsufficientQuotaReservation
	}

	return &quotaReservation{key: redisKey, amount: amount}, nil
}

func (s *Server) releaseAPIKeyQuotaReservation(ctx context.Context, reservation *quotaReservation) {
	if s.redis == nil || reservation == nil || reservation.amount <= 0 {
		return
	}
	if err := s.redis.DecrBy(ctx, reservation.key, reservation.amount).Err(); err != nil && !errors.Is(err, redis.Nil) {
		s.logger.Error("release quota reservation failed", "error", err)
	}
}

func estimateChatQuotaCost(raw map[string]json.RawMessage, defaultMaxOutputTokens int32, price dbgen.GetActiveModelPriceRow) int64 {
	outputTokens := requestedOutputTokens(raw, defaultMaxOutputTokens)
	return tokenCost(outputTokens, price.TenantOutputPricePer1m)
}

func requestedOutputTokens(raw map[string]json.RawMessage, defaultMaxOutputTokens int32) int32 {
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if value, ok := raw[key]; ok {
			var parsed int32
			if err := json.Unmarshal(value, &parsed); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	if defaultMaxOutputTokens > 0 {
		return defaultMaxOutputTokens
	}
	return 0
}
