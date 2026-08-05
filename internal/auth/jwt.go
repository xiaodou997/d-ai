package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"time"

	"xiaodou/dai/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ActClaim 是 RFC 8693 的 `act`（actor）声明：委托 Token 中记录实际发起调用的服务身份，
// 便于审计「谁代表主体执行」（OBO Subject 是被代表方，act.sub 是代理方）。
type ActClaim struct {
	Sub string `json:"sub,omitempty"`
}

// Claims JWT Token 声明
type Claims struct {
	PrincipalType   string `json:"principal_type"`
	TokenUse        string `json:"token_use"`
	ClientID        string `json:"client_id,omitempty"`
	ClientType      string `json:"client_type,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	Username        string `json:"username,omitempty"`
	TenantID        string `json:"tenant_id,omitempty"`
	UserType        int    `json:"user_type,omitempty"`
	UserTypeDisplay string `json:"user_type_display,omitempty"`
	Scope           string `json:"scope,omitempty"`
	InstanceID      string `json:"instance_id,omitempty"`
	SourceCIDR      string `json:"source_cidr,omitempty"`
	// BillingScope / Act 仅出现在委托（delegated）Token 中，见 GenerateDelegatedToken。
	BillingScope string    `json:"billing_scope,omitempty"`
	Act          *ActClaim `json:"act,omitempty"`
	jwt.RegisteredClaims
}

// TokenPair Token 对（Access Token + Refresh Token）
type TokenPair struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresIn        int64  `json:"expiresIn"`
	RefreshExpiresIn int64  `json:"refreshExpiresIn"`
}

// keyEntry 内存中的密钥条目
type keyEntry struct {
	kid        string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
}

// JWKSKey RFC 7517 JWK 格式的单个 RSA 公钥
type JWKSKey struct {
	Kty string `json:"kty"` // "RSA"
	Use string `json:"use"` // "sig"
	Alg string `json:"alg"` // "RS256"
	Kid string `json:"kid"`
	N   string `json:"n"` // base64.RawURLEncoding
	E   string `json:"e"` // base64.RawURLEncoding
}

// JWKSResponse JWKS 响应体（符合 RFC 7517，第三方库直接消费）
type JWKSResponse struct {
	Keys []JWKSKey `json:"keys"`
}

// KeyInfo 密钥信息（管理员查看用）
type KeyInfo struct {
	ID          int64  `json:"id"`
	Kid         string `json:"kid"`
	Status      string `json:"status"`
	CreatedTime int64  `json:"createdTime"`
	GraceUntil  *int64 `json:"graceUntil,omitempty"`
	RetiredTime *int64 `json:"retiredTime,omitempty"`
}

// JWTService JWT 服务（RS256 + JWKS + 密钥轮换）
type JWTService struct {
	database               *pgxpool.Pool
	cfg                    config.JWTConfig
	mu                     sync.RWMutex
	activeKey              *keyEntry
	graceKeys              []*keyEntry
	accessTokenExpiration  time.Duration
	refreshTokenExpiration time.Duration
	issuer                 string
}

// NewJWTService 创建 JWT 服务
// 从数据库加载密钥，若无则自动生成并写入数据库
func NewJWTService(cfg config.JWTConfig, database *pgxpool.Pool) *JWTService {
	refreshExpiration := cfg.RefreshExpiration
	if refreshExpiration == 0 {
		refreshExpiration = 7 * 24 * time.Hour
	}

	s := &JWTService{
		database:               database,
		cfg:                    cfg,
		accessTokenExpiration:  cfg.Expiration,
		refreshTokenExpiration: refreshExpiration,
		issuer:                 cfg.Issuer,
	}

	if err := s.reloadKeys(); err != nil {
		panic("JWTService: failed to load keys from DB: " + err.Error())
	}

	// 若无 active key，自动生成并写入 DB
	if s.activeKey == nil {
		if err := s.generateAndSaveKey(); err != nil {
			panic("JWTService: failed to generate initial key: " + err.Error())
		}
		if err := s.reloadKeys(); err != nil {
			panic("JWTService: failed to reload keys after generation: " + err.Error())
		}
	}

	if s.activeKey == nil {
		panic("JWTService: no active key available")
	}

	return s
}

// generateAndSaveKey 生成新 RSA-2048 密钥对并写入数据库（不更新内存，由 reloadKeys 完成）
func (s *JWTService) generateAndSaveKey() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate RSA key: %w", err)
	}

	kid := fmt.Sprintf("key-%s-%s", time.Now().Format("20060102"), uuid.New().String()[:8])

	privateKeyPEM, err := marshalPrivateKey(privateKey)
	if err != nil {
		return err
	}
	publicKeyPEM, err := marshalPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	ctx := context.Background()
	_, err = s.database.Exec(ctx, `
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status, key_use, created_at)
		VALUES ($1, $2, $3, 'active', 'shared', $4)
	`, kid, privateKeyPEM, publicKeyPEM, now)
	return err
}

// reloadKeys 从数据库重新加载 active 和 grace 密钥到内存
// DB 查询在锁外完成，内存 swap 在写锁内完成，避免 IO 阻塞验签并发
func (s *JWTService) reloadKeys() error {
	ctx := context.Background()
	rows, err := s.database.Query(ctx, `
		SELECT kid, private_key, public_key, status
		FROM auth_signing_keys
		WHERE status IN ('active', 'grace')
		ORDER BY created_at DESC
	`)
	if err != nil {
		return fmt.Errorf("query auth_signing_keys: %w", err)
	}
	defer rows.Close()

	var newActive *keyEntry
	var newGrace []*keyEntry

	for rows.Next() {
		var kid, privPEM, pubPEM, status string
		if err := rows.Scan(&kid, &privPEM, &pubPEM, &status); err != nil {
			continue
		}
		privateKey, err := parsePrivateKey(privPEM)
		if err != nil {
			continue
		}
		publicKey, err := parsePublicKey(pubPEM)
		if err != nil {
			continue
		}
		entry := &keyEntry{kid: kid, privateKey: privateKey, publicKey: publicKey}
		if status == "active" {
			newActive = entry
		} else {
			newGrace = append(newGrace, entry)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	s.activeKey = newActive
	s.graceKeys = newGrace
	s.mu.Unlock()

	return nil
}

// GenerateToken 生成单一 Access Token
func (s *JWTService) GenerateToken(userID, username, tenantID string, userType int, userTypeDisplay, clientID, clientType string) (string, error) {
	pair, err := s.GenerateTokenPair(userID, username, tenantID, userType, userTypeDisplay, clientID, clientType)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

// GenerateTokenPair 生成 Token 对，token header 携带 kid
func (s *JWTService) GenerateTokenPair(userID, username, tenantID string, userType int, userTypeDisplay, clientID, clientType string) (*TokenPair, error) {
	s.mu.RLock()
	activeKey := s.activeKey
	s.mu.RUnlock()

	if activeKey == nil {
		return nil, fmt.Errorf("no active signing key")
	}

	now := time.Now()

	makeToken := func(tokenType string, expiration time.Duration) (string, error) {
		claims := Claims{
			PrincipalType:   "user",
			TokenUse:        tokenType,
			ClientID:        clientID,
			ClientType:      clientType,
			UserID:          userID,
			Username:        username,
			TenantID:        tenantID,
			UserType:        userType,
			UserTypeDisplay: userTypeDisplay,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(now.Add(expiration)),
				IssuedAt:  jwt.NewNumericDate(now),
				Issuer:    s.issuer,
				ID:        uuid.New().String(),
				Subject:   userID,
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = activeKey.kid
		return token.SignedString(activeKey.privateKey)
	}

	accessTokenStr, err := makeToken("access", s.accessTokenExpiration)
	if err != nil {
		return nil, err
	}
	refreshTokenStr, err := makeToken("refresh", s.refreshTokenExpiration)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:      accessTokenStr,
		RefreshToken:     refreshTokenStr,
		ExpiresIn:        int64(s.accessTokenExpiration.Seconds()),
		RefreshExpiresIn: int64(s.refreshTokenExpiration.Seconds()),
	}, nil
}

func (s *JWTService) GenerateServiceAccessToken(clientID, instanceID, sourceCIDR string, expiration time.Duration) (string, int64, error) {
	s.mu.RLock()
	activeKey := s.activeKey
	s.mu.RUnlock()

	if activeKey == nil {
		return "", 0, fmt.Errorf("no active signing key")
	}
	if expiration <= 0 {
		expiration = 5 * time.Minute
	}

	now := time.Now()
	claims := Claims{
		PrincipalType: "service",
		TokenUse:      "access",
		ClientID:      clientID,
		InstanceID:    instanceID,
		SourceCIDR:    sourceCIDR,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			ID:        uuid.New().String(),
			Subject:   clientID,
			Audience:  jwt.ClaimStrings{"unihub-services"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = activeKey.kid
	tokenStr, err := token.SignedString(activeKey.privateKey)
	if err != nil {
		return "", 0, err
	}
	return tokenStr, int64(expiration.Seconds()), nil
}

// DelegationRequest 描述一次 OBO 委托签发（服务代表某主体调用下游服务）。
type DelegationRequest struct {
	// ClientID 发起委托的服务身份（写入 client_id 与 act.sub）。
	ClientID string
	// InstanceID 发起委托的服务副本标识。
	InstanceID string
	// TenantID / UserID 被代表的计费/权限主体。UserID 仅在 BillingScope=user 时有值。
	TenantID string
	UserID   string
	// BillingScope 计费归属：user | tenant。
	BillingScope string
	// Audience 允许消费该 Token 的下游服务（如 ["ai-service"]）。
	Audience []string
	// Scope 授予的能力（如 "ai.invoke"）。
	Scope string
	// Expiration 有效期（2~5 分钟）。
	Expiration time.Duration
}

// GenerateDelegatedToken 签发短期 OBO 委托 Token（§8.3）。服务身份用于认证与审计，
// OBO Subject（tenant/user + billing_scope）用于下游的权限、配额与计费。不入库、无 Refresh。
func (s *JWTService) GenerateDelegatedToken(req DelegationRequest) (string, int64, error) {
	s.mu.RLock()
	activeKey := s.activeKey
	s.mu.RUnlock()

	if activeKey == nil {
		return "", 0, fmt.Errorf("no active signing key")
	}
	if req.Expiration <= 0 || req.Expiration > 5*time.Minute {
		req.Expiration = 3 * time.Minute
	}

	// Subject 取被代表主体：user 作用域用 userID，tenant 作用域用 tenantID。
	subject := req.TenantID
	if req.BillingScope == "user" && req.UserID != "" {
		subject = req.UserID
	}

	now := time.Now()
	claims := Claims{
		PrincipalType: "delegated",
		TokenUse:      "access",
		ClientID:      req.ClientID,
		InstanceID:    req.InstanceID,
		TenantID:      req.TenantID,
		UserID:        req.UserID,
		BillingScope:  req.BillingScope,
		Scope:         req.Scope,
		Act:           &ActClaim{Sub: req.ClientID},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(req.Expiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			ID:        uuid.New().String(),
			Subject:   subject,
			Audience:  jwt.ClaimStrings(req.Audience),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = activeKey.kid
	tokenStr, err := token.SignedString(activeKey.privateKey)
	if err != nil {
		return "", 0, err
	}
	return tokenStr, int64(req.Expiration.Seconds()), nil
}

// ParseToken 解析并验证 Token，根据 kid 选择公钥
// 新系统不兼容无 kid 的 token
func (s *JWTService) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kidVal, ok := token.Header["kid"]
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}
		kid, ok := kidVal.(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("invalid kid in token header")
		}

		s.mu.RLock()
		activeKey := s.activeKey
		graceKeys := s.graceKeys
		s.mu.RUnlock()

		if activeKey != nil && activeKey.kid == kid {
			return activeKey.publicKey, nil
		}
		for _, gk := range graceKeys {
			if gk.kid == kid {
				return gk.publicKey, nil
			}
		}

		return nil, fmt.Errorf("unknown kid: %s", kid)
	}, jwt.WithIssuer(s.issuer), jwt.WithValidMethods([]string{"RS256"}))

	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

// RefreshToken 刷新 Token（仅 Access Token）
func (s *JWTService) RefreshToken(tokenString string) (string, error) {
	claims, err := s.ParseToken(tokenString)
	if err != nil {
		return "", err
	}
	return s.GenerateToken(claims.UserID, claims.Username, claims.TenantID, claims.UserType, claims.UserTypeDisplay, claims.ClientID, claims.ClientType)
}

// RefreshTokenPair 使用 Refresh Token 获取新的 Token 对
func (s *JWTService) RefreshTokenPair(refreshTokenString string, rotateRefreshToken bool) (*TokenPair, error) {
	claims, err := s.ParseToken(refreshTokenString)
	if err != nil {
		return nil, err
	}
	if claims.PrincipalType != "user" || claims.TokenUse != "refresh" {
		return nil, ErrInvalidTokenType
	}

	s.mu.RLock()
	activeKey := s.activeKey
	s.mu.RUnlock()

	now := time.Now()

	makeToken := func(tokenType string, expiration time.Duration) (string, error) {
		newClaims := Claims{
			PrincipalType:   "user",
			TokenUse:        tokenType,
			ClientID:        claims.ClientID,
			ClientType:      claims.ClientType,
			UserID:          claims.UserID,
			Username:        claims.Username,
			TenantID:        claims.TenantID,
			UserType:        claims.UserType,
			UserTypeDisplay: claims.UserTypeDisplay,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(now.Add(expiration)),
				IssuedAt:  jwt.NewNumericDate(now),
				Issuer:    s.issuer,
				ID:        uuid.New().String(),
				Subject:   claims.UserID,
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, newClaims)
		token.Header["kid"] = activeKey.kid
		return token.SignedString(activeKey.privateKey)
	}

	accessTokenStr, err := makeToken("access", s.accessTokenExpiration)
	if err != nil {
		return nil, err
	}

	var newRefreshToken string
	if rotateRefreshToken {
		newRefreshToken, err = makeToken("refresh", s.refreshTokenExpiration)
		if err != nil {
			return nil, err
		}
	}

	return &TokenPair{
		AccessToken:      accessTokenStr,
		RefreshToken:     newRefreshToken,
		ExpiresIn:        int64(s.accessTokenExpiration.Seconds()),
		RefreshExpiresIn: int64(s.refreshTokenExpiration.Seconds()),
	}, nil
}

// RotateKey 密钥轮换：生成新密钥，旧 active 密钥进入 24 小时宽限期
// 整个过程在一个事务中完成
func (s *JWTService) RotateKey() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate RSA key: %w", err)
	}

	newKid := fmt.Sprintf("key-%s-%s", time.Now().Format("20060102"), uuid.New().String()[:8])
	privateKeyPEM, err := marshalPrivateKey(privateKey)
	if err != nil {
		return err
	}
	publicKeyPEM, err := marshalPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	graceUntil := now.Add(24 * time.Hour)

	ctx := context.Background()
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 插入新 active 密钥
	_, err = tx.Exec(ctx, `
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status, key_use, created_at)
		VALUES ($1, $2, $3, 'active', $4, $5)
	`, newKid, privateKeyPEM, publicKeyPEM, "shared", now)
	if err != nil {
		return fmt.Errorf("insert new key: %w", err)
	}

	// 将旧 active 密钥降级为 grace
	_, err = tx.Exec(ctx, `
		UPDATE auth_signing_keys
		SET status = 'grace', grace_until = $1
		WHERE status = 'active' AND kid != $2
	`, graceUntil, newKid)
	if err != nil {
		return fmt.Errorf("demote old key to grace: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rotation: %w", err)
	}

	return s.reloadKeys()
}

// RetireExpiredGraceKeys 退役宽限期已过的密钥（由调度器定期调用）
func (s *JWTService) RetireExpiredGraceKeys() error {
	now := time.Now().UTC()
	ctx := context.Background()
	result, err := s.database.Exec(ctx, `
		UPDATE auth_signing_keys
		SET status = 'retired', retired_at = $1
		WHERE status = 'grace' AND grace_until IS NOT NULL AND grace_until < $2
	`, now, now)
	if err != nil {
		return err
	}
	affected := result.RowsAffected()
	if affected > 0 {
		return s.reloadKeys()
	}
	return nil
}

// GetJWKS 返回当前所有可用公钥的 JWKS 格式（active + grace）
func (s *JWTService) GetJWKS() *JWKSResponse {
	s.mu.RLock()
	activeKey := s.activeKey
	graceKeys := s.graceKeys
	s.mu.RUnlock()

	var keys []JWKSKey
	if activeKey != nil {
		keys = append(keys, toJWKSKey(activeKey))
	}
	for _, gk := range graceKeys {
		keys = append(keys, toJWKSKey(gk))
	}
	if keys == nil {
		keys = []JWKSKey{}
	}
	return &JWKSResponse{Keys: keys}
}

// ListKeys 列出所有密钥信息（管理员用）
func (s *JWTService) ListKeys() ([]KeyInfo, error) {
	ctx := context.Background()
	rows, err := s.database.Query(ctx, `
		SELECT id, kid, status, created_at, grace_until, retired_at
		FROM auth_signing_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []KeyInfo
	for rows.Next() {
		var info KeyInfo
		var createdAt time.Time
		var graceUntil *time.Time
		var retiredAt *time.Time
		if err := rows.Scan(&info.ID, &info.Kid, &info.Status, &createdAt, &graceUntil, &retiredAt); err != nil {
			continue
		}
		info.CreatedTime = createdAt.UnixMilli()
		if graceUntil != nil {
			value := graceUntil.UnixMilli()
			info.GraceUntil = &value
		}
		if retiredAt != nil {
			value := retiredAt.UnixMilli()
			info.RetiredTime = &value
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

// GetPublicKeyPEM 返回当前 active 密钥的 PEM 公钥字符串
func (s *JWTService) GetPublicKeyPEM() (string, error) {
	s.mu.RLock()
	activeKey := s.activeKey
	s.mu.RUnlock()
	if activeKey == nil {
		return "", fmt.Errorf("no active key")
	}
	return marshalPublicKey(activeKey.publicKey)
}

// ErrInvalidTokenType Token 类型错误
var ErrInvalidTokenType = fmt.Errorf("invalid token type")

// ==================== 内部工具函数 ====================

func toJWKSKey(e *keyEntry) JWKSKey {
	pub := e.publicKey
	return JWKSKey{
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		Kid: e.kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func marshalPrivateKey(key *rsa.PrivateKey) (string, error) {
	data, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data})), nil
}

func marshalPublicKey(key *rsa.PublicKey) (string, error) {
	data, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: data})), nil
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}
	return rsaKey, nil
}

func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaKey, nil
}
