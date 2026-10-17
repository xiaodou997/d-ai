package clientruntime

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"xiaodou/dai/internal/ai/domain"
)

func TestNativeQuotaInspectionContracts(t *testing.T) {
	for _, tc := range []struct {
		provider           domain.FixedProviderType
		path, method, body string
	}{
		{domain.FixedProviderCodex, "chatgpt.com/backend-api/wham/usage", "GET", `{"rate_limit":{"primary_window":{"used_percent":42}}}`},
		{domain.FixedProviderClaudeOAuth, "api.anthropic.com/api/oauth/usage", "GET", `{"five_hour":{"utilization":42}}`},
		{domain.FixedProviderGeminiCLI, "cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota", "POST", `{"buckets":[{"modelId":"gemini-3-pro","remainingFraction":0.5}]}`},
		{domain.FixedProviderAntigravity, "cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels", "POST", `{"models":{"gemini-3-pro":{"quotaInfo":{"remainingFraction":0.5}}}}`},
	} {
		t.Run(string(tc.provider), func(t *testing.T) {
			r := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
				if req.URL != "https://"+tc.path || req.Method != tc.method || req.Headers["Authorization"] != "Bearer access" {
					t.Fatalf("request contract URL=%s method=%s", req.URL, req.Method)
				}
				if tc.method == "POST" && !strings.Contains(string(req.Body), `"project":"test-project"`) {
					t.Fatalf("missing project: %s", req.Body)
				}
				if tc.provider == domain.FixedProviderCodex && req.Headers["chatgpt-account-id"] != "workspace" {
					t.Fatal("missing codex workspace")
				}
				return response(http.StatusOK, tc.body), nil
			}), nil)
			defer r.Stop(context.Background())
			snapshot, err := r.Inspect(context.Background(), Inspection{Provider: tc.provider, Want: InspectQuota, Credential: Credential{ID: "c", AccessToken: "access", AccountID: "workspace", TokenVersion: 3, Metadata: map[string]any{"project_id": "test-project"}}})
			if err != nil || len(snapshot.Quotas) != 1 || snapshot.TokenVersion != 3 {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
		})
	}
}

func TestQuotaInspectionRefreshFencesTheNewCredentialVersion(t *testing.T) {
	calls := 0
	r := New(transportFunc(func(_ context.Context, req *WireRequest) (*WireResponse, error) {
		calls++
		if calls == 1 {
			return response(401, `{"error":"expired"}`), nil
		}
		if req.Headers["Authorization"] != "Bearer new-access" {
			t.Fatal("retry used old access token")
		}
		return response(200, `{"rate_limit":{"primary_window":{"used_percent":1}}}`), nil
	}), refresherFunc(func(context.Context, string) (Credential, error) {
		return Credential{ID: "c", AccessToken: "new-access", TokenVersion: 4}, nil
	}))
	defer r.Stop(context.Background())
	snapshot, err := r.Inspect(context.Background(), Inspection{Provider: domain.FixedProviderCodex, Want: InspectQuota, Credential: Credential{ID: "c", AccessToken: "old", RefreshToken: "refresh", TokenVersion: 3}})
	if err != nil || calls != 2 || snapshot.TokenVersion != 4 {
		t.Fatalf("version=%d calls=%d err=%v", snapshot.TokenVersion, calls, err)
	}
}
