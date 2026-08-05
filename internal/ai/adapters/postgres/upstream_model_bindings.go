package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
)

const (
	upstreamKindDirect = "direct_upstream"
	upstreamKindPool   = "oauth_pool"
)

type upstreamModelBindingRow struct {
	ModelCode         string
	CapabilityType    domain.CapabilityType
	APIFormat         domain.UpstreamProtocol
	UpstreamModelName string
	Status            string
	Config            map[string]any
}

// loadUpstreamModelBinding loads the single active binding for a given
// (upstreamKind, upstreamID, modelCode, capability). The DB unique constraint
// guarantees at most one row. Returns pgx.ErrNoRows when no binding exists.
func loadUpstreamModelBinding(
	ctx context.Context,
	db dbgen.DBTX,
	upstreamKind string,
	upstreamID string,
	modelCode string,
	capability domain.CapabilityType,
) (upstreamModelBindingRow, error) {
	var item upstreamModelBindingRow
	var capabilityType string
	var apiFormat string
	var configJSON []byte
	err := db.QueryRow(ctx, `
		SELECT
			model_code,
			capability_type,
			api_format,
			upstream_model_name,
			status,
			config_json
		FROM ai_upstream_models
		WHERE upstream_kind = $1
		  AND upstream_id = $2::uuid
		  AND model_code = $3
		  AND capability_type = $4
		  AND status = 'active'
		LIMIT 1
	`, upstreamKind, upstreamID, strings.TrimSpace(modelCode), string(capability)).Scan(
		&item.ModelCode,
		&capabilityType,
		&apiFormat,
		&item.UpstreamModelName,
		&item.Status,
		&configJSON,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return upstreamModelBindingRow{}, err
		}
		return upstreamModelBindingRow{}, fmt.Errorf("load upstream model binding: %w", err)
	}
	item.CapabilityType = domain.CapabilityType(capabilityType)
	item.APIFormat = domain.UpstreamProtocol(apiFormat)
	item.Config = unmarshalBindingConfig(configJSON)
	return item, nil
}

func unmarshalBindingConfig(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
