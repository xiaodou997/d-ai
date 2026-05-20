package routing

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"

	"github.com/redis/go-redis/v9"
)

const healthPubSubChannel = "uni_ai_api:health:events"

// healthEvent is the Redis Pub/Sub message published on every state transition.
type healthEvent struct {
	TargetID      string     `json:"tid"`
	Kind          TargetKind `json:"k"`
	Event         string     `json:"e"`             // "opened" | "closed"
	NextProbeAtNS int64      `json:"npa,omitempty"` // Unix nanoseconds
	OpenCount     int        `json:"oc,omitempty"`
}

// RedisHealthTracker wraps an InMemoryTracker and propagates state transitions
// to other nodes via Redis Pub/Sub. If Redis is unavailable, all operations
// degrade silently to single-node in-memory behaviour.
type RedisHealthTracker struct {
	inner *InMemoryTracker
	rdb   *redis.Client
}

// NewRedisHealthTracker creates a multi-node tracker.
// Call Start(ctx) in a goroutine to begin consuming remote events.
func NewRedisHealthTracker(inner *InMemoryTracker, rdb *redis.Client) *RedisHealthTracker {
	return &RedisHealthTracker{inner: inner, rdb: rdb}
}

// RecordSuccess delegates to the inner tracker and broadcasts a "closed" event
// when the state actually changes.
func (r *RedisHealthTracker) RecordSuccess(targetID string, kind TargetKind) {
	before := r.inner.State(targetID)
	r.inner.RecordSuccess(targetID, kind)
	if before != StateClosed {
		r.publish(healthEvent{TargetID: targetID, Kind: kind, Event: "closed"})
	}
}

// RecordFailure delegates to the inner tracker and broadcasts an "opened" event
// when a CLOSED→OPEN or HALF_OPEN→OPEN transition occurs.
func (r *RedisHealthTracker) RecordFailure(targetID string, kind TargetKind) {
	before := r.inner.State(targetID)
	r.inner.RecordFailure(targetID, kind)
	after := r.inner.State(targetID)
	if before != StateOpen && after == StateOpen {
		snap, ok := r.inner.entrySnapshot(targetID)
		if !ok {
			return
		}
		r.publish(healthEvent{
			TargetID:      targetID,
			Kind:          kind,
			Event:         "opened",
			NextProbeAtNS: snap.nextProbeAt.UnixNano(),
			OpenCount:     snap.openCount,
		})
	}
}

func (r *RedisHealthTracker) IsBlocked(targetID string) bool {
	return r.inner.IsBlocked(targetID)
}

// StateOf satisfies HealthTracker.StateOf: pure read, no probe-slot side effect.
func (r *RedisHealthTracker) StateOf(targetID string) HealthState {
	return r.inner.State(targetID)
}

func (r *RedisHealthTracker) Snapshot() []HealthRecord {
	return r.inner.Snapshot()
}

// Start subscribes to health events broadcast by peer nodes and updates the
// local tracker accordingly. Blocks until ctx is cancelled.
func (r *RedisHealthTracker) Start(ctx context.Context) {
	sub := r.rdb.Subscribe(ctx, healthPubSubChannel)
	defer func() { _ = sub.Close() }()
	ch := sub.Channel()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var e healthEvent
			if err := json.Unmarshal([]byte(msg.Payload), &e); err != nil {
				zap.L().Warn("health_redis: malformed event", zap.String("payload", msg.Payload))
				continue
			}
			r.inner.syncFromEvent(e)
		case <-ctx.Done():
			return
		}
	}
}

func (r *RedisHealthTracker) publish(e healthEvent) {
	payload, err := json.Marshal(e)
	if err != nil {
		return
	}
	if pubErr := r.rdb.Publish(context.Background(), healthPubSubChannel, payload).Err(); pubErr != nil {
		zap.L().Debug("health_redis: publish failed", zap.Error(pubErr))
	}
}
