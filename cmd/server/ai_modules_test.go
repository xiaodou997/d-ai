package main

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"xiaodou/dai/internal/ai/clientcatalog"
	"xiaodou/dai/internal/ai/gateway"
)

func TestAIModulesLifecycleIsSafeForEmptyBundle(t *testing.T) {
	modules := &aiModules{}
	modules.Start(context.Background())
	modules.Start(context.Background())
	modules.Stop(context.Background())
	modules.Stop(context.Background())
}

func TestAIModulesOwnsRuntimeGatewayTelemetryLifecycle(t *testing.T) {
	runtimeGateway := gateway.New(gateway.Deps{Logger: zap.NewNop()})
	modules := &aiModules{RuntimeGateway: runtimeGateway}

	modules.Start(context.Background())
	if health := runtimeGateway.Health(); !health.Started || health.Stopped {
		t.Fatalf("runtime gateway health after Start = %+v", health)
	}
	modules.Stop(context.Background())
	if health := runtimeGateway.Health(); !health.Started || !health.Stopped {
		t.Fatalf("runtime gateway health after Stop = %+v", health)
	}
}

func TestAIModulesOwnsClientCatalogLifecycle(t *testing.T) {
	catalog := clientcatalog.New(nil, nil, zap.NewNop())
	modules := &aiModules{clientCatalog: catalog}

	modules.Start(context.Background())
	if health := catalog.Health(); !health.Started || health.Stopped {
		t.Fatalf("client catalog health after Start = %+v", health)
	}
	modules.Stop(context.Background())
	if health := catalog.Health(); !health.Started || !health.Stopped {
		t.Fatalf("client catalog health after Stop = %+v", health)
	}
}
