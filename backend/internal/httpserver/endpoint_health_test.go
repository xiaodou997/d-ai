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

func TestEndpointCooldownKey(t *testing.T) {
	var id pgtype.UUID
	if err := id.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	got := endpointCooldownKey(dbgen.ListDeploymentsForModelRow{EndpointID: id})
	want := "uni_ai_api:endpoint:11111111-1111-1111-1111-111111111111:cooldown"
	if got != want {
		t.Fatalf("endpointCooldownKey = %q, want %q", got, want)
	}
}

func TestChooseWeightedDeploymentUsesLowestPriority(t *testing.T) {
	lowPriorityID := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	highPriorityID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	got, ok := chooseWeightedDeployment([]dbgen.ListDeploymentsForModelRow{
		{
			EndpointID:       highPriorityID,
			Priority:         200,
			DeploymentWeight: 100,
			EndpointWeight:   100,
		},
		{
			EndpointID:       lowPriorityID,
			Priority:         100,
			DeploymentWeight: 1,
			EndpointWeight:   1,
		},
	})
	if !ok {
		t.Fatal("chooseWeightedDeployment returned no deployment")
	}
	if got.EndpointID.String() != lowPriorityID.String() {
		t.Fatalf("EndpointID = %s, want %s", got.EndpointID.String(), lowPriorityID.String())
	}
}

func TestDeploymentRouteWeight(t *testing.T) {
	got := deploymentRouteWeight(dbgen.ListDeploymentsForModelRow{
		DeploymentWeight: 2,
		EndpointWeight:   3,
	})
	if got != 6 {
		t.Fatalf("deploymentRouteWeight = %d, want 6", got)
	}

	got = deploymentRouteWeight(dbgen.ListDeploymentsForModelRow{
		DeploymentWeight: 0,
		EndpointWeight:   3,
	})
	if got != 0 {
		t.Fatalf("deploymentRouteWeight zero deployment = %d, want 0", got)
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
