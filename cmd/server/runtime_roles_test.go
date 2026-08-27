package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAssembleAIRuntimeRoleRequiresPlatform(t *testing.T) {
	if _, err := assembleAIRuntimeRole(context.Background(), nil, nil, nil, nil, nil, nil, nil, nil, runtimeRoleAll); err == nil {
		t.Fatal("expected missing platform runtime role to fail")
	}
}

func TestParseRuntimeRoleAndCapabilities(t *testing.T) {
	tests := []struct {
		arg                       string
		role                      runtimeRole
		control, gateway, workers bool
	}{
		{"", runtimeRoleAll, true, true, true},
		{"control-api", runtimeRoleControlAPI, true, false, false},
		{"gateway", runtimeRoleGateway, false, true, false},
		{"worker", runtimeRoleWorker, false, false, true},
	}
	for _, tt := range tests {
		got, err := parseRuntimeRole([]string{tt.arg})
		if err != nil || got != tt.role {
			t.Fatalf("parseRuntimeRole(%q) = %q, %v", tt.arg, got, err)
		}
		if got.HasControlAPI() != tt.control || got.HasGateway() != tt.gateway || got.HasWorkers() != tt.workers {
			t.Fatalf("capabilities for %q = control:%v gateway:%v workers:%v", got, got.HasControlAPI(), got.HasGateway(), got.HasWorkers())
		}
	}
	if _, err := parseRuntimeRole([]string{"invalid"}); err == nil {
		t.Fatal("expected invalid runtime role to fail")
	}
}

func TestHTTPServersCanRunManagementOnly(t *testing.T) {
	servers := newHTTPServers(httpServerOptions{
		ManagementAddr:    "127.0.0.1:0",
		ManagementHandler: http.NewServeMux(),
	})
	if servers.public != nil {
		t.Fatal("management-only role must not create a public listener")
	}
	servers.Start(nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := servers.Shutdown(ctx); err != nil {
		t.Fatalf("management-only shutdown = %v", err)
	}
}
