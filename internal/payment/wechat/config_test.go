package wechat

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestValidateConfigAllowsMockWithMissingMerchantFields(t *testing.T) {
	cfg := &MerchantConfig{
		Enabled:    true,
		Mock:       true,
		VerifyMode: VerifyModePublicKey,
		OrderTTL:   5 * time.Minute,
	}

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig returned error: %v", err)
	}
}

func TestValidateConfigRejectsInvalidRealConfig(t *testing.T) {
	privateKeyPEM, publicKeyPEM := testKeyPairPEM(t)

	tests := []struct {
		name string
		cfg  *MerchantConfig
		want string
	}{
		{
			name: "ttl too short",
			cfg: &MerchantConfig{
				Enabled: true, Mock: false, VerifyMode: VerifyModePlatformCert,
				AppID: "app", MchID: "mch", MchCertSerialNo: "serial", MchPrivateKey: privateKeyPEM,
				APIv3Key: strings.Repeat("a", apiV3KeyLength), NotifyBaseURL: "https://pay.example.com", OrderTTL: time.Minute,
			},
			want: "300~86400",
		},
		{
			name: "api v3 key length",
			cfg: &MerchantConfig{
				Enabled: true, Mock: false, VerifyMode: VerifyModePlatformCert,
				AppID: "app", MchID: "mch", MchCertSerialNo: "serial", MchPrivateKey: privateKeyPEM,
				APIv3Key: "short", NotifyBaseURL: "https://pay.example.com", OrderTTL: 5 * time.Minute,
			},
			want: "APIv3Key",
		},
		{
			name: "notify url must be https",
			cfg: &MerchantConfig{
				Enabled: true, Mock: false, VerifyMode: VerifyModePlatformCert,
				AppID: "app", MchID: "mch", MchCertSerialNo: "serial", MchPrivateKey: privateKeyPEM,
				APIv3Key: strings.Repeat("a", apiV3KeyLength), NotifyBaseURL: "http://pay.example.com", OrderTTL: 5 * time.Minute,
			},
			want: "HTTPS",
		},
		{
			name: "public key mode requires public key",
			cfg: &MerchantConfig{
				Enabled: true, Mock: false, VerifyMode: VerifyModePublicKey,
				AppID: "app", MchID: "mch", MchCertSerialNo: "serial", MchPrivateKey: privateKeyPEM,
				APIv3Key: strings.Repeat("a", apiV3KeyLength), NotifyBaseURL: "https://pay.example.com", OrderTTL: 5 * time.Minute,
			},
			want: "公钥",
		},
		{
			name: "public key mode rejects invalid public key",
			cfg: &MerchantConfig{
				Enabled: true, Mock: false, VerifyMode: VerifyModePublicKey,
				AppID: "app", MchID: "mch", MchCertSerialNo: "serial", MchPrivateKey: privateKeyPEM,
				APIv3Key: strings.Repeat("a", apiV3KeyLength), NotifyBaseURL: "https://pay.example.com", OrderTTL: 5 * time.Minute,
				WechatPublicKeyID: "PUB_KEY_ID", WechatPublicKey: "not a key",
			},
			want: "公钥无法解析",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if err == nil {
				t.Fatal("validateConfig returned nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.want)
			}
		})
	}

	t.Run("valid public key mode", func(t *testing.T) {
		cfg := &MerchantConfig{
			Enabled: true, Mock: false, VerifyMode: VerifyModePublicKey,
			AppID: "app", MchID: "mch", MchCertSerialNo: "serial", MchPrivateKey: privateKeyPEM,
			APIv3Key: strings.Repeat("a", apiV3KeyLength), NotifyBaseURL: "https://pay.example.com", OrderTTL: 5 * time.Minute,
			WechatPublicKeyID: "PUB_KEY_ID", WechatPublicKey: publicKeyPEM,
		}
		if err := validateConfig(cfg); err != nil {
			t.Fatalf("validateConfig returned error: %v", err)
		}
	})
}

func TestShouldBlockCredentialChange(t *testing.T) {
	current := &MerchantConfig{Mock: false, VerifyMode: VerifyModePlatformCert, AppID: "app", MchID: "mch"}
	next := *current
	next.NotifyBaseURL = "https://new.example.com"
	if shouldBlockCredentialChange(current, &next) {
		t.Fatal("notify URL changes should not block while orders are open")
	}

	next = *current
	next.MchID = "new-mch"
	if !shouldBlockCredentialChange(current, &next) {
		t.Fatal("merchant identity changes should block while orders are open")
	}

	mockNext := next
	mockCurrent := *current
	mockCurrent.Mock = true
	mockNext.Mock = true
	if shouldBlockCredentialChange(&mockCurrent, &mockNext) {
		t.Fatal("mock-only credential changes should not block")
	}
}

func testKeyPairPEM(t *testing.T) (privateKeyPEM, publicKeyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	privateBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicBlock := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return string(privateBlock), string(publicBlock)
}
