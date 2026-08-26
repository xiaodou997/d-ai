package transport

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/auth"
)

type TokenVerifier interface {
	ParseToken(token string) (*auth.Claims, error)
}

type TokenRevocationChecker interface {
	IsBlacklisted(tokenID string) bool
	GetUserLogoutTime(userID string) int64
}

type HumaBanChecker interface {
	IsBanned(ctx context.Context, userID string) (bool, error)
	IsTenantBanned(ctx context.Context, tenantID string) (bool, error)
}

type TenantEndUserVerifier interface {
	CheckTenantEndUser(ctx context.Context, tenantID, userID string) error
}

// HTTPAuthDeps contains only the capabilities required by user-facing Huma
// authentication middleware. Route modules can own this bundle without
// receiving the complete AI transport dependency graph.
type HTTPAuthDeps struct {
	TokenVerifier    TokenVerifier
	TokenRevocations TokenRevocationChecker
	BanChecker       HumaBanChecker
}

type authClaimsContextKey struct{}

func platformUserAuth(api huma.API, d HTTPAuthDeps) func(huma.Context, func(huma.Context)) {
	return userAuth(api, d, map[int]bool{1: true, 2: true}, "platform user required")
}

func tenantUserAuth(api huma.API, d HTTPAuthDeps) func(huma.Context, func(huma.Context)) {
	return userAuth(api, d, map[int]bool{3: true}, "tenant user required")
}

func endUserAuth(api huma.API, d HTTPAuthDeps) func(huma.Context, func(huma.Context)) {
	return userAuth(api, d, map[int]bool{4: true}, "end user required")
}

func userAuth(api huma.API, d HTTPAuthDeps, allowedTypes map[int]bool, forbiddenMessage string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if d.TokenVerifier == nil {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication is not configured")
			return
		}

		token := bearerToken(ctx.Header("Authorization"))
		if token == "" {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing bearer token")
			return
		}

		claims, err := d.TokenVerifier.ParseToken(token)
		if err != nil || claims == nil || claims.PrincipalType != "user" || claims.TokenUse != "access" || claims.SessionID == "" {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		if tokenRevoked(d.TokenRevocations, claims) {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "bearer token has been revoked")
			return
		}
		if len(allowedTypes) > 0 && !allowedTypes[claims.UserType] {
			huma.WriteErr(api, ctx, http.StatusForbidden, forbiddenMessage)
			return
		}
		if status, msg, blocked := bannedMessage(ctx.Context(), d.BanChecker, claims.TenantID, claims.UserID); blocked {
			huma.WriteErr(api, ctx, status, msg)
			return
		}

		next(huma.WithValue(ctx, authClaimsContextKey{}, claims))
	}
}

func tokenRevoked(checker TokenRevocationChecker, claims *auth.Claims) bool {
	if checker == nil || claims == nil {
		return false
	}
	if checker.IsBlacklisted(claims.ID) {
		return true
	}
	return claims.IssuedAt != nil && checker.GetUserLogoutTime(claims.UserID) > claims.IssuedAt.Unix()
}

// bannedMessage checks tenantID (always) and userID (when non-empty)
// against bc, tenant first. Returns the HTTP status/message to surface and
// true when access should be blocked. No-op when bc is nil.
func bannedMessage(ctx context.Context, bc HumaBanChecker, tenantID, userID string) (int, string, bool) {
	if bc == nil {
		return 0, "", false
	}
	if tenantID != "" {
		banned, err := bc.IsTenantBanned(ctx, tenantID)
		if err != nil {
			return http.StatusServiceUnavailable, "ban check unavailable", true
		}
		if banned {
			return http.StatusForbidden, "tenant is disabled", true
		}
	}
	if userID != "" {
		banned, err := bc.IsBanned(ctx, userID)
		if err != nil {
			return http.StatusServiceUnavailable, "ban check unavailable", true
		}
		if banned {
			return http.StatusForbidden, "user is banned", true
		}
	}
	return 0, "", false
}

func claimsFromContext(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(authClaimsContextKey{}).(*auth.Claims)
	return claims
}

func tenantIDFromContext(ctx context.Context) string {
	claims := claimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return strings.TrimSpace(claims.TenantID)
}

func userIDFromContext(ctx context.Context) string {
	claims := claimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return strings.TrimSpace(claims.UserID)
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
