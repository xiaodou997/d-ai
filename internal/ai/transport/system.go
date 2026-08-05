package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/ai/serving"
	"xiaodou/dai/libs/go/httpx"
)

type systemStatusOutput struct {
	Body systemStatusDTO
}

type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type systemStatusDTO struct {
	Timestamp int64            `json:"timestamp" doc:"状态生成时间，Unix 毫秒"`
	DB        componentStatus  `json:"db" doc:"PostgreSQL 状态"`
	Redis     componentStatus  `json:"redis" doc:"Redis 状态"`
	Health    healthSummaryDTO `json:"health" doc:"运行时健康跟踪快照"`
}

type healthSummaryDTO struct {
	TotalTracked  int               `json:"total_tracked" doc:"被跟踪目标数"`
	OpenCount     int               `json:"open_count" doc:"open 状态目标数"`
	HalfOpenCount int               `json:"half_open_count" doc:"half_open 状态目标数"`
	Records       []healthRecordDTO `json:"records" doc:"健康状态记录"`
}

type healthRecordDTO struct {
	TargetID    string `json:"target_id" doc:"目标 ID"`
	Kind        string `json:"kind" enum:"account,pool,unknown" doc:"目标类型"`
	State       string `json:"state" enum:"closed,open,half_open,unknown" doc:"健康状态"`
	ConsecFail  int    `json:"consecutive_failures" doc:"连续失败次数"`
	OpenedAt    *int64 `json:"opened_at,omitempty" doc:"打开时间，Unix 毫秒"`
	NextProbeAt *int64 `json:"next_probe_at,omitempty" doc:"下次探测时间，Unix 毫秒"`
}

type routeWeightsInput struct {
	Scope string `path:"scope" doc:"权重作用域，例如 global"`
}

type scoreWeightsDTO struct {
	Cost    float64 `json:"cost" doc:"成本权重"`
	Latency float64 `json:"latency" doc:"延迟权重"`
	Load    float64 `json:"load" doc:"负载权重"`
	Health  float64 `json:"health" doc:"健康权重"`
}

type routeWeightsOutput struct {
	Body struct {
		Scope   string          `json:"scope" doc:"权重作用域"`
		Weights scoreWeightsDTO `json:"weights" doc:"评分权重"`
	}
}

type putRouteWeightsInput struct {
	Scope string `path:"scope" doc:"权重作用域，例如 global"`
	Body  scoreWeightsDTO
}

func registerSystem(api huma.API, d AIDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "ai-get-system-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/status",
		Summary:     "系统状态",
		Description: "返回 PostgreSQL、Redis 与路由健康跟踪快照。",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*systemStatusOutput, error) {
		out := &systemStatusOutput{}
		out.Body.Timestamp = time.Now().UTC().UnixMilli()
		out.Body.DB = dbStatus(ctx, d)
		out.Body.Redis = redisStatus(ctx, d)
		out.Body.Health = healthSummaryFromTracker(d.Health)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-get-route-weights",
		Method:      http.MethodGet,
		Path:        "/api/v1/route-weights/{scope}",
		Summary:     "路由评分权重",
		Description: "返回指定 scope 的路由评分权重；底层读取失败时沿用运行时默认权重。",
		Tags:        []string{"system"},
	}, func(ctx context.Context, in *routeWeightsInput) (*routeWeightsOutput, error) {
		if d.Weights == nil {
			return nil, httpx.ErrUnavailable.WithDetail("route weights store is not configured")
		}
		if in.Scope == "" {
			return nil, httpx.ErrBadRequest.WithDetail("scope path param is required")
		}
		out := &routeWeightsOutput{}
		out.Body.Scope = in.Scope
		out.Body.Weights = scoreWeightsToDTO(d.Weights.Get(ctx, in.Scope))
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-put-route-weights",
		Method:      http.MethodPut,
		Path:        "/api/v1/route-weights/{scope}",
		Summary:     "保存路由评分权重",
		Description: "保存指定 scope 的路由评分权重；四项权重之和建议为 1.0。",
		Tags:        []string{"system"},
	}, func(ctx context.Context, in *putRouteWeightsInput) (*routeWeightsOutput, error) {
		if d.Weights == nil {
			return nil, httpx.ErrUnavailable.WithDetail("route weights store is not configured")
		}
		if in.Scope == "" {
			return nil, httpx.ErrBadRequest.WithDetail("scope path param is required")
		}
		weights := serving.ScoreWeights{
			Cost:    in.Body.Cost,
			Latency: in.Body.Latency,
			Load:    in.Body.Load,
			Health:  in.Body.Health,
		}
		if err := d.Weights.Upsert(ctx, in.Scope, weights); err != nil {
			return nil, mapServiceError(err)
		}
		out := &routeWeightsOutput{}
		out.Body.Scope = in.Scope
		out.Body.Weights = scoreWeightsToDTO(d.Weights.Get(ctx, in.Scope))
		return out, nil
	})
}

func dbStatus(ctx context.Context, d AIDeps) componentStatus {
	if d.Postgres == nil {
		return componentStatus{Status: "disabled"}
	}
	if err := d.Postgres.Ping(ctx); err != nil {
		return componentStatus{Status: "error", Error: err.Error()}
	}
	return componentStatus{Status: "ok"}
}

func redisStatus(ctx context.Context, d AIDeps) componentStatus {
	if d.Redis == nil {
		return componentStatus{Status: "disabled"}
	}
	if err := d.Redis.Ping(ctx).Err(); err != nil {
		return componentStatus{Status: "error", Error: err.Error()}
	}
	return componentStatus{Status: "ok"}
}

func healthSummaryFromTracker(tracker routing.HealthTracker) healthSummaryDTO {
	if tracker == nil {
		return healthSummaryDTO{Records: []healthRecordDTO{}}
	}
	records := tracker.Snapshot()
	out := healthSummaryDTO{
		TotalTracked: len(records),
		Records:      make([]healthRecordDTO, 0, len(records)),
	}
	for _, record := range records {
		switch record.State {
		case routing.StateOpen:
			out.OpenCount++
		case routing.StateHalfOpen:
			out.HalfOpenCount++
		}
		out.Records = append(out.Records, healthRecordToDTO(record))
	}
	return out
}

func healthRecordToDTO(record routing.HealthRecord) healthRecordDTO {
	return healthRecordDTO{
		TargetID:    record.TargetID,
		Kind:        healthTargetKindToString(record.Kind),
		State:       record.State.String(),
		ConsecFail:  record.ConsecFail,
		OpenedAt:    timePtrToMillis(record.OpenedAt),
		NextProbeAt: timePtrToMillis(record.NextProbeAt),
	}
}

func healthTargetKindToString(kind routing.TargetKind) string {
	switch kind {
	case routing.TargetAccount:
		return "account"
	case routing.TargetPool:
		return "pool"
	default:
		return "unknown"
	}
}

func scoreWeightsToDTO(weights serving.ScoreWeights) scoreWeightsDTO {
	return scoreWeightsDTO{
		Cost:    weights.Cost,
		Latency: weights.Latency,
		Load:    weights.Load,
		Health:  weights.Health,
	}
}
