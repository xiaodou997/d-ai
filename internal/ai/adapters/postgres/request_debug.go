package postgres

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"go.uber.org/zap"
	"io"
	"strings"
	"time"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/serving"
)

const maxDebugBytes = 1 << 20

func (s *RequestStore) StartDebug(ctx context.Context, in domain.DebugSessionInput, operator string) (domain.DebugSession, error) {
	var out domain.DebugSession
	if in.TenantID == "" && in.APIKeyID == "" && in.Model == "" {
		return out, errors.New("debug recording requires tenant, API key or model scope")
	}
	if in.Hours == 0 {
		in.Hours = 1
	}
	if in.Hours < 1 || in.Hours > 24 {
		return out, errors.New("debug duration must be 1–24 hours")
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO ai_debug_sessions(tenant_id,api_key_id,model_code,created_by,expires_at) VALUES($1,$2,$3,$4,now()+make_interval(hours=>$5)) RETURNING id::text,expires_at`, in.TenantID, in.APIKeyID, in.Model, operator, in.Hours).Scan(&out.ID, &out.ExpiresAt)
	return out, err
}
func scrubDebug(v any) any {
	switch x := v.(type) {
	case map[string]any:
		mediaType, _ := x["type"].(string)
		for key, value := range x {
			if (mediaType == "image_generation_call" && key == "result") || (mediaType == "base64" && key == "data") || key == "bytesBase64Encoded" {
				x[key] = "[MEDIA OMITTED]"
				continue
			}
			name := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			switch name {
			case "authorization", "proxy_authorization", "api_key", "x_api_key", "access_token", "refresh_token", "password", "cookie", "set_cookie", "apikey", "accesstoken", "refreshtoken", "client_secret", "clientsecret":
				x[key] = "[REDACTED]"
			case "b64_json", "inline_data", "inlinedata":
				delete(x, key)
			default:
				if str, ok := value.(string); ok && strings.HasPrefix(str, "data:") {
					x[key] = "[MEDIA OMITTED]"
				} else {
					x[key] = scrubDebug(value)
				}
			}
		}
		return x
	case []any:
		for i := range x {
			x[i] = scrubDebug(x[i])
		}
		return x
	case string:
		if strings.HasPrefix(x, "data:") {
			return "[MEDIA OMITTED]"
		}
		return serving.RedactCredentialText(x)
	default:
		return v
	}
}
func (s *RequestStore) CaptureDebug(parent context.Context, req *serving.Request) {
	if req == nil || !req.CaptureBody || req.RecordDebugSessionID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
	defer cancel()
	requestBody := []byte(nil)
	if req.Envelope != nil {
		requestBody = req.Envelope.ClientBody
	}
	response := req.AuditResponseMessage
	if len(response) == 0 {
		response = req.UpstreamResponseBody
	}
	original := len(requestBody) + len(response)
	truncated := original > maxDebugBytes
	payload := map[string]any{"request_id": req.RequestID}
	if !truncated {
		for key, raw := range map[string][]byte{"request": requestBody, "response": response} {
			var value any
			if json.Unmarshal(raw, &value) == nil {
				payload[key] = scrubDebug(value)
			}
		}
	} else {
		payload["content_omitted"] = "payload exceeds 1 MiB"
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if len(raw) > maxDebugBytes {
		raw = []byte(`{"content_omitted":"encoded payload exceeds 1 MiB"}`)
		truncated = true
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if _, err = z.Write(raw); err != nil {
		return
	}
	if err = z.Close(); err != nil {
		return
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO ai_request_debug_payloads(created_at,request_id,session_id,content_gzip,original_bytes,truncated) SELECT registered_at,$1,$2::uuid,$3,$4,$5 FROM ai_request_keys WHERE request_id=$1 ON CONFLICT DO NOTHING`, req.RequestID, req.RecordDebugSessionID, buf.Bytes(), original, truncated)
	if err != nil {
		s.logger.Warn("optional request debug storage", zap.Error(err))
	}
}
func (s *RequestStore) DebugPayload(ctx context.Context, id string) (json.RawMessage, error) {
	var compressed []byte
	if err := s.pool.QueryRow(ctx, `SELECT content_gzip FROM ai_request_debug_payloads WHERE request_id=$1 AND created_at>=now()-interval '7 days'`, id).Scan(&compressed); err != nil {
		return nil, domain.ErrNotFound
	}
	z, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, maxDebugBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxDebugBytes {
		return nil, errors.New("debug payload size exceeds limit")
	}
	return raw, nil
}
