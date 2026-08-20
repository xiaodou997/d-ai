// Package wechat 实现微信支付网关：商户配置读写（含敏感字段加解密）和
// Gateway 接口（真实 wechatpay-go 实现 / mock 仿真实现）。它只处理金额、商户单号和回调。
package wechat

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/domain"
)

const (
	VerifyModePlatformCert = "platform_cert"
	VerifyModePublicKey    = "public_key"

	minOrderTTLSeconds = 300
	maxOrderTTLSeconds = 86400
	apiV3KeyLength     = 32
)

// MerchantConfig 是某一时刻生效的微信商户配置（敏感字段已解密，仅在内存中短暂持有）。
type MerchantConfig struct {
	Enabled           bool
	Mock              bool
	VerifyMode        string
	AppID             string
	MchID             string
	MchCertSerialNo   string
	MchPrivateKey     string // PEM 明文
	APIv3Key          string
	WechatPublicKeyID string
	WechatPublicKey   string
	NotifyBaseURL     string
	OrderTTL          time.Duration
	UpdatedAt         time.Time // 用作缓存版本号
}

// AdminView 是管理端 GET 接口的回显：不包含私钥/APIv3Key 明文，只给出「是否已配置」。
type AdminView struct {
	Enabled            bool
	Mock               bool
	VerifyMode         string
	AppID              string
	MchID              string
	MchCertSerialNo    string
	NotifyBaseURL      string
	OrderTTLSeconds    int
	HasPrivateKey      bool
	HasAPIv3Key        bool
	WechatPublicKeyID  string
	HasWechatPublicKey bool
	UpdatedAt          time.Time
}

// UpdateInput 是管理端 PUT 接口的入参。MchPrivateKey/APIv3Key 为 nil 表示不修改现有密钥；
// 传入非 nil 空字符串则视为显式清空（极少见，一般只在切换商户号时才会同时改注这两项）。
type UpdateInput struct {
	Enabled           bool
	Mock              bool
	VerifyMode        string
	AppID             string
	MchID             string
	MchCertSerialNo   string
	NotifyBaseURL     string
	OrderTTLSeconds   int
	MchPrivateKey     *string
	APIv3Key          *string
	WechatPublicKeyID *string
	WechatPublicKey   *string
}

// ConfigStore 读写 pay_wechat_config 单例行。
type ConfigStore struct {
	pool *pgxpool.Pool
}

func NewConfigStore(pool *pgxpool.Pool) *ConfigStore {
	return &ConfigStore{pool: pool}
}

// Load 读取当前商户配置并解密私钥/APIv3Key。clientsecret 未初始化（未配置主密钥）时，
// 加密字段无法解密——此时返回错误，调用方应视为"微信支付不可用"。
func (s *ConfigStore) Load(ctx context.Context) (*MerchantConfig, error) {
	var (
		cfg                        MerchantConfig
		mchPrivateKeyEnc, apiv3Enc string
		orderTTLSeconds            int
	)
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, mock, COALESCE(verify_mode,'platform_cert'),
		       COALESCE(app_id,''), COALESCE(mch_id,''), COALESCE(mch_cert_serial_no,''),
		       COALESCE(mch_private_key_enc,''), COALESCE(apiv3_key_enc,''),
		       COALESCE(wechat_public_key_id,''), COALESCE(wechat_public_key,''),
		       COALESCE(notify_base_url,''), order_ttl_seconds, updated_at
		FROM pay_wechat_config WHERE id = 1
	`).Scan(&cfg.Enabled, &cfg.Mock, &cfg.VerifyMode, &cfg.AppID, &cfg.MchID, &cfg.MchCertSerialNo,
		&mchPrivateKeyEnc, &apiv3Enc, &cfg.WechatPublicKeyID, &cfg.WechatPublicKey,
		&cfg.NotifyBaseURL, &orderTTLSeconds, &cfg.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("加载微信商户配置失败: %w", err)
	}
	cfg.OrderTTL = time.Duration(orderTTLSeconds) * time.Second
	cfg.NotifyBaseURL = normalizeBaseURL(cfg.NotifyBaseURL)

	if mchPrivateKeyEnc != "" {
		plain, err := clientsecret.Decrypt(mchPrivateKeyEnc)
		if err != nil {
			return nil, fmt.Errorf("解密商户私钥失败: %w", err)
		}
		cfg.MchPrivateKey = plain
		s.maybeReencrypt(ctx, "mch_private_key_enc", mchPrivateKeyEnc)
	}
	if apiv3Enc != "" {
		plain, err := clientsecret.Decrypt(apiv3Enc)
		if err != nil {
			return nil, fmt.Errorf("解密 APIv3Key 失败: %w", err)
		}
		cfg.APIv3Key = plain
		s.maybeReencrypt(ctx, "apiv3_key_enc", apiv3Enc)
	}
	return &cfg, nil
}

// maybeReencrypt upgrades credentials written with a previous key without
// changing payment configuration semantics. Errors are intentionally ignored:
// a successful decrypt keeps the current request available, while a later
// load retries the migration.
func (s *ConfigStore) maybeReencrypt(ctx context.Context, column, ciphertext string) {
	if !clientsecret.NeedsReencrypt(ciphertext) {
		return
	}
	reencrypted, err := clientsecret.Reencrypt(ciphertext)
	if err != nil {
		return
	}
	if column != "mch_private_key_enc" && column != "apiv3_key_enc" {
		return
	}
	_, _ = s.pool.Exec(ctx, "UPDATE pay_wechat_config SET "+column+" = $1, updated_at = now() WHERE id = 1", reencrypted)
}

// LoadAdminView 返回脱敏视图，供管理端 GET 接口展示。
func (s *ConfigStore) LoadAdminView(ctx context.Context) (*AdminView, error) {
	var (
		v                                     AdminView
		mchPrivateKeyEnc, apiv3Enc, publicKey string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, mock, COALESCE(verify_mode,'platform_cert'),
		       COALESCE(app_id,''), COALESCE(mch_id,''), COALESCE(mch_cert_serial_no,''),
		       COALESCE(mch_private_key_enc,''), COALESCE(apiv3_key_enc,''),
		       COALESCE(wechat_public_key_id,''), COALESCE(wechat_public_key,''),
		       COALESCE(notify_base_url,''), order_ttl_seconds, updated_at
		FROM pay_wechat_config WHERE id = 1
	`).Scan(&v.Enabled, &v.Mock, &v.VerifyMode, &v.AppID, &v.MchID, &v.MchCertSerialNo,
		&mchPrivateKeyEnc, &apiv3Enc, &v.WechatPublicKeyID, &publicKey,
		&v.NotifyBaseURL, &v.OrderTTLSeconds, &v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("加载微信商户配置失败: %w", err)
	}
	v.NotifyBaseURL = normalizeBaseURL(v.NotifyBaseURL)
	v.HasPrivateKey = mchPrivateKeyEnc != ""
	v.HasAPIv3Key = apiv3Enc != ""
	v.HasWechatPublicKey = publicKey != ""
	return &v, nil
}

// Update 落库管理端修改。密钥字段为 nil 时保留原加密值（"留空=不改"）。
func (s *ConfigStore) Update(ctx context.Context, in UpdateInput, operatorID string) error {
	current, err := s.Load(ctx)
	if err != nil {
		return err
	}
	next := mergeUpdate(current, in)
	if err := validateConfig(next); err != nil {
		return err
	}
	if shouldBlockCredentialChange(current, next) {
		count, err := s.countOpenOrders(ctx)
		if err != nil {
			return err
		}
		if count > 0 {
			return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message,
				fmt.Sprintf("存在 %d 笔未完成支付订单，请等待完成/过期后再切换微信商户身份或密钥", count))
		}
	}

	var privateKeyEncClause, apiv3EncClause, publicKeyIDClause, publicKeyClause string
	args := []any{in.Enabled, in.Mock, normalizeVerifyMode(in.VerifyMode), strings.TrimSpace(in.AppID),
		strings.TrimSpace(in.MchID), strings.TrimSpace(in.MchCertSerialNo), normalizeBaseURL(in.NotifyBaseURL),
		normalizedTTL(in.OrderTTLSeconds), operatorID}

	// 依次拼接可选密钥字段，保持参数序号可控
	setSQL := `UPDATE pay_wechat_config SET
		enabled = $1, mock = $2, verify_mode = $3, app_id = NULLIF($4,''), mch_id = NULLIF($5,''),
		mch_cert_serial_no = NULLIF($6,''), notify_base_url = NULLIF($7,''),
		order_ttl_seconds = $8, updated_by = $9, updated_at = now()`

	nextIdx := 10
	if in.MchPrivateKey != nil {
		enc := ""
		if strings.TrimSpace(*in.MchPrivateKey) != "" {
			var err error
			enc, err = clientsecret.Encrypt(strings.TrimSpace(*in.MchPrivateKey))
			if err != nil {
				return fmt.Errorf("加密商户私钥失败: %w", err)
			}
		}
		privateKeyEncClause = fmt.Sprintf(", mch_private_key_enc = NULLIF($%d,'')", nextIdx)
		args = append(args, enc)
		nextIdx++
	}
	if in.APIv3Key != nil {
		enc := ""
		if strings.TrimSpace(*in.APIv3Key) != "" {
			var err error
			enc, err = clientsecret.Encrypt(strings.TrimSpace(*in.APIv3Key))
			if err != nil {
				return fmt.Errorf("加密 APIv3Key 失败: %w", err)
			}
		}
		apiv3EncClause = fmt.Sprintf(", apiv3_key_enc = NULLIF($%d,'')", nextIdx)
		args = append(args, enc)
		nextIdx++
	}
	if in.WechatPublicKeyID != nil {
		publicKeyIDClause = fmt.Sprintf(", wechat_public_key_id = NULLIF($%d,'')", nextIdx)
		args = append(args, strings.TrimSpace(*in.WechatPublicKeyID))
		nextIdx++
	}
	if in.WechatPublicKey != nil {
		publicKeyClause = fmt.Sprintf(", wechat_public_key = NULLIF($%d,'')", nextIdx)
		args = append(args, strings.TrimSpace(*in.WechatPublicKey))
		nextIdx++
	}

	setSQL += privateKeyEncClause + apiv3EncClause + publicKeyIDClause + publicKeyClause + " WHERE id = 1"
	_, err = s.pool.Exec(ctx, setSQL, args...)
	if err != nil {
		return fmt.Errorf("更新微信商户配置失败: %w", err)
	}
	return nil
}

func mergeUpdate(current *MerchantConfig, in UpdateInput) *MerchantConfig {
	next := *current
	next.Enabled = in.Enabled
	next.Mock = in.Mock
	next.VerifyMode = normalizeVerifyMode(in.VerifyMode)
	next.AppID = strings.TrimSpace(in.AppID)
	next.MchID = strings.TrimSpace(in.MchID)
	next.MchCertSerialNo = strings.TrimSpace(in.MchCertSerialNo)
	next.NotifyBaseURL = normalizeBaseURL(in.NotifyBaseURL)
	next.OrderTTL = time.Duration(normalizedTTL(in.OrderTTLSeconds)) * time.Second
	if in.MchPrivateKey != nil {
		next.MchPrivateKey = strings.TrimSpace(*in.MchPrivateKey)
	}
	if in.APIv3Key != nil {
		next.APIv3Key = strings.TrimSpace(*in.APIv3Key)
	}
	if in.WechatPublicKeyID != nil {
		next.WechatPublicKeyID = strings.TrimSpace(*in.WechatPublicKeyID)
	}
	if in.WechatPublicKey != nil {
		next.WechatPublicKey = strings.TrimSpace(*in.WechatPublicKey)
	}
	return &next
}

func validateConfig(cfg *MerchantConfig) error {
	if cfg.OrderTTL < minOrderTTLSeconds*time.Second || cfg.OrderTTL > maxOrderTTLSeconds*time.Second {
		return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "订单有效期需在 300~86400 秒之间")
	}
	if !cfg.Enabled || cfg.Mock {
		return nil
	}
	if cfg.AppID == "" || cfg.MchID == "" || cfg.MchCertSerialNo == "" || cfg.MchPrivateKey == "" || cfg.APIv3Key == "" || cfg.NotifyBaseURL == "" {
		return domain.ErrPaymentConfigIncomplete
	}
	if len(cfg.APIv3Key) != apiV3KeyLength {
		return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "APIv3Key 必须为 32 个字符")
	}
	u, err := url.Parse(cfg.NotifyBaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "真实模式下 NotifyBaseURL 必须是 HTTPS 绝对地址")
	}
	if _, err := utils.LoadPrivateKey(cfg.MchPrivateKey); err != nil {
		return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "商户私钥无法解析")
	}
	switch normalizeVerifyMode(cfg.VerifyMode) {
	case VerifyModePublicKey:
		if cfg.WechatPublicKeyID == "" || cfg.WechatPublicKey == "" {
			return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "微信支付公钥模式必须配置公钥 ID 和公钥")
		}
		if _, err := utils.LoadPublicKey(formatPEM(cfg.WechatPublicKey, "PUBLIC KEY")); err != nil {
			return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "微信支付公钥无法解析")
		}
	case VerifyModePlatformCert:
	default:
		return domain.NewErrorWithDetail(domain.ErrBadRequest.Code, domain.ErrBadRequest.Message, "微信验签模式不合法")
	}
	return nil
}

func normalizeVerifyMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return VerifyModePlatformCert
	}
	return mode
}

func normalizedTTL(seconds int) int {
	if seconds == 0 {
		return 7200
	}
	return seconds
}

func normalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func shouldBlockCredentialChange(current, next *MerchantConfig) bool {
	if current == nil || next == nil || (current.Mock && next.Mock) {
		return false
	}
	return credentialConfigFingerprint(current) != credentialConfigFingerprint(next)
}

func credentialConfigFingerprint(cfg *MerchantConfig) string {
	return strings.Join([]string{
		cfg.VerifyMode, cfg.AppID, cfg.MchID, cfg.MchCertSerialNo, cfg.MchPrivateKey,
		cfg.APIv3Key, cfg.WechatPublicKeyID, cfg.WechatPublicKey,
	}, "\x00")
}

func (s *ConfigStore) countOpenOrders(ctx context.Context) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pay_orders WHERE status IN ('created', 'paying')
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("检查未完成支付订单失败: %w", err)
	}
	return count, nil
}

func formatPEM(key, keyType string) string {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "-----BEGIN") {
		return key
	}
	return fmt.Sprintf("-----BEGIN %s-----\n%s\n-----END %s-----", keyType, key, keyType)
}
