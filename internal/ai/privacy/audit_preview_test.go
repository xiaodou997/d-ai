package privacy

import (
	"strings"
	"testing"
)

func TestAuditPreviewWithholdsShortTextAndSecrets(t *testing.T) {
	if got := AuditPreview("short confidential", 96); got != "***" {
		t.Fatalf("short preview = %q", got)
	}
	got := AuditPreview("contact alice@example.com using token: abcdefghijklmnop and continue with this sufficiently long confidential request", 96)
	if strings.Contains(got, "alice@example.com") || strings.Contains(got, "abcdefghijklmnop") {
		t.Fatalf("secret leaked in %q", got)
	}
}
