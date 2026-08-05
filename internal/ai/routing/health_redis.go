package routing

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	healthKeyPrefix = "uni_ai_api:health:target:"
	healthIndexKey  = "uni_ai_api:health:targets"
	healthStateTTL  = 7 * 24 * time.Hour
	healthRedisWait = 500 * time.Millisecond
)

var recordHealthSuccessScript = redis.NewScript(`
redis.call('HSET', KEYS[1],
  'kind', ARGV[2],
  'state', 0,
  'consec_fail', 0,
  'open_count', 0,
  'probing', 0,
  'probe_until_ms', 0,
  'opened_at_ms', 0,
  'next_probe_at_ms', 0)
redis.call('SADD', KEYS[2], ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return 1
`)

var recordHealthFailureScript = redis.NewScript(`
local state = tonumber(redis.call('HGET', KEYS[1], 'state') or '0')
local fails = tonumber(redis.call('HGET', KEYS[1], 'consec_fail') or '0')
local opens = tonumber(redis.call('HGET', KEYS[1], 'open_count') or '0')
local opened = tonumber(redis.call('HGET', KEYS[1], 'opened_at_ms') or '0')
local next_probe = tonumber(redis.call('HGET', KEYS[1], 'next_probe_at_ms') or '0')
local probing = tonumber(redis.call('HGET', KEYS[1], 'probing') or '0')
local probe_until = tonumber(redis.call('HGET', KEYS[1], 'probe_until_ms') or '0')

if state == 0 then
  fails = fails + 1
  if fails >= tonumber(ARGV[4]) then
    state = 1
    opens = opens + 1
    opened = tonumber(ARGV[3])
    local duration = tonumber(ARGV[5]) * (2 ^ (opens - 1))
    if duration > tonumber(ARGV[6]) then duration = tonumber(ARGV[6]) end
    next_probe = opened + duration
    probing = 0
    probe_until = 0
  end
elseif state == 2 then
  state = 1
  opens = opens + 1
  opened = tonumber(ARGV[3])
  local duration = tonumber(ARGV[5]) * (2 ^ (opens - 1))
  if duration > tonumber(ARGV[6]) then duration = tonumber(ARGV[6]) end
  next_probe = opened + duration
  probing = 0
  probe_until = 0
end

redis.call('HSET', KEYS[1],
  'kind', ARGV[2],
  'state', state,
  'consec_fail', fails,
  'open_count', opens,
  'probing', probing,
  'probe_until_ms', probe_until,
  'opened_at_ms', opened,
  'next_probe_at_ms', next_probe)
redis.call('SADD', KEYS[2], ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[7])
return {state, fails, opens, next_probe}
`)

var claimHealthProbeScript = redis.NewScript(`
local state = tonumber(redis.call('HGET', KEYS[1], 'state') or '0')
if state == 0 then return 0 end
if state == 1 then
  local next_probe = tonumber(redis.call('HGET', KEYS[1], 'next_probe_at_ms') or '0')
  if tonumber(ARGV[1]) < next_probe then return 1 end
  redis.call('HSET', KEYS[1], 'state', 2, 'probing', 1,
    'probe_until_ms', tonumber(ARGV[1]) + tonumber(ARGV[3]))
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return 0
end
local probe_until = tonumber(redis.call('HGET', KEYS[1], 'probe_until_ms') or '0')
if tonumber(redis.call('HGET', KEYS[1], 'probing') or '0') == 1
  and tonumber(ARGV[1]) < probe_until then return 1 end
redis.call('HSET', KEYS[1], 'probing', 1,
  'probe_until_ms', tonumber(ARGV[1]) + tonumber(ARGV[3]))
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return 0
`)

var releaseHealthProbeScript = redis.NewScript(`
local state = tonumber(redis.call('HGET', KEYS[1], 'state') or '0')
if state ~= 2 then return 0 end
redis.call('HSET', KEYS[1], 'probing', 0, 'probe_until_ms', 0)
redis.call('PEXPIRE', KEYS[1], ARGV[1])
return 1
`)

// RedisHealthTracker stores the circuit-breaker FSM in Redis. Every node sees
// the same failure counter and half-open probe claim; inner is only a fallback
// for brief Redis outages.
type RedisHealthTracker struct {
	inner *InMemoryTracker
	rdb   *redis.Client
}

func NewRedisHealthTracker(inner *InMemoryTracker, rdb *redis.Client) *RedisHealthTracker {
	if inner == nil {
		inner = DefaultInMemoryTracker()
	}
	return &RedisHealthTracker{inner: inner, rdb: rdb}
}

func (r *RedisHealthTracker) RecordSuccess(targetID string, kind TargetKind) {
	if targetID == "" {
		return
	}
	r.inner.RecordSuccess(targetID, kind)
	if r.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRedisWait)
	defer cancel()
	if err := recordHealthSuccessScript.Run(ctx, r.rdb, []string{r.key(targetID), healthIndexKey},
		targetID, int(kind), healthStateTTL.Milliseconds()).Err(); err != nil {
		zap.L().Debug("health redis success update failed", zap.String("target_id", targetID), zap.Error(err))
	}
}

func (r *RedisHealthTracker) RecordFailure(targetID string, kind TargetKind) {
	if targetID == "" {
		return
	}
	r.inner.RecordFailure(targetID, kind)
	if r.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRedisWait)
	defer cancel()
	if err := recordHealthFailureScript.Run(ctx, r.rdb, []string{r.key(targetID), healthIndexKey},
		targetID,
		int(kind),
		time.Now().UnixMilli(),
		r.inner.failThreshold,
		r.inner.baseDuration.Milliseconds(),
		maxOpenDuration.Milliseconds(),
		healthStateTTL.Milliseconds(),
	).Err(); err != nil {
		zap.L().Debug("health redis failure update failed", zap.String("target_id", targetID), zap.Error(err))
	}
}

func (r *RedisHealthTracker) IsBlocked(targetID string, probeLease time.Duration) bool {
	if targetID == "" {
		return false
	}
	if probeLease <= 0 {
		probeLease = defaultProbeLease
	}
	if r.rdb == nil {
		return r.inner.IsBlocked(targetID, probeLease)
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRedisWait)
	defer cancel()
	blocked, err := claimHealthProbeScript.Run(ctx, r.rdb, []string{r.key(targetID)},
		time.Now().UnixMilli(), healthStateTTL.Milliseconds(), probeLease.Milliseconds()).Int()
	if err != nil {
		return r.inner.IsBlocked(targetID, probeLease)
	}
	return blocked == 1
}

func (r *RedisHealthTracker) ReleaseProbe(targetID string) {
	if targetID == "" {
		return
	}
	r.inner.ReleaseProbe(targetID)
	if r.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRedisWait)
	defer cancel()
	if err := releaseHealthProbeScript.Run(ctx, r.rdb, []string{r.key(targetID)}, healthStateTTL.Milliseconds()).Err(); err != nil {
		zap.L().Debug("health redis probe release failed", zap.String("target_id", targetID), zap.Error(err))
	}
}

func (r *RedisHealthTracker) StateOf(targetID string) HealthState {
	if targetID == "" {
		return StateClosed
	}
	if r.rdb == nil {
		return r.inner.StateOf(targetID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRedisWait)
	defer cancel()
	value, err := r.rdb.HGet(ctx, r.key(targetID), "state").Int()
	if err == redis.Nil {
		return StateClosed
	}
	if err != nil {
		return r.inner.StateOf(targetID)
	}
	return HealthState(value)
}

func (r *RedisHealthTracker) StatesOf(targetIDs []string) map[string]HealthState {
	out := make(map[string]HealthState, len(targetIDs))
	if len(targetIDs) == 0 {
		return out
	}
	if r.rdb == nil {
		return r.inner.StatesOf(targetIDs)
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthRedisWait)
	defer cancel()
	pipe := r.rdb.Pipeline()
	commands := make(map[string]*redis.StringCmd, len(targetIDs))
	for _, targetID := range targetIDs {
		if targetID == "" {
			continue
		}
		commands[targetID] = pipe.HGet(ctx, r.key(targetID), "state")
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return r.inner.StatesOf(targetIDs)
	}
	for _, targetID := range targetIDs {
		command, ok := commands[targetID]
		if !ok {
			out[targetID] = StateClosed
			continue
		}
		value, readErr := command.Int()
		if readErr == redis.Nil {
			out[targetID] = StateClosed
		} else if readErr != nil {
			out[targetID] = r.inner.StateOf(targetID)
		} else {
			out[targetID] = HealthState(value)
		}
	}
	return out
}

func (r *RedisHealthTracker) Snapshot() []HealthRecord {
	if r.rdb == nil {
		return r.inner.Snapshot()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	targetIDs, err := r.rdb.SMembers(ctx, healthIndexKey).Result()
	if err != nil {
		return r.inner.Snapshot()
	}
	pipe := r.rdb.Pipeline()
	commands := make(map[string]*redis.SliceCmd, len(targetIDs))
	for _, targetID := range targetIDs {
		commands[targetID] = pipe.HMGet(ctx, r.key(targetID),
			"kind", "state", "consec_fail", "opened_at_ms", "next_probe_at_ms")
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return r.inner.Snapshot()
	}
	records := make([]HealthRecord, 0, len(targetIDs))
	stale := make([]any, 0)
	for _, targetID := range targetIDs {
		values, readErr := commands[targetID].Result()
		if readErr != nil || len(values) != 5 {
			continue
		}
		if values[0] == nil {
			stale = append(stale, targetID)
			continue
		}
		kind := TargetKind(redisInt(values[0]))
		state := HealthState(redisInt(values[1]))
		record := HealthRecord{
			TargetID:   targetID,
			Kind:       kind,
			State:      state,
			StateStr:   state.String(),
			ConsecFail: redisInt(values[2]),
		}
		if openedAt := redisInt64(values[3]); openedAt > 0 {
			value := time.UnixMilli(openedAt)
			record.OpenedAt = &value
		}
		if nextProbeAt := redisInt64(values[4]); nextProbeAt > 0 {
			value := time.UnixMilli(nextProbeAt)
			record.NextProbeAt = &value
		}
		records = append(records, record)
	}
	if len(stale) > 0 {
		_ = r.rdb.SRem(ctx, healthIndexKey, stale...).Err()
	}
	sort.Slice(records, func(i, j int) bool { return records[i].TargetID < records[j].TargetID })
	return records
}

func (r *RedisHealthTracker) key(targetID string) string {
	return healthKeyPrefix + targetID
}

func redisInt(value any) int {
	parsed, _ := strconv.Atoi(redisString(value))
	return parsed
}

func redisInt64(value any) int64 {
	parsed, _ := strconv.ParseInt(redisString(value), 10, 64)
	return parsed
}

func redisString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}
