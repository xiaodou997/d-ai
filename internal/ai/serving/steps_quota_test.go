package serving

import (
	"context"
	"errors"
	"testing"

	coreidentity "xiaodou/dai/internal/ai/core/identity"
)

func TestQuotaCheckUsesPositiveRemainingOnly(t *testing.T) {
	limit := int64(100)
	for _, tc := range []struct {
		name    string
		used    int64
		wantErr bool
	}{
		{name: "positive", used: 99},
		{name: "zero", used: 100, wantErr: true},
		{name: "negative", used: 101, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &Request{Subject: &coreidentity.Subject{
				AuthMethod: coreidentity.AuthMethodAPIKey,
				APIKeyID:   "key",
				QuotaLimit: &limit,
				QuotaUsed:  tc.used,
			}}
			err := (&QuotaCheckStep{}).Execute(context.Background(), req)
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Code != "quota_exceeded" {
					t.Fatalf("error = %#v, want quota_exceeded", err)
				}
			}
		})
	}
}

func TestQuotaCheckUnlimited(t *testing.T) {
	req := &Request{Subject: &coreidentity.Subject{
		AuthMethod: coreidentity.AuthMethodAPIKey,
		APIKeyID:   "key",
		QuotaUsed:  1 << 62,
	}}
	if err := (&QuotaCheckStep{}).Execute(context.Background(), req); err != nil {
		t.Fatalf("unlimited quota should pass: %v", err)
	}
}
