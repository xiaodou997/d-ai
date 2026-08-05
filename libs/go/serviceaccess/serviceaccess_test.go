package serviceaccess

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCheckerFailsClosedWithoutRequiredSnapshots(t *testing.T) {
	ctx := context.Background()
	checker, _, closeRedis := newTestChecker(t)
	defer closeRedis()

	if err := checker.Check(ctx, 1, "root", "", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing service snapshot error = %v, want ErrUnavailable", err)
	}
	if err := (*Checker)(nil).Check(ctx, 1, "root", "", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil checker error = %v, want ErrUnavailable", err)
	}
}

func TestCheckerEnforcesClientAndServiceStateForAllUserTypes(t *testing.T) {
	ctx := context.Background()
	checker, client, closeRedis := newTestChecker(t)
	defer closeRedis()
	setJSON(t, client, ServiceKey("ai"), ServiceSnapshot{ServiceID: "ai", Active: true, PortalEnabled: true})

	if err := checker.Check(ctx, 1, "root", "", "ai", "proxy"); !errors.Is(err, ErrDenied) {
		t.Fatalf("super-admin client mismatch error = %v, want ErrDenied", err)
	}
	if err := checker.Check(ctx, 1, "root", "", "ai", "ai"); err != nil {
		t.Fatalf("active portal service rejected for super-admin: %v", err)
	}

	setJSON(t, client, ServiceKey("ai"), ServiceSnapshot{ServiceID: "ai", Active: false, PortalEnabled: true})
	if err := checker.Check(ctx, 1, "root", "", "ai", "ai"); !errors.Is(err, ErrDenied) {
		t.Fatalf("disabled service error = %v, want ErrDenied", err)
	}
	setJSON(t, client, ServiceKey("ai"), ServiceSnapshot{ServiceID: "ai", Active: true, PortalEnabled: false})
	if err := checker.Check(ctx, 1, "root", "", "ai", "ai"); !errors.Is(err, ErrDenied) {
		t.Fatalf("non-portal service error = %v, want ErrDenied", err)
	}
}

func TestCheckerTreatsUpdateFencesAsUnavailable(t *testing.T) {
	ctx := context.Background()
	checker, client, closeRedis := newTestChecker(t)
	defer closeRedis()
	setJSON(t, client, ServiceKey("ai"), ServiceSnapshot{ServiceID: "ai", Active: true, PortalEnabled: true})
	setJSON(t, client, SubjectKey("tenant", "tenant-1"), SubjectSnapshot{SubjectType: "tenant", SubjectID: "tenant-1", Mode: "all", ServiceIDs: []string{}, Version: 1})
	if err := client.Set(ctx, GlobalFenceKey(), "reconciling", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := checker.Check(ctx, 4, "user-1", "tenant-1", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("global fence error = %v, want ErrUnavailable", err)
	}
	if err := client.Del(ctx, GlobalFenceKey()).Err(); err != nil {
		t.Fatal(err)
	}

	if err := client.Set(ctx, ServiceFenceKey("ai"), "updating", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := checker.Check(ctx, 4, "user-1", "tenant-1", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("service fence error = %v, want ErrUnavailable", err)
	}
	if err := client.Del(ctx, ServiceFenceKey("ai")).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, FenceKey("tenant", "tenant-1"), "updating", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := checker.Check(ctx, 4, "user-1", "tenant-1", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("subject fence error = %v, want ErrUnavailable", err)
	}
}

func TestCheckerAppliesAllAndSelectedPolicies(t *testing.T) {
	ctx := context.Background()
	checker, client, closeRedis := newTestChecker(t)
	defer closeRedis()
	for _, serviceID := range []string{"ai", "future"} {
		setJSON(t, client, ServiceKey(serviceID), ServiceSnapshot{ServiceID: serviceID, Active: true, PortalEnabled: true})
	}
	setJSON(t, client, SubjectKey("admin", "admin-1"), SubjectSnapshot{SubjectType: "admin", SubjectID: "admin-1", Mode: "all", ServiceIDs: []string{}, Version: 1})
	if err := checker.Check(ctx, 2, "admin-1", "", "future", "future"); err != nil {
		t.Fatalf("all policy did not include future service: %v", err)
	}

	setJSON(t, client, SubjectKey("tenant", "tenant-1"), SubjectSnapshot{SubjectType: "tenant", SubjectID: "tenant-1", Mode: "selected", ServiceIDs: []string{"ai"}, Version: 2})
	if err := checker.Check(ctx, 4, "user-1", "tenant-1", "ai", "ai"); err != nil {
		t.Fatalf("tenant-selected service rejected for inherited end user: %v", err)
	}
	if err := checker.Check(ctx, 4, "user-1", "tenant-1", "future", "future"); !errors.Is(err, ErrDenied) {
		t.Fatalf("service outside selected policy error = %v, want ErrDenied", err)
	}
	setJSON(t, client, SubjectKey("tenant", "tenant-1"), SubjectSnapshot{SubjectType: "tenant", SubjectID: "tenant-1", Mode: "selected", ServiceIDs: []string{}, Version: 3})
	if err := checker.Check(ctx, 3, "tenant-user", "tenant-1", "ai", "ai"); !errors.Is(err, ErrDenied) {
		t.Fatalf("empty selected policy error = %v, want ErrDenied", err)
	}
}

func TestCheckerRejectsMalformedOrMissingSubjectPolicy(t *testing.T) {
	ctx := context.Background()
	checker, client, closeRedis := newTestChecker(t)
	defer closeRedis()
	setJSON(t, client, ServiceKey("ai"), ServiceSnapshot{ServiceID: "ai", Active: true, PortalEnabled: true})

	if err := checker.Check(ctx, 3, "tenant-user", "tenant-1", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing subject policy error = %v, want ErrUnavailable", err)
	}
	setJSON(t, client, SubjectKey("tenant", "tenant-1"), SubjectSnapshot{SubjectType: "tenant", SubjectID: "tenant-1", Mode: "all", ServiceIDs: []string{"ai"}, Version: 1})
	if err := checker.Check(ctx, 3, "tenant-user", "tenant-1", "ai", "ai"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("malformed all policy error = %v, want ErrUnavailable", err)
	}
}

func newTestChecker(t *testing.T) (*Checker, *redis.Client, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	return NewChecker(client), client, func() { _ = client.Close() }
}

func setJSON(t *testing.T, client *redis.Client, key string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(context.Background(), key, raw, 0).Err(); err != nil {
		t.Fatal(err)
	}
}
