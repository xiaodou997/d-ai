package console

import (
	"net/http"
	"time"

	"xiaodou/unihub/ai-service/internal/routing"
)

type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type systemStatusResponse struct {
	Timestamp time.Time       `json:"timestamp"`
	DB        componentStatus `json:"db"`
	Redis     componentStatus `json:"redis"`
	Health    healthSummary   `json:"health"`
}

type healthSummary struct {
	TotalTracked  int                    `json:"total_tracked"`
	OpenCount     int                    `json:"open_count"`
	HalfOpenCount int                    `json:"half_open_count"`
	Records       []routing.HealthRecord `json:"records"`
}

func (s *Console) handleAdminSystemStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	resp := systemStatusResponse{Timestamp: time.Now().UTC()}

	// DB health
	if err := s.postgres.Ping(ctx); err != nil {
		resp.DB = componentStatus{Status: "error", Error: err.Error()}
	} else {
		resp.DB = componentStatus{Status: "ok"}
	}

	// Redis health
	if s.redis == nil {
		resp.Redis = componentStatus{Status: "disabled"}
	} else if err := s.redis.Ping(ctx).Err(); err != nil {
		resp.Redis = componentStatus{Status: "error", Error: err.Error()}
	} else {
		resp.Redis = componentStatus{Status: "ok"}
	}

	// Health tracker snapshot
	records := s.routeSelector.HealthSnapshot()
	openCount, halfOpenCount := 0, 0
	for _, rec := range records {
		switch rec.State {
		case routing.StateOpen:
			openCount++
		case routing.StateHalfOpen:
			halfOpenCount++
		}
	}
	resp.Health = healthSummary{
		TotalTracked:  len(records),
		OpenCount:     openCount,
		HalfOpenCount: halfOpenCount,
		Records:       records,
	}

	writeOK(w, resp)
}
