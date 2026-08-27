package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServersBuildsIndependentPublicAndManagementListeners(t *testing.T) {
	servers := newHTTPServers(httpServerOptions{
		PublicAddr:        ":19641",
		ManagementAddr:    "127.0.0.1:19642",
		PublicHandler:     http.NewServeMux(),
		ManagementHandler: http.NewServeMux(),
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	})

	if servers.public == nil || servers.public.Addr != ":19641" {
		t.Fatalf("public listener = %#v, want :19641", servers.public)
	}
	if servers.management == nil || servers.management.Addr != "127.0.0.1:19642" {
		t.Fatalf("management listener = %#v, want loopback listener", servers.management)
	}
	if servers.public.WriteTimeout != 0 {
		t.Fatalf("public write timeout = %s, want streaming-safe zero", servers.public.WriteTimeout)
	}
	if servers.management.WriteTimeout != 10*time.Second {
		t.Fatalf("management write timeout = %s, want 10s", servers.management.WriteTimeout)
	}
	if err := servers.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestHTTPServersStartIsIdempotentAndShutdownWaitsForListeners(t *testing.T) {
	servers := newHTTPServers(httpServerOptions{
		PublicAddr:        "127.0.0.1:0",
		ManagementAddr:    "127.0.0.1:0",
		PublicHandler:     http.NewServeMux(),
		ManagementHandler: http.NewServeMux(),
	})

	servers.Start(nil)
	servers.Start(nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := servers.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-servers.publicDone:
	default:
		t.Fatal("public listener goroutine remained after Shutdown")
	}
	select {
	case <-servers.managementDone:
	default:
		t.Fatal("management listener goroutine remained after Shutdown")
	}

	if err := servers.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}
