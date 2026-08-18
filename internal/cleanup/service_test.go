package cleanup

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizePolicyUsesDefaultsAndRejectsUnsafeValues(t *testing.T) {
	policy, err := normalizePolicy(Policy{Enabled: true})
	if err != nil {
		t.Fatalf("normalize defaults: %v", err)
	}
	if policy.RequestBodyDays != 30 || policy.RequestPayloadDays != 180 || policy.BatchSize != 1000 {
		t.Fatalf("normalized defaults = %+v", policy)
	}

	if _, err := normalizePolicy(Policy{RequestBodyDays: 3}); err == nil {
		t.Fatal("expected short retention to be rejected")
	}
	if _, err := normalizePolicy(Policy{RequestBodyDays: 180, RequestPayloadDays: 30}); err == nil {
		t.Fatal("expected payload retention shorter than body retention to be rejected")
	}
	if _, err := normalizePolicy(Policy{BatchSize: 5001}); err == nil {
		t.Fatal("expected oversized batch to be rejected")
	}
}

func TestNormalizeTargetsDefaultsAndDeduplicates(t *testing.T) {
	targets, err := normalizeTargets([]string{TargetNotifications, TargetNotifications, TargetRiskEvents})
	if err != nil {
		t.Fatalf("normalize targets: %v", err)
	}
	if want := []string{TargetNotifications, TargetRiskEvents}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}

	if _, err := normalizeTargets([]string{"billing"}); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("invalid target error = %v", err)
	}

	all, err := normalizeTargets(nil)
	if err != nil || !reflect.DeepEqual(all, AllTargets()) {
		t.Fatalf("default targets = %#v, err=%v", all, err)
	}
}

func TestRetentionDaysMatchTargets(t *testing.T) {
	policy := Policy{RequestBodyDays: 1, RequestPayloadDays: 2, NotificationDays: 3, ModerationDays: 4, RiskEventDays: 5, AdminAuditDays: 6, AuditBlobDays: 7, UsageRollupDays: 8}
	for target, want := range map[string]int{
		TargetRequestBody: 1, TargetRequestPayloads: 2, TargetNotifications: 3,
		TargetModerationLogs: 4, TargetRiskEvents: 5, TargetAdminAuditLogs: 6, TargetAuditBlobs: 7, TargetUsageRollups: 8,
	} {
		if got := policy.retentionDays(target); got != want {
			t.Errorf("retentionDays(%q) = %d, want %d", target, got, want)
		}
	}
}
