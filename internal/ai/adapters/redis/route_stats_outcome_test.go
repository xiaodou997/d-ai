package redis

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestRouteStatsExcludeFailuresFromLatencyAndExpireIndividualLeases(t *testing.T) {
	mini := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mini.Addr()})
	defer client.Close()
	s := NewRedisRouteStats(client)
	ctx := context.Background()
	s.Observe(ctx, "model", true, false, 200, 500)
	s.Observe(ctx, "model", false, true, 1, 1)
	s.Observe(ctx, "model", false, false, 1, 1)
	v := s.Snapshot(ctx, []string{"model"})["model"]
	if v.Successes != 1 || v.Failures != 1 || v.EWMAFirstByteMs != 200 || v.EWMATotalMs != 500 {
		t.Fatal(v)
	}
	s.BeginInflight(ctx, "account:a", "old", time.Minute)
	s.BeginInflight(ctx, "account:a", "new", time.Minute)
	s.EndInflight(ctx, "account:a", "old")
	s.EndInflight(ctx, "account:a", "old")
	if v = s.Snapshot(ctx, []string{"account:a"})["account:a"]; v.InflightCount != 1 {
		t.Fatal(v)
	}
	// A crashed call is removed from scoring even when continuing traffic keeps
	// the containing Redis key alive.
	if err := client.ZAdd(ctx, inflightLeasePrefix+"account:a", goredis.Z{Score: float64(time.Now().Add(-time.Second).UnixMilli()), Member: "crashed"}).Err(); err != nil {
		t.Fatal(err)
	}
	if v = s.Snapshot(ctx, []string{"account:a"})["account:a"]; v.InflightCount != 1 {
		t.Fatal(v)
	}
	s.EndInflight(ctx, "account:a", "new")
}
