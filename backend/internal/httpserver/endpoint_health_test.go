package httpserver

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	dbgen "uni-ai-api/backend/internal/db/gen"
)

func TestShouldCooldownUpstreamStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusOK, want: false},
		{status: http.StatusBadRequest, want: false},
		{status: http.StatusUnauthorized, want: false},
		{status: http.StatusTooManyRequests, want: true},
		{status: http.StatusInternalServerError, want: true},
		{status: http.StatusBadGateway, want: true},
	}

	for _, tt := range tests {
		if got := shouldCooldownUpstreamStatus(tt.status); got != tt.want {
			t.Fatalf("shouldCooldownUpstreamStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestUpstreamDeploymentCooldownKey(t *testing.T) {
	var id pgtype.UUID
	if err := id.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	got := upstreamDeploymentCooldownKey(id)
	want := "uni_ai_api:deployment:11111111-1111-1111-1111-111111111111:cooldown"
	if got != want {
		t.Fatalf("upstreamDeploymentCooldownKey = %q, want %q", got, want)
	}
}

func TestChooseWeightedRouteUsesLowestPriority(t *testing.T) {
	lowPriorityID := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	highPriorityID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	got, ok := chooseWeightedRoute([]dbgen.ListRoutesForModelRow{
		{
			UpstreamDeploymentID: highPriorityID,
			RoutePriority:        200,
			RouteWeight:          100,
			EndpointWeight:       100,
		},
		{
			UpstreamDeploymentID: lowPriorityID,
			RoutePriority:        100,
			RouteWeight:          1,
			EndpointWeight:       1,
		},
	})
	if !ok {
		t.Fatal("chooseWeightedRoute returned no route")
	}
	if got.UpstreamDeploymentID.String() != lowPriorityID.String() {
		t.Fatalf("UpstreamDeploymentID = %s, want %s", got.UpstreamDeploymentID.String(), lowPriorityID.String())
	}
}

func TestRouteWeight(t *testing.T) {
	got := routeWeight(dbgen.ListRoutesForModelRow{
		RouteWeight:    2,
		EndpointWeight: 3,
	})
	if got != 6 {
		t.Fatalf("routeWeight = %d, want 6", got)
	}

	got = routeWeight(dbgen.ListRoutesForModelRow{
		RouteWeight:    0,
		EndpointWeight: 3,
	})
	if got != 0 {
		t.Fatalf("routeWeight zero route = %d, want 0", got)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}