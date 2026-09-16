package routing

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAvailabilityRealRedisReplicaAdmission(t *testing.T) {
	addr := os.Getenv("DAI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("DAI_TEST_REDIS_ADDR required")
	}
	ctx := context.Background()
	clients := []*redis.Client{redis.NewClient(&redis.Options{Addr: addr}), redis.NewClient(&redis.Options{Addr: addr})}
	for _, c := range clients {
		if err := c.Ping(ctx).Err(); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
	}
	services := []*RedisAvailability{NewRedisAvailability(clients[0]), NewRedisAvailability(clients[1])}
	scope := NewFaultScope("model", "direct_upstream", uuid.NewString(), "e", "", "m", "op")
	if err := services[0].Reset(ctx, []FaultScope{scope}); err != nil {
		t.Fatal(err)
	}
	defer clients[0].Del(ctx, availabilityKey(scope.Key), resourceIndex(scope.ResourceKind, scope.ResourceID))
	defer clients[0].SRem(ctx, "dai:availability:v2:scopes", availabilityKey(scope.Key))
	var count atomic.Int32
	var winner *AdmissionPermit
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, e := services[i%2].Acquire(ctx, []FaultScope{scope}, time.Minute)
			if e == nil {
				count.Add(1)
				mu.Lock()
				winner = p
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("replicas admitted %d", count.Load())
	}
	if _, err := services[1].Complete(ctx, winner, AvailabilityOutcome{}); err != nil {
		t.Fatal(err)
	}
	p, err := services[0].Acquire(ctx, []FaultScope{scope}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = services[0].Complete(ctx, winner, AvailabilityOutcome{Success: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = services[1].Acquire(ctx, []FaultScope{scope}, time.Minute); err == nil {
		t.Fatal("duplicate completion released another replica's lease")
	}
	services[1].Complete(ctx, p, AvailabilityOutcome{})
}

func TestAvailabilityRealRedisHealthyListAndResume(t *testing.T) {
	addr := os.Getenv("DAI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("DAI_TEST_REDIS_ADDR required")
	}
	ctx := context.Background()
	c := redis.NewClient(&redis.Options{Addr: addr})
	defer c.Close()
	s := NewRedisAvailability(c)
	scope := NewFaultScope("model", "direct_upstream", uuid.NewString(), "e", "", "m", "responses")
	defer c.Del(ctx, availabilityKey(scope.Key), resourceIndex(scope.ResourceKind, scope.ResourceID))
	defer c.SRem(ctx, "dai:availability:v2:scopes", availabilityKey(scope.Key))
	p, err := s.Acquire(ctx, []FaultScope{scope}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Complete(ctx, p, AvailabilityOutcome{Success: true}); err != nil {
		t.Fatal(err)
	}
	if records, err := s.List(ctx, scope.ResourceKind, scope.ResourceID); err != nil || len(records) != 1 {
		t.Fatalf("list=%+v err=%v", records, err)
	}
	if err = s.Suspend(ctx, scope, time.Now().Add(time.Minute), "test"); err != nil {
		t.Fatal(err)
	}
	if err = s.Resume(ctx, scope.ResourceKind, scope.ResourceID); err != nil {
		t.Fatal(err)
	}
	records, err := s.List(ctx, scope.ResourceKind, scope.ResourceID)
	if err != nil || len(records) != 1 || records[0].Phase != Recovering {
		t.Fatalf("resume=%+v err=%v", records, err)
	}
}

func TestAvailabilityDataCleanupPreservesLiveState(t *testing.T) {
	addr := os.Getenv("DAI_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("DAI_TEST_REDIS_ADDR required")
	}
	ctx := context.Background()
	c := redis.NewClient(&redis.Options{Addr: addr})
	defer c.Close()
	scope := NewFaultScope("model", "direct_upstream", uuid.NewString(), "e", "", "m", "responses")
	key := availabilityKey(scope.Key)
	defer c.Del(ctx, key)
	raw := `{"phase":"recovering","epoch":17,"owner":"live-permit","lease_until":9999999999999,"recent_trips":{}}`
	if err := c.Set(ctx, key, raw, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../../../scripts/cleanup_upstream_empty_history.lua")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{1, 0} {
		n, err := c.Eval(ctx, string(script), []string{key}).Int()
		if err != nil || n != want {
			t.Fatalf("cleanup=%d err=%v", n, err)
		}
	}
	data, err := c.Get(ctx, key).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err = json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state["phase"] != "recovering" || state["epoch"] != float64(17) || state["owner"] != "live-permit" || state["lease_until"] != float64(9999999999999) {
		t.Fatalf("state changed: %s", data)
	}
	if _, exists := state["recent_trips"]; exists {
		t.Fatal("empty history retained")
	}
	if ttl := c.PTTL(ctx, key).Val(); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL not preserved: %v", ttl)
	}
}
