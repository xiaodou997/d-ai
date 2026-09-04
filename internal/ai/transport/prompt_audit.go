package transport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/promptaudit"
	"xiaodou/dai/libs/go/httpx"
)

type promptAuditEndpointDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	HasAPIKey  bool   `json:"has_api_key"`
	TimeoutMS  int    `json:"timeout_ms"`
	InputLimit int    `json:"input_limit"`
	Enabled    bool   `json:"enabled"`
}
type promptAuditConfigDTO struct {
	Enabled         bool                     `json:"enabled"`
	Mode            string                   `json:"mode"`
	LatestTurnOnly  bool                     `json:"latest_turn_only"`
	StorePassEvents bool                     `json:"store_pass_events"`
	WorkerCount     int                      `json:"worker_count"`
	QueueCapacity   int                      `json:"queue_capacity"`
	Scanners        []string                 `json:"scanners"`
	TenantIDs       []string                 `json:"tenant_ids"`
	Endpoints       []promptAuditEndpointDTO `json:"endpoints"`
	ConfigRevision  int64                    `json:"config_revision"`
}
type promptAuditEndpointWriteDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	BaseURL    string  `json:"base_url"`
	Model      string  `json:"model"`
	APIKey     *string `json:"api_key,omitempty"`
	TimeoutMS  int     `json:"timeout_ms"`
	InputLimit int     `json:"input_limit"`
	Enabled    bool    `json:"enabled"`
}
type promptAuditConfigWriteDTO struct {
	ExpectedConfigRevision int64                         `json:"expected_config_revision"`
	Enabled                bool                          `json:"enabled"`
	Mode                   string                        `json:"mode" enum:"off,observe,blocking"`
	LatestTurnOnly         bool                          `json:"latest_turn_only"`
	StorePassEvents        bool                          `json:"store_pass_events"`
	WorkerCount            int                           `json:"worker_count"`
	QueueCapacity          int                           `json:"queue_capacity"`
	Scanners               []string                      `json:"scanners"`
	TenantIDs              []string                      `json:"tenant_ids"`
	Endpoints              []promptAuditEndpointWriteDTO `json:"endpoints"`
}
type promptAuditConfigOutput struct{ Body promptAuditConfigDTO }
type updatePromptAuditConfigInput struct{ Body promptAuditConfigWriteDTO }
type probePromptAuditInput struct{ Body promptAuditEndpointWriteDTO }
type promptAuditProbeOutput struct {
	Body struct {
		OK        bool                `json:"ok"`
		Result    *promptaudit.Result `json:"result,omitempty"`
		ErrorCode string              `json:"error_code,omitempty"`
	}
}
type promptAuditEventsInput struct {
	TenantID string `query:"tenant_id"`
	UserID   string `query:"user_id"`
	Decision string `query:"decision" enum:",pass,flag,critical,error"`
	Limit    int32  `query:"limit" default:"50"`
	Offset   int32  `query:"offset" default:"0"`
}
type promptAuditEventsOutput struct{ Body promptaudit.EventPage }
type promptAuditRuntimeOutput struct{ Body promptaudit.Runtime }
type deletePromptAuditEventInput struct {
	EventID string `path:"eventID"`
}
type deletePromptAuditEventOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

func registerPromptAudit(api huma.API, d RiskControlHTTPDeps) {
	huma.Register(api, huma.Operation{OperationID: "ai-get-prompt-audit-config", Method: http.MethodGet, Path: "/api/v1/prompt-audit/config", Summary: "提示词审计配置", Tags: []string{"prompt-audit"}}, func(ctx context.Context, _ *struct{}) (*promptAuditConfigOutput, error) {
		if d.PromptAuditConfig == nil {
			return nil, httpx.ErrUnavailable.WithDetail("prompt audit is not configured")
		}
		cfg, err := d.PromptAuditConfig.Get(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &promptAuditConfigOutput{Body: promptAuditConfigToDTO(cfg)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-update-prompt-audit-config", Method: http.MethodPut, Path: "/api/v1/prompt-audit/config", Summary: "更新提示词审计配置", Tags: []string{"prompt-audit"}}, func(ctx context.Context, in *updatePromptAuditConfigInput) (*promptAuditConfigOutput, error) {
		if d.PromptAuditConfig == nil || d.ProviderSecrets == nil {
			return nil, httpx.ErrUnavailable.WithDetail("prompt audit is not configured")
		}
		current, err := d.PromptAuditConfig.Get(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		if in.Body.ExpectedConfigRevision != current.ConfigRevision {
			return nil, httpx.ErrConflict.WithDetail("prompt audit config revision conflict")
		}
		cfg, err := promptAuditConfigFromWrite(in.Body, current, d.ProviderSecrets)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		if err = d.PromptAuditConfig.Update(ctx, cfg); err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		saved, err := d.PromptAuditConfig.Get(ctx)
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &promptAuditConfigOutput{Body: promptAuditConfigToDTO(saved)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-probe-prompt-audit-endpoint", Method: http.MethodPost, Path: "/api/v1/prompt-audit/endpoints/probe", Summary: "探测提示词审计节点", Tags: []string{"prompt-audit"}}, func(ctx context.Context, in *probePromptAuditInput) (*promptAuditProbeOutput, error) {
		if d.PromptAuditProbe == nil {
			return nil, httpx.ErrUnavailable.WithDetail("prompt audit probe is not configured")
		}
		key := ""
		if in.Body.APIKey != nil {
			key = strings.TrimSpace(*in.Body.APIKey)
		} else if d.PromptAuditConfig != nil && d.ProviderSecrets != nil {
			if cfg, configErr := d.PromptAuditConfig.Get(ctx); configErr == nil {
				for _, endpoint := range cfg.Endpoints {
					if endpoint.ID == in.Body.ID && endpoint.APIKeyCiphertext != "" {
						key, _ = d.ProviderSecrets.Decrypt(endpoint.APIKeyCiphertext)
						break
					}
				}
			}
		}
		result, err := d.PromptAuditProbe.Probe(ctx, promptAuditEndpointFromWrite(in.Body, ""), key)
		out := &promptAuditProbeOutput{}
		if err != nil {
			out.Body.ErrorCode = promptaudit.ErrorUnavailable
			var ge *promptaudit.GuardError
			if errors.As(err, &ge) {
				out.Body.ErrorCode = ge.Code
			}
			return out, nil
		}
		out.Body.OK = true
		out.Body.Result = result
		return out, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-list-prompt-audit-events", Method: http.MethodGet, Path: "/api/v1/prompt-audit/events", Summary: "提示词审计事件", Tags: []string{"prompt-audit"}}, func(ctx context.Context, in *promptAuditEventsInput) (*promptAuditEventsOutput, error) {
		if d.PromptAuditEvents == nil {
			return nil, httpx.ErrUnavailable.WithDetail("prompt audit events are not configured")
		}
		page, err := d.PromptAuditEvents.ListPromptAuditEvents(ctx, promptaudit.EventFilter{TenantID: in.TenantID, UserID: in.UserID, Decision: in.Decision, Limit: in.Limit, Offset: in.Offset})
		if err != nil {
			return nil, mapServiceError(err)
		}
		return &promptAuditEventsOutput{Body: page}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-get-prompt-audit-runtime", Method: http.MethodGet, Path: "/api/v1/prompt-audit/runtime", Summary: "提示词审计运行状态", Tags: []string{"prompt-audit"}}, func(ctx context.Context, _ *struct{}) (*promptAuditRuntimeOutput, error) {
		if d.PromptAuditRuntime == nil {
			return nil, httpx.ErrUnavailable.WithDetail("prompt audit runtime is not configured")
		}
		return &promptAuditRuntimeOutput{Body: d.PromptAuditRuntime.Runtime(ctx)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-delete-prompt-audit-event", Method: http.MethodDelete, Path: "/api/v1/prompt-audit/events/{eventID}", Summary: "删除提示词审计事件", Tags: []string{"prompt-audit"}}, func(ctx context.Context, in *deletePromptAuditEventInput) (*deletePromptAuditEventOutput, error) {
		if d.PromptAuditEvents == nil {
			return nil, httpx.ErrUnavailable.WithDetail("prompt audit events are not configured")
		}
		if _, err := parseTransportUUID(in.EventID); err != nil {
			return nil, httpx.ErrBadRequest.WithDetail("invalid prompt audit event id")
		}
		deleted, err := d.PromptAuditEvents.DeletePromptAuditEvent(ctx, in.EventID)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &deletePromptAuditEventOutput{}
		out.Body.Deleted = deleted
		return out, nil
	})
}

func promptAuditConfigToDTO(cfg promptaudit.Config) promptAuditConfigDTO {
	out := promptAuditConfigDTO{Enabled: cfg.Enabled, Mode: cfg.Mode, LatestTurnOnly: cfg.LatestTurnOnly, StorePassEvents: cfg.StorePassEvents, WorkerCount: cfg.WorkerCount, QueueCapacity: cfg.QueueCapacity, Scanners: cfg.Scanners, TenantIDs: cfg.TenantIDs, ConfigRevision: cfg.ConfigRevision, Endpoints: make([]promptAuditEndpointDTO, 0, len(cfg.Endpoints))}
	for _, ep := range cfg.Endpoints {
		out.Endpoints = append(out.Endpoints, promptAuditEndpointDTO{ID: ep.ID, Name: ep.Name, BaseURL: ep.BaseURL, Model: ep.Model, HasAPIKey: ep.APIKeyCiphertext != "", TimeoutMS: ep.TimeoutMS, InputLimit: ep.InputLimit, Enabled: ep.Enabled})
	}
	return out
}
func promptAuditConfigFromWrite(in promptAuditConfigWriteDTO, current promptaudit.Config, secrets ProviderSecretCodec) (promptaudit.Config, error) {
	old := map[string]promptaudit.Endpoint{}
	for _, ep := range current.Endpoints {
		old[ep.ID] = ep
	}
	cfg := promptaudit.Config{Enabled: in.Enabled, Mode: in.Mode, LatestTurnOnly: in.LatestTurnOnly, StorePassEvents: in.StorePassEvents, WorkerCount: in.WorkerCount, QueueCapacity: in.QueueCapacity, Scanners: in.Scanners, TenantIDs: in.TenantIDs, ConfigRevision: current.ConfigRevision, Endpoints: make([]promptaudit.Endpoint, 0, len(in.Endpoints))}
	for _, item := range in.Endpoints {
		cipher := ""
		if prior, ok := old[item.ID]; ok {
			cipher = prior.APIKeyCiphertext
		}
		if item.APIKey != nil {
			plain := strings.TrimSpace(*item.APIKey)
			if plain == "" {
				cipher = ""
			} else {
				var err error
				cipher, err = secrets.Encrypt(plain)
				if err != nil {
					return promptaudit.Config{}, err
				}
			}
		}
		cfg.Endpoints = append(cfg.Endpoints, promptAuditEndpointFromWrite(item, cipher))
	}
	return cfg, nil
}
func promptAuditEndpointFromWrite(in promptAuditEndpointWriteDTO, cipher string) promptaudit.Endpoint {
	return promptaudit.Endpoint{ID: in.ID, Name: in.Name, BaseURL: in.BaseURL, Model: in.Model, APIKeyCiphertext: cipher, TimeoutMS: in.TimeoutMS, InputLimit: in.InputLimit, Enabled: in.Enabled}
}
