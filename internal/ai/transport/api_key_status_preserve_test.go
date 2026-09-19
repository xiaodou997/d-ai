package transport

import (
	"encoding/json"
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestAPIKeyUpdateInputPreservesOmittedStatus(t *testing.T) {
	write, err := apiKeyUpdateInput("tenant-preserve", "key-preserve", apiKeyWriteRequest{
		Name:    "renamed",
		GroupID: "group-preserve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if write.Status != "" {
		t.Fatalf("omitted PATCH status = %q, want empty preserve sentinel", write.Status)
	}
}

func TestAPIKeyCreateInputStillDefaultsActive(t *testing.T) {
	write, err := tenantAPIKeyCreateInput("tenant-create-default", apiKeyWriteRequest{
		Name:    "created",
		GroupID: "group-create-default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if write.Status != domain.APIKeyStatusActive {
		t.Fatalf("create status = %q, want %q", write.Status, domain.APIKeyStatusActive)
	}
}

func TestAPIKeyUpdateInputDistinguishesOmittedFromExplicitNull(t *testing.T) {
	var omitted apiKeyWriteRequest
	if err := json.Unmarshal([]byte(`{"name":"renamed","group_id":"group-preserve"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	omittedWrite, err := apiKeyUpdateInput("tenant-preserve", "key-preserve", omitted)
	if err != nil {
		t.Fatal(err)
	}
	if omittedWrite.QuotaLimitSet || omittedWrite.ExpiresAtSet {
		t.Fatalf("omitted nullable fields = quota:%v expires:%v, want both false", omittedWrite.QuotaLimitSet, omittedWrite.ExpiresAtSet)
	}

	var cleared apiKeyWriteRequest
	if err := json.Unmarshal([]byte(`{"name":"renamed","group_id":"group-preserve","quota_limit_micro_usd":null,"expires_at":null}`), &cleared); err != nil {
		t.Fatal(err)
	}
	clearedWrite, err := apiKeyUpdateInput("tenant-preserve", "key-preserve", cleared)
	if err != nil {
		t.Fatal(err)
	}
	if !clearedWrite.QuotaLimitSet || clearedWrite.QuotaLimitMicroUSD != nil {
		t.Fatalf("explicit null quota = set:%v value:%v", clearedWrite.QuotaLimitSet, clearedWrite.QuotaLimitMicroUSD)
	}
	if !clearedWrite.ExpiresAtSet || clearedWrite.ExpiresAt != nil {
		t.Fatalf("explicit null expiry = set:%v value:%v", clearedWrite.ExpiresAtSet, clearedWrite.ExpiresAt)
	}
}
