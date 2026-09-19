package transport

import (
	"testing"

	"xiaodou/dai/internal/ai/domain"
)

func TestAPIKeyUpdateInputPreservesOmittedStatus(t *testing.T) {
	write, err := apiKeyUpdateInput("tenant-preserve", "key-preserve", apiKeyWriteRequest{
		Name: "renamed",
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
		Name: "created",
		GroupID: "group-create-default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if write.Status != domain.APIKeyStatusActive {
		t.Fatalf("create status = %q, want %q", write.Status, domain.APIKeyStatusActive)
	}
}
