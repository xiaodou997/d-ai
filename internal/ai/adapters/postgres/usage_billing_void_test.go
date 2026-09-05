package postgres

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestVoidBillingPreservesRawBreakdownButZeroesCustomerAmounts(t *testing.T) {
	result := voidBilling(domain.BillingResult{
		CatalogBaseMicro:     100,
		TenantPayableMicro:   120,
		RetailBaseMicro:      150,
		UserPayableMicro:     180,
		UserChargedMicro:     180,
		APIKeyQuotaCostMicro: 180,
		BillableUnits:        42,
		BillingBreakdownJSON: []byte(`{"input_cost_usd":1.2}`),
	}, "client_disconnect_before_terminal")
	if result.CatalogBaseMicro != 0 || result.TenantPayableMicro != 0 || result.UserChargedMicro != 0 || result.BillableUnits != 0 {
		t.Fatalf("void billing retained chargeable amounts: %+v", result)
	}
	var breakdown map[string]any
	if err := json.Unmarshal(result.BillingBreakdownJSON, &breakdown); err != nil {
		t.Fatal(err)
	}
	if breakdown["billing_status"] != "void" || breakdown["billing_reason"] != "client_disconnect_before_terminal" {
		t.Fatalf("void breakdown = %#v", breakdown)
	}
}
