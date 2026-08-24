package service

import (
	"context"
	"errors"
	"testing"

	"xiaodou/dai/internal/dbtest"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
)

func TestPaymentServiceValidatesWithdrawalListStatus(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := dbtest.OpenIsolatedSchemaPool(ctx, dbtest.PoolOptions{MaxConns: 2})
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = cleanup(context.Background()) })

	if _, err := pool.Exec(ctx, `
		INSERT INTO pay_withdrawals (
			withdrawal_id, tenant_id, amount_micro_usd, fee_amount_micro_usd, payout_amount_micro_usd,
			account_name, bank_name, account_no, status, applied_by
		) VALUES
			('WDR_QUERY_PAID', 'tenant-withdrawal-query', 1000, 10, 990, 'Paid Account', 'Bank', '001', 'paid', 'admin'),
			('WDR_QUERY_PENDING', 'tenant-withdrawal-query', 2000, 20, 1980, 'Pending Account', 'Bank', '002', 'pending', 'admin');
	`); err != nil {
		t.Fatalf("seed withdrawal query fixtures: %v", err)
	}

	svc := &PaymentService{pool: pool}
	items, total, err := svc.ListWithdrawals(ctx, payment.WithdrawalListParams{Status: " paid ", Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("list paid withdrawals: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].WithdrawalID != "WDR_QUERY_PAID" {
		t.Fatalf("paid withdrawal list = total:%d items:%+v", total, items)
	}
	if _, _, err := svc.ListWithdrawals(ctx, payment.WithdrawalListParams{Status: "unknown", Page: 1, Size: 20}); !errors.Is(err, domain.ErrBadRequest) {
		t.Fatalf("invalid withdrawal status error = %v, want bad request", err)
	}
}
