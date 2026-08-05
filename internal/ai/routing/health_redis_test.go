package routing

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisHealthTrackerSharesFailureCounterAcrossNodes(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first := NewRedisHealthTracker(NewInMemoryTracker(2, time.Minute), client)
	second := NewRedisHealthTracker(NewInMemoryTracker(2, time.Minute), client)
	first.RecordFailure("account-1", TargetAccount)
	if got := second.StateOf("account-1"); got != StateClosed {
		t.Fatalf("state after first failure = %s, want closed", got)
	}
	second.RecordFailure("account-1", TargetAccount)
	if got := first.StateOf("account-1"); got != StateOpen {
		t.Fatalf("state after cross-node threshold = %s, want open", got)
	}
	if !first.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("open target must be blocked on every node")
	}

	restarted := NewRedisHealthTracker(NewInMemoryTracker(2, time.Minute), client)
	if got := restarted.StateOf("account-1"); got != StateOpen {
		t.Fatalf("state after tracker restart = %s, want open", got)
	}
	second.RecordSuccess("account-1", TargetAccount)
	if got := restarted.StateOf("account-1"); got != StateClosed {
		t.Fatalf("state after remote success = %s, want closed", got)
	}
}

func TestRedisHealthTrackerClaimsSingleHalfOpenProbe(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first := NewRedisHealthTracker(NewInMemoryTracker(1, time.Millisecond), client)
	second := NewRedisHealthTracker(NewInMemoryTracker(1, time.Millisecond), client)
	first.RecordFailure("pool-1", TargetPool)
	time.Sleep(3 * time.Millisecond)
	if first.IsBlocked("pool-1", defaultProbeLease) {
		t.Fatal("first caller after probe deadline should claim the probe")
	}
	if !second.IsBlocked("pool-1", defaultProbeLease) {
		t.Fatal("second caller must be blocked while the shared probe is in flight")
	}

	records := second.Snapshot()
	if len(records) != 1 || records[0].TargetID != "pool-1" || records[0].State != StateHalfOpen {
		t.Fatalf("snapshot = %+v, want one half-open pool", records)
	}
}

func TestRedisHealthTrackerReclaimsExpiredHalfOpenProbe(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first := NewRedisHealthTracker(NewInMemoryTracker(1, time.Millisecond), client)
	second := NewRedisHealthTracker(NewInMemoryTracker(1, time.Millisecond), client)
	first.RecordFailure("account-1", TargetAccount)
	time.Sleep(3 * time.Millisecond)
	if first.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("first caller should claim the half-open probe")
	}
	if err := client.HSet(context.Background(), first.key("account-1"), "probe_until_ms", 0).Err(); err != nil {
		t.Fatalf("expire probe lease: %v", err)
	}
	if second.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("expired probe lease must be reclaimable after a worker disappears")
	}
	if !first.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("reclaimed probe lease must still admit only one caller")
	}
}

func TestRedisHealthTrackerReleasesSharedHalfOpenProbe(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first := NewRedisHealthTracker(NewInMemoryTracker(1, time.Millisecond), client)
	second := NewRedisHealthTracker(NewInMemoryTracker(1, time.Millisecond), client)
	first.RecordFailure("account-1", TargetAccount)
	time.Sleep(3 * time.Millisecond)
	if first.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("first caller should claim probe")
	}
	first.ReleaseProbe("account-1")
	if second.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("released shared probe should be immediately reusable")
	}
	if !first.IsBlocked("account-1", defaultProbeLease) {
		t.Fatal("reclaimed shared probe must still admit only one caller")
	}
}

func TestRedisHealthTrackerPrunesExpiredSnapshotIndexMembers(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	tracker := NewRedisHealthTracker(NewInMemoryTracker(2, time.Minute), client)
	tracker.RecordFailure("deleted-account", TargetAccount)
	if err := client.Del(context.Background(), tracker.key("deleted-account")).Err(); err != nil {
		t.Fatalf("delete health record: %v", err)
	}
	if records := tracker.Snapshot(); len(records) != 0 {
		t.Fatalf("snapshot = %+v, want no expired records", records)
	}
	indexed, err := client.SIsMember(context.Background(), healthIndexKey, "deleted-account").Result()
	if err != nil {
		t.Fatalf("read health index: %v", err)
	}
	if indexed {
		t.Fatal("expired target should be removed from the health index")
	}
}
