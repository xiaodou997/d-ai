package wechat

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"

	"xiaodou/dai/internal/domain"
)

// 微信支付交易状态（官方文档固定字符串，SDK 未提供常量）。
const (
	TradeStateSuccess    = "SUCCESS"
	TradeStateRefund     = "REFUND"
	TradeStateNotPay     = "NOTPAY"
	TradeStateClosed     = "CLOSED"
	TradeStateRevoked    = "REVOKED"
	TradeStateUserPaying = "USERPAYING"
	TradeStatePayError   = "PAYERROR"
)

// QueryResult 是查单/回调结果的领域内表示，与 wechatpay-go 的 SDK 类型解耦。
type QueryResult struct {
	TradeState    string
	TransactionID string
	OutTradeNo    string
	AmountTotal   int64
	Appid         string
	Mchid         string
}

// Gateway 是微信支付 Native 下单/查单/关单/回调解析的统一接口；service 层只依赖此接口，
// 不感知 wechatpay-go SDK 或 mock 细节。
type Gateway interface {
	// Prepay 生成 Native 支付二维码链接。
	Prepay(ctx context.Context, outTradeNo string, amountFen int64, expireAt time.Time, description string) (codeURL string, err error)
	// Query 查询商户单号对应的交易状态。
	Query(ctx context.Context, outTradeNo string) (*QueryResult, error)
	// Close 关闭一笔未支付的订单。
	Close(ctx context.Context, outTradeNo string) error
	// ParseNotify 从回调 HTTP 请求中验签 + 解密，返回交易结果（仅真实模式下微信会回调）。
	ParseNotify(ctx context.Context, r *http.Request) (*QueryResult, error)
	// SimulateSuccess 仅 mock 模式下有意义：把 outTradeNo 标记为"已支付"，供管理端 sync 端点驱动
	// 下一次 Query 返回 SUCCESS。真实模式下调用无效果。
	SimulateSuccess(outTradeNo string, amountFen int64)
}

type mockPaidState struct {
	amountFen int64
	txnID     string
}

// gateway 是 Gateway 的唯一实现：根据 pay_wechat_config.mock 实时决定走真实 wechatpay-go
// SDK 还是内存仿真，因为管理端可以随时切换开关/模式而无需重启进程。
type gateway struct {
	store *ConfigStore

	mu           sync.RWMutex
	cachedCredFp string // 商户号/私钥/证书序列号/APIv3Key 指纹，见 credentialFingerprint
	client       *core.Client

	mockMu   sync.Mutex
	mockPaid map[string]mockPaidState
}

// NewGateway 构造 Gateway。
func NewGateway(store *ConfigStore) Gateway {
	return &gateway{store: store, mockPaid: make(map[string]mockPaidState)}
}

func (g *gateway) SimulateSuccess(outTradeNo string, amountFen int64) {
	g.mockMu.Lock()
	defer g.mockMu.Unlock()
	g.mockPaid[outTradeNo] = mockPaidState{amountFen: amountFen, txnID: "MOCK_" + outTradeNo}
}

func (g *gateway) Prepay(ctx context.Context, outTradeNo string, amountFen int64, expireAt time.Time, description string) (string, error) {
	cfg, err := g.store.Load(ctx)
	if err != nil {
		return "", err
	}
	if cfg.Mock {
		return "weixin://wxpay/bizpayurl?mock_order=" + outTradeNo, nil
	}
	client, err := g.realClient(ctx, cfg)
	if err != nil {
		return "", err
	}
	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(cfg.AppID),
		Mchid:       core.String(cfg.MchID),
		Description: core.String(description),
		OutTradeNo:  core.String(outTradeNo),
		TimeExpire:  core.Time(expireAt),
		NotifyUrl:   core.String(cfg.NotifyBaseURL + "/api/v1/payments/wechat/notify"),
		Amount:      &native.Amount{Total: core.Int64(amountFen)},
	})
	if err != nil {
		return "", fmt.Errorf("微信下单失败: %w", err)
	}
	if resp == nil || resp.CodeUrl == nil {
		return "", fmt.Errorf("微信下单响应缺少 code_url")
	}
	return *resp.CodeUrl, nil
}

func (g *gateway) Query(ctx context.Context, outTradeNo string) (*QueryResult, error) {
	cfg, err := g.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Mock {
		g.mockMu.Lock()
		st, ok := g.mockPaid[outTradeNo]
		g.mockMu.Unlock()
		if !ok {
			return &QueryResult{TradeState: TradeStateNotPay, OutTradeNo: outTradeNo}, nil
		}
		return &QueryResult{
			TradeState: TradeStateSuccess, OutTradeNo: outTradeNo,
			TransactionID: st.txnID, AmountTotal: st.amountFen,
			Appid: cfg.AppID, Mchid: cfg.MchID,
		}, nil
	}
	client, err := g.realClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	svc := native.NativeApiService{Client: client}
	txn, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(cfg.MchID),
	})
	if err != nil {
		return nil, fmt.Errorf("微信查单失败: %w", err)
	}
	return transactionToResult(txn), nil
}

func (g *gateway) Close(ctx context.Context, outTradeNo string) error {
	cfg, err := g.store.Load(ctx)
	if err != nil {
		return err
	}
	if cfg.Mock {
		return nil
	}
	client, err := g.realClient(ctx, cfg)
	if err != nil {
		return err
	}
	svc := native.NativeApiService{Client: client}
	_, err = svc.CloseOrder(ctx, native.CloseOrderRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(cfg.MchID),
	})
	if err != nil {
		return fmt.Errorf("微信关单失败: %w", err)
	}
	return nil
}

func (g *gateway) ParseNotify(ctx context.Context, r *http.Request) (*QueryResult, error) {
	cfg, err := g.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Mock {
		return nil, fmt.Errorf("mock 模式下不接受微信回调，请用管理端 sync 端点仿真支付")
	}
	if _, err := g.realClient(ctx, cfg); err != nil { // 确保证书下载器已按最新配置注册
		return nil, err
	}
	verifier, err := buildVerifier(cfg)
	if err != nil {
		return nil, err
	}
	handler, err := notify.NewRSANotifyHandler(cfg.APIv3Key, verifier)
	if err != nil {
		return nil, fmt.Errorf("构造回调验签器失败: %w", err)
	}
	txn := new(payments.Transaction)
	if _, err := handler.ParseNotifyRequest(ctx, r, txn); err != nil {
		return nil, fmt.Errorf("回调验签/解密失败: %w", err)
	}
	return transactionToResult(txn), nil
}

func transactionToResult(txn *payments.Transaction) *QueryResult {
	r := &QueryResult{}
	if txn.TradeState != nil {
		r.TradeState = *txn.TradeState
	}
	if txn.TransactionId != nil {
		r.TransactionID = *txn.TransactionId
	}
	if txn.OutTradeNo != nil {
		r.OutTradeNo = *txn.OutTradeNo
	}
	if txn.Appid != nil {
		r.Appid = *txn.Appid
	}
	if txn.Mchid != nil {
		r.Mchid = *txn.Mchid
	}
	if txn.Amount != nil && txn.Amount.Total != nil {
		r.AmountTotal = *txn.Amount.Total
	}
	return r
}

// credentialFingerprint 只取影响 wechatpay-go client 构造的字段（商户号/私钥/证书序列号/
// APIv3Key）。不能直接用 cfg.UpdatedAt 判断是否需要重建——管理端修改 OrderTTL、
// NotifyBaseURL、Mock 开关等无关字段也会更新 UpdatedAt，若以此为缓存键，会让下一次
// 用户下单/查单请求触发一次不必要的同步阻塞式证书重新下载（NewCertificateDownloaderWithClient
// 注册时会立即发起一次真实 HTTPS 请求），造成与本次改动无关的延迟尖峰。
func credentialFingerprint(cfg *MerchantConfig) string {
	return cfg.VerifyMode + "\x00" + cfg.MchID + "\x00" + cfg.MchCertSerialNo + "\x00" +
		cfg.APIv3Key + "\x00" + cfg.MchPrivateKey + "\x00" + cfg.WechatPublicKeyID + "\x00" + cfg.WechatPublicKey
}

// realClient 惰性构建/重建 core.Client，只在商户凭证本身变化时才重建；同一 mchID 下也会
// 强制重新向 downloader.MgrInstance() 注册私钥，因为 option.WithWechatPayAutoAuthCipher
// 对已注册过的 mchID 会跳过注册（见其 HasDownloader 判断），无法感知私钥已被管理端更换——
// 这里绕开该判断，直接用 *WithDownloaderMgr 变体。
func (g *gateway) realClient(ctx context.Context, cfg *MerchantConfig) (*core.Client, error) {
	fp := credentialFingerprint(cfg)

	g.mu.RLock()
	if g.client != nil && g.cachedCredFp == fp {
		client := g.client
		g.mu.RUnlock()
		return client, nil
	}
	g.mu.RUnlock()

	if cfg.MchID == "" || cfg.MchCertSerialNo == "" || cfg.MchPrivateKey == "" || cfg.APIv3Key == "" || cfg.AppID == "" {
		return nil, domain.ErrPaymentConfigIncomplete
	}

	privateKey, err := utils.LoadPrivateKey(cfg.MchPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("解析商户私钥失败: %w", err)
	}

	var client *core.Client
	if normalizeVerifyMode(cfg.VerifyMode) == VerifyModePublicKey {
		publicKey, loadErr := utils.LoadPublicKey(formatPEM(cfg.WechatPublicKey, "PUBLIC KEY"))
		if loadErr != nil {
			return nil, fmt.Errorf("解析微信支付公钥失败: %w", loadErr)
		}
		client, err = core.NewClient(ctx,
			option.WithMerchantCredential(cfg.MchID, cfg.MchCertSerialNo, privateKey),
			option.WithVerifier(verifiers.NewSHA256WithRSAPubkeyVerifier(cfg.WechatPublicKeyID, *publicKey)))
	} else {
		mgr := downloader.MgrInstance()
		if err := mgr.RegisterDownloaderWithPrivateKey(ctx, privateKey, cfg.MchCertSerialNo, cfg.MchID, cfg.APIv3Key); err != nil {
			return nil, fmt.Errorf("注册商户证书下载器失败: %w", err)
		}
		client, err = core.NewClient(ctx,
			option.WithWechatPayAutoAuthCipherUsingDownloaderMgr(cfg.MchID, cfg.MchCertSerialNo, privateKey, mgr))
	}
	if err != nil {
		return nil, fmt.Errorf("初始化微信支付客户端失败: %w", err)
	}

	g.mu.Lock()
	g.client, g.cachedCredFp = client, fp
	g.mu.Unlock()
	return client, nil
}

func buildVerifier(cfg *MerchantConfig) (auth.Verifier, error) {
	if normalizeVerifyMode(cfg.VerifyMode) == VerifyModePublicKey {
		publicKey, err := utils.LoadPublicKey(formatPEM(cfg.WechatPublicKey, "PUBLIC KEY"))
		if err != nil {
			return nil, fmt.Errorf("解析微信支付公钥失败: %w", err)
		}
		return verifiers.NewSHA256WithRSAPubkeyVerifier(cfg.WechatPublicKeyID, *publicKey), nil
	}
	return verifiers.NewSHA256WithRSAVerifier(downloader.MgrInstance().GetCertificateVisitor(cfg.MchID)), nil
}
