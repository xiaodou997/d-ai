package httpserver

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

const upstreamDeploymentCooldownTTL = 60 * time.Second
const conversationBindingTTL = 24 * time.Hour

type conversationBinding struct {
	UpstreamDeploymentID string `json:"upstream_deployment_id"`
	EndpointID           string `json:"endpoint_id"`
}

// chooseRoute selects a model route with weighted random selection
// Routes are grouped by priority (lower = higher priority)
// Within the same priority group, routes are selected by route_weight * endpoint_weight
func (s *Server) chooseRoute(ctx context.Context, routes []dbgen.ListRoutesForModelRow, stickyKey string) (dbgen.ListRoutesForModelRow, bool) {
	if len(routes) == 0 {
		return dbgen.ListRoutesForModelRow{}, false
	}
	if s.redis == nil {
		return chooseWeightedRoute(routes)
	}

	if stickyKey != "" {
		if route, ok := s.getStickyRoute(ctx, routes, stickyKey); ok {
			return route, true
		}
	}

	candidates := make([]dbgen.ListRoutesForModelRow, 0, len(routes))
	for _, route := range routes {
		cooling, err := s.upstreamDeploymentInCooldown(ctx, route)
		if err != nil {
			s.logger.Error("deployment cooldown check failed", "error", err, "upstream_deployment_id", route.UpstreamDeploymentID.String())
			candidates = append(candidates, route)
			continue
		}
		if !cooling {
			candidates = append(candidates, route)
		}
	}

	chosen, ok := chooseWeightedRoute(candidates)
	if ok && stickyKey != "" {
		s.setStickyRoute(ctx, stickyKey, chosen)
	}
	return chosen, ok
}

func (s *Server) getStickyRoute(ctx context.Context, routes []dbgen.ListRoutesForModelRow, stickyKey string) (dbgen.ListRoutesForModelRow, bool) {
	raw, err := s.redis.Get(ctx, stickyKey).Result()
	if err != nil {
		return dbgen.ListRoutesForModelRow{}, false
	}
	var binding conversationBinding
	if err := json.Unmarshal([]byte(raw), &binding); err != nil {
		return dbgen.ListRoutesForModelRow{}, false
	}
	for _, route := range routes {
		if route.UpstreamDeploymentID.String() != binding.UpstreamDeploymentID || route.EndpointID.String() != binding.EndpointID {
			continue
		}
		cooling, err := s.upstreamDeploymentInCooldown(ctx, route)
		if err != nil {
			s.logger.Error("sticky deployment cooldown check failed", "error", err, "upstream_deployment_id", route.UpstreamDeploymentID.String())
			return route, true
		}
		if !cooling {
			return route, true
		}
		return dbgen.ListRoutesForModelRow{}, false
	}
	return dbgen.ListRoutesForModelRow{}, false
}

func (s *Server) setStickyRoute(ctx context.Context, stickyKey string, route dbgen.ListRoutesForModelRow) {
	value, err := json.Marshal(conversationBinding{
		UpstreamDeploymentID: route.UpstreamDeploymentID.String(),
		EndpointID:           route.EndpointID.String(),
	})
	if err != nil {
		return
	}
	if err := s.redis.Set(ctx, stickyKey, value, conversationBindingTTL).Err(); err != nil {
		s.logger.Error("set conversation binding failed", "error", err, "upstream_deployment_id", route.UpstreamDeploymentID.String())
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

func (s *Server) upstreamDeploymentInCooldown(ctx context.Context, route dbgen.ListRoutesForModelRow) (bool, error) {
	if s.redis == nil {
		return false, nil
	}
	value, err := s.redis.Exists(ctx, upstreamDeploymentCooldownKey(route.UpstreamDeploymentID)).Result()
	if err != nil {
		return false, err
	}
	return value > 0, nil
}

func (s *Server) markUpstreamDeploymentCooldown(ctx context.Context, upstreamDeploymentID pgtype.UUID, reason string) {
	if s.redis == nil {
		return
	}
	err := s.redis.Set(ctx, upstreamDeploymentCooldownKey(upstreamDeploymentID), reason, upstreamDeploymentCooldownTTL).Err()
	if err != nil {
		s.logger.Error("mark deployment cooldown failed", "error", err, "upstream_deployment_id", upstreamDeploymentID.String())
	}
}

func upstreamDeploymentCooldownKey(upstreamDeploymentID pgtype.UUID) string {
	return "uni_ai_api:deployment:" + upstreamDeploymentID.String() + ":cooldown"
}

func shouldCooldownUpstreamStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func chooseWeightedRoute(routes []dbgen.ListRoutesForModelRow) (dbgen.ListRoutesForModelRow, bool) {
	if len(routes) == 0 {
		return dbgen.ListRoutesForModelRow{}, false
	}

	priority := routes[0].RoutePriority
	for _, route := range routes[1:] {
		if route.RoutePriority < priority {
			priority = route.RoutePriority
		}
	}

	candidates := make([]dbgen.ListRoutesForModelRow, 0, len(routes))
	totalWeight := int64(0)
	for _, route := range routes {
		if route.RoutePriority != priority {
			continue
		}
		weight := routeWeight(route)
		if weight <= 0 {
			continue
		}
		candidates = append(candidates, route)
		totalWeight += weight
	}

	if len(candidates) == 0 {
		return dbgen.ListRoutesForModelRow{}, false
	}
	if totalWeight <= 0 {
		return candidates[0], true
	}

	pick := rand.Int63n(totalWeight) + 1
	running := int64(0)
	for _, candidate := range candidates {
		running += routeWeight(candidate)
		if pick <= running {
			return candidate, true
		}
	}

	return candidates[len(candidates)-1], true
}

func routeWeight(route dbgen.ListRoutesForModelRow) int64 {
	routeWeight := int64(route.RouteWeight)
	endpointWeight := int64(route.EndpointWeight)
	if routeWeight <= 0 || endpointWeight <= 0 {
		return 0
	}
	return routeWeight * endpointWeight
}

func filterStreamingRoutes(routes []dbgen.ListRoutesForModelRow) []dbgen.ListRoutesForModelRow {
	filtered := make([]dbgen.ListRoutesForModelRow, 0, len(routes))
	for _, route := range routes {
		if route.SupportsStream {
			filtered = append(filtered, route)
		}
	}
	return filtered
}
