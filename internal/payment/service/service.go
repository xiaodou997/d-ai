// Package service orchestrates USD top-ups, settlement, unified tenant
// balances and withdrawals.
package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/money"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
	"xiaodou/dai/internal/payment/wechat"
)

const (
	// ClosedOrderRetention keeps an unpaid order available for delayed callback
	// investigation and reconciliation before physical cleanup.
	ClosedOrderRetention    = 30 * 24 * time.Hour
	closedOrderCleanupBatch = 500
)

// PaymentService 是 payment 域的唯一编排入口。
type PaymentService struct {
	pool      *pgxpool.Pool
	settings  *payment.SettingsStore
	cfgStore  *wechat.ConfigStore
	gateway   wechat.Gateway
	logger    *zap.Logger
	deduction *billingsvc.DeductionService
}

func New(pool *pgxpool.Pool, gateway wechat.Gateway, cfgStore *wechat.ConfigStore, logger *zap.Logger, deductions ...*billingsvc.DeductionService) *PaymentService {
	var deduction *billingsvc.DeductionService
	if len(deductions) > 0 {
		deduction = deductions[0]
	}
	return &PaymentService{
		pool:      pool,
		settings:  payment.NewSettingsStore(pool),
		cfgStore:  cfgStore,
		gateway:   gateway,
		logger:    logger,
		deduction: deduction,
	}
}

// ==================== 下单 ====================

type CreateTopupOrderParams struct {
	Scene          string // payment.SceneUserTopup / SceneTenantTopup
	TenantID       string
	UserID         string
	AmountMicroUSD int64
	PackageID      string
}

// CreateTopupOrder validates a USD offer, snapshots fees/expiry and creates a
// payment order. The gateway amount is USD cents, derived without FX.
func (s *PaymentService) CreateTopupOrder(ctx context.Context, p CreateTopupOrderParams) (*payment.Order, error) {
	cfg, err := s.cfgStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, domain.ErrPaymentDisabled
	}
	p.PackageID = strings.TrimSpace(p.PackageID)
	if p.AmountMicroUSD < 0 {
		return nil, domain.ErrInvalidAmount
	}
	if p.PackageID == "" && p.AmountMicroUSD <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return nil, err
	}

	var params payment.TopupParams
	if p.Scene == payment.SceneUserTopup {
		tenantSettings, err := s.settings.LoadTenantSettings(ctx, p.TenantID, global)
		if err != nil {
			return nil, err
		}
		params = payment.ResolveUserTopup(tenantSettings)
	} else {
		params = payment.ResolveTenantTopup(global)
	}

	calc, err := calculateTopupSnapshot(params, p.AmountMicroUSD, p.PackageID)
	if err != nil {
		return nil, err
	}
	if calc.CreditedAmountMicroUSD <= 0 {
		return nil, domain.ErrPaymentAmountOutOfRange
	}
	tenantIncomeMicroUSD := int64(0)
	if p.Scene == payment.SceneUserTopup {
		tenantIncomeMicroUSD = calc.PaymentAmountMicroUSD
	}

	now := billingdomain.NowUTC()
	expiresAt := now.Add(cfg.OrderTTL)
	var balanceExpiresAt *time.Time
	if calc.ValidityDays != nil {
		v := now.Add(time.Duration(*calc.ValidityDays) * 24 * time.Hour)
		balanceExpiresAt = &v
	}
	outTradeNo, err := generateOutTradeNo()
	if err != nil {
		return nil, fmt.Errorf("生成商户单号失败: %w", err)
	}

	order := &payment.Order{
		OrderID: "PAY_" + uuid.New().String()[:24], OutTradeNo: outTradeNo, Scene: p.Scene, TenantID: p.TenantID, UserID: p.UserID,
		TopupMode: calc.Mode, PackageID: calc.PackageID, PackageName: calc.PackageName, PackageBadge: calc.PackageBadge,
		PaymentCurrency: money.CurrencyUSD, PaymentAmountMinor: calc.PaymentAmountMinor, LedgerCurrency: money.CurrencyUSD,
		GrossAmountMicroUSD: calc.GrossAmountMicroUSD, FeeRateBp: calc.FeeRateBp,
		FeeAmountMicroUSD: calc.FeeAmountMicroUSD, GiftAmountMicroUSD: calc.GiftAmountMicroUSD,
		CreditedAmountMicroUSD: calc.CreditedAmountMicroUSD, TenantIncomeMicroUSD: tenantIncomeMicroUSD,
		BalanceExpiresAt: balanceExpiresAt, Channel: "wechat_native", Status: payment.OrderStatusCreated,
		FulfillmentStatus: payment.FulfillmentStatusPending, RefundStatus: payment.RefundStatusNone, ExpiresAt: expiresAt,
	}
	if err := paymentpg.InsertOrder(ctx, s.pool, order); err != nil {
		return nil, err
	}

	codeURL, err := s.gateway.Prepay(ctx, outTradeNo, calc.PaymentAmountMinor, expiresAt, topupDescription(p.Scene))
	if err != nil {
		_ = paymentpg.MarkOrderFailed(ctx, s.pool, order.OrderID, err.Error())
		return nil, fmt.Errorf("微信下单失败: %w", err)
	}
	if err := paymentpg.SetCodeURL(ctx, s.pool, order.OrderID, codeURL); err != nil {
		return nil, err
	}
	order.CodeURL = codeURL
	return order, nil
}

func (s *PaymentService) GetOrder(ctx context.Context, orderID string) (*payment.Order, error) {
	o, err := paymentpg.GetOrderByID(ctx, s.pool, orderID)
	if err != nil {
		return nil, domain.ErrPaymentOrderNotFound
	}
	return o, nil
}

func (s *PaymentService) ListOrders(ctx context.Context, p payment.ListOrdersParams) ([]*payment.Order, int64, error) {
	return paymentpg.ListOrders(ctx, s.pool, paymentpg.ListOrdersParams{
		Scene: p.Scene, Status: p.Status, TenantID: p.TenantID, UserID: p.UserID, Page: p.Page, Size: p.Size,
	})
}

// ListAdminRechargeOrders returns the unified online/manual recharge
// projection through the payment application boundary.
func (s *PaymentService) ListAdminRechargeOrders(ctx context.Context, p payment.ListAdminRechargeOrdersParams) ([]payment.AdminRechargeOrder, int64, error) {
	return paymentpg.ListAdminRechargeOrders(ctx, s.pool, paymentpg.ListAdminRechargeOrdersParams{
		Keyword: p.Keyword, Method: p.Method, TargetType: p.TargetType,
		PaymentStatus: p.PaymentStatus, FulfillmentStatus: p.FulfillmentStatus,
		RefundStatus: p.RefundStatus, TimeFrom: p.TimeFrom, TimeTo: p.TimeTo,
		Page: p.Page, Size: p.Size,
	})
}

// GetAdminRechargeOrder reads one unified recharge projection. Missing rows
// are normalized here so HTTP does not need to know pgx error details.
func (s *PaymentService) GetAdminRechargeOrder(ctx context.Context, orderID string) (*payment.AdminRechargeOrder, error) {
	item, err := paymentpg.GetAdminRechargeOrder(ctx, s.pool, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPaymentOrderNotFound
	}
	return item, err
}

// SyncAdminRechargeOrder synchronizes only an online recharge projection. The
// method check lives beside the payment state machine instead of in HTTP.
func (s *PaymentService) SyncAdminRechargeOrder(ctx context.Context, orderID string) (*payment.AdminRechargeOrder, error) {
	item, err := s.GetAdminRechargeOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if item.Method != "online" {
		return nil, rechargeActionBadRequest("手动充值不需要同步支付状态")
	}
	if _, err := s.SyncOrder(ctx, item.OrderID); err != nil {
		return nil, err
	}
	return s.GetAdminRechargeOrder(ctx, item.OrderID)
}

// ReverseManualRechargeCredit reverses only an active manual recharge. The
// deduction service takes the balance-order row lock, so a concurrent reverse
// cannot pass the state check twice.
func (s *PaymentService) ReverseManualRechargeCredit(ctx context.Context, orderID, reason, operatorID string) (*payment.AdminRechargeOrder, error) {
	reason = strings.TrimSpace(reason)
	operatorID = strings.TrimSpace(operatorID)
	if reason == "" || operatorID == "" {
		return nil, rechargeActionBadRequest("撤回原因和操作人不能为空")
	}
	item, err := s.GetAdminRechargeOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if item.Method != "manual" {
		return nil, rechargeActionBadRequest("在线充值必须在退款完成后执行整单冲正")
	}
	if item.BalanceOrderID == "" || item.FulfillmentStatus != payment.FulfillmentStatusCredited {
		return nil, domain.ErrRechargeNotReversible
	}
	deduction := s.deduction
	if deduction == nil {
		deduction = billingsvc.NewDeductionService(s.pool, s.logger)
	}
	if _, err := deduction.ReverseOrder(item.BalanceOrderID, reason, operatorID); err != nil {
		return nil, err
	}
	return s.GetAdminRechargeOrder(ctx, item.OrderID)
}

// RecordAdminRechargeRefund validates that the management order is online,
// then delegates the locked refund transaction and returns the refreshed
// unified projection.
func (s *PaymentService) RecordAdminRechargeRefund(ctx context.Context, orderID string, p RecordCompletedRefundParams) (*payment.AdminRechargeOrder, error) {
	item, err := s.GetAdminRechargeOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if item.Method != "online" {
		return nil, rechargeActionBadRequest("手动充值没有支付退款流程")
	}
	p.PaymentOrderID = item.OrderID
	if _, err := s.RecordCompletedRefund(ctx, p); err != nil {
		return nil, err
	}
	return s.GetAdminRechargeOrder(ctx, item.OrderID)
}

func rechargeActionBadRequest(detail string) error {
	return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, detail)
}

// ==================== 回调核销 ====================

// Settle 对应设计文档 §4.3：行锁串行化 + 幂等快路径 + 三验 + 记账 + 现金入账，可重入。
func (s *PaymentService) Settle(ctx context.Context, outTradeNo string, txn *wechat.QueryResult, notifyRaw []byte) error {
	cfg, err := s.cfgStore.Load(ctx)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	order, err := paymentpg.GetOrderByOutTradeNoForUpdate(ctx, tx, outTradeNo)
	if err != nil {
		return domain.ErrPaymentOrderNotFound
	}

	if order.Status == payment.OrderStatusPaid {
		return tx.Commit(ctx) // 幂等快路径：微信重试通知/并发回调
	}

	if txn.TradeState != wechat.TradeStateSuccess {
		if order.Status == payment.OrderStatusClosed || order.Status == payment.OrderStatusExpired {
			return tx.Commit(ctx) // 终态订单不再因非成功查单结果被改写
		}
		newStatus := order.Status
		switch txn.TradeState {
		case wechat.TradeStateUserPaying:
			newStatus = payment.OrderStatusPaying
		case wechat.TradeStateNotPay:
			// 维持 created/paying 不变
		default:
			newStatus = payment.OrderStatusClosed
		}
		if newStatus != order.Status {
			if err := paymentpg.UpdateStatusTx(ctx, tx, order.OrderID, newStatus, "", notifyRaw); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}

	// 金额/身份三验，不符绝不入账——回 nil（HTTP 200）止损防重试风暴，人工经 sync 端点处置
	if txn.AmountTotal != order.PaymentAmountMinor ||
		(cfg.MchID != "" && txn.Mchid != "" && txn.Mchid != cfg.MchID) ||
		(cfg.AppID != "" && txn.Appid != "" && txn.Appid != cfg.AppID) {
		failNote := fmt.Sprintf("金额/商户号/AppID 校验不符: amount=%d mchid=%s appid=%s", txn.AmountTotal, txn.Mchid, txn.Appid)
		if err := paymentpg.UpdateStatusTx(ctx, tx, order.OrderID, order.Status, failNote, notifyRaw); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		s.logger.Error("[微信支付] 回调校验不符，拒绝入账", zap.String("outTradeNo", outTradeNo), zap.String("failNote", failNote))
		return nil
	}

	orderType := billingdomain.OrderTypeOnlineTenantTopup
	source := billingdomain.PackageSourceOnlineTopup
	if order.Scene == payment.SceneUserTopup {
		orderType = billingdomain.OrderTypeOnlineUserTopup
	}
	grant, err := billingsvc.GrantBalance(ctx, tx, billingsvc.GrantParams{
		OrderType: orderType, TenantID: order.TenantID, UserID: order.UserID,
		AmountMicroUSD: order.CreditedAmountMicroUSD, PaidAmount: order.PaymentAmountMinor,
		PaymentRef: txn.TransactionID, PaymentOrderID: order.OrderID, OperatorID: "system:wechatpay", Source: source, ExpiresAt: order.BalanceExpiresAt,
	})
	if err != nil {
		return err
	}

	if order.Scene == payment.SceneUserTopup && order.TenantIncomeMicroUSD > 0 {
		idemKey := "wxpay:" + outTradeNo
		exists, err := paymentpg.ExistsCashLedgerIdempotencyKey(ctx, tx, idemKey)
		if err != nil {
			return err
		}
		if !exists {
			incomeGrant, err := billingsvc.GrantBalance(ctx, tx, billingsvc.GrantParams{
				OrderType: billingdomain.OrderTypeUserTopupIncome, TenantID: order.TenantID,
				AmountMicroUSD: order.TenantIncomeMicroUSD, PaidAmount: order.PaymentAmountMinor,
				PaymentRef: txn.TransactionID, PaymentOrderID: order.OrderID, OperatorID: "system:wechatpay", Source: billingdomain.PackageSourceUserTopupIncome,
			})
			if err != nil {
				return err
			}
			balanceAfter, err := paymentpg.TenantBalanceAfterTx(ctx, tx, order.TenantID)
			if err != nil {
				return err
			}
			entry := &payment.CashLedgerEntry{
				TxnID: "CSH_" + uuid.New().String()[:24], TenantID: order.TenantID,
				TxnType: payment.CashTxnTopupIncome, AmountMicroUSD: order.TenantIncomeMicroUSD, BalanceAfterMicroUSD: balanceAfter,
				RefType: "recharge_order", RefID: incomeGrant.OrderID, OperatorID: "system:wechatpay",
			}
			if err := paymentpg.InsertCashLedgerTx(ctx, tx, entry, idemKey); err != nil {
				return err
			}
		}
	}

	if err := paymentpg.MarkPaidTx(ctx, tx, order.OrderID, txn.TransactionID, grant.OrderID, notifyRaw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SyncOrder 管理端手动查单同步：mock 模式下先驱动 SimulateSuccess，再走与真实一致的
// Query -> Settle 路径（这就是 mock 模式下的"仿真支付成功"入口）。
func (s *PaymentService) SyncOrder(ctx context.Context, orderID string) (*payment.Order, error) {
	order, err := paymentpg.GetOrderByID(ctx, s.pool, orderID)
	if err != nil {
		return nil, domain.ErrPaymentOrderNotFound
	}
	cfg, err := s.cfgStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Mock {
		s.gateway.SimulateSuccess(order.OutTradeNo, order.PaymentAmountMinor)
	}
	result, err := s.gateway.Query(ctx, order.OutTradeNo)
	if err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, order.OutTradeNo, result, nil); err != nil {
		return nil, err
	}
	return paymentpg.GetOrderByID(ctx, s.pool, orderID)
}

// HandleNotify 处理微信支付回调 HTTP 请求：验签解密 -> Settle。调用方（chi 原生 handler）
// 必须传入未被消费过 body 的原始 *http.Request。
func (s *PaymentService) HandleNotify(ctx context.Context, r *http.Request) error {
	result, err := s.gateway.ParseNotify(ctx, r)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{
		"transaction": result,
		"headers": map[string]string{
			"wechatpay-timestamp": r.Header.Get("Wechatpay-Timestamp"),
			"wechatpay-nonce":     r.Header.Get("Wechatpay-Nonce"),
			"wechatpay-serial":    r.Header.Get("Wechatpay-Serial"),
			"wechatpay-signature": truncateForAudit(r.Header.Get("Wechatpay-Signature"), 64),
		},
	})
	return s.Settle(ctx, result.OutTradeNo, result, raw)
}

// ==================== sweep 兜底（供 scheduler 调用） ====================

// SweepOnce 对应设计文档 §4.5：超时关单 + 在途补偿。
// It returns all cycle errors so the scheduler can publish task health and
// retry on the next interval instead of treating a partially failed cycle as
// successful.
func (s *PaymentService) SweepOnce(ctx context.Context) error {
	now := billingdomain.NowUTC()
	var cycleErrors []error

	expired, err := paymentpg.ListSweepCandidates(ctx, s.pool, now, 100)
	if err != nil {
		cycleErrors = append(cycleErrors, fmt.Errorf("查询超时订单失败: %w", err))
	}
	for _, o := range expired {
		if err := s.sweepExpiredOrder(ctx, o); err != nil {
			cycleErrors = append(cycleErrors, fmt.Errorf("处理超时订单 %s 失败: %w", o.OrderID, err))
		}
	}

	inFlight, err := paymentpg.ListInFlightCandidates(ctx, s.pool, now.Add(-5*time.Minute), now, 100)
	if err != nil {
		cycleErrors = append(cycleErrors, fmt.Errorf("查询在途订单失败: %w", err))
	}
	for _, o := range inFlight {
		if err := s.sweepInFlightOrder(ctx, o); err != nil {
			cycleErrors = append(cycleErrors, fmt.Errorf("处理在途订单 %s 失败: %w", o.OrderID, err))
		}
	}
	return errors.Join(cycleErrors...)
}

// CleanupClosedOrders removes only stale unpaid payment shells. Paid orders,
// fulfilled orders and any order with a balance/refund link are retained.
func (s *PaymentService) CleanupClosedOrders(ctx context.Context) error {
	cutoff := billingdomain.NowUTC().Add(-ClosedOrderRetention)
	total := 0
	for {
		deleted, err := paymentpg.DeleteStaleClosedOrders(ctx, s.pool, cutoff, closedOrderCleanupBatch)
		if err != nil {
			return fmt.Errorf("删除已关闭未支付订单失败: %w", err)
		}
		total += deleted
		if deleted < closedOrderCleanupBatch {
			break
		}
	}
	if total > 0 {
		s.logger.Info("[支付清理] 已删除长期未支付订单", zap.Int("orderCount", total), zap.Duration("retention", ClosedOrderRetention))
	}
	return nil
}

func (s *PaymentService) sweepExpiredOrder(ctx context.Context, o *payment.Order) error {
	result, queryErr := s.gateway.Query(ctx, o.OutTradeNo)
	if queryErr == nil && result != nil && result.TradeState == wechat.TradeStateSuccess {
		if err := s.Settle(ctx, o.OutTradeNo, result, nil); err != nil {
			return fmt.Errorf("补偿入账: %w", err)
		}
		return nil
	}
	if closeErr := s.gateway.Close(ctx, o.OutTradeNo); closeErr != nil {
		updated, err := paymentpg.UpdateStatusIfCurrent(ctx, s.pool, o.OrderID, o.Status, payment.OrderStatusExpired, closeErr.Error())
		if err != nil {
			return fmt.Errorf("标记订单 expired: %w", err)
		}
		if !updated {
			return nil
		}
		if queryErr != nil {
			return errors.Join(queryErr, closeErr)
		}
		return closeErr
	}
	updated, err := paymentpg.UpdateStatusIfCurrent(ctx, s.pool, o.OrderID, o.Status, payment.OrderStatusClosed, "")
	if err != nil {
		return fmt.Errorf("标记订单 closed: %w", err)
	}
	if !updated {
		return nil
	}
	return queryErr
}

func (s *PaymentService) sweepInFlightOrder(ctx context.Context, o *payment.Order) error {
	result, err := s.gateway.Query(ctx, o.OutTradeNo)
	if err != nil {
		return fmt.Errorf("查询支付状态: %w", err)
	}
	if result == nil || result.TradeState != wechat.TradeStateSuccess {
		return nil
	}
	if err := s.Settle(ctx, o.OutTradeNo, result, nil); err != nil {
		return fmt.Errorf("补偿入账: %w", err)
	}
	return nil
}

// ==================== 提现 ====================

// CreateWithdrawalParams describes an administrator-created withdrawal. The
// operation is deliberately one stage: it deducts the tenant balance, writes
// the cash ledger entry, and records the withdrawal as paid in one transaction.
type CreateWithdrawalParams struct {
	TenantID       string
	AmountMicroUSD int64
	AccountName    string
	BankName       string
	AccountNo      string
	Note           string
	OperatorID     string
	PaymentRef     string
}

func (s *PaymentService) CreateWithdrawal(ctx context.Context, p CreateWithdrawalParams) (*payment.Withdrawal, error) {
	p.TenantID = strings.TrimSpace(p.TenantID)
	p.AccountName = strings.TrimSpace(p.AccountName)
	p.BankName = strings.TrimSpace(p.BankName)
	p.AccountNo = strings.TrimSpace(p.AccountNo)
	p.Note = strings.TrimSpace(p.Note)
	p.OperatorID = strings.TrimSpace(p.OperatorID)
	p.PaymentRef = strings.TrimSpace(p.PaymentRef)
	if p.TenantID == "" || p.AmountMicroUSD <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return nil, err
	}
	feeAmount, err := money.ApplyBasisPointsCeil(p.AmountMicroUSD, global.TenantWithdrawFeeBp)
	if err != nil {
		return nil, domain.ErrInvalidAmount
	}
	payoutAmount := p.AmountMicroUSD - feeAmount
	if payoutAmount < 0 {
		payoutAmount = 0
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	account, err := paymentpg.GetBalanceAccountForUpdate(ctx, tx, p.TenantID)
	if err != nil {
		return nil, err
	}
	// A tenant may run AI into a negative balance, but cash must not be paid out
	// while it is negative. With a signed balance that is the same comparison as
	// "can they afford the withdrawal", so it needs no separate debt check.
	if account.BalanceMicroUSD < p.AmountMicroUSD {
		return nil, domain.ErrCashInsufficientBalance
	}
	if err := paymentpg.DeductTenantBalanceTx(ctx, tx, p.TenantID, p.AmountMicroUSD); err != nil {
		return nil, domain.ErrCashInsufficientBalance
	}

	w := &payment.Withdrawal{
		WithdrawalID: "WDR_" + uuid.New().String()[:24], TenantID: p.TenantID,
		AmountMicroUSD: p.AmountMicroUSD, FeeAmountMicroUSD: feeAmount, PayoutAmountMicroUSD: payoutAmount,
		AccountName: p.AccountName, BankName: p.BankName, AccountNo: p.AccountNo,
		ApplyNote: p.Note, AppliedBy: p.OperatorID, PaidBy: p.OperatorID, PaymentRef: p.PaymentRef,
		Status: payment.WithdrawalStatusPaid,
	}
	if err := paymentpg.InsertWithdrawalTx(ctx, tx, w); err != nil {
		return nil, err
	}
	balanceAfter, err := paymentpg.TenantBalanceAfterTx(ctx, tx, p.TenantID)
	if err != nil {
		return nil, err
	}
	entry := &payment.CashLedgerEntry{
		TxnID: "CSH_" + uuid.New().String()[:24], TenantID: p.TenantID,
		TxnType: payment.CashTxnWithdraw, AmountMicroUSD: -p.AmountMicroUSD,
		BalanceAfterMicroUSD: balanceAfter, RefType: "withdrawal", RefID: w.WithdrawalID,
		OperatorID: p.OperatorID, Note: p.Note,
	}
	if err := paymentpg.InsertCashLedgerTx(ctx, tx, entry, "withdrawal:"+w.WithdrawalID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *PaymentService) GetBalanceAccount(ctx context.Context, tenantID string) (*payment.BalanceAccount, error) {
	return paymentpg.GetBalanceAccount(ctx, s.pool, tenantID)
}

func (s *PaymentService) ListCashLedger(ctx context.Context, tenantID, txnType string, page, size int) ([]*payment.CashLedgerEntry, int64, error) {
	return paymentpg.ListCashLedger(ctx, s.pool, tenantID, txnType, page, size)
}

func (s *PaymentService) ListWithdrawals(ctx context.Context, p payment.WithdrawalListParams) ([]*payment.Withdrawal, int64, error) {
	p.TenantID = strings.TrimSpace(p.TenantID)
	p.Status = strings.TrimSpace(p.Status)
	if p.Status != "" && !payment.ValidWithdrawalStatus(p.Status) {
		return nil, 0, domain.ErrBadRequest
	}
	return paymentpg.ListWithdrawals(ctx, s.pool, p.TenantID, p.Status, p.Page, p.Size)
}

func (s *PaymentService) GetWithdrawal(ctx context.Context, withdrawalID string) (*payment.Withdrawal, error) {
	w, err := paymentpg.GetWithdrawal(ctx, s.pool, withdrawalID)
	if err != nil {
		return nil, domain.ErrWithdrawalNotFound
	}
	return w, nil
}

// ==================== 配置（全局费率/租户覆盖/微信商户配置） ====================

func (s *PaymentService) GetGlobalSettings(ctx context.Context) (*payment.GlobalSettings, error) {
	return s.settings.LoadGlobal(ctx)
}

func (s *PaymentService) UpdateGlobalSettings(ctx context.Context, g *payment.GlobalSettings, operatorID string) error {
	if err := validateGlobalSettings(g); err != nil {
		return err
	}
	return s.settings.SaveGlobal(ctx, g, operatorID)
}

func (s *PaymentService) GetTenantPaymentSettings(ctx context.Context, tenantID string) (*payment.TenantSettings, error) {
	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return nil, err
	}
	return s.settings.LoadTenantSettings(ctx, tenantID, global)
}

func (s *PaymentService) UpdateTenantPaymentSettings(ctx context.Context, tenantID string, ts *payment.TenantSettings, operatorID string) error {
	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return err
	}
	if err := validateTenantSettings(ts); err != nil {
		return err
	}
	return s.settings.SaveTenantSettings(ctx, tenantID, ts, global, operatorID)
}

func (s *PaymentService) GetWechatConfigView(ctx context.Context) (*wechat.AdminView, error) {
	return s.cfgStore.LoadAdminView(ctx)
}

func (s *PaymentService) UpdateWechatConfig(ctx context.Context, in wechat.UpdateInput, operatorID string) error {
	return s.cfgStore.Update(ctx, in, operatorID)
}

// TopupConfigView 是给用户/租户端展示的下单前信息（不暴露费率）。
type TopupConfigView struct {
	Currency     string
	FeeRateBp    int
	MinMicroUSD  int64
	MaxMicroUSD  int64
	ValidityDays *int32
	Enabled      bool
	Packages     []payment.TopupPackage
}

func (s *PaymentService) GetTopupConfigView(ctx context.Context, scene, tenantID string) (*TopupConfigView, error) {
	cfg, err := s.cfgStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return nil, err
	}
	var params payment.TopupParams
	if scene == payment.SceneUserTopup {
		tenantSettings, err := s.settings.LoadTenantSettings(ctx, tenantID, global)
		if err != nil {
			return nil, err
		}
		params = payment.ResolveUserTopup(tenantSettings)
	} else {
		params = payment.ResolveTenantTopup(global)
	}
	return &TopupConfigView{
		Currency: money.CurrencyUSD, FeeRateBp: params.FeeRateBp,
		MinMicroUSD: params.MinMicroUSD, MaxMicroUSD: params.MaxMicroUSD,
		ValidityDays: params.ValidityDays, Enabled: cfg.Enabled,
		Packages: enabledTopupPackages(params.Packages),
	}, nil
}

// ==================== helpers ====================

type topupSnapshot struct {
	Mode                   string
	PackageID              string
	PackageName            string
	PackageBadge           string
	PaymentAmountMicroUSD  int64
	PaymentAmountMinor     int64
	GrossAmountMicroUSD    int64
	FeeRateBp              int
	FeeAmountMicroUSD      int64
	GiftAmountMicroUSD     int64
	CreditedAmountMicroUSD int64
	ValidityDays           *int32
}

func calculateTopupSnapshot(params payment.TopupParams, amountMicroUSD int64, packageID string) (topupSnapshot, error) {
	packageID = strings.TrimSpace(packageID)
	if packageID != "" {
		for _, p := range params.Packages {
			if p.ID == packageID && p.Enabled {
				if amountMicroUSD > 0 && amountMicroUSD != p.PaymentAmountMicroUSD {
					return topupSnapshot{}, domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "充值套餐金额与实付金额不一致")
				}
				paymentMinor, err := paymentMinorFromMicroUSD(p.PaymentAmountMicroUSD)
				if err != nil {
					return topupSnapshot{}, err
				}
				return topupSnapshot{
					Mode: payment.TopupModePackage, PackageID: p.ID, PackageName: p.Name, PackageBadge: p.Badge,
					PaymentAmountMicroUSD: p.PaymentAmountMicroUSD, PaymentAmountMinor: paymentMinor,
					GrossAmountMicroUSD: p.PaymentAmountMicroUSD, GiftAmountMicroUSD: p.GiftAmountMicroUSD,
					CreditedAmountMicroUSD: p.PaymentAmountMicroUSD + p.GiftAmountMicroUSD, ValidityDays: p.ValidityDays,
				}, nil
			}
		}
		return topupSnapshot{}, domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "充值套餐不存在或已停用")
	}
	if amountMicroUSD < params.MinMicroUSD || amountMicroUSD > params.MaxMicroUSD {
		return topupSnapshot{}, domain.ErrPaymentAmountOutOfRange
	}
	paymentMinor, err := paymentMinorFromMicroUSD(amountMicroUSD)
	if err != nil {
		return topupSnapshot{}, err
	}
	feeMicroUSD, err := money.ApplyBasisPointsCeil(amountMicroUSD, params.FeeRateBp)
	if err != nil {
		return topupSnapshot{}, domain.ErrInvalidAmount
	}
	creditedMicroUSD := amountMicroUSD - feeMicroUSD
	if creditedMicroUSD <= 0 {
		return topupSnapshot{}, domain.ErrPaymentAmountOutOfRange
	}
	return topupSnapshot{
		Mode: payment.TopupModeCustom, PaymentAmountMicroUSD: amountMicroUSD, PaymentAmountMinor: paymentMinor,
		GrossAmountMicroUSD: amountMicroUSD, FeeRateBp: params.FeeRateBp,
		FeeAmountMicroUSD: feeMicroUSD, CreditedAmountMicroUSD: creditedMicroUSD, ValidityDays: params.ValidityDays,
	}, nil
}

func validateGlobalSettings(g *payment.GlobalSettings) error {
	if g.TenantCustomTopupFeeBp < 0 || g.TenantCustomTopupFeeBp > 10000 || g.TenantWithdrawFeeBp < 0 || g.TenantWithdrawFeeBp > 10000 {
		return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, "手续费必须在 0%~100% 之间")
	}
	return validatePackages(g.TenantTopupPackages)
}

func validateTenantSettings(ts *payment.TenantSettings) error {
	if ts.UserCustomTopupFeeBp < 0 || ts.UserCustomTopupFeeBp > 10000 {
		return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, "手续费必须在 0%~100% 之间")
	}
	return validatePackages(ts.UserTopupPackages)
}

func validatePackages(packages []payment.TopupPackage) error {
	if len(packages) > payment.MaxTopupPackages {
		return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "快捷充值套餐最多 12 个")
	}
	seen := map[string]struct{}{}
	for _, p := range packages {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "套餐 ID 不能为空")
		}
		if _, ok := seen[id]; ok {
			return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "套餐 ID 不能重复")
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(p.Name) == "" {
			return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "套餐名称不能为空")
		}
		if p.PaymentAmountMicroUSD < payment.TopupMinAmountMicroUSD || p.PaymentAmountMicroUSD > payment.TopupMaxAmountMicroUSD {
			return domain.NewErrorWithDetail(domain.ErrPaymentAmountOutOfRange.Code, domain.ErrPaymentAmountOutOfRange.Message, "套餐金额必须在 $10~$10000 之间")
		}
		if p.GiftAmountMicroUSD < 0 || p.PaymentAmountMicroUSD > payment.MaxPackageAmountMicroUSD-p.GiftAmountMicroUSD {
			return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, "套餐到账金额超出支持范围")
		}
	}
	return nil
}

func enabledTopupPackages(packages []payment.TopupPackage) []payment.TopupPackage {
	out := make([]payment.TopupPackage, 0, len(packages))
	for _, p := range packages {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out
}

func paymentMinorFromMicroUSD(amount int64) (int64, error) {
	const microPerCent = money.MicrosPerUSD / 100
	if amount <= 0 || amount%microPerCent != 0 {
		return 0, domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, "支付金额最多保留两位小数")
	}
	return amount / microPerCent, nil
}

const outTradeNoAlnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// generateOutTradeNo 生成符合微信要求的商户单号：P + 12位时间戳 + 8位随机字母数字，
// 共 21 位，≤32 位限制且不含 '-'（uuid 原文不合规，见设计文档 §3.1）。
func generateOutTradeNo() (string, error) {
	suffix := make([]byte, 8)
	for i := range suffix {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(outTradeNoAlnum))))
		if err != nil {
			return "", err
		}
		suffix[i] = outTradeNoAlnum[n.Int64()]
	}
	return "P" + time.Now().UTC().Format("060102150405") + string(suffix), nil
}

func topupDescription(scene string) string {
	if scene == payment.SceneUserTopup {
		return "DouStack 用户在线充值"
	}
	return "DouStack 租户在线充值"
}

func truncateForAudit(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
