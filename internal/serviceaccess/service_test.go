package serviceaccess

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNormalizeServiceIDs(t *testing.T) {
	got := NormalizeServiceIDs([]string{" proxy ", "ai", "", "ai"})
	if !slices.Equal(got, []string{"ai", "proxy"}) {
		t.Fatalf("NormalizeServiceIDs() = %v", got)
	}
}

func TestRequestedPolicyNormalizesAndValidates(t *testing.T) {
	mode, ids, err := requestedPolicy(&PolicyInput{Mode: "selected", ServiceIDs: []string{" proxy ", "ai", "ai"}}, "all", nil)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "selected" || !slices.Equal(ids, []string{"ai", "proxy"}) {
		t.Fatalf("requestedPolicy() = %q, %v", mode, ids)
	}
	if _, _, err := requestedPolicy(&PolicyInput{Mode: "all", ServiceIDs: []string{"ai"}}, "selected", []string{"proxy"}); err == nil {
		t.Fatal("requestedPolicy() accepted service IDs in all mode")
	}
}

func TestLockMutationSerializesServiceInstances(t *testing.T) {
	dsn := os.Getenv("URM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set URM_TEST_DATABASE_URL to run this DB-backed test")
	}
	ctx := context.Background()
	newPool := func() *pgxpool.Pool {
		config, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		config.MaxConns = 2
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		return pool
	}
	pool1, pool2 := newPool(), newPool()
	defer pool1.Close()
	defer pool2.Close()

	first := New(pool1, nil, nil)
	second := New(pool2, nil, nil)
	unlockFirst, err := first.lockMutation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstReleased := false
	defer func() {
		if !firstReleased {
			unlockFirst()
		}
	}()

	acquired := make(chan func(), 1)
	errs := make(chan error, 1)
	go func() {
		unlock, err := second.lockMutation(ctx)
		if err != nil {
			errs <- err
			return
		}
		acquired <- unlock
	}()

	select {
	case unlock := <-acquired:
		unlock()
		t.Fatal("second service instance acquired the mutation lock before release")
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(150 * time.Millisecond):
	}

	unlockFirst()
	firstReleased = true
	select {
	case unlock := <-acquired:
		unlock()
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("second service instance did not acquire the released mutation lock")
	}
}

func TestCreateTenantTxStoresRequestedPolicyAtomically(t *testing.T) {
	pool := openServiceAccessTestPool(t, 1)
	defer pool.Close()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE gov_clients (
			client_id text PRIMARY KEY,
			portal_enabled boolean NOT NULL
		);
		CREATE TEMP TABLE gov_subject_service_access (
			subject_type text NOT NULL,
			subject_id text NOT NULL,
			access_mode text NOT NULL,
			service_ids text[] NOT NULL,
			version bigint NOT NULL,
			created_by text NOT NULL,
			updated_by text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (subject_type, subject_id)
		);
		CREATE TEMP TABLE auth_audit_logs (
			id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			event_type text NOT NULL,
			principal_type text NOT NULL,
			user_id text,
			decision text NOT NULL,
			reason_code text,
			metadata jsonb NOT NULL
		);
		INSERT INTO gov_clients (client_id, portal_enabled) VALUES ('ai', true), ('proxy', true), ('internal', false);
	`); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	requested := &PolicyInput{Mode: "selected", ServiceIDs: []string{" proxy ", "ai", "ai"}}
	if err := CreateTenantTx(ctx, tx, Actor{UserType: 1, UserID: "root"}, "tenant-1", requested); err != nil {
		t.Fatal(err)
	}
	policy, err := getPolicy(ctx, tx, "tenant", "tenant-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != "selected" || !slices.Equal(policy.ServiceIDs, []string{"ai", "proxy"}) || policy.Version != 1 {
		t.Fatalf("unexpected created policy: %#v", policy)
	}
	var eventType, actorID, targetID string
	if err := tx.QueryRow(ctx, `
		SELECT event_type, user_id, metadata->'after'->>'subjectId'
		FROM auth_audit_logs
	`).Scan(&eventType, &actorID, &targetID); err != nil {
		t.Fatal(err)
	}
	if eventType != "service_access_create" || actorID != "root" || targetID != "tenant-1" {
		t.Fatalf("unexpected creation audit: event=%q actor=%q target=%q", eventType, actorID, targetID)
	}
}

func TestRedisUnavailableWrapsStableError(t *testing.T) {
	err := redisUnavailable("write snapshot", errors.New("read only replica"))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("redisUnavailable() = %v, want ErrUnavailable", err)
	}
}

func TestCreateTenantTxLocksOperatorAndEnforcesSubset(t *testing.T) {
	pool := openServiceAccessTestPool(t, 1)
	defer pool.Close()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE gov_clients (client_id text PRIMARY KEY, portal_enabled boolean NOT NULL);
		CREATE TEMP TABLE gov_subject_service_access (
			subject_type text NOT NULL, subject_id text NOT NULL, access_mode text NOT NULL,
			service_ids text[] NOT NULL, version bigint NOT NULL, created_by text NOT NULL,
			updated_by text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (subject_type, subject_id)
		);
		INSERT INTO gov_clients (client_id, portal_enabled) VALUES ('ai', true), ('proxy', true);
		INSERT INTO gov_subject_service_access
			(subject_type, subject_id, access_mode, service_ids, version, created_by, updated_by)
		VALUES ('admin', 'operator-1', 'selected', ARRAY['proxy'], 3, 'root', 'root');
	`); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	err = CreateTenantTx(ctx, tx, Actor{UserType: 2, UserID: "operator-1"}, "tenant-2", &PolicyInput{Mode: "selected", ServiceIDs: []string{"ai"}})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("CreateTenantTx() error = %v, want ErrForbidden", err)
	}
}

func TestSuperAdminCapabilitiesFollowRuntimePortalState(t *testing.T) {
	pool := openServiceAccessTestPool(t, 1)
	defer pool.Close()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TEMP TABLE gov_clients (
			client_id text PRIMARY KEY,
			portal_enabled boolean NOT NULL,
			status text NOT NULL
		);
		INSERT INTO gov_clients (client_id, portal_enabled, status) VALUES
			('ai', false, 'active'),
			('proxy', true, 'active'),
			('disabled', true, 'disabled');
	`); err != nil {
		t.Fatal(err)
	}

	service := New(pool, nil, nil)
	capabilities, err := service.ListCapabilities(ctx, 1, "root", "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(capabilities, []string{"proxy"}) {
		t.Fatalf("initial capabilities = %v, want [proxy]", capabilities)
	}

	if _, err := pool.Exec(ctx, `UPDATE gov_clients SET portal_enabled = true WHERE client_id = 'ai'`); err != nil {
		t.Fatal(err)
	}
	capabilities, err = service.ListCapabilities(ctx, 1, "root", "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(capabilities, []string{"ai", "proxy"}) {
		t.Fatalf("enabled capabilities = %v, want [ai proxy]", capabilities)
	}

	if _, err := pool.Exec(ctx, `UPDATE gov_clients SET portal_enabled = false WHERE client_id = 'proxy'`); err != nil {
		t.Fatal(err)
	}
	capabilities, err = service.ListCapabilities(ctx, 1, "root", "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(capabilities, []string{"ai"}) {
		t.Fatalf("disabled capabilities = %v, want [ai]", capabilities)
	}
}

func openServiceAccessTestPool(t *testing.T, maxConns int32) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("URM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set URM_TEST_DATABASE_URL to run this DB-backed test")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}
