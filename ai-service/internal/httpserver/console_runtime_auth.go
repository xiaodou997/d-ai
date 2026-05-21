package httpserver

import (
	"net/http"

	"go.uber.org/zap"

	"xiaodou/uni-ai-api/internal/domain"
)

func (s *Server) consoleRuntimeIdentity(w http.ResponseWriter, r *http.Request) (*domain.RuntimeIdentity, bool) {
	if s.jwksValidator == nil {
		writeRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusUnauthorized,
			"Authentication is not configured.", "invalid_token")
		return nil, false
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusUnauthorized,
			"Missing token.", "missing_token")
		return nil, false
	}

	claims, err := s.jwksValidator.ValidateToken(r.Context(), token)
	if err != nil {
		s.logger.Warn("console runtime token validation failed",
			zap.Error(err),
			zap.String("request_id", requestIDFromContext(r.Context())),
		)
		writeRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusUnauthorized,
			"Invalid token.", "invalid_token")
		return nil, false
	}

	if s.urmClientID != "" && claims.ClientID != s.urmClientID {
		writeRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusForbidden,
			"Token not authorized for this service.", "forbidden")
		return nil, false
	}
	if s.banSubscriber != nil && claims.UserID != "" && s.banSubscriber.IsBanned(claims.UserID) {
		writeRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusForbidden,
			"Account banned.", "account_banned")
		return nil, false
	}

	role := roleForUserType(claims.UserType)
	var ownerType domain.OwnerType
	userID := ""
	switch role {
	case apiRoleTenant:
		ownerType = domain.OwnerTenant
	case apiRoleUser:
		ownerType = domain.OwnerUser
		userID = claims.UserID
	default:
		writeRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusForbidden,
			"Console chat is only available for tenant and user accounts.", "forbidden")
		return nil, false
	}

	setRequestIdentity(r.Context(), claims.TenantID, userID, string(role))
	return &domain.RuntimeIdentity{
		AuthMethod:    domain.AuthMethodJWT,
		RequestSource: domain.RequestSourceWebChat,
		OwnerType:     ownerType,
		TenantID:      claims.TenantID,
		UserID:        userID,
	}, true
}
