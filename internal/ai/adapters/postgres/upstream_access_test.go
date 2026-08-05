package postgres

import (
	"testing"

	"xiaodou/dai/internal/ai/upstreamaccess"
)

func TestUpstreamAccessRepoReplacesTenantPolicies(t *testing.T) {
	pool, ctx := openCommercialGroupTestPool(t)
	repo := NewUpstreamAccessRepo(pool)

	const (
		publicID     = "90000000-0000-0000-0000-000000000001"
		restrictedID = "90000000-0000-0000-0000-000000000002"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_upstream_accounts (id, name, tenant_display_name, tenant_access_mode, status)
		VALUES
		  ($1::uuid, 'public internal', 'Public', 'public', 'active'),
		  ($2::uuid, 'restricted internal', 'Restricted', 'restricted', 'active')
	`, publicID, restrictedID); err != nil {
		t.Fatalf("seed upstream resources: %v", err)
	}

	publicAllowed, err := repo.CanAccess(ctx, "tenant-a", upstreamaccess.ResourceRef{Kind: upstreamaccess.KindDirectUpstream, ID: publicID})
	if err != nil || !publicAllowed {
		t.Fatalf("public CanAccess() = %v, %v, want true, nil", publicAllowed, err)
	}
	restrictedAllowed, err := repo.CanAccess(ctx, "tenant-a", upstreamaccess.ResourceRef{Kind: upstreamaccess.KindDirectUpstream, ID: restrictedID})
	if err != nil || restrictedAllowed {
		t.Fatalf("restricted CanAccess() = %v, %v, want false, nil", restrictedAllowed, err)
	}

	override := 1.5
	if err := repo.ReplacePolicies(ctx, "tenant-a", []upstreamaccess.TenantResourcePolicy{{
		ResourceRef:   upstreamaccess.ResourceRef{Kind: upstreamaccess.KindDirectUpstream, ID: restrictedID},
		AccessGranted: true, TenantMultiplierOverride: &override,
	}}); err != nil {
		t.Fatalf("ReplacePolicies() error = %v", err)
	}
	restrictedAllowed, err = repo.CanAccess(ctx, "tenant-a", upstreamaccess.ResourceRef{Kind: upstreamaccess.KindDirectUpstream, ID: restrictedID})
	if err != nil || !restrictedAllowed {
		t.Fatalf("granted CanAccess() = %v, %v, want true, nil", restrictedAllowed, err)
	}
	items, err := repo.ListForTenant(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("ListForTenant() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListForTenant() items = %#v, want two resources", items)
	}
	var restrictedItem *upstreamaccess.ResourceAccess
	for index := range items {
		if items[index].ID == restrictedID {
			restrictedItem = &items[index]
		}
	}
	if restrictedItem == nil || !restrictedItem.AccessGranted || !restrictedItem.Allowed || restrictedItem.EffectiveTenantMultiplier != override {
		t.Fatalf("restricted resource state = %#v, want granted and allowed", restrictedItem)
	}

	if err := repo.ReplacePolicies(ctx, "tenant-a", nil); err != nil {
		t.Fatalf("clear ReplacePolicies() error = %v", err)
	}
	restrictedAllowed, err = repo.CanAccess(ctx, "tenant-a", upstreamaccess.ResourceRef{Kind: upstreamaccess.KindDirectUpstream, ID: restrictedID})
	if err != nil || restrictedAllowed {
		t.Fatalf("cleared CanAccess() = %v, %v, want false, nil", restrictedAllowed, err)
	}
}
