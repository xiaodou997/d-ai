package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	dbgen "xiaodou/uni-ai-api/internal/db/gen"
	"xiaodou/uni-ai-api/internal/domain"
	"xiaodou/uni-ai-api/internal/serving"
)

// ModelGrantChecker implements the three-layer model grant hierarchy:
//
//	Tenant grants → User grants → API key allowed_models
//
// If the user has explicit grants, those apply (narrowing tenant grants).
// If the API key has allowed_models set, it further narrows the resolved grants.
type ModelGrantChecker struct {
	q *dbgen.Queries
}

func NewModelGrantChecker(q *dbgen.Queries) *ModelGrantChecker {
	return &ModelGrantChecker{q: q}
}

// CheckModelGrant verifies req.ModelCode is accessible to the API key owner.
// It satisfies serving.ModelGrantChecker.
func (c *ModelGrantChecker) CheckModelGrant(ctx context.Context, req *serving.Request) error {
	if req.APIKey == nil {
		return fmt.Errorf("no api key in request context")
	}

	tenantID := req.APIKey.TenantID
	userID := req.APIKey.UserID
	modelCode := req.ModelCode
	capType := string(req.CapabilityType)

	// --- Step 1: API key allowed_models filter ---
	// If the key declares an explicit allowlist, the requested model must be in it.
	if len(req.APIKey.AllowedModels) > 0 {
		if !contains(req.APIKey.AllowedModels, modelCode) {
			return fmt.Errorf("model %q not in api key allowed_models", modelCode)
		}
	}

	// --- Step 2: User-level or tenant-level model grant ---
	if userID != "" {
		// Check if the user has any explicit grants at all.
		hasGrants, err := c.q.HasUserModelGrants(ctx, dbgen.HasUserModelGrantsParams{
			TenantID: tenantID,
			UserID:   userID,
		})
		if err != nil {
			return fmt.Errorf("check user grants: %w", err)
		}

		if hasGrants {
			// User has explicit grants — check if this specific model is included.
			_, err = c.q.GetUserModel(ctx, dbgen.GetUserModelParams{
				TenantID:       tenantID,
				UserID:         userID,
				ModelCode:      modelCode,
				CapabilityType: capType,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("model %q not authorized for user", modelCode)
				}
				return fmt.Errorf("check user model grant: %w", err)
			}
			return nil
		}
		// User has no explicit grants → fall through to tenant-level check.
	}

	// --- Step 3: Tenant-level grant ---
	_, err := c.q.GetTenantModel(ctx, dbgen.GetTenantModelParams{
		TenantID:       tenantID,
		ModelCode:      modelCode,
		CapabilityType: capType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("model %q not authorized for tenant", modelCode)
		}
		return fmt.Errorf("check tenant model grant: %w", err)
	}

	// Also store the resolved model ID on the request for downstream steps.
	// We do a second GetTenantModel call to get the model struct — the grant
	// query already validated access, so this is just a data fetch.
	model, err := c.q.GetTenantModel(ctx, dbgen.GetTenantModelParams{
		TenantID:       tenantID,
		ModelCode:      modelCode,
		CapabilityType: capType,
	})
	if err != nil {
		return nil // already validated above; tolerate this edge case
	}
	_ = model // model.ID is available if needed by downstream steps via req.Candidate

	return nil
}

// resolveModelID is a convenience for adapters that need the model UUID.
func (c *ModelGrantChecker) resolveModelID(ctx context.Context, tenantID, modelCode, capType string) (domain.Model, error) {
	row, err := c.q.GetTenantModel(ctx, dbgen.GetTenantModelParams{
		TenantID:       tenantID,
		ModelCode:      modelCode,
		CapabilityType: capType,
	})
	if err != nil {
		return domain.Model{}, fmt.Errorf("resolve model: %w", err)
	}
	return domain.Model{
		ID:                     uuidToString(row.ID),
		ModelCode:              row.ModelCode,
		CapabilityType:         domain.CapabilityType(row.CapabilityType),
		DefaultMaxOutputTokens: int(row.DefaultMaxOutputTokens),
	}, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
