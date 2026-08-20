package console

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/gateway"
	"xiaodou/dai/internal/ai/httpx"
)

// bannedMessage checks tenantID (always) and userID (when non-empty)
// against bc, tenant first. Returns the message/code to surface and true
// when either is banned. No-op (returns false) when bc is nil.
func bannedMessage(ctx context.Context, bc BanChecker, tenantID, userID string) (msg, code string, status int, blocked bool, err error) {
	if bc == nil {
		return "", "", 0, false, nil
	}
	if tenantID != "" {
		banned, err := bc.IsTenantBanned(ctx, tenantID)
		if err != nil {
			return "Unable to verify account status.", "service_unavailable", http.StatusServiceUnavailable, true, err
		}
		if banned {
			return "Tenant is disabled.", "tenant_banned", http.StatusForbidden, true, nil
		}
	}
	if userID != "" {
		banned, err := bc.IsBanned(ctx, userID)
		if err != nil {
			return "Unable to verify account status.", "service_unavailable", http.StatusServiceUnavailable, true, err
		}
		if banned {
			return "Account banned.", "account_banned", http.StatusForbidden, true, nil
		}
	}
	return "", "", 0, false, nil
}

func (s *Console) consoleRuntimeSubject(w http.ResponseWriter, r *http.Request) (*coreidentity.Subject, bool) {
	if s.tokenVerifier == nil {
		gateway.WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusUnauthorized,
			"Authentication is not configured.", "invalid_token")
		return nil, false
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		gateway.WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusUnauthorized,
			"Missing token.", "missing_token")
		return nil, false
	}

	claims, err := s.tokenVerifier.ParseToken(token)
	if err != nil || claims == nil || claims.PrincipalType != "user" || claims.TokenUse != "access" || claims.SessionID == "" {
		s.logger.Warn("runtime token validation failed",
			consoleRequestLogFields(r, zap.Error(err))...,
		)
		gateway.WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusUnauthorized,
			"Invalid token.", "invalid_token")
		return nil, false
	}

	if msg, code, status, blocked, err := bannedMessage(r.Context(), s.banChecker, claims.TenantID, claims.UserID); blocked {
		if err != nil {
			s.logger.Warn("runtime ban check failed",
				consoleRequestLogFields(r,
					zap.Error(err),
					zap.String("tenant_id", claims.TenantID),
					zap.String("user_id", claims.UserID),
				)...,
			)
		}
		gateway.WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, status, msg, code)
		return nil, false
	}

	role := roleForUserType(claims.UserType)
	scope := coreidentity.ScopeTenant
	userID := ""
	switch role {
	case apiRoleTenant:
	case apiRoleUser:
		scope = coreidentity.ScopeUser
		userID = claims.UserID
	default:
		gateway.WriteRuntimeErrorByProtocol(w, domain.ProtocolOpenAIChat, http.StatusForbidden,
			"Web runtime is only available for tenant and user accounts.", "forbidden")
		return nil, false
	}

	httpx.SetIdentity(r.Context(), claims.TenantID, userID, string(role))
	return &coreidentity.Subject{
		AuthMethod:    coreidentity.AuthMethodJWT,
		RequestSource: coreidentity.RequestSourceWebChat,
		Scope:         scope,
		TenantID:      claims.TenantID,
		UserID:        userID,
	}, true
}

func consoleSubjectOwnerType(subject *coreidentity.Subject) domain.OwnerType {
	if subject == nil {
		return domain.OwnerTenant
	}
	if subject.Scope == coreidentity.ScopeUser {
		return domain.OwnerUser
	}
	return domain.OwnerTenant
}
