package urm

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	sharedjwks "xiaodou/unihub/shared/jwks"
)

// Claims JWT claims — mirrors the URM JWT structure (both user and service tokens)
type Claims struct {
	PrincipalType   string `json:"principal_type"`
	TokenUse        string `json:"token_use"`
	UserID          string `json:"user_id"`
	Username        string `json:"username"`
	TenantID        string `json:"tenant_id"`
	UserType        int    `json:"user_type"`
	UserTypeDisplay string `json:"user_type_display"`
	ClientID        string `json:"client_id"`
	Scope           string `json:"scope"`
	jwt.RegisteredClaims
}

// JWKSValidator fetches URM JWKS and validates JWT tokens locally
type JWKSValidator struct {
	jwksURL    string
	httpClient *http.Client
	issuer     string

	mu         sync.RWMutex
	keys       map[string]*rsa.PublicKey
	lastUpdate time.Time
}

const jwksRefreshInterval = 24 * time.Hour

func NewJWKSValidator(urmBaseURL string, timeout time.Duration) *JWKSValidator {
	return &JWKSValidator{
		jwksURL:    strings.TrimRight(urmBaseURL, "/") + "/public/jwks.json",
		httpClient: &http.Client{Timeout: timeout},
		issuer:     "urm",
		keys:       make(map[string]*rsa.PublicKey),
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

	var jwks sharedjwks.Set
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("jwks parse: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := sharedjwks.ParseRSAPublicKey(k)
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
	stale := time.Since(v.lastUpdate) > jwksRefreshInterval
	v.mu.RUnlock()

	if ok && !stale {
		return key, nil
	}

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
	if claims.TokenUse != "access" {
		return nil, fmt.Errorf("not an access token")
	}

	return claims, nil
}
