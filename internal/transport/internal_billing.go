package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"xiaodou/dai/libs/go/httpx"
	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
)

// ---- 计费/结算 DTO ----

type freezeInput struct {
	Body struct {
		IdempotencyKey string `json:"idempotencyKey" doc:"幂等键"`
		TenantID       string `json:"tenantId" doc:"租户 ID"`
		UserID         string `json:"userId" required:"false"`
		Description    string `json:"description" required:"false"`
		TenantAmount   int64  `json:"tenantAmount" required:"false"`
		UserAmount     int64  `json:"userAmount" required:"false"`
		AllowOverdraft bool   `json:"allowOverdraft" required:"false"`
	}
}

type freezeOutput struct {
	Body struct {
		EventID           string `json:"eventId"`
		FrozenTenant      int64  `json:"frozenTenant"`
		FrozenUser        int64  `json:"frozenUser"`
		AccountState      string `json:"accountState"`
		AllowFurtherUsage bool   `json:"allowFurtherUsage"`
		Status            string `json:"status"`
	}
}

type confirmInput struct {
	Body struct {
		EventID            string `json:"eventId" required:"false"`
		IdempotencyKey     string `json:"idempotencyKey" required:"false"`
		ActualTenantAmount int64  `json:"actualTenantAmount" required:"false"`
		ActualUserAmount   int64  `json:"actualUserAmount" required:"false"`
		AllowOverdraft     bool   `json:"allowOverdraft" required:"false"`
	}
}

type cancelInput struct {
	Body struct {
		EventID        string `json:"eventId" required:"false"`
		IdempotencyKey string `json:"idempotencyKey" required:"false"`
	}
}

type eventStatusOutput struct {
	Body struct {
		EventID            string `json:"eventId"`
		Status             string `json:"status"`
		TenantDeducted     int64  `json:"tenantDeducted,omitempty"`
		UserDeducted       int64  `json:"userDeducted,omitempty"`
		TenantOverdraftAdd int64  `json:"tenantOverdraftAdd,omitempty"`
		UserOverdraftAdd   int64  `json:"userOverdraftAdd,omitempty"`
		AccountState       string `json:"accountState,omitempty"`
		AllowFurtherUsage  bool   `json:"allowFurtherUsage,omitempty"`
	}
}

type consumeInput struct {
	Body struct {
		IdempotencyKey string `json:"idempotencyKey" doc:"幂等键"`
		TenantID       string `json:"tenantId" doc:"租户 ID"`
		UserID         string `json:"userId" required:"false"`
		Description    string `json:"description" required:"false"`
		TenantAmount   int64  `json:"tenantAmount" required:"false"`
		UserAmount     int64  `json:"userAmount" required:"false"`
		// V1 compatibility flags. Debit is strict unless allowOverdraft is explicit.
		DisallowOverdraft bool `json:"disallowOverdraft" required:"false"`
		AllowOverdraft    bool `json:"allowOverdraft" required:"false"`
	}
}

type consumeOutput struct {
	Body struct {
		EventID            string `json:"eventId"`
		TenantDeducted     int64  `json:"tenantDeducted"`
		UserDeducted       int64  `json:"userDeducted"`
		TenantOverdraftAdd int64  `json:"tenantOverdraftAdd"`
		UserOverdraftAdd   int64  `json:"userOverdraftAdd"`
		AccountState       string `json:"accountState"`
		AllowFurtherUsage  bool   `json:"allowFurtherUsage"`
		Status             string `json:"status"`
	}
}

type packageSummary struct {
	PackageID  string `json:"packageId"`
	Remaining  int64  `json:"remaining"`
	ExpireTime *int64 `json:"expireTime"`
	Source     string `json:"source"`
}

type balanceInput struct {
	PackageType string `query:"packageType" default:"2" doc:"1=租户账户 2=用户账户"`
	AccountID   string `query:"accountId" doc:"账户 ID"`
	Detail      bool   `query:"detail" doc:"是否返回积分包明细"`
}

type balanceOutput struct {
	Body struct {
		PackageType      int              `json:"packageType"`
		AccountID        string           `json:"accountId"`
		TotalCredits     int64            `json:"totalCredits"`
		FrozenCredits    int64            `json:"frozenCredits"`
		AvailableCredits int64            `json:"availableCredits"`
		OverdraftLimit   int64            `json:"overdraftLimit"`
		CurrentOverdraft int64            `json:"currentOverdraft"`
		Packages         []packageSummary `json:"packages,omitempty"`
	}
}

type ledgerAuthorizationInput struct {
	Body struct {
		IdempotencyKey       string `json:"idempotency_key" doc:"Stable block idempotency key"`
		TenantID             string `json:"tenant_id"`
		UserID               string `json:"user_id,omitempty"`
		Description          string `json:"description,omitempty"`
		RequestedTenantMicro int64  `json:"requested_tenant_micro,omitempty" minimum:"0"`
		RequestedUserMicro   int64  `json:"requested_user_micro,omitempty" minimum:"0"`
	}
}

type ledgerAuthorizationOutput struct {
	Body struct {
		AuthorizationID    string `json:"authorization_id"`
		GrantedTenantMicro int64  `json:"granted_tenant_micro"`
		GrantedUserMicro   int64  `json:"granted_user_micro"`
		AccountState       string `json:"account_state"`
		AllowFurtherUsage  bool   `json:"allow_further_usage"`
	}
}

type ledgerCaptureInput struct {
	ID   string `path:"id"`
	Body struct {
		ActualTenantMicro int64 `json:"actual_tenant_micro,omitempty" minimum:"0"`
		ActualUserMicro   int64 `json:"actual_user_micro,omitempty" minimum:"0"`
	}
}

type ledgerCaptureOutput struct {
	Body struct {
		AuthorizationID      string `json:"authorization_id"`
		TenantDeductedMicro  int64  `json:"tenant_deducted_micro"`
		UserDeductedMicro    int64  `json:"user_deducted_micro"`
		TenantDebtAddedMicro int64  `json:"tenant_debt_added_micro"`
		UserDebtAddedMicro   int64  `json:"user_debt_added_micro"`
		AccountState         string `json:"account_state"`
		AllowFurtherUsage    bool   `json:"allow_further_usage"`
	}
}

type ledgerDebitInput struct {
	Body struct {
		IdempotencyKey string `json:"idempotency_key"`
		TenantID       string `json:"tenant_id"`
		UserID         string `json:"user_id,omitempty"`
		Description    string `json:"description,omitempty"`
		TenantMicro    int64  `json:"tenant_micro,omitempty" minimum:"0"`
		UserMicro      int64  `json:"user_micro,omitempty" minimum:"0"`
	}
}

type ledgerBalanceV2Input struct {
	OwnerType string `path:"owner_type" enum:"tenant,user"`
	ID        string `path:"id"`
}

type ledgerBalanceV2Output struct {
	Body struct {
		OwnerType            string `json:"owner_type"`
		AccountID            string `json:"account_id"`
		AvailableMicro       int64  `json:"available_micro"`
		FrozenMicro          int64  `json:"frozen_micro"`
		OutstandingDebtMicro int64  `json:"outstanding_debt_micro"`
		ServiceState         string `json:"service_state" enum:"active,insufficient_balance,blocked_debt"`
	}
}

type creditLeaseAcquireInput struct {
	Body struct {
		ClientWindowID       string `json:"client_window_id"`
		TenantID             string `json:"tenant_id"`
		UserID               string `json:"user_id,omitempty"`
		Description          string `json:"description,omitempty"`
		RequestedTenantMicro int64  `json:"requested_tenant_micro,omitempty" minimum:"0"`
		RequestedUserMicro   int64  `json:"requested_user_micro,omitempty" minimum:"0"`
		TTLSeconds           int64  `json:"ttl_seconds,omitempty" minimum:"0"`
		GraceSeconds         int64  `json:"grace_seconds,omitempty" minimum:"0"`
	}
}

type creditLeaseRenewInput struct {
	ID   string `path:"id"`
	Body struct {
		Version      int64 `json:"version" minimum:"1"`
		TTLSeconds   int64 `json:"ttl_seconds,omitempty" minimum:"0"`
		GraceSeconds int64 `json:"grace_seconds,omitempty" minimum:"0"`
	}
}

type creditLeaseSettleInput struct {
	ID   string `path:"id"`
	Body struct {
		SettlementID      string `json:"settlement_id"`
		ActualTenantMicro int64  `json:"actual_tenant_micro,omitempty" minimum:"0"`
		ActualUserMicro   int64  `json:"actual_user_micro,omitempty" minimum:"0"`
	}
}

type creditLeaseGetInput struct {
	ID string `path:"id"`
}

type creditLeaseOutput struct {
	Body struct {
		LeaseID              string     `json:"lease_id"`
		ClientWindowID       string     `json:"client_window_id"`
		TenantID             string     `json:"tenant_id"`
		UserID               string     `json:"user_id,omitempty"`
		GrantedTenantMicro   int64      `json:"granted_tenant_micro"`
		GrantedUserMicro     int64      `json:"granted_user_micro"`
		EscrowState          string     `json:"escrow_state" enum:"active,grace,released"`
		SettlementState      string     `json:"settlement_state" enum:"unsettled,settled"`
		Version              int64      `json:"version"`
		ExpiresAt            time.Time  `json:"expires_at"`
		GraceUntil           time.Time  `json:"grace_until"`
		SettlementID         string     `json:"settlement_id,omitempty"`
		ActualTenantMicro    *int64     `json:"actual_tenant_micro,omitempty"`
		ActualUserMicro      *int64     `json:"actual_user_micro,omitempty"`
		SettledEventID       string     `json:"settled_event_id,omitempty"`
		SettledAt            *time.Time `json:"settled_at,omitempty"`
		TenantDeductedMicro  int64      `json:"tenant_deducted_micro"`
		UserDeductedMicro    int64      `json:"user_deducted_micro"`
		TenantDebtAddedMicro int64      `json:"tenant_debt_added_micro"`
		UserDebtAddedMicro   int64      `json:"user_debt_added_micro"`
		AccountState         string     `json:"account_state"`
		AllowFurtherUsage    bool       `json:"allow_further_usage"`
	}
}

// registerInternalBilling 注册 /internal/v1/settle/* 与 /internal/v1/assets/balance
// 两阶段预授权（Freeze/Confirm/Cancel）、单阶段扣款（Consume）与余额查询（Service Token）。
func registerInternalBilling(api huma.API, d Deps, readMW, writeMW huma.Middlewares) {
	ded := d.Deduction
	repo := d.BillingRepo
	registerInternalLedgerV2(api, d, readMW, writeMW)
	registerInternalLedgerV3(api, d, readMW, writeMW)

	huma.Register(api, huma.Operation{
		OperationID: "internal-settle-freeze",
		Method:      http.MethodPost,
		Path:        "/internal/v1/settle/freeze",
		Summary:     "双账户预授权冻结",
		Tags:        []string{"internal-billing"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *freezeInput) (*freezeOutput, error) {
		tenantMicro, err := billingdomain.CreditsToMicro(in.Body.TenantAmount)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		userMicro, err := billingdomain.CreditsToMicro(in.Body.UserAmount)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		res, err := ded.Freeze(billingsvc.FreezeParams{
			IdempotencyKey: in.Body.IdempotencyKey,
			ClientID:       clientIDFromCtx(ctx),
			TenantID:       in.Body.TenantID,
			UserID:         in.Body.UserID,
			Description:    in.Body.Description,
			TenantAmount:   tenantMicro,
			UserAmount:     userMicro,
			AllowOverdraft: in.Body.AllowOverdraft,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &freezeOutput{}
		out.Body.EventID = res.EventID
		out.Body.FrozenTenant = billingdomain.MicroToWholeCredits(res.FrozenTenant)
		out.Body.FrozenUser = billingdomain.MicroToWholeCredits(res.FrozenUser)
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		out.Body.Status = res.Status
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-settle-confirm",
		Method:      http.MethodPost,
		Path:        "/internal/v1/settle/confirm",
		Summary:     "确认扣费（多退少补）",
		Tags:        []string{"internal-billing"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *confirmInput) (*eventStatusOutput, error) {
		tenantMicro, err := billingdomain.CreditsToMicro(in.Body.ActualTenantAmount)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		userMicro, err := billingdomain.CreditsToMicro(in.Body.ActualUserAmount)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		res, err := ded.Confirm(billingsvc.ConfirmParams{
			EventID:            in.Body.EventID,
			IdempotencyKey:     in.Body.IdempotencyKey,
			ClientID:           clientIDFromCtx(ctx),
			ActualTenantAmount: tenantMicro,
			ActualUserAmount:   userMicro,
			AllowOverdraft:     in.Body.AllowOverdraft,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &eventStatusOutput{}
		out.Body.EventID = res.EventID
		out.Body.Status = res.Status
		out.Body.TenantDeducted = billingdomain.MicroToWholeCredits(res.TenantDeducted)
		out.Body.UserDeducted = billingdomain.MicroToWholeCredits(res.UserDeducted)
		out.Body.TenantOverdraftAdd = billingdomain.MicroToWholeCredits(res.TenantOverdraftAdd)
		out.Body.UserOverdraftAdd = billingdomain.MicroToWholeCredits(res.UserOverdraftAdd)
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-settle-cancel",
		Method:      http.MethodPost,
		Path:        "/internal/v1/settle/cancel/{id}",
		Summary:     "取消预授权",
		Tags:        []string{"internal-billing"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *userIDInput) (*eventStatusOutput, error) {
		res, err := ded.Cancel(billingsvc.CancelParams{EventID: in.ID, ClientID: clientIDFromCtx(ctx)})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &eventStatusOutput{}
		out.Body.EventID = res.EventID
		out.Body.Status = res.Status
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "internal-settle-cancel-by-key",
		Method:      http.MethodPost,
		Path:        "/internal/v1/settle/cancel",
		Summary:     "取消预授权（按 eventId 或幂等键）",
		Tags:        []string{"internal-billing"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *cancelInput) (*eventStatusOutput, error) {
		res, err := ded.Cancel(billingsvc.CancelParams{
			EventID:        in.Body.EventID,
			IdempotencyKey: in.Body.IdempotencyKey,
			ClientID:       clientIDFromCtx(ctx),
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &eventStatusOutput{}
		out.Body.EventID = res.EventID
		out.Body.Status = res.Status
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-settle-consume",
		Method:      http.MethodPost,
		Path:        "/internal/v1/settle/consume",
		Summary:     "单阶段幂等扣款",
		Tags:        []string{"internal-billing"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *consumeInput) (*consumeOutput, error) {
		tenantMicro, err := billingdomain.CreditsToMicro(in.Body.TenantAmount)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		userMicro, err := billingdomain.CreditsToMicro(in.Body.UserAmount)
		if err != nil {
			return nil, httpx.ErrBadRequest.WithDetail(err.Error())
		}
		res, err := ded.Consume(billingsvc.ConsumeParams{
			IdempotencyKey:    in.Body.IdempotencyKey,
			ClientID:          clientIDFromCtx(ctx),
			TenantID:          in.Body.TenantID,
			UserID:            in.Body.UserID,
			Description:       in.Body.Description,
			TenantAmount:      tenantMicro,
			UserAmount:        userMicro,
			DisallowOverdraft: in.Body.DisallowOverdraft,
			AllowOverdraft:    in.Body.AllowOverdraft,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &consumeOutput{}
		out.Body.EventID = res.EventID
		out.Body.TenantDeducted = billingdomain.MicroToWholeCredits(res.TenantDeducted)
		out.Body.UserDeducted = billingdomain.MicroToWholeCredits(res.UserDeducted)
		out.Body.TenantOverdraftAdd = billingdomain.MicroToWholeCredits(res.TenantOverdraftAdd)
		out.Body.UserOverdraftAdd = billingdomain.MicroToWholeCredits(res.UserOverdraftAdd)
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		out.Body.Status = "SUCCESS"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-query-balance",
		Method:      http.MethodGet,
		Path:        "/internal/v1/assets/balance",
		Summary:     "查询账户余额",
		Tags:        []string{"internal-billing"},
		Middlewares: readMW,
	}, func(ctx context.Context, in *balanceInput) (*balanceOutput, error) {
		if in.AccountID == "" {
			return nil, httpx.ErrBadRequest.WithDetail("accountId required")
		}
		packageType := 2
		if in.PackageType == "1" {
			packageType = 1
		}

		now := billingdomain.NowUTC()
		var totalCredits, frozenCredits, currentOverdraft int64
		var err error

		if packageType == 1 {
			if totalCredits, err = repo.GetTenantBalance(ctx, in.AccountID, now); err != nil {
				return nil, toProblem(err)
			}
			frozenCredits, _ = repo.GetTenantFrozenCredits(ctx, in.AccountID)
			_, currentOverdraft, _ = repo.GetTenantOverdraftInfo(ctx, in.AccountID)
		} else {
			if totalCredits, err = repo.GetEndUserBalance(ctx, in.AccountID, now); err != nil {
				return nil, toProblem(err)
			}
			frozenCredits, _ = repo.GetEndUserFrozenCredits(ctx, in.AccountID)
			_, currentOverdraft, _ = repo.GetEndUserOverdraftInfo(ctx, in.AccountID)
		}

		out := &balanceOutput{}
		out.Body.PackageType = packageType
		out.Body.AccountID = in.AccountID
		out.Body.TotalCredits = billingdomain.MicroToWholeCredits(totalCredits)
		out.Body.FrozenCredits = billingdomain.MicroToWholeCredits(frozenCredits)
		out.Body.AvailableCredits = billingdomain.MicroToWholeCredits(totalCredits - frozenCredits)
		out.Body.OverdraftLimit = 0
		out.Body.CurrentOverdraft = billingdomain.MicroToWholeCredits(currentOverdraft)

		if in.Detail {
			var tenantID, userID *string
			packageKind := billingdomain.PackageTypeUser
			if packageType == 1 {
				packageKind = billingdomain.PackageTypeTenant
				tenantID = &in.AccountID
			} else {
				userID = &in.AccountID
			}
			if packages, perr := repo.ListCreditPackagesForBalance(ctx, packageKind, tenantID, userID, &now); perr == nil {
				out.Body.Packages = make([]packageSummary, 0, len(packages))
				for _, p := range packages {
					s := packageSummary{PackageID: p.PackageID, Remaining: billingdomain.MicroToWholeCredits(p.RemainingCredits), Source: p.Source}
					if p.ExpiresAt != nil {
						t := p.ExpiresAt.UnixMilli()
						s.ExpireTime = &t
					}
					out.Body.Packages = append(out.Body.Packages, s)
				}
			}
		}
		return out, nil
	})
}

func registerInternalLedgerV3(api huma.API, d Deps, readMW, writeMW huma.Middlewares) {
	leases := d.CreditLeases
	ded := d.Deduction

	huma.Register(api, huma.Operation{
		OperationID: "internal-v3-ledger-acquire-credit-lease",
		Method:      http.MethodPost,
		Path:        "/internal/v3/ledger/leases",
		Summary:     "Acquire an expiring, renewable credit lease",
		Tags:        []string{"internal-ledger-v3"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *creditLeaseAcquireInput) (*creditLeaseOutput, error) {
		res, err := leases.Acquire(ctx, billingsvc.AcquireLeaseParams{
			ClientID:             clientIDFromCtx(ctx),
			ClientWindowID:       in.Body.ClientWindowID,
			TenantID:             in.Body.TenantID,
			UserID:               in.Body.UserID,
			Description:          in.Body.Description,
			RequestedTenantMicro: in.Body.RequestedTenantMicro,
			RequestedUserMicro:   in.Body.RequestedUserMicro,
			TTL:                  time.Duration(in.Body.TTLSeconds) * time.Second,
			Grace:                time.Duration(in.Body.GraceSeconds) * time.Second,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		return creditLeaseResponse(res), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v3-ledger-renew-credit-lease",
		Method:      http.MethodPost,
		Path:        "/internal/v3/ledger/leases/{id}/renew",
		Summary:     "Renew a credit lease using its fencing version",
		Tags:        []string{"internal-ledger-v3"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *creditLeaseRenewInput) (*creditLeaseOutput, error) {
		res, err := leases.Renew(ctx, billingsvc.RenewLeaseParams{
			LeaseID: in.ID, ClientID: clientIDFromCtx(ctx), Version: in.Body.Version,
			TTL:   time.Duration(in.Body.TTLSeconds) * time.Second,
			Grace: time.Duration(in.Body.GraceSeconds) * time.Second,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		return creditLeaseResponse(res), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v3-ledger-settle-credit-lease",
		Method:      http.MethodPost,
		Path:        "/internal/v3/ledger/leases/{id}/settle",
		Summary:     "Idempotently settle a credit lease, including after escrow release",
		Tags:        []string{"internal-ledger-v3"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *creditLeaseSettleInput) (*creditLeaseOutput, error) {
		res, err := leases.Settle(ctx, billingsvc.SettleLeaseParams{
			LeaseID: in.ID, ClientID: clientIDFromCtx(ctx), SettlementID: in.Body.SettlementID,
			ActualTenantMicro: in.Body.ActualTenantMicro,
			ActualUserMicro:   in.Body.ActualUserMicro,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		return creditLeaseResponse(res), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v3-ledger-get-credit-lease",
		Method:      http.MethodGet,
		Path:        "/internal/v3/ledger/leases/{id}",
		Summary:     "Read credit lease state for reconciliation",
		Tags:        []string{"internal-ledger-v3"},
		Middlewares: readMW,
	}, func(ctx context.Context, in *creditLeaseGetInput) (*creditLeaseOutput, error) {
		res, err := leases.Get(ctx, in.ID, clientIDFromCtx(ctx))
		if err != nil {
			return nil, toProblem(err)
		}
		return creditLeaseResponse(res), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v3-ledger-settle-legacy-authorization",
		Method:      http.MethodPost,
		Path:        "/internal/v3/ledger/legacy-authorizations/{id}/settle",
		Summary:     "Settle a V2 authorization during the controlled V3 cutover",
		Tags:        []string{"internal-ledger-v3-migration"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *ledgerCaptureInput) (*ledgerCaptureOutput, error) {
		res, err := ded.Confirm(billingsvc.ConfirmParams{
			EventID: in.ID, ClientID: clientIDFromCtx(ctx),
			ActualTenantAmount: in.Body.ActualTenantMicro,
			ActualUserAmount:   in.Body.ActualUserMicro,
			AllowOverdraft:     true, AllowReleased: true,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &ledgerCaptureOutput{}
		out.Body.AuthorizationID = res.EventID
		out.Body.TenantDeductedMicro = res.TenantDeducted
		out.Body.UserDeductedMicro = res.UserDeducted
		out.Body.TenantDebtAddedMicro = res.TenantOverdraftAdd
		out.Body.UserDebtAddedMicro = res.UserOverdraftAdd
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})
}

func creditLeaseResponse(res *billingsvc.CreditLease) *creditLeaseOutput {
	out := &creditLeaseOutput{}
	out.Body.LeaseID = res.LeaseID
	out.Body.ClientWindowID = res.ClientWindowID
	out.Body.TenantID = res.TenantID
	out.Body.UserID = res.UserID
	out.Body.GrantedTenantMicro = res.GrantedTenantMicro
	out.Body.GrantedUserMicro = res.GrantedUserMicro
	out.Body.EscrowState = res.EscrowState
	out.Body.SettlementState = res.SettlementState
	out.Body.Version = res.Version
	out.Body.ExpiresAt = res.ExpiresAt
	out.Body.GraceUntil = res.GraceUntil
	out.Body.SettlementID = res.SettlementID
	out.Body.ActualTenantMicro = res.ActualTenantMicro
	out.Body.ActualUserMicro = res.ActualUserMicro
	out.Body.SettledEventID = res.SettledEventID
	out.Body.SettledAt = res.SettledAt
	out.Body.TenantDeductedMicro = res.TenantDeducted
	out.Body.UserDeductedMicro = res.UserDeducted
	out.Body.TenantDebtAddedMicro = res.TenantDebtAdded
	out.Body.UserDebtAddedMicro = res.UserDebtAdded
	out.Body.AccountState = res.AccountState
	out.Body.AllowFurtherUsage = res.AllowFurtherUsage
	return out
}

func registerInternalLedgerV2(api huma.API, d Deps, readMW, writeMW huma.Middlewares) {
	ded := d.Deduction
	repo := d.BillingRepo

	huma.Register(api, huma.Operation{
		OperationID: "internal-v2-ledger-open-authorization",
		Method:      http.MethodPost,
		Path:        "/internal/v2/ledger/authorizations",
		Summary:     "Open a partially grantable microcredit authorization block",
		Tags:        []string{"internal-ledger-v2"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *ledgerAuthorizationInput) (*ledgerAuthorizationOutput, error) {
		res, err := ded.Freeze(billingsvc.FreezeParams{
			IdempotencyKey: in.Body.IdempotencyKey,
			ClientID:       clientIDFromCtx(ctx),
			TenantID:       in.Body.TenantID,
			UserID:         in.Body.UserID,
			Description:    in.Body.Description,
			TenantAmount:   in.Body.RequestedTenantMicro,
			UserAmount:     in.Body.RequestedUserMicro,
			AllowOverdraft: true,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &ledgerAuthorizationOutput{}
		out.Body.AuthorizationID = res.EventID
		out.Body.GrantedTenantMicro = res.FrozenTenant
		out.Body.GrantedUserMicro = res.FrozenUser
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v2-ledger-capture-authorization",
		Method:      http.MethodPost,
		Path:        "/internal/v2/ledger/authorizations/{id}/capture",
		Summary:     "Capture actual microcredits for completed work",
		Tags:        []string{"internal-ledger-v2"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *ledgerCaptureInput) (*ledgerCaptureOutput, error) {
		res, err := ded.Confirm(billingsvc.ConfirmParams{
			EventID:            in.ID,
			ClientID:           clientIDFromCtx(ctx),
			ActualTenantAmount: in.Body.ActualTenantMicro,
			ActualUserAmount:   in.Body.ActualUserMicro,
			AllowOverdraft:     true,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &ledgerCaptureOutput{}
		out.Body.AuthorizationID = res.EventID
		out.Body.TenantDeductedMicro = res.TenantDeducted
		out.Body.UserDeductedMicro = res.UserDeducted
		out.Body.TenantDebtAddedMicro = res.TenantOverdraftAdd
		out.Body.UserDebtAddedMicro = res.UserOverdraftAdd
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v2-ledger-void-authorization",
		Method:      http.MethodPost,
		Path:        "/internal/v2/ledger/authorizations/{id}/void",
		Summary:     "Void an unused microcredit authorization block",
		Tags:        []string{"internal-ledger-v2"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *userIDInput) (*eventStatusOutput, error) {
		res, err := ded.Cancel(billingsvc.CancelParams{EventID: in.ID, ClientID: clientIDFromCtx(ctx)})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &eventStatusOutput{}
		out.Body.EventID = res.EventID
		out.Body.Status = res.Status
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v2-ledger-strict-debit",
		Method:      http.MethodPost,
		Path:        "/internal/v2/ledger/debits",
		Summary:     "Strictly debit a known microcredit amount",
		Tags:        []string{"internal-ledger-v2"},
		Middlewares: writeMW,
	}, func(ctx context.Context, in *ledgerDebitInput) (*ledgerCaptureOutput, error) {
		res, err := ded.Consume(billingsvc.ConsumeParams{
			IdempotencyKey:    in.Body.IdempotencyKey,
			ClientID:          clientIDFromCtx(ctx),
			TenantID:          in.Body.TenantID,
			UserID:            in.Body.UserID,
			Description:       in.Body.Description,
			TenantAmount:      in.Body.TenantMicro,
			UserAmount:        in.Body.UserMicro,
			DisallowOverdraft: true,
		})
		if err != nil {
			return nil, toProblem(err)
		}
		out := &ledgerCaptureOutput{}
		out.Body.AuthorizationID = res.EventID
		out.Body.TenantDeductedMicro = res.TenantDeducted
		out.Body.UserDeductedMicro = res.UserDeducted
		out.Body.TenantDebtAddedMicro = res.TenantOverdraftAdd
		out.Body.UserDebtAddedMicro = res.UserOverdraftAdd
		out.Body.AccountState = res.AccountState
		out.Body.AllowFurtherUsage = res.AllowFurtherUsage
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "internal-v2-ledger-get-balance",
		Method:      http.MethodGet,
		Path:        "/internal/v2/ledger/balances/{owner_type}/{id}",
		Summary:     "Get microcredit balance and debt state",
		Tags:        []string{"internal-ledger-v2"},
		Middlewares: readMW,
	}, func(ctx context.Context, in *ledgerBalanceV2Input) (*ledgerBalanceV2Output, error) {
		now := billingdomain.NowUTC()
		var total, frozen, debt int64
		var err error
		switch in.OwnerType {
		case "tenant":
			total, err = repo.GetTenantBalance(ctx, in.ID, now)
			if err == nil {
				frozen, err = repo.GetTenantFrozenCredits(ctx, in.ID)
			}
			if err == nil {
				_, debt, err = repo.GetTenantOverdraftInfo(ctx, in.ID)
			}
		case "user":
			total, err = repo.GetEndUserBalance(ctx, in.ID, now)
			if err == nil {
				frozen, err = repo.GetEndUserFrozenCredits(ctx, in.ID)
			}
			if err == nil {
				_, debt, err = repo.GetEndUserOverdraftInfo(ctx, in.ID)
			}
		default:
			return nil, httpx.ErrBadRequest.WithDetail("owner_type must be tenant or user")
		}
		if err != nil {
			return nil, toProblem(err)
		}
		available := total - frozen
		if available < 0 {
			available = 0
		}
		state := "active"
		if debt > 0 {
			state = "blocked_debt"
		} else if available <= 0 {
			state = "insufficient_balance"
		}
		out := &ledgerBalanceV2Output{}
		out.Body.OwnerType = in.OwnerType
		out.Body.AccountID = in.ID
		out.Body.AvailableMicro = available
		out.Body.FrozenMicro = frozen
		out.Body.OutstandingDebtMicro = debt
		out.Body.ServiceState = state
		return out, nil
	})
}
