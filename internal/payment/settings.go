package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TopupModeCustom  = "custom"
	TopupModePackage = "package"

	TopupMinAmountFen = int64(1000)
	TopupMaxAmountFen = int64(1000000)
	MaxTopupPackages  = 12
	MaxCreditsPerCNY  = int64(1000000)
	MaxPackageCredits = int64(10000000000)
)

// TopupPackage 是展示给用户点击的快捷充值包。amount 单位为分，credits 是固定到账积分。
type TopupPackage struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Amount    int64  `json:"amount"`
	Credits   int64  `json:"credits"`
	Badge     string `json:"badge,omitempty"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
}

// GlobalSettings 是平台统一支付设置：租户自己充值、提现规则、默认快捷套餐。
type GlobalSettings struct {
	CreditsPerCNY          int64          `json:"creditsPerCny"`
	TenantCustomTopupFeeBp int            `json:"tenantCustomTopupFeeBp"`
	TenantWithdrawFeeBp    int            `json:"tenantWithdrawFeeBp"`
	TenantTopupPackages    []TopupPackage `json:"tenantTopupPackages"`
}

// TenantSettings 是租户配置给终端用户看的充值规则。
type TenantSettings struct {
	UserCreditsPerCNY    int64          `json:"userCreditsPerCny"`
	UserCustomTopupFeeBp int            `json:"userCustomTopupFeeBp"`
	UserTopupPackages    []TopupPackage `json:"userTopupPackages"`
}

// TopupParams 是下单时使用的充值规则快照。
type TopupParams struct {
	CreditsPerCNY int64
	FeeRateBp     int
	Packages      []TopupPackage
	Min           int64
	Max           int64
}

type SettingsStore struct {
	pool *pgxpool.Pool
}

func NewSettingsStore(pool *pgxpool.Pool) *SettingsStore {
	return &SettingsStore{pool: pool}
}

func DefaultTopupPackages() []TopupPackage {
	return []TopupPackage{
		{ID: "p10", Name: "10 元体验包", Amount: 1000, Credits: 1000, Enabled: true, SortOrder: 10},
		{ID: "p20", Name: "20 元基础包", Amount: 2000, Credits: 2000, Enabled: true, SortOrder: 20},
		{ID: "p50", Name: "50 元常用包", Amount: 5000, Credits: 5000, Enabled: true, SortOrder: 30},
		{ID: "p100", Name: "100 元进阶包", Amount: 10000, Credits: 10000, Enabled: true, SortOrder: 40},
	}
}

func DefaultGlobalSettings() *GlobalSettings {
	return &GlobalSettings{
		CreditsPerCNY:          100,
		TenantCustomTopupFeeBp: 160,
		TenantWithdrawFeeBp:    160,
		TenantTopupPackages:    DefaultTopupPackages(),
	}
}

func DefaultTenantSettings(g *GlobalSettings) *TenantSettings {
	if g == nil {
		g = DefaultGlobalSettings()
	}
	return &TenantSettings{
		UserCreditsPerCNY:    g.CreditsPerCNY,
		UserCustomTopupFeeBp: g.TenantCustomTopupFeeBp,
		UserTopupPackages:    clonePackages(g.TenantTopupPackages),
	}
}

func (s *SettingsStore) LoadGlobal(ctx context.Context) (*GlobalSettings, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM sys_settings WHERE key = 'payment'`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultGlobalSettings(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("加载支付配置失败: %w", err)
	}
	var g GlobalSettings
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("解析支付配置失败: %w", err)
	}
	normalizeGlobalSettings(&g)
	if err := validateGlobalSettings(&g); err != nil {
		return nil, fmt.Errorf("支付配置不合法: %w", err)
	}
	return &g, nil
}

func (s *SettingsStore) SaveGlobal(ctx context.Context, g *GlobalSettings, operatorID string) error {
	normalizeGlobalSettings(g)
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO sys_settings (key, value, updated_by, updated_at)
		VALUES ('payment', $1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = $1, updated_by = $2, updated_at = now()
	`, raw, operatorID)
	return err
}

func (s *SettingsStore) LoadTenantSettings(ctx context.Context, tenantID string, g *GlobalSettings) (*TenantSettings, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM pay_tenant_settings WHERE tenant_id = $1`, tenantID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultTenantSettings(g), nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询租户充值设置失败: %w", err)
	}
	var ts TenantSettings
	if err := json.Unmarshal(raw, &ts); err != nil {
		return nil, fmt.Errorf("解析租户充值设置失败: %w", err)
	}
	normalizeTenantSettings(&ts, g)
	if err := validateTenantSettings(&ts); err != nil {
		return nil, fmt.Errorf("租户充值设置不合法: %w", err)
	}
	return &ts, nil
}

func (s *SettingsStore) SaveTenantSettings(ctx context.Context, tenantID string, ts *TenantSettings, g *GlobalSettings, operatorID string) error {
	normalizeTenantSettings(ts, g)
	raw, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO pay_tenant_settings (tenant_id, value, updated_by, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (tenant_id) DO UPDATE SET value = $2, updated_by = $3, updated_at = now()
	`, tenantID, raw, operatorID)
	return err
}

func ResolveTenantTopup(g *GlobalSettings) TopupParams {
	return TopupParams{
		CreditsPerCNY: g.CreditsPerCNY,
		FeeRateBp:     g.TenantCustomTopupFeeBp,
		Packages:      clonePackages(g.TenantTopupPackages),
		Min:           TopupMinAmountFen,
		Max:           TopupMaxAmountFen,
	}
}

func ResolveUserTopup(ts *TenantSettings) TopupParams {
	return TopupParams{
		CreditsPerCNY: ts.UserCreditsPerCNY,
		FeeRateBp:     ts.UserCustomTopupFeeBp,
		Packages:      clonePackages(ts.UserTopupPackages),
		Min:           TopupMinAmountFen,
		Max:           TopupMaxAmountFen,
	}
}

func normalizeGlobalSettings(g *GlobalSettings) {
	g.TenantTopupPackages = normalizePackages(g.TenantTopupPackages)
}

func normalizeTenantSettings(ts *TenantSettings, g *GlobalSettings) {
	def := DefaultTenantSettings(g)
	if ts.UserTopupPackages == nil {
		ts.UserTopupPackages = clonePackages(def.UserTopupPackages)
	}
	ts.UserTopupPackages = normalizePackages(ts.UserTopupPackages)
}

func normalizePackages(in []TopupPackage) []TopupPackage {
	out := make([]TopupPackage, 0, len(in))
	seen := map[string]struct{}{}
	for idx, p := range in {
		if len(out) >= MaxTopupPackages {
			break
		}
		p.ID = strings.TrimSpace(p.ID)
		seen[p.ID] = struct{}{}
		p.Name = strings.TrimSpace(p.Name)
		p.Badge = strings.TrimSpace(p.Badge)
		if p.SortOrder == 0 {
			p.SortOrder = (idx + 1) * 10
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].Amount < out[j].Amount
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out
}

func clonePackages(in []TopupPackage) []TopupPackage {
	out := make([]TopupPackage, len(in))
	copy(out, in)
	return out
}

func validateGlobalSettings(g *GlobalSettings) error {
	if g.CreditsPerCNY <= 0 || g.CreditsPerCNY > MaxCreditsPerCNY {
		return fmt.Errorf("1 元兑换积分必须在 1~%d 之间", MaxCreditsPerCNY)
	}
	if g.TenantCustomTopupFeeBp < 0 || g.TenantCustomTopupFeeBp > 10000 || g.TenantWithdrawFeeBp < 0 || g.TenantWithdrawFeeBp > 10000 {
		return fmt.Errorf("手续费必须在 0%%~100%% 之间")
	}
	return validatePackages(g.TenantTopupPackages)
}

func validateTenantSettings(ts *TenantSettings) error {
	if ts.UserCreditsPerCNY <= 0 || ts.UserCreditsPerCNY > MaxCreditsPerCNY {
		return fmt.Errorf("1 元兑换积分必须在 1~%d 之间", MaxCreditsPerCNY)
	}
	if ts.UserCustomTopupFeeBp < 0 || ts.UserCustomTopupFeeBp > 10000 {
		return fmt.Errorf("手续费必须在 0%%~100%% 之间")
	}
	return validatePackages(ts.UserTopupPackages)
}

func validatePackages(packages []TopupPackage) error {
	if len(packages) > MaxTopupPackages {
		return fmt.Errorf("快捷充值套餐最多 %d 个", MaxTopupPackages)
	}
	seen := map[string]struct{}{}
	for _, p := range packages {
		if p.ID == "" {
			return fmt.Errorf("套餐 ID 不能为空")
		}
		if _, ok := seen[p.ID]; ok {
			return fmt.Errorf("套餐 ID 不能重复")
		}
		seen[p.ID] = struct{}{}
		if p.Name == "" {
			return fmt.Errorf("套餐名称不能为空")
		}
		if p.Amount < TopupMinAmountFen || p.Amount > TopupMaxAmountFen {
			return fmt.Errorf("套餐金额必须在 10~10000 元之间")
		}
		if p.Credits <= 0 || p.Credits > MaxPackageCredits {
			return fmt.Errorf("套餐到账积分必须在 1~%d 之间", MaxPackageCredits)
		}
	}
	return nil
}
