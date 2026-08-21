package redis

import (
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestHealthProbeChecksRedis(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	probe := NewHealthProbe(client)

	if err := probe.Check(t.Context()); err != nil {
		t.Fatalf("check healthy Redis: %v", err)
	}
	server.Close()
	if err := probe.Check(t.Context()); err == nil {
		t.Fatal("expected an error after Redis stops")
	}
}

func TestHealthProbeRejectsMissingClient(t *testing.T) {
	if err := NewHealthProbe(nil).Check(t.Context()); err == nil {
		t.Fatal("expected an error for a missing Redis client")
	}
}
