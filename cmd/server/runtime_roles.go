package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"xiaodou/dai/internal/auth"
	"xiaodou/dai/internal/cleanup"
	"xiaodou/dai/internal/config"
)

// platformRuntimeRole owns the platform application bundle and the services
// used by process-level cleanup jobs. Its assembly function is the single
// place where platform startup and shutdown are projected to root health.
type platformRuntimeRole struct {
	modules     *platformModules
	sessions    *auth.SessionService
	activations *auth.ActivationService
	dataCleanup *cleanup.Service
}

func assemblePlatformRuntimeRole(ctx context.Context, cfg *config.Config, pool, billingPool *pgxpool.Pool, redisClient *redis.Client, logger *zap.Logger, shutdowns *shutdownStack, lifecycle *lifecycleHealth) (*platformRuntimeRole, error) {
	modules, err := buildPlatformModules(cfg, pool, billingPool, redisClient, logger)
	if err != nil {
		return nil, err
	}
	modules.Start(ctx)
	shutdowns.Add("platform modules", func(ctx context.Context) error {
		if err := modules.Stop(ctx); err != nil {
			return err
		}
		lifecycle.MarkStopped(healthPlatformModules)
		if modules.banReconciler != nil {
			lifecycle.MarkStopped(healthBanReconciler)
		}
		if modules.sched != nil {
			lifecycle.MarkStopped(healthScheduler)
		}
		return nil
	})
	lifecycle.MarkStarted(healthPlatformModules)
	if modules.banReconciler != nil {
		lifecycle.MarkStarted(healthBanReconciler)
	}
	if modules.sched != nil {
		lifecycle.MarkStarted(healthScheduler)
	}
	return &platformRuntimeRole{
		modules:     modules,
		sessions:    modules.Sessions,
		activations: modules.Activations,
		dataCleanup: modules.DataCleanup,
	}, nil
}

// aiRuntimeRole owns the AI application bundle. Keeping this boundary beside
// platformRuntimeRole makes the composition root explicit about two runtime
// roles while preserving their existing module-level lifecycle contracts.
type aiRuntimeRole struct {
	modules *aiModules
}

func assembleAIRuntimeRole(ctx context.Context, cfg *config.Config, pool, billingPool *pgxpool.Pool, redisClient *redis.Client, logger *zap.Logger, platform *platformRuntimeRole, shutdowns *shutdownStack, lifecycle *lifecycleHealth) (*aiRuntimeRole, error) {
	if platform == nil || platform.modules == nil {
		return nil, fmt.Errorf("platform runtime role is required")
	}
	modules, err := buildAIModules(cfg, pool, billingPool, redisClient, logger, aiPlatformDeps{
		JWT: platform.modules.JWT, ProxyNodes: platform.modules.ProxyNodes, Modules: platform.modules.Modules,
	})
	if err != nil {
		return nil, err
	}
	modules.Start(ctx)
	shutdowns.Add("AI modules", func(ctx context.Context) error {
		if err := modules.Stop(ctx); err != nil {
			return err
		}
		lifecycle.MarkStopped(healthAIModules)
		if modules.RuntimeGateway != nil && modules.RuntimeGateway.Health().Stopped {
			lifecycle.MarkStopped(healthRuntimeGateway)
		}
		if modules.AsyncTasks != nil {
			lifecycle.MarkStopped(healthAsyncTasks)
		}
		return nil
	})
	lifecycle.MarkStarted(healthAIModules)
	if modules.RuntimeGateway != nil && modules.RuntimeGateway.Health().Started {
		lifecycle.MarkStarted(healthRuntimeGateway)
	}
	if modules.AsyncTasks != nil {
		lifecycle.MarkStarted(healthAsyncTasks)
	}
	return &aiRuntimeRole{modules: modules}, nil
}
