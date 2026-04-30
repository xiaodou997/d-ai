package httpserver

import (
	"net/http"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

func (s *Server) logGatewayRequestReceived(r *http.Request, capability string, model string, stream bool) {
	s.logger.Debug("gateway request received",
		"request_id", requestIDFromContext(r.Context()),
		"capability_type", capability,
		"public_model", model,
		"stream", stream,
	)
}

func (s *Server) logGatewayRouteSelected(r *http.Request, capability string, model string, route dbgen.ListRoutesForModelRow) {
	s.logger.Debug("gateway route selected",
		"request_id", requestIDFromContext(r.Context()),
		"capability_type", capability,
		"public_model", model,
		"route_id", route.RouteID.String(),
		"provider_id", route.ProviderID.String(),
		"provider_code", route.ProviderCode,
		"endpoint_id", route.EndpointID.String(),
		"deployment_id", route.UpstreamDeploymentID.String(),
		"upstream_protocol", route.UpstreamProtocol,
		"upstream_model", route.UpstreamModel,
		"request_path", optionalText(route.RequestPath),
		"health_status", route.HealthStatus,
		"priority", route.RoutePriority,
		"weight", route.RouteWeight,
		"supports_stream", route.SupportsStream,
	)
}

func (s *Server) logGatewayUpstreamStarted(r *http.Request, capability string, model string, route dbgen.ListRoutesForModelRow) {
	s.logger.Debug("gateway upstream request started",
		"request_id", requestIDFromContext(r.Context()),
		"capability_type", capability,
		"public_model", model,
		"provider_code", route.ProviderCode,
		"deployment_id", route.UpstreamDeploymentID.String(),
		"upstream_protocol", route.UpstreamProtocol,
		"upstream_model", route.UpstreamModel,
		"request_path", optionalText(route.RequestPath),
		"timeout_ms", route.TimeoutMs,
	)
}

func (s *Server) logGatewayUpstreamFailed(r *http.Request, capability string, model string, route dbgen.ListRoutesForModelRow, statusCode int, body []byte) {
	s.logger.Warn("gateway upstream request failed",
		"request_id", requestIDFromContext(r.Context()),
		"capability_type", capability,
		"public_model", model,
		"provider_code", route.ProviderCode,
		"deployment_id", route.UpstreamDeploymentID.String(),
		"upstream_protocol", route.UpstreamProtocol,
		"upstream_model", route.UpstreamModel,
		"status", statusCode,
		"error_code", errorCodeUpstreamBadStatus,
		"upstream_error_body", upstreamErrorBodyForLog(body),
	)
}
