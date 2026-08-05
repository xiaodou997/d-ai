// Package service 编排 payment 域的业务逻辑：下单、回调核销、sweep 兜底、余额购积分、
// 提现全流程。依赖 internal/payment（类型+配置存储）、internal/payment/pg（仓储）、
// internal/payment/wechat（支付网关）与 internal/billing/service（复用 GrantCredits）。
package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	billingdomain "xiaodou/dai/internal/billing"
	billingsvc "xiaodou/dai/internal/billing/service"
	"xiaodou/dai/internal/domain"
	"xiaodou/dai/internal/payment"
	paymentpg "xiaodou/dai/internal/payment/pg"
	"xiaodou/dai/internal/payment/wechat"
)

// PaymentService 是 payment 域的唯一编排入口。
type PaymentService struct {
	pool     *pgxpool.Pool
	settings *payment.SettingsStore
	cfgStore *wechat.ConfigStore
	gateway  wechat.Gateway
	logger   *zap.Logger
}

func New(pool *pgxpool.Pool, gateway wechat.Gateway, cfgStore *wechat.ConfigStore, logger *zap.Logger) *PaymentService {
	return &PaymentService{
		pool:     pool,
		settings: payment.NewSettingsStore(pool),
		cfgStore: cfgStore,
		gateway:  gateway,
		logger:   logger,
	}
}

// ==================== 下单 ====================

type CreateTopupOrderParams struct {
	Scene     string // payment.SceneUserTopup / SceneTenantTopup
	TenantID  string
	UserID    string
	AmountFen int64
	PackageID string
}

// CreateTopupOrder 对应设计文档 §4.2：校验限额 -> 快照汇率/费率 -> 建单 -> 微信 Prepay。
func (s *PaymentService) CreateTopupOrder(ctx context.Context, p CreateTopupOrderParams) (*payment.Order, error) {
	cfg, err := s.cfgStore.Load(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, domain.ErrPaymentDisabled
	}
	p.PackageID = strings.TrimSpace(p.PackageID)
	if p.AmountFen < 0 {
		return nil, domain.ErrInvalidAmount
	}
	if p.PackageID == "" && p.AmountFen <= 0 {
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

	calc, err := calculateTopupSnapshot(params, p.AmountFen, p.PackageID)
	if err != nil {
		return nil, err
	}
	if calc.CreditAmount <= 0 {
		return nil, domain.ErrPaymentAmountOutOfRange
	}
	netAmount := int64(0)
	if p.Scene == payment.SceneUserTopup {
		netAmount = calc.AmountFen
	}

	now := billingdomain.NowUTC()
	expiresAt := now.Add(cfg.OrderTTL)
	outTradeNo, err := generateOutTradeNo()
	if err != nil {
		return nil, fmt.Errorf("生成商户单号失败: %w", err)
	}

	order := &payment.Order{
		OrderID: "PAY_" + uuid.New().String()[:24], OutTradeNo: outTradeNo, Scene: p.Scene, TenantID: p.TenantID, UserID: p.UserID,
		TopupMode: calc.Mode, PackageID: calc.PackageID, PackageName: calc.PackageName, PackageBadge: calc.PackageBadge,
		Amount: calc.AmountFen, ExchangeRate: params.CreditsPerCNY, GrossCreditAmount: calc.GrossCredits,
		FeeRateBp: calc.FeeRateBp, FeeCreditAmount: calc.FeeCredits, CreditAmount: calc.CreditAmount,
		FeeAmount: 0, NetAmount: netAmount,
		Channel: "wechat_native", Status: payment.OrderStatusCreated, ExpiresAt: expiresAt,
	}
	if err := paymentpg.InsertOrder(ctx, s.pool, order); err != nil {
		return nil, err
	}

	codeURL, err := s.gateway.Prepay(ctx, outTradeNo, calc.AmountFen, expiresAt, topupDescription(p.Scene))
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

func (s *PaymentService) ListOrders(ctx context.Context, p paymentpg.ListOrdersParams) ([]*payment.Order, int64, error) {
	return paymentpg.ListOrders(ctx, s.pool, p)
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
	if txn.AmountTotal != order.Amount ||
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
	creditMicro, err := billingdomain.CreditsToMicro(order.CreditAmount)
	if err != nil {
		return err
	}
	grant, err := billingsvc.GrantCredits(ctx, tx, billingsvc.GrantParams{
		OrderType: orderType, TenantID: order.TenantID, UserID: order.UserID,
		CreditAmountMicro: creditMicro, PaidAmount: order.Amount,
		PaymentRef: txn.TransactionID, OperatorID: "system:wechatpay", Source: source,
	})
	if err != nil {
		return err
	}

	if order.Scene == payment.SceneUserTopup && order.NetAmount > 0 {
		idemKey := "wxpay:" + outTradeNo
		exists, err := paymentpg.ExistsCashLedgerIdempotencyKey(ctx, tx, idemKey)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := paymentpg.GetOrCreateCashAccountForUpdate(ctx, tx, order.TenantID); err != nil {
				return err
			}
			balanceAfter, err := paymentpg.AddCashBalanceTx(ctx, tx, order.TenantID, order.NetAmount)
			if err != nil {
				return err
			}
			entry := &payment.CashLedgerEntry{
				TxnID: "CSH_" + uuid.New().String()[:24], TenantID: order.TenantID,
				TxnType: payment.CashTxnTopupIncome, Amount: order.NetAmount, BalanceAfter: balanceAfter,
				RefType: "pay_order", RefID: order.OrderID, OperatorID: "system:wechatpay",
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
		s.gateway.SimulateSuccess(order.OutTradeNo, order.Amount)
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
func (s *PaymentService) SweepOnce(ctx context.Context) {
	now := billingdomain.NowUTC()

	expired, err := paymentpg.ListSweepCandidates(ctx, s.pool, now, 100)
	if err != nil {
		s.logger.Error("[支付sweep] 查询超时订单失败", zap.Error(err))
	}
	for _, o := range expired {
		s.sweepExpiredOrder(ctx, o)
	}

	inFlight, err := paymentpg.ListInFlightCandidates(ctx, s.pool, now.Add(-5*time.Minute), now, 100)
	if err != nil {
		s.logger.Error("[支付sweep] 查询在途订单失败", zap.Error(err))
	}
	for _, o := range inFlight {
		s.sweepInFlightOrder(ctx, o)
	}
}

func (s *PaymentService) sweepExpiredOrder(ctx context.Context, o *payment.Order) {
	result, err := s.gateway.Query(ctx, o.OutTradeNo)
	if err == nil && result.TradeState == wechat.TradeStateSuccess {
		if err := s.Settle(ctx, o.OutTradeNo, result, nil); err != nil {
			s.logger.Error("[支付sweep] 超时订单补偿入账失败", zap.String("orderId", o.OrderID), zap.Error(err))
		}
		return
	}
	if closeErr := s.gateway.Close(ctx, o.OutTradeNo); closeErr != nil {
		if err := paymentpg.UpdateStatus(ctx, s.pool, o.OrderID, payment.OrderStatusExpired, closeErr.Error()); err != nil {
			s.logger.Error("[支付sweep] 标记订单 expired 失败", zap.String("orderId", o.OrderID), zap.Error(err))
		}
		return
	}
	if err := paymentpg.UpdateStatus(ctx, s.pool, o.OrderID, payment.OrderStatusClosed, ""); err != nil {
		s.logger.Error("[支付sweep] 标记订单 closed 失败", zap.String("orderId", o.OrderID), zap.Error(err))
	}
}

func (s *PaymentService) sweepInFlightOrder(ctx context.Context, o *payment.Order) {
	result, err := s.gateway.Query(ctx, o.OutTradeNo)
	if err != nil || result.TradeState != wechat.TradeStateSuccess {
		return
	}
	if err := s.Settle(ctx, o.OutTradeNo, result, nil); err != nil {
		s.logger.Error("[支付sweep] 在途订单补偿入账失败", zap.String("orderId", o.OrderID), zap.Error(err))
	}
}

// ==================== 现金余额购积分（§4.4，内部划转不走微信） ====================

func (s *PaymentService) BuyCredits(ctx context.Context, tenantID string, amountFen int64, operatorID string) (*billingsvc.GrantResult, error) {
	if amountFen <= 0 {
		return nil, domain.ErrInvalidAmount
	}
	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	account, err := paymentpg.GetOrCreateCashAccountForUpdate(ctx, tx, tenantID)
	if err != nil {
		return nil, err
	}
	if account.Available() < amountFen {
		return nil, domain.ErrCashInsufficientBalance
	}

	credits, err := creditsFromAmount(amountFen, global.CreditsPerCNY)
	if err != nil {
		return nil, err
	}

	creditMicro, err := billingdomain.CreditsToMicro(credits)
	if err != nil {
		return nil, err
	}
	grant, err := billingsvc.GrantCredits(ctx, tx, billingsvc.GrantParams{
		OrderType: billingdomain.OrderTypeCashPurchase, TenantID: tenantID,
		CreditAmountMicro: creditMicro, PaidAmount: amountFen, PaymentRef: "cash",
		OperatorID: operatorID, Source: billingdomain.PackageSourceCashPurchase,
	})
	if err != nil {
		return nil, err
	}

	balanceAfter, err := paymentpg.AddCashBalanceTx(ctx, tx, tenantID, -amountFen)
	if err != nil {
		return nil, err
	}
	entry := &payment.CashLedgerEntry{
		TxnID: "CSH_" + uuid.New().String()[:24], TenantID: tenantID,
		TxnType: payment.CashTxnBuyCredits, Amount: -amountFen, BalanceAfter: balanceAfter,
		RefType: "recharge_order", RefID: grant.OrderID, OperatorID: operatorID,
	}
	if err := paymentpg.InsertCashLedgerTx(ctx, tx, entry, ""); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return grant, nil
}

// ==================== 提现 ====================

type ApplyWithdrawalParams struct {
	TenantID    string
	Amount      int64
	AccountName string
	BankName    string
	AccountNo   string
	Note        string
	AppliedBy   string
}

func (s *PaymentService) ApplyWithdrawal(ctx context.Context, p ApplyWithdrawalParams) (*payment.Withdrawal, error) {
	if p.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}
	global, err := s.settings.LoadGlobal(ctx)
	if err != nil {
		return nil, err
	}
	feeAmount := ceilDiv(p.Amount*int64(global.TenantWithdrawFeeBp), 10000)
	payoutAmount := p.Amount - feeAmount
	if payoutAmount < 0 {
		payoutAmount = 0
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	account, err := paymentpg.GetOrCreateCashAccountForUpdate(ctx, tx, p.TenantID)
	if err != nil {
		return nil, err
	}
	if account.Available() < p.Amount {
		return nil, domain.ErrCashInsufficientBalance
	}
	if err := paymentpg.AddCashFrozenTx(ctx, tx, p.TenantID, p.Amount); err != nil {
		return nil, err
	}

	w := &payment.Withdrawal{
		WithdrawalID: "WDR_" + uuid.New().String()[:24], TenantID: p.TenantID, Amount: p.Amount,
		FeeAmount: feeAmount, PayoutAmount: payoutAmount,
		AccountName: p.AccountName, BankName: p.BankName, AccountNo: p.AccountNo,
		ApplyNote: p.Note, AppliedBy: p.AppliedBy, Status: payment.WithdrawalStatusPending,
	}
	if err := paymentpg.InsertWithdrawalTx(ctx, tx, w); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *PaymentService) ReviewWithdrawal(ctx context.Context, withdrawalID string, approve bool, reviewerID, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	w, err := paymentpg.GetWithdrawalForUpdateTx(ctx, tx, withdrawalID)
	if err != nil {
		return domain.ErrWithdrawalNotFound
	}
	if w.Status != payment.WithdrawalStatusPending {
		return domain.ErrWithdrawalInvalidState
	}

	toStatus := payment.WithdrawalStatusApproved
	if !approve {
		toStatus = payment.WithdrawalStatusRejected
	}
	if err := paymentpg.UpdateWithdrawalReviewTx(ctx, tx, withdrawalID, toStatus, reviewerID, note); err != nil {
		return domain.ErrWithdrawalInvalidState
	}
	if !approve {
		// 驳回：解冻，不写流水（余额未变）
		if err := paymentpg.AddCashFrozenTx(ctx, tx, w.TenantID, -w.Amount); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PaymentService) SettleWithdrawal(ctx context.Context, withdrawalID, paidBy, paymentRef string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	w, err := paymentpg.GetWithdrawalForUpdateTx(ctx, tx, withdrawalID)
	if err != nil {
		return domain.ErrWithdrawalNotFound
	}
	if w.Status != payment.WithdrawalStatusApproved {
		return domain.ErrWithdrawalInvalidState
	}
	if err := paymentpg.UpdateWithdrawalSettleTx(ctx, tx, withdrawalID, paidBy, paymentRef); err != nil {
		return domain.ErrWithdrawalInvalidState
	}

	// 先减冻结、再减余额：pay_cash_accounts 的 CHECK(frozen<=balance) 是逐语句立即校验（非
	// deferred），若先减 balance，则当同一租户还有其他在途冻结提现时，中间态会先出现
	// frozen(未变)>balance(已减)而触发约束违规——必须先减 frozen 收窄冻结区间，两条语句
	// 才都能维持 frozen<=balance 不变式。
	if err := paymentpg.AddCashFrozenTx(ctx, tx, w.TenantID, -w.Amount); err != nil {
		return err
	}
	balanceAfter, err := paymentpg.AddCashBalanceTx(ctx, tx, w.TenantID, -w.Amount)
	if err != nil {
		return err
	}
	entry := &payment.CashLedgerEntry{
		TxnID: "CSH_" + uuid.New().String()[:24], TenantID: w.TenantID,
		TxnType: payment.CashTxnWithdraw, Amount: -w.Amount, BalanceAfter: balanceAfter,
		RefType: "withdrawal", RefID: withdrawalID, OperatorID: paidBy,
	}
	if err := paymentpg.InsertCashLedgerTx(ctx, tx, entry, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PaymentService) CancelWithdrawal(ctx context.Context, withdrawalID, requesterTenantID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	w, err := paymentpg.GetWithdrawalForUpdateTx(ctx, tx, withdrawalID)
	if err != nil {
		return domain.ErrWithdrawalNotFound
	}
	if w.TenantID != requesterTenantID {
		return domain.ErrWithdrawalNotFound
	}
	if w.Status != payment.WithdrawalStatusPending {
		return domain.ErrWithdrawalInvalidState
	}
	if err := paymentpg.UpdateWithdrawalCancelTx(ctx, tx, withdrawalID); err != nil {
		return domain.ErrWithdrawalInvalidState
	}
	if err := paymentpg.AddCashFrozenTx(ctx, tx, w.TenantID, -w.Amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ==================== 只读查询透传（现金账户/流水/提现列表） ====================

func (s *PaymentService) GetCashAccount(ctx context.Context, tenantID string) (*payment.CashAccount, error) {
	return paymentpg.GetCashAccount(ctx, s.pool, tenantID)
}

func (s *PaymentService) ListCashLedger(ctx context.Context, tenantID, txnType string, page, size int) ([]*payment.CashLedgerEntry, int64, error) {
	return paymentpg.ListCashLedger(ctx, s.pool, tenantID, txnType, page, size)
}

func (s *PaymentService) ListCashAccounts(ctx context.Context, page, size int) ([]*paymentpg.CashAccountRow, int64, error) {
	return paymentpg.ListCashAccounts(ctx, s.pool, page, size)
}

func (s *PaymentService) ListWithdrawals(ctx context.Context, tenantID, status string, page, size int) ([]*payment.Withdrawal, int64, error) {
	return paymentpg.ListWithdrawals(ctx, s.pool, tenantID, status, page, size)
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
	ExchangeRate int64
	FeeRateBp    int
	Min          int64
	Max          int64
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
		ExchangeRate: params.CreditsPerCNY,
		FeeRateBp:    params.FeeRateBp,
		Min:          params.Min,
		Max:          params.Max,
		Enabled:      cfg.Enabled,
		Packages:     enabledTopupPackages(params.Packages),
	}, nil
}

// ==================== helpers ====================

type topupSnapshot struct {
	Mode         string
	PackageID    string
	PackageName  string
	PackageBadge string
	AmountFen    int64
	GrossCredits int64
	FeeRateBp    int
	FeeCredits   int64
	CreditAmount int64
}

func calculateTopupSnapshot(params payment.TopupParams, amountFen int64, packageID string) (topupSnapshot, error) {
	packageID = strings.TrimSpace(packageID)
	if packageID != "" {
		for _, p := range params.Packages {
			if p.ID == packageID && p.Enabled {
				if amountFen > 0 && amountFen != p.Amount {
					return topupSnapshot{}, domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "充值套餐金额与实付金额不一致")
				}
				return topupSnapshot{
					Mode:         payment.TopupModePackage,
					PackageID:    p.ID,
					PackageName:  p.Name,
					PackageBadge: p.Badge,
					AmountFen:    p.Amount,
					GrossCredits: p.Credits,
					FeeRateBp:    0,
					FeeCredits:   0,
					CreditAmount: p.Credits,
				}, nil
			}
		}
		return topupSnapshot{}, domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "充值套餐不存在或已停用")
	}
	if amountFen < payment.TopupMinAmountFen || amountFen > payment.TopupMaxAmountFen {
		return topupSnapshot{}, domain.ErrPaymentAmountOutOfRange
	}
	grossCredits, err := creditsFromAmount(amountFen, params.CreditsPerCNY)
	if err != nil {
		return topupSnapshot{}, err
	}
	feeCredits := ceilDiv(grossCredits*int64(params.FeeRateBp), 10000)
	creditAmount := grossCredits - feeCredits
	if creditAmount <= 0 {
		return topupSnapshot{}, domain.ErrPaymentAmountOutOfRange
	}
	return topupSnapshot{
		Mode:         payment.TopupModeCustom,
		AmountFen:    amountFen,
		GrossCredits: grossCredits,
		FeeRateBp:    params.FeeRateBp,
		FeeCredits:   feeCredits,
		CreditAmount: creditAmount,
	}, nil
}

func validateGlobalSettings(g *payment.GlobalSettings) error {
	if g.CreditsPerCNY <= 0 || g.CreditsPerCNY > payment.MaxCreditsPerCNY {
		return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, fmt.Sprintf("1 元兑换积分必须在 1~%d 之间", payment.MaxCreditsPerCNY))
	}
	if g.TenantCustomTopupFeeBp < 0 || g.TenantCustomTopupFeeBp > 10000 || g.TenantWithdrawFeeBp < 0 || g.TenantWithdrawFeeBp > 10000 {
		return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, "手续费必须在 0%~100% 之间")
	}
	return validatePackages(g.TenantTopupPackages)
}

func validateTenantSettings(ts *payment.TenantSettings) error {
	if ts.UserCreditsPerCNY <= 0 || ts.UserCreditsPerCNY > payment.MaxCreditsPerCNY {
		return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, fmt.Sprintf("1 元兑换积分必须在 1~%d 之间", payment.MaxCreditsPerCNY))
	}
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
		if p.Amount < payment.TopupMinAmountFen || p.Amount > payment.TopupMaxAmountFen {
			return domain.NewErrorWithDetail(domain.ErrPaymentAmountOutOfRange.Code, domain.ErrPaymentAmountOutOfRange.Message, "套餐金额必须在 10~10000 元之间")
		}
		if p.Credits <= 0 || p.Credits > payment.MaxPackageCredits {
			return domain.NewErrorWithDetail(domain.ErrInvalidAmount.Code, domain.ErrInvalidAmount.Message, fmt.Sprintf("套餐到账积分必须在 1~%d 之间", payment.MaxPackageCredits))
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

func creditsFromAmount(amountFen, creditsPerCNY int64) (int64, error) {
	if amountFen <= 0 || creditsPerCNY <= 0 || creditsPerCNY > payment.MaxCreditsPerCNY {
		return 0, domain.ErrInvalidAmount
	}
	const maxInt64 = int64(1<<63 - 1)
	if amountFen > maxInt64/creditsPerCNY {
		return 0, domain.ErrInvalidAmount
	}
	credits := amountFen * creditsPerCNY / 100
	if credits <= 0 {
		return 0, domain.ErrInvalidAmount
	}
	return credits, nil
}

func ceilDiv(a, b int64) int64 {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
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
