package httpserver

import (
	"encoding/json"
	"net/http"
	"time"
)

type healthResponse struct {
	Status     string                     `json:"status"`
	Components map[string]componentStatus `json:"components,omitempty"`
	Timestamp  time.Time                  `json:"timestamp"`
}

type componentStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	components := map[string]componentStatus{}
	ready := true

	if err := s.postgres.Ping(ctx); err != nil {
		ready = false
		components["postgres"] = componentStatus{Status: "error", Error: err.Error()}
	} else {
		components["postgres"] = componentStatus{Status: "ok"}
	}

	if s.redis != nil {
		if err := s.redis.Ping(ctx).Err(); err != nil {
			ready = false
			components["redis"] = componentStatus{Status: "error", Error: err.Error()}
		} else {
			components["redis"] = componentStatus{Status: "ok"}
		}
	} else {
		components["redis"] = componentStatus{Status: "disabled"}
	}

	status := http.StatusOK
	bodyStatus := "ok"
	if !ready {
		status = http.StatusServiceUnavailable
		bodyStatus = "error"
	}

	writeJSON(w, status, healthResponse{
		Status:     bodyStatus,
		Components: components,
		Timestamp:  time.Now().UTC(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
