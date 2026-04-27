package httpserver

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"time"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

const endpointCooldownTTL = 60 * time.Second
const conversationBindingTTL = 24 * time.Hour

type conversationBinding struct {
	DeploymentID string `json:"deployment_id"`
	EndpointID   string `json:"endpoint_id"`
}

func (s *Server) chooseDeployment(ctx context.Context, deployments []dbgen.ListDeploymentsForModelRow, stickyKey string) (dbgen.ListDeploymentsForModelRow, bool) {
	if len(deployments) == 0 {
		return dbgen.ListDeploymentsForModelRow{}, false
	}
	if s.redis == nil {
		return chooseWeightedDeployment(deployments)
	}

	if stickyKey != "" {
		if deployment, ok := s.getStickyDeployment(ctx, deployments, stickyKey); ok {
			return deployment, true
		}
	}

	candidates := make([]dbgen.ListDeploymentsForModelRow, 0, len(deployments))
	for _, deployment := range deployments {
		cooling, err := s.endpointInCooldown(ctx, deployment)
		if err != nil {
			s.logger.Error("endpoint cooldown check failed", "error", err, "endpoint_id", deployment.EndpointID.String())
			candidates = append(candidates, deployment)
			continue
		}
		if !cooling {
			candidates = append(candidates, deployment)
		}
	}

	chosen, ok := chooseWeightedDeployment(candidates)
	if ok && stickyKey != "" {
		s.setStickyDeployment(ctx, stickyKey, chosen)
	}
	return chosen, ok
}

func (s *Server) getStickyDeployment(ctx context.Context, deployments []dbgen.ListDeploymentsForModelRow, stickyKey string) (dbgen.ListDeploymentsForModelRow, bool) {
	raw, err := s.redis.Get(ctx, stickyKey).Result()
	if err != nil {
		return dbgen.ListDeploymentsForModelRow{}, false
	}
	var binding conversationBinding
	if err := json.Unmarshal([]byte(raw), &binding); err != nil {
		return dbgen.ListDeploymentsForModelRow{}, false
	}
	for _, deployment := range deployments {
		if deployment.DeploymentID.String() != binding.DeploymentID || deployment.EndpointID.String() != binding.EndpointID {
			continue
		}
		cooling, err := s.endpointInCooldown(ctx, deployment)
		if err != nil {
			s.logger.Error("sticky endpoint cooldown check failed", "error", err, "endpoint_id", deployment.EndpointID.String())
			return deployment, true
		}
		if !cooling {
			return deployment, true
		}
		return dbgen.ListDeploymentsForModelRow{}, false
	}
	return dbgen.ListDeploymentsForModelRow{}, false
}

func (s *Server) setStickyDeployment(ctx context.Context, stickyKey string, deployment dbgen.ListDeploymentsForModelRow) {
	value, err := json.Marshal(conversationBinding{
		DeploymentID: deployment.DeploymentID.String(),
		EndpointID:   deployment.EndpointID.String(),
	})
	if err != nil {
		return
	}
	if err := s.redis.Set(ctx, stickyKey, value, conversationBindingTTL).Err(); err != nil {
		s.logger.Error("set conversation binding failed", "error", err, "deployment_id", deployment.DeploymentID.String())
	}
}

func conversationStickyKey(auth RuntimeAuth, modelCode string, conversationID string) string {
	if conversationID == "" {
		return ""
	}
	identity := auth.APIKey.TenantID
	if auth.APIKey.UserID.Valid {
		identity = auth.APIKey.UserID.String
	}
	return "uni_ai_api:conv:" + auth.APIKey.TenantID + ":" + identity + ":" + modelCode + ":" + conversationID
}

func (s *Server) endpointInCooldown(ctx context.Context, deployment dbgen.ListDeploymentsForModelRow) (bool, error) {
	if s.redis == nil {
		return false, nil
	}
	value, err := s.redis.Exists(ctx, endpointCooldownKey(deployment)).Result()
	if err != nil {
		return false, err
	}
	return value > 0, nil
}

func (s *Server) markEndpointCooldown(ctx context.Context, deployment dbgen.ListDeploymentsForModelRow, reason string) {
	if s.redis == nil {
		return
	}
	err := s.redis.Set(ctx, endpointCooldownKey(deployment), reason, endpointCooldownTTL).Err()
	if err != nil {
		s.logger.Error("mark endpoint cooldown failed", "error", err, "endpoint_id", deployment.EndpointID.String())
	}
}

func endpointCooldownKey(deployment dbgen.ListDeploymentsForModelRow) string {
	return "uni_ai_api:endpoint:" + deployment.EndpointID.String() + ":cooldown"
}

func shouldCooldownUpstreamStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func chooseWeightedDeployment(deployments []dbgen.ListDeploymentsForModelRow) (dbgen.ListDeploymentsForModelRow, bool) {
	if len(deployments) == 0 {
		return dbgen.ListDeploymentsForModelRow{}, false
	}

	priority := deployments[0].Priority
	for _, deployment := range deployments[1:] {
		if deployment.Priority < priority {
			priority = deployment.Priority
		}
	}

	candidates := make([]dbgen.ListDeploymentsForModelRow, 0, len(deployments))
	totalWeight := int64(0)
	for _, deployment := range deployments {
		if deployment.Priority != priority {
			continue
		}
		weight := deploymentRouteWeight(deployment)
		if weight <= 0 {
			continue
		}
		candidates = append(candidates, deployment)
		totalWeight += weight
	}

	if len(candidates) == 0 {
		return dbgen.ListDeploymentsForModelRow{}, false
	}
	if totalWeight <= 0 {
		return candidates[0], true
	}

	pick := rand.Int63n(totalWeight) + 1
	running := int64(0)
	for _, candidate := range candidates {
		running += deploymentRouteWeight(candidate)
		if pick <= running {
			return candidate, true
		}
	}

	return candidates[len(candidates)-1], true
}

func deploymentRouteWeight(deployment dbgen.ListDeploymentsForModelRow) int64 {
	deploymentWeight := int64(deployment.DeploymentWeight)
	endpointWeight := int64(deployment.EndpointWeight)
	if deploymentWeight <= 0 || endpointWeight <= 0 {
		return 0
	}
	return deploymentWeight * endpointWeight
}
