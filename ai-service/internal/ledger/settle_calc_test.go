package ledger

import (
	"context"
	"testing"

	"xiaodou/unihub/ai-service/internal/domain"
)

func TestComputeSettleAction(t *testing.T) {
	const C = domain.MicroCreditsPerCredit // 10000 micro = 1 credit

	cases := []struct {
		name                                 string
		pendingTenant, pendingUser           int64
		reuseT, reuseU                       int64
		wantNoOp                             bool
		wantTC, wantUC                       int64
		wantLeftoverTenant, wantLeftoverUser int64
	}{
		{
			name:          "both sides below 1 credit → NoOp, leftover preserved",
			pendingTenant: 3, pendingUser: 7,
			wantNoOp: true, wantTC: 0, wantUC: 0,
			wantLeftoverTenant: 3, wantLeftoverUser: 7,
		},
		{
			name:          "tenant 1.5 credit, user 0",
			pendingTenant: 15000, pendingUser: 0,
			wantNoOp: false, wantTC: 1, wantUC: 0,
			wantLeftoverTenant: 5000, wantLeftoverUser: 0,
		},
		{
			name:          "tenant 0, user 200.0001 credit",
			pendingTenant: 0, pendingUser: 200*C + 1,
			wantNoOp: false, wantTC: 0, wantUC: 200,
			wantLeftoverTenant: 0, wantLeftoverUser: 1,
		},
		{
			name:          "both sides settle, mixed remainders",
			pendingTenant: 12345, pendingUser: 99999,
			wantNoOp: false, wantTC: 1, wantUC: 9,
			wantLeftoverTenant: 2345, wantLeftoverUser: 9999,
		},
		{
			name:          "exact multiples — no remainder",
			pendingTenant: 5 * C, pendingUser: 12 * C,
			wantNoOp: false, wantTC: 5, wantUC: 12,
			wantLeftoverTenant: 0, wantLeftoverUser: 0,
		},
		{
			name:          "zero pending → NoOp",
			pendingTenant: 0, pendingUser: 0,
			wantNoOp: true, wantTC: 0, wantUC: 0,
			wantLeftoverTenant: 0, wantLeftoverUser: 0,
		},
		{
			// Crash-recovery: previous attempt locked in window of 30000+50000
			// micro but URM call result was lost. Even if pending has grown
			// since (more requests accrued), we must replay the exact same
			// window so URM dedups it.
			name:          "reuse window — overrides current pending",
			pendingTenant: 90000, pendingUser: 120000,
			reuseT: 30000, reuseU: 50000,
			wantNoOp: false, wantTC: 3, wantUC: 5,
			wantLeftoverTenant: 60000, wantLeftoverUser: 70000,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := computeSettleAction(c.pendingTenant, c.pendingUser, c.reuseT, c.reuseU)
			if a.NoOp != c.wantNoOp {
				t.Errorf("NoOp = %v, want %v", a.NoOp, c.wantNoOp)
			}
			if a.TenantCredits != c.wantTC {
				t.Errorf("TenantCredits = %d, want %d", a.TenantCredits, c.wantTC)
			}
			if a.UserCredits != c.wantUC {
				t.Errorf("UserCredits = %d, want %d", a.UserCredits, c.wantUC)
			}
			if a.LeftoverTenantMicro != c.wantLeftoverTenant {
				t.Errorf("LeftoverTenantMicro = %d, want %d", a.LeftoverTenantMicro, c.wantLeftoverTenant)
			}
			if a.LeftoverUserMicro != c.wantLeftoverUser {
				t.Errorf("LeftoverUserMicro = %d, want %d", a.LeftoverUserMicro, c.wantLeftoverUser)
			}
			// Invariants the rest of the code depends on:
			if a.SettleTenantMicro%C != 0 {
				t.Errorf("SettleTenantMicro (%d) must be multiple of %d", a.SettleTenantMicro, C)
			}
			if a.SettleUserMicro%C != 0 {
				t.Errorf("SettleUserMicro (%d) must be multiple of %d", a.SettleUserMicro, C)
			}
			if a.LeftoverTenantMicro < 0 || a.LeftoverUserMicro < 0 {
				t.Errorf("negative leftover: %d / %d", a.LeftoverTenantMicro, a.LeftoverUserMicro)
			}
		})
	}
}

// TestAddChargeValidation covers the param checks in AddCharge that fail
// before any DB call — no pool needed.
func TestAddChargeValidation(t *testing.T) {
	l := &Ledger{}

	cases := []struct {
		name    string
		params  AddChargeParams
		wantErr bool
	}{
		{
			name:    "both zero amounts → no-op success",
			params:  AddChargeParams{OwnerType: domain.OwnerUser, TenantID: "T", UserID: "U"},
			wantErr: false,
		},
		{
			name:    "negative tenant micro rejected",
			params:  AddChargeParams{OwnerType: domain.OwnerUser, TenantID: "T", UserID: "U", TenantMicro: -1},
			wantErr: true,
		},
		{
			name:    "invalid owner type rejected",
			params:  AddChargeParams{OwnerType: "bogus", TenantID: "T", UserID: "U", TenantMicro: 100},
			wantErr: true,
		},
		{
			name:    "tenant-owned key with user cost rejected",
			params:  AddChargeParams{OwnerType: domain.OwnerTenant, TenantID: "T", TenantMicro: 100, UserMicro: 50},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := l.AddCharge(context.TODO(), c.params)
			if (err != nil) != c.wantErr {
				t.Errorf("AddCharge err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}
