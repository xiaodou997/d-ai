package routing

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
	"xiaodou/dai/internal/ai/domain"
)

func credentialQuotaKey(id string) string { return "dai:oauth-quota:v1:" + id }

var observeQuotaScript = redis.NewScript(`
 local old=redis.call('HGET',KEYS[1],ARGV[1])
 if old and cjson.decode(old).observed_at>tonumber(ARGV[3]) then return 0 end
 redis.call('HSET',KEYS[1],ARGV[1],ARGV[2])
 redis.call('PEXPIRE',KEYS[1],2678400000)
 return 1
`)

func (r *RedisAvailability) ObserveCredentialQuota(ctx context.Context, id string, quota domain.CredentialQuota) error {
	if r == nil || r.client == nil {
		return ErrAvailabilityUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	raw, err := json.Marshal(quota)
	if err != nil {
		return err
	}
	_, err = observeQuotaScript.Run(ctx, r.client, []string{credentialQuotaKey(id)}, quota.Model, string(raw), quota.ObservedAt).Result()
	return err
}
func (r *RedisAvailability) CredentialQuotas(ctx context.Context, ids []string) (map[string][]domain.CredentialQuota, error) {
	out := map[string][]domain.CredentialQuota{}
	if len(ids) == 0 {
		return out, nil
	}
	if r == nil || r.client == nil {
		return out, ErrAvailabilityUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	pipe := r.client.Pipeline()
	commands := make([]*redis.MapStringStringCmd, len(ids))
	for i, id := range ids {
		commands[i] = pipe.HGetAll(ctx, credentialQuotaKey(id))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	for i, command := range commands {
		for _, raw := range command.Val() {
			var q domain.CredentialQuota
			if json.Unmarshal([]byte(raw), &q) == nil {
				out[ids[i]] = append(out[ids[i]], q)
			}
		}
		sort.Slice(out[ids[i]], func(a, b int) bool { return out[ids[i]][a].Model < out[ids[i]][b].Model })
	}
	return out, nil
}
func (r *RedisAvailability) ClearCredentialQuota(ctx context.Context, id string) error {
	return r.client.Del(ctx, credentialQuotaKey(id)).Err()
}

func (r *RedisAvailability) CredentialQuotaEligibility(ctx context.Context, id, model string) (bool, int64, error) {
	quotas, err := r.CredentialQuotas(ctx, []string{id})
	if err != nil {
		return false, 0, err
	}
	var until int64
	for _, q := range quotas[id] {
		if q.Model == "" || q.Model == model {
			until = max(until, q.BlockedUntil(time.Now().UnixMilli()))
		}
	}
	return until == 0, until, nil
}
