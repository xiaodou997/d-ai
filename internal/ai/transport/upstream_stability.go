package transport

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/libs/go/httpx"
)

type UpstreamStabilityReader interface {
	ListUpstreamStability(context.Context, string, string) ([]domain.UpstreamStability, error)
	UpstreamStabilityDetails(context.Context, string, string, string) ([]domain.UpstreamModelStability, error)
}
type upstreamStabilityDTO struct {
	domain.UpstreamStability
	Availability string                         `json:"availability" enum:"available,partial,unavailable,unknown"`
	StateError   string                         `json:"state_error,omitempty"`
	States       []routing.AvailabilitySnapshot `json:"states"`
}
type stabilityListInput struct {
	Kind   string `query:"kind" enum:"direct_upstream,oauth_pool" required:"false"`
	Window string `query:"window" enum:"1h,24h,7d" default:"24h"`
}
type stabilityDetailInput struct {
	Kind   string `path:"kind" enum:"direct_upstream,oauth_pool"`
	ID     string `path:"id"`
	Window string `query:"window" enum:"1h,24h,7d" default:"24h"`
}
type stabilityListOutput struct {
	Body struct {
		Items []upstreamStabilityDTO `json:"items"`
	}
}
type stabilityDetailOutput struct{ Body upstreamStabilityDTO }

func registerUpstreamStability(api huma.API, d UpstreamAccountManagementHTTPDeps) {
	read := func(ctx context.Context, kind, window string) ([]upstreamStabilityDTO, error) {
		if d.Stability == nil {
			return nil, httpx.ErrUnavailable
		}
		items, err := d.Stability.ListUpstreamStability(ctx, kind, window)
		if err != nil {
			return nil, mapServiceError(err)
		}
		states := []routing.AvailabilitySnapshot{}
		var stateErr error
		if d.RuntimeHealth != nil {
			states, stateErr = d.RuntimeHealth.List(ctx, "", "")
		} else {
			stateErr = routing.ErrAvailabilityUnavailable
		}
		out := make([]upstreamStabilityDTO, 0, len(items))
		for _, item := range items {
			dto := upstreamStabilityDTO{UpstreamStability: item, Availability: "available", States: []routing.AvailabilitySnapshot{}}
			for _, state := range states {
				if state.Scope.ResourceKind == item.ResourceKind && state.Scope.ResourceID == item.ResourceID {
					dto.States = append(dto.States, state)
				}
			}
			dto.Availability = resourceAvailability(item, dto.States)

			if stateErr != nil {
				dto.Availability = "unknown"
				dto.StateError = "运行状态暂不可读取"
			}
			if item.ConfigStatus == "disabled" {
				dto.Availability = "unavailable"
			}
			out = append(out, dto)
		}
		return out, nil
	}
	huma.Register(api, huma.Operation{OperationID: "ai-list-upstream-stability", Method: http.MethodGet, Path: "/api/v1/upstream-stability", Summary: "管理员上游稳定性", Tags: []string{"upstream-accounts"}}, func(ctx context.Context, in *stabilityListInput) (*stabilityListOutput, error) {
		items, err := read(ctx, in.Kind, in.Window)
		if err != nil {
			return nil, err
		}
		out := &stabilityListOutput{}
		out.Body.Items = items
		return out, nil
	})
	huma.Register(api, huma.Operation{OperationID: "ai-get-upstream-stability", Method: http.MethodGet, Path: "/api/v1/upstream-stability/{kind}/{id}", Summary: "上游运行状态及模型调用结果", Tags: []string{"upstream-accounts"}}, func(ctx context.Context, in *stabilityDetailInput) (*stabilityDetailOutput, error) {
		items, err := read(ctx, in.Kind, in.Window)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ResourceID == in.ID {
				item.Models, err = d.Stability.UpstreamStabilityDetails(ctx, in.Kind, in.ID, in.Window)
				if err != nil {
					return nil, mapServiceError(err)
				}
				return &stabilityDetailOutput{Body: item}, nil
			}
		}
		return nil, httpx.ErrNotFound
	})
	huma.Register(api, huma.Operation{OperationID: "ai-resume-upstream", Method: http.MethodPost, Path: "/api/v1/upstream-stability/{kind}/{id}/resume", Summary: "立即恢复业务试用（不发送探测请求）", Tags: []string{"upstream-accounts"}}, func(ctx context.Context, in *stabilityDetailInput) (*stabilityDetailOutput, error) {
		items, err := read(ctx, in.Kind, in.Window)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.ResourceID == in.ID {
				if item.ConfigStatus == "disabled" {
					return nil, httpx.ErrBadRequest.WithDetail("manually disabled resources cannot be resumed")
				}
				if d.RuntimeHealth == nil {
					return nil, httpx.ErrUnavailable
				}
				if resumer, ok := d.Stability.(interface {
					ResumeUpstreamCredentials(context.Context, string, string) error
				}); ok {
					if err = resumer.ResumeUpstreamCredentials(ctx, in.Kind, in.ID); err != nil {
						return nil, httpx.ErrUnavailable
					}
				}
				if err = d.RuntimeHealth.Resume(ctx, in.Kind, in.ID); err != nil {
					return nil, httpx.ErrUnavailable
				}
				voidAdminAudit(ctx, d.AdminAudit, "upstream.resume", in.Kind, in.ID, nil, "success", 200)
				states, e := d.RuntimeHealth.List(ctx, in.Kind, in.ID)
				if e != nil {
					return nil, httpx.ErrUnavailable
				}
				item.States = states
				item.Availability = resourceAvailability(item.UpstreamStability, states)
				return &stabilityDetailOutput{Body: item}, nil
			}
		}
		return nil, httpx.ErrNotFound
	})
}

func resourceAvailability(item domain.UpstreamStability, states []routing.AvailabilitySnapshot) string {
	if item.ConfigStatus != "active" || len(item.Paths) == 0 {
		return "unavailable"
	}
	blocked := 0
	for _, path := range item.Paths {
		for _, state := range states {
			if state.Phase != routing.Cooling {
				continue
			}
			scope := state.Scope
			if scope.EndpointID != "" && scope.EndpointID != path.EndpointID {
				continue
			}
			if scope.CredentialID != "" && scope.CredentialID != path.CredentialID {
				continue
			}
			if scope.Model != "" && scope.Model != path.Model {
				continue
			}
			if scope.Operation != "" && scope.Operation != path.Operation {
				continue
			}
			blocked++
			break
		}
	}
	if blocked == len(item.Paths) {
		return "unavailable"
	}
	if blocked > 0 {
		return "partial"
	}
	return "available"
}
