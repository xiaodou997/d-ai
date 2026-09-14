package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/internal/ai/routing"
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
	StateError    string            `json:"state_error,omitempty"`
	TotalTracked  int               `json:"total_tracked" doc:"被跟踪目标数"`
	OpenCount     int               `json:"open_count" doc:"open 状态目标数"`
	HalfOpenCount int               `json:"half_open_count" doc:"half_open 状态目标数"`
	Records       []healthRecordDTO `json:"records" doc:"健康状态记录"`
}

type healthRecordDTO struct {
	TargetID    string `json:"target_id" doc:"目标 ID"`
	Kind        string `json:"kind" enum:"authentication,transport,model,rate_limit,unknown" doc:"目标类型"`
	State       string `json:"state" enum:"available,cooling,recovering,unknown" doc:"健康状态"`
	ConsecFail  int    `json:"consecutive_failures" doc:"连续失败次数"`
	OpenedAt    *int64 `json:"opened_at,omitempty" doc:"打开时间，Unix 毫秒"`
	NextProbeAt *int64 `json:"next_probe_at,omitempty" doc:"下次探测时间，Unix 毫秒"`
}

func registerSystem(api huma.API, d SystemHTTPDeps) {
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
		out.Body.DB = componentProbeStatus(ctx, d.DatabaseHealth)
		out.Body.Redis = componentProbeStatus(ctx, d.RedisHealth)
		out.Body.Health = healthSummaryFromTracker(ctx, d.Health)
		return out, nil
	})
}

func componentProbeStatus(ctx context.Context, probe ComponentHealthProbe) componentStatus {
	if probe == nil {
		return componentStatus{Status: "disabled"}
	}
	if err := probe.Check(ctx); err != nil {
		return componentStatus{Status: "error", Error: err.Error()}
	}
	return componentStatus{Status: "ok"}
}

func healthSummaryFromTracker(ctx context.Context, tracker routing.Availability) healthSummaryDTO {
	out := healthSummaryDTO{Records: []healthRecordDTO{}}
	if tracker == nil {
		return out
	}
	records, err := tracker.List(ctx, "", "")
	if err != nil {
		out.StateError = "availability_state_unavailable"
		return out
	}
	out.TotalTracked = len(records)
	for _, r := range records {
		if r.Phase == routing.Cooling {
			out.OpenCount++
		}
		if r.Phase == routing.Recovering {
			out.HalfOpenCount++
		}
		var retry *int64
		if r.RetryAt > 0 {
			v := r.RetryAt
			retry = &v
		}
		out.Records = append(out.Records, healthRecordDTO{TargetID: r.Scope.ResourceID, Kind: r.Scope.Kind, State: string(r.Phase), ConsecFail: r.ConsecutiveFailures, NextProbeAt: retry})
	}
	return out
}
