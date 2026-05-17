package httpserver

import (
	"net/http"
	"time"

	"uni-ai-api/backend/internal/routing"
)

type systemStatusResponse struct {
	Timestamp      time.Time              `json:"timestamp"`
	DB             componentStatus        `json:"db"`
	Redis          componentStatus        `json:"redis"`
	CircuitBreaker circuitBreakerSummary  `json:"circuit_breaker"`
}

type circuitBreakerSummary struct {
	TotalTracked int                    `json:"total_tracked"`
	OpenCount    int                    `json:"open_count"`
	States       []routing.BreakerState `json:"states"`
}

func (s *Server) handleAdminSystemStatus(w http.ResponseWriter, r *http.Request) {
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

	// Circuit breaker snapshot
	states := s.routeSelector.BreakerSnapshot()
	openCount := 0
	for _, st := range states {
		if st.Open {
			openCount++
		}
	}
	resp.CircuitBreaker = circuitBreakerSummary{
		TotalTracked: len(states),
		OpenCount:    openCount,
		States:       states,
	}

	writeOK(w, resp)
}
