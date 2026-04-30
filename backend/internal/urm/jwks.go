package urm

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims JWT claims — mirrors the URM JWT structure
type Claims struct {
	UserID          string `json:"user_id"`
	Username        string `json:"username"`
	TenantID        string `json:"tenant_id"`
	UserType        int    `json:"user_type"`
	UserTypeDisplay string `json:"user_type_display"`
	AppKey          string `json:"app_key"`
	TokenType       string `json:"token_type"`
	jwt.RegisteredClaims
}

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// JWKSValidator fetches URM JWKS and validates JWT tokens locally
type JWKSValidator struct {
	jwksURL         string
	httpClient      *http.Client
	refreshInterval time.Duration
	issuer          string

	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	lastUpdate time.Time
}

func NewJWKSValidator(urmBaseURL string, refreshInterval time.Duration, timeout time.Duration) *JWKSValidator {
	if refreshInterval <= 0 {
		refreshInterval = 24 * time.Hour
	}
	return &JWKSValidator{
		jwksURL:         strings.TrimRight(urmBaseURL, "/") + "/public/jwks.json",
		refreshInterval: refreshInterval,
		httpClient:      &http.Client{Timeout: timeout},
		issuer:          "urm",
		keys:            make(map[string]*rsa.PublicKey),
	}
}

// Start pre-loads keys; returns error if initial fetch fails
func (v *JWKSValidator) Start(ctx context.Context) error {
	return v.refresh(ctx)
}

func (v *JWKSValidator) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("jwks build request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jwks read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("jwks parse: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		newKeys[k.Kid] = pub
	}

	v.mu.Lock()
	v.keys = newKeys
	v.lastUpdate = time.Now()
	v.mu.Unlock()

	return nil
}

func (v *JWKSValidator) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	stale := time.Since(v.lastUpdate) > v.refreshInterval
	v.mu.RUnlock()

	if ok && !stale {
		return key, nil
	}

	// Try to refresh (unknown kid or stale)
	if err := v.refresh(ctx); err != nil {
		if ok {
			return key, nil // stale but usable
		}
		return nil, fmt.Errorf("jwks refresh failed and kid %q not in cache: %w", kid, err)
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown kid: %s", kid)
	}
	return key, nil
}

// ValidateToken parses and validates a JWT token string, returning the Claims.
func (v *JWKSValidator) ValidateToken(ctx context.Context, tokenStr string) (*Claims, error) {
	// Parse without verification first to extract kid
	unverified, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, fmt.Errorf("missing kid in token header")
	}

	pubKey, err := v.getKey(ctx, kid)
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	}, jwt.WithIssuer(v.issuer))
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	if claims.TokenType != "access" {
		return nil, fmt.Errorf("not an access token")
	}

	return claims, nil
}

func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode E: %w", err)
	}

	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, fmt.Errorf("E too large")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e.Int64()),
	}, nil
}
