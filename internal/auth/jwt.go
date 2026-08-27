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

	"xiaodou/dai/internal/clientsecret"
	"xiaodou/dai/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Claims JWT Token 声明
type Claims struct {
	PrincipalType     string `json:"principal_type"`
	TokenUse          string `json:"token_use"`
	SessionID         string `json:"sid,omitempty"`
	CredentialVersion int64  `json:"credential_version,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	Username          string `json:"username,omitempty"`
	TenantID          string `json:"tenant_id,omitempty"`
	UserType          int    `json:"user_type,omitempty"`
	UserTypeDisplay   string `json:"user_type_display,omitempty"`
	jwt.RegisteredClaims
}

// Principal is the account snapshot bound to a login session.
type Principal struct {
	UserID            string
	Username          string
	TenantID          string
	UserType          int
	UserTypeDisplay   string
	CredentialVersion int64
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
	database              *pgxpool.Pool
	cfg                   config.JWTConfig
	mu                    sync.RWMutex
	activeKey             *keyEntry
	graceKeys             []*keyEntry
	accessTokenExpiration time.Duration
	issuer                string
}

const accessSessionValidationTimeout = 2 * time.Second

// NewJWTService 创建 JWT 服务
// 从数据库加载密钥，若无则自动生成并写入数据库
func NewJWTService(cfg config.JWTConfig, database *pgxpool.Pool) *JWTService {
	s := &JWTService{
		database:              database,
		cfg:                   cfg,
		accessTokenExpiration: cfg.Expiration,
		issuer:                cfg.Issuer,
	}
	if s.accessTokenExpiration == 0 {
		s.accessTokenExpiration = 15 * time.Minute
	}

	if err := s.reloadKeys(context.Background()); err != nil {
		panic("JWTService: failed to load keys from DB: " + err.Error())
	}

	// 若无 active key，自动生成并写入 DB
	if s.activeKey == nil {
		if err := s.generateAndSaveKey(); err != nil {
			panic("JWTService: failed to generate initial key: " + err.Error())
		}
		if err := s.reloadKeys(context.Background()); err != nil {
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
	privateKeyCiphertext, err := clientsecret.Encrypt(privateKeyPEM)
	if err != nil {
		return fmt.Errorf("encrypt JWT private key: %w", err)
	}
	publicKeyPEM, err := marshalPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	ctx := context.Background()
	_, err = s.database.Exec(ctx, `
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status, created_at)
		VALUES ($1, $2, $3, 'active', $4)
	`, kid, privateKeyCiphertext, publicKeyPEM, now)
	return err
}

// reloadKeys 从数据库重新加载 active 和 grace 密钥到内存
// DB 查询在锁外完成，内存 swap 在写锁内完成，避免 IO 阻塞验签并发
func (s *JWTService) reloadKeys(ctx context.Context) error {
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
			return fmt.Errorf("scan auth signing key %q: %w", kid, err)
		}
		privateKeyPEM, decryptErr := clientsecret.Decrypt(privPEM)
		legacyPlaintext := false
		if decryptErr != nil {
			// Pre-P0-07 installations stored PEM directly. Accept it only for
			// this migration path and immediately replace it with ciphertext.
			if _, parseErr := parsePrivateKey(privPEM); parseErr != nil {
				return fmt.Errorf("decrypt JWT private key %q: %w", kid, decryptErr)
			}
			privateKeyPEM = privPEM
			legacyPlaintext = true
		}
		privateKey, err := parsePrivateKey(privateKeyPEM)
		if err != nil {
			return fmt.Errorf("parse JWT private key %q: %w", kid, err)
		}
		if legacyPlaintext {
			ciphertext, err := clientsecret.Encrypt(privateKeyPEM)
			if err != nil {
				return fmt.Errorf("encrypt migrated JWT private key %q: %w", kid, err)
			}
			if _, err := s.database.Exec(ctx, `
				UPDATE auth_signing_keys SET private_key = $1 WHERE kid = $2
			`, ciphertext, kid); err != nil {
				return fmt.Errorf("persist migrated JWT private key %q: %w", kid, err)
			}
		}
		publicKey, err := parsePublicKey(pubPEM)
		if err != nil {
			return fmt.Errorf("parse JWT public key %q: %w", kid, err)
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

// GenerateAccessToken creates a session-bound access token.
func (s *JWTService) GenerateAccessToken(principal Principal, sessionID string) (string, error) {
	s.mu.RLock()
	activeKey := s.activeKey
	s.mu.RUnlock()

	if activeKey == nil {
		return "", fmt.Errorf("no active signing key")
	}

	now := time.Now()
	claims := Claims{
		PrincipalType:     "user",
		TokenUse:          "access",
		SessionID:         sessionID,
		CredentialVersion: principal.CredentialVersion,
		UserID:            principal.UserID,
		Username:          principal.Username,
		TenantID:          principal.TenantID,
		UserType:          principal.UserType,
		UserTypeDisplay:   principal.UserTypeDisplay,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenExpiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			ID:        uuid.New().String(),
			Subject:   principal.UserID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = activeKey.kid
	return token.SignedString(activeKey.privateKey)
}

func (s *JWTService) AccessTokenExpiration() time.Duration { return s.accessTokenExpiration }

// ParseToken 解析并验证 Token，根据 kid 选择公钥；访问令牌的 session
// 校验沿用调用方 context，以便 HTTP 请求取消时及时停止数据库查询。
// 新系统不兼容无 kid 的 token
func (s *JWTService) ParseToken(ctx context.Context, tokenString string) (*Claims, error) {
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
		if claims.PrincipalType == "user" && claims.TokenUse == "access" {
			if err := s.validateAccessSession(ctx, claims); err != nil {
				return nil, err
			}
		}
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

func (s *JWTService) validateAccessSession(ctx context.Context, claims *Claims) error {
	if claims.SessionID == "" || claims.CredentialVersion <= 0 {
		return ErrSessionInactive
	}
	ctx, cancel := context.WithTimeout(ctx, accessSessionValidationTimeout)
	defer cancel()
	var valid bool
	err := s.database.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM auth_sessions s
			JOIN iam_accounts a ON a.user_id = s.user_id
			LEFT JOIN iam_tenants t ON t.tenant_id = a.tenant_id
			WHERE s.session_id = $1
			  AND s.user_id = $2
			  AND s.status = 'active'
			  AND s.expires_at > now()
			  AND s.credential_version = $3
			  AND a.credential_version = $3
			  AND a.user_type = $4
			  AND COALESCE(a.tenant_id, '') = $5
			  AND a.status = 'active'
			  AND a.credential_state = 'active'
			  AND (a.user_type < 3 OR t.status = 'active')
		)
	`, claims.SessionID, claims.UserID, claims.CredentialVersion, claims.UserType, claims.TenantID).Scan(&valid)
	if err != nil {
		return fmt.Errorf("validate access session: %w", err)
	}
	if !valid {
		return ErrSessionInactive
	}
	return nil
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
	privateKeyCiphertext, err := clientsecret.Encrypt(privateKeyPEM)
	if err != nil {
		return fmt.Errorf("encrypt JWT private key: %w", err)
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
		INSERT INTO auth_signing_keys (kid, private_key, public_key, status, created_at)
		VALUES ($1, $2, $3, 'active', $4)
	`, newKid, privateKeyCiphertext, publicKeyPEM, now)
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

	return s.reloadKeys(context.Background())
}

// RetireExpiredGraceKeys 退役宽限期已过的密钥（由调度器定期调用）
func (s *JWTService) RetireExpiredGraceKeys(ctx context.Context) error {
	now := time.Now().UTC()
	_, err := s.database.Exec(ctx, `
		UPDATE auth_signing_keys
		SET status = 'retired', retired_at = $1
		WHERE status = 'grace' AND grace_until IS NOT NULL AND grace_until < $2
	`, now, now)
	if err != nil {
		return err
	}
	// Every replica reloads, including replicas whose UPDATE matched zero rows.
	// The first replica retires the row in PostgreSQL; the others still need to
	// evict that key from their local JWKS cache instead of accepting it forever.
	return s.reloadKeys(ctx)
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
