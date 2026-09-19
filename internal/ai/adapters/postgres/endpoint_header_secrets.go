package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/upstreamcontrol"
	"xiaodou/dai/internal/clientsecret"
)

const endpointHeaderSecretPrefix = "enc:v1:"

// ProtectSensitiveEndpointHeaders migrates legacy plaintext sensitive
// extra_headers values to the process keyring. It is safe for concurrent
// startup: updates use the original JSON value as a compare-and-swap guard.
func ProtectSensitiveEndpointHeaders(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("postgres pool is required")
	}
	rows, err := pool.Query(ctx, `
		SELECT id::text, extra_headers
		FROM ai_upstream_account_endpoints
		ORDER BY id
	`)
	if err != nil {
		return fmt.Errorf("list upstream endpoint headers: %w", err)
	}
	type storedEndpoint struct {
		id  string
		raw []byte
	}
	items := make([]storedEndpoint, 0)
	for rows.Next() {
		var item storedEndpoint
		if err := rows.Scan(&item.id, &item.raw); err != nil {
			rows.Close()
			return fmt.Errorf("scan upstream endpoint headers: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate upstream endpoint headers: %w", err)
	}
	rows.Close()

	for _, item := range items {
		protected, changed, err := protectStoredEndpointHeaders(item.raw)
		if err != nil {
			return fmt.Errorf("protect upstream endpoint %s headers: %w", item.id, err)
		}
		if !changed {
			continue
		}
		if _, err := pool.Exec(ctx, `
			UPDATE ai_upstream_account_endpoints
			SET extra_headers = $2::jsonb, updated_at = now()
			WHERE id = $1::uuid AND extra_headers = $3::jsonb
		`, item.id, protected, item.raw); err != nil {
			return fmt.Errorf("persist protected upstream endpoint %s headers: %w", item.id, err)
		}
	}
	return nil
}

func protectStoredEndpointHeaders(raw []byte) ([]byte, bool, error) {
	if len(raw) == 0 {
		return raw, false, nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, false, fmt.Errorf("decode extra_headers: %w", err)
	}
	changed := false
	for key, value := range headers {
		if !upstreamcontrol.IsSensitiveHeaderKey(key) || value == "" {
			continue
		}
		plaintext := value
		if strings.HasPrefix(value, endpointHeaderSecretPrefix) {
			decrypted, err := clientsecret.Decrypt(value)
			if err != nil {
				return nil, false, fmt.Errorf("decrypt sensitive header %q: %w", key, err)
			}
			if !clientsecret.NeedsReencrypt(value) {
				continue
			}
			plaintext = decrypted
		}
		protected, err := clientsecret.Encrypt(plaintext)
		if err != nil {
			return nil, false, fmt.Errorf("encrypt sensitive header %q: %w", key, err)
		}
		headers[key] = protected
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	protected, err := json.Marshal(headers)
	if err != nil {
		return nil, false, fmt.Errorf("encode protected extra_headers: %w", err)
	}
	return protected, true, nil
}

func decryptStoredEndpointHeaders(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, fmt.Errorf("decode extra_headers: %w", err)
	}
	decrypted, err := decryptStoredEndpointHeaderMap(headers)
	if err != nil {
		return nil, err
	}
	return json.Marshal(decrypted)
}

func decryptStoredEndpointHeaderMap(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return headers, nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
		if !upstreamcontrol.IsSensitiveHeaderKey(key) || !strings.HasPrefix(value, endpointHeaderSecretPrefix) {
			continue
		}
		plaintext, err := clientsecret.Decrypt(value)
		if err != nil {
			return nil, fmt.Errorf("decrypt sensitive header %q: %w", key, err)
		}
		out[key] = plaintext
	}
	return out, nil
}
