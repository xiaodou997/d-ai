package upstreamaccess

import (
	"context"
	"testing"
)

type recordingRepository struct {
	replacedTenant   string
	replacedPolicies []TenantResourcePolicy
}

func (r *recordingRepository) ListForTenant(context.Context, string) ([]ResourceAccess, error) {
	return nil, nil
}

func (r *recordingRepository) ReplacePolicies(_ context.Context, tenantID string, policies []TenantResourcePolicy) error {
	r.replacedTenant = tenantID
	r.replacedPolicies = policies
	return nil
}

func (r *recordingRepository) CanAccess(context.Context, string, ResourceRef) (bool, error) {
	return true, nil
}

func TestReplacePoliciesNormalizesResources(t *testing.T) {
	repo := &recordingRepository{}
	svc := New(repo)
	override := 1.25
	err := svc.ReplacePolicies(context.Background(), " tenant-a ", []TenantResourcePolicy{
		{ResourceRef: ResourceRef{Kind: KindDirectUpstream, ID: " account-a "}, AccessGranted: true},
		{ResourceRef: ResourceRef{Kind: KindOAuthPool, ID: "pool-a"}, TenantMultiplierOverride: &override},
	})
	if err != nil {
		t.Fatalf("ReplacePolicies() error = %v", err)
	}
	if repo.replacedTenant != "tenant-a" {
		t.Fatalf("tenant = %q, want tenant-a", repo.replacedTenant)
	}
	if len(repo.replacedPolicies) != 2 {
		t.Fatalf("policies = %#v, want two policies", repo.replacedPolicies)
	}
}

func TestNormalizeModeRejectsUnknownValue(t *testing.T) {
	if _, err := NormalizeMode("private"); err == nil {
		t.Fatal("NormalizeMode(private) error = nil")
	}
}

func TestNormalizeDisplayNameFallsBackToInternalName(t *testing.T) {
	if got := NormalizeDisplayName(" Internal ", "  "); got != "Internal" {
		t.Fatalf("NormalizeDisplayName() = %q, want Internal", got)
	}
}
