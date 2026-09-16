package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/danielgtaylor/huma/v2/humatest"
	"testing"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/routing"
	"xiaodou/dai/internal/auth"
)

func TestResourceAvailabilityUsesEveryConfiguredPathAndOperation(t *testing.T) {
	item := domain.UpstreamStability{ConfigStatus: "active", Paths: []domain.UpstreamRuntimePath{{EndpointID: "a", Model: "model", Operation: "responses"}, {EndpointID: "b", Model: "model", Operation: "chat"}}}
	a := routing.AvailabilitySnapshot{Phase: routing.Cooling, Scope: routing.NewFaultScope("model", "direct_upstream", "account", "a", "", "model", "responses")}
	b := routing.AvailabilitySnapshot{Phase: routing.Cooling, Scope: routing.NewFaultScope("transport", "direct_upstream", "account", "b", "", "", "")}
	for _, tt := range []struct {
		states []routing.AvailabilitySnapshot
		want   string
	}{{nil, "available"}, {[]routing.AvailabilitySnapshot{a}, "partial"}, {[]routing.AvailabilitySnapshot{a, b}, "unavailable"}} {
		if got := resourceAvailability(item, tt.states); got != tt.want {
			t.Fatalf("got %s want %s", got, tt.want)
		}
	}
	a.Scope.Operation = "responses:compact"
	if got := resourceAvailability(item, []routing.AvailabilitySnapshot{a, b}); got != "partial" {
		t.Fatal("compact error blocked normal responses:", got)
	}
	a.Scope.EndpointID = "deleted"
	if got := resourceAvailability(item, []routing.AvailabilitySnapshot{a}); got != "available" {
		t.Fatal("deleted endpoint affected current configuration:", got)
	}
	item.ConfigStatus = "disabled"
	if got := resourceAvailability(item, nil); got != "unavailable" {
		t.Fatal(got)
	}
}

type stabilityReaderStub struct{ calls int }

func (s *stabilityReaderStub) ListUpstreamStability(context.Context, string, string) ([]domain.UpstreamStability, error) {
	s.calls++
	return []domain.UpstreamStability{}, nil
}
func (*stabilityReaderStub) UpstreamStabilityDetails(context.Context, string, string, string) ([]domain.UpstreamModelStability, error) {
	return nil, nil
}

type stabilityTenantVerifier struct{}

func (stabilityTenantVerifier) ParseToken(context.Context, string) (*auth.Claims, error) {
	return &auth.Claims{PrincipalType: "user", TokenUse: "access", SessionID: "s", UserID: "tenant-user", UserType: int(auth.UserTypeTenant)}, nil
}
func TestStabilityAdminInterfacesRejectTenantBeforeReadingState(t *testing.T) {
	_, api := humatest.New(t)
	reader := &stabilityReaderStub{}
	RegisterUpstreamAccountManagement(api, UpstreamAccountManagementHTTPDeps{Auth: HTTPAuthDeps{TokenVerifier: stabilityTenantVerifier{}}, Stability: reader})
	for _, path := range []string{"/api/v1/upstream-stability", "/api/v1/upstream-stability/direct_upstream/account"} {
		response := api.Get(path, "Authorization: Bearer token")
		if response.Code != 403 {
			t.Fatalf("%s returned %d %s", path, response.Code, response.Body)
		}
	}
	response := api.Post("/api/v1/upstream-stability/direct_upstream/account/resume", "Authorization: Bearer token")
	if response.Code != 403 {
		t.Fatal(response.Code, response.Body)
	}
	if reader.calls != 0 {
		t.Fatal("tenant read admin data")
	}
}

type isolatedStabilityReader struct{}

func (isolatedStabilityReader) ListUpstreamStability(context.Context, string, string) ([]domain.UpstreamStability, error) {
	out := []domain.UpstreamStability{}
	for _, id := range []string{"broken", "healthy"} {
		out = append(out, domain.UpstreamStability{ResourceKind: "direct_upstream", ResourceID: id, ConfigStatus: "active", Paths: []domain.UpstreamRuntimePath{{EndpointID: id}}})
	}
	return out, nil
}
func (isolatedStabilityReader) UpstreamStabilityDetails(context.Context, string, string, string) ([]domain.UpstreamModelStability, error) {
	return nil, nil
}

type isolatedAvailability struct {
	routing.Availability
	calls []string
}

func (a *isolatedAvailability) List(_ context.Context, kind, id string) ([]routing.AvailabilitySnapshot, error) {
	a.calls = append(a.calls, id)
	if id == "broken" {
		return nil, fmt.Errorf("invalid state")
	}
	return nil, nil
}
func TestStabilityStateFailureIsLimitedToItsAccount(t *testing.T) {
	_, api := humatest.New(t)
	state := &isolatedAvailability{}
	registerUpstreamStability(api, UpstreamAccountManagementHTTPDeps{Stability: isolatedStabilityReader{}, RuntimeHealth: state})
	response := api.Get("/api/v1/upstream-stability")
	var result struct {
		Items []upstreamStabilityDTO `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(result.Items) != 2 || result.Items[0].StateError == "" || result.Items[1].StateError != "" || result.Items[1].Availability != "available" {
		t.Fatalf("response: %s", response.Body)
	}
	state.calls = nil
	response = api.Get("/api/v1/upstream-stability/direct_upstream/healthy")
	if response.Code != 200 || len(state.calls) != 1 || state.calls[0] != "healthy" {
		t.Fatalf("detail scanned other accounts: %v status=%d", state.calls, response.Code)
	}
}
