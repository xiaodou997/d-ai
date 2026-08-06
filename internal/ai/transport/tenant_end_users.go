package transport

import (
	"context"
	"errors"

	"xiaodou/dai/internal/ai/platform"
	"xiaodou/dai/libs/go/httpx"
)

func ensureTenantOwnsEndUser(ctx context.Context, verifier TenantEndUserVerifier, tenantID, userID string) error {
	if verifier == nil {
		return httpx.ErrUnavailable.WithDetail("tenant end-user verifier is not configured")
	}
	if tenantID == "" || userID == "" {
		return httpx.ErrBadRequest.WithDetail("tenant id and user id are required")
	}
	if err := verifier.CheckTenantEndUser(ctx, tenantID, userID); err != nil {
		if errors.Is(err, platform.ErrEndUserNotFound) {
			return httpx.ErrNotFound.WithDetail("user is not found in current tenant").WithCause(err)
		}
		return httpx.ErrInternal.WithDetail("failed to verify tenant end user").WithCause(err)
	}
	return nil
}
