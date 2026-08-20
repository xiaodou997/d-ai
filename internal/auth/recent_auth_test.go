package auth

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRecentAuthExpiresAndFailsClosed(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mini.Close)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	service := NewRecentAuthService(client)
	ctx := context.Background()
	if err := service.Mark(ctx, "admin-1", "password"); err != nil {
		t.Fatal(err)
	}
	valid, err := service.Check(ctx, "admin-1")
	if err != nil || !valid {
		t.Fatalf("recent auth immediately after mark = valid:%v err:%v", valid, err)
	}
	mini.FastForward(recentAuthTTL)
	valid, err = service.Check(ctx, "admin-1")
	if err != nil || valid {
		t.Fatalf("recent auth after expiry = valid:%v err:%v", valid, err)
	}

	valid, err = NewRecentAuthService(nil).Check(ctx, "admin-1")
	if err == nil || valid {
		t.Fatalf("nil Redis check = valid:%v err:%v, want fail closed", valid, err)
	}
}
