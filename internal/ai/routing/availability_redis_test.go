package routing

import (
	"context"
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
