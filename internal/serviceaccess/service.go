package serviceaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	shared "xiaodou/dai/libs/go/serviceaccess"
)

var (
	ErrNotFound    = errors.New("service access policy not found")
	ErrInvalid     = errors.New("invalid service access policy")
	ErrForbidden   = errors.New("service access policy update forbidden")
	ErrDenied      = shared.ErrDenied
	ErrUnavailable = shared.ErrUnavailable
)

type Policy struct {
	SubjectType string    `json:"subjectType"`
	SubjectID   string    `json:"subjectId"`
	Mode        string    `json:"mode"`
	ServiceIDs  []string  `json:"serviceIds"`
	Version     int64     `json:"version"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Actor struct {
	UserType int
	UserID   string
}

type PolicyInput struct {
	Mode       string   `json:"mode" enum:"all,selected"`
	ServiceIDs []string `json:"serviceIds"`
}

const serviceAccessAdvisoryLockKey int64 = 0x55524d5341434345

type Service struct {
	pool     *pgxpool.Pool
	redis    *redis.Client
	checker  *shared.Checker
	log      *zap.Logger
	mutation sync.Mutex
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
}

func New(pool *pgxpool.Pool, redisClient *redis.Client, log *zap.Logger) *Service {
	return &Service{pool: pool, redis: redisClient, checker: shared.NewChecker(redisClient), log: log, stop: make(chan struct{}), done: make(chan struct{})}
}

func (s *Service) Start(interval time.Duration) error {
	if err := s.Reconcile(context.Background()); err != nil {
		return err
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				if err := s.Reconcile(ctx); err != nil && s.log != nil {
					s.log.Error("service access reconciliation failed", zap.Error(err))
				}
				cancel()
			case <-s.stop:
				return
			}
		}
	}()
	return nil
}

func (s *Service) Stop() {
	s.once.Do(func() { close(s.stop); <-s.done })
}

func LockMutationTx(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, serviceAccessAdvisoryLockKey)
	return err
}

func CreateAdminTx(ctx context.Context, tx pgx.Tx, actor Actor, adminID string, requested *PolicyInput) error {
	if actor.UserType != 1 {
		return ErrForbidden
	}
	mode, serviceIDs, err := requestedPolicy(requested, "all", nil)
	if err != nil {
		return err
	}
	if err := validatePortalServices(ctx, tx, serviceIDs); err != nil {
		return err
	}
	return insertPolicy(ctx, tx, "admin", adminID, mode, serviceIDs, actor.UserID)
}

func CreateTenantTx(ctx context.Context, tx pgx.Tx, actor Actor, tenantID string, requested *PolicyInput) error {
	mode := "all"
	serviceIDs := []string{}
	if actor.UserType == 2 {
		operator, err := getPolicy(ctx, tx, "admin", actor.UserID, true)
		if err != nil {
			return err
		}
		mode = operator.Mode
		serviceIDs = append(serviceIDs, operator.ServiceIDs...)
		if requested != nil {
			mode, serviceIDs, err = requestedPolicy(requested, mode, serviceIDs)
			if err != nil {
				return err
			}
			if operator.Mode == "selected" {
				if mode == "all" || !isSubset(serviceIDs, operator.ServiceIDs) {
					return ErrForbidden
				}
			}
		}
	} else if actor.UserType == 1 {
		var err error
		mode, serviceIDs, err = requestedPolicy(requested, mode, serviceIDs)
		if err != nil {
			return err
		}
	} else {
		return ErrForbidden
	}
	if err := validatePortalServices(ctx, tx, serviceIDs); err != nil {
		return err
	}
	return insertPolicy(ctx, tx, "tenant", tenantID, mode, serviceIDs, actor.UserID)
}

func requestedPolicy(requested *PolicyInput, defaultMode string, defaultServiceIDs []string) (string, []string, error) {
	if requested == nil {
		return defaultMode, append([]string{}, defaultServiceIDs...), nil
	}
	serviceIDs := NormalizeServiceIDs(requested.ServiceIDs)
	if requested.Mode != "all" && requested.Mode != "selected" {
		return "", nil, fmt.Errorf("%w: mode must be all or selected", ErrInvalid)
	}
	if requested.Mode == "all" && len(serviceIDs) != 0 {
		return "", nil, fmt.Errorf("%w: all mode requires an empty serviceIds list", ErrInvalid)
	}
	return requested.Mode, serviceIDs, nil
}

func insertPolicy(ctx context.Context, tx pgx.Tx, subjectType, subjectID, mode string, serviceIDs []string, actorID string) error {
	var updatedAt time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO gov_subject_service_access
		(subject_type, subject_id, access_mode, service_ids, version, created_by, updated_by)
		VALUES ($1, $2, $3, $4, 1, $5, $5)
		RETURNING updated_at
	`, subjectType, subjectID, mode, serviceIDs, actorID).Scan(&updatedAt)
	if err != nil {
		return err
	}
	after := Policy{SubjectType: subjectType, SubjectID: subjectID, Mode: mode, ServiceIDs: serviceIDs, Version: 1, UpdatedAt: updatedAt}
	return writePolicyAuditTx(ctx, tx, "service_access_create", "service_access_created", actorID, nil, &after, nil)
}

func writePolicyAuditTx(ctx context.Context, tx pgx.Tx, eventType, reasonCode, actorID string, before, after *Policy, extra map[string]any) error {
	metadata := map[string]any{"before": before, "after": after}
	for key, value := range extra {
		metadata[key] = value
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO auth_audit_logs
		(event_type, principal_type, user_id, decision, reason_code, metadata)
		VALUES ($1, 'admin', $2, 'success', $3, $4::jsonb)
	`, eventType, actorID, reasonCode, raw)
	return err
}

func DeleteSubjectTx(ctx context.Context, tx pgx.Tx, subjectType, subjectID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM gov_subject_service_access WHERE subject_type = $1 AND subject_id = $2`, subjectType, subjectID)
	return err
}

func (s *Service) Get(ctx context.Context, subjectType, subjectID string) (Policy, error) {
	return getPolicy(ctx, s.pool, subjectType, subjectID, false)
}

func getPolicy(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, subjectType, subjectID string, lock bool) (Policy, error) {
	var p Policy
	query := `SELECT subject_type, subject_id, access_mode, service_ids, version, updated_at
		FROM gov_subject_service_access WHERE subject_type = $1 AND subject_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	if err := q.QueryRow(ctx, query, subjectType, subjectID).Scan(&p.SubjectType, &p.SubjectID, &p.Mode, &p.ServiceIDs, &p.Version, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Policy{}, ErrNotFound
		}
		return Policy{}, err
	}
	if p.ServiceIDs == nil {
		p.ServiceIDs = []string{}
	}
	return p, nil
}

func NormalizeServiceIDs(ids []string) []string {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func isSubset(ids, allowed []string) bool {
	for _, id := range ids {
		if !slices.Contains(allowed, id) {
			return false
		}
	}
	return true
}

func (s *Service) lockMutation(ctx context.Context) (func(), error) {
	if s.pool.Config().MaxConns < 2 {
		return nil, fmt.Errorf("%w: service access mutations require at least two PostgreSQL connections", ErrUnavailable)
	}
	s.mutation.Lock()
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		s.mutation.Unlock()
		return nil, err
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, serviceAccessAdvisoryLockKey); err != nil {
		conn.Release()
		s.mutation.Unlock()
		return nil, err
	}
	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var unlocked bool
			unlockErr := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock($1)`, serviceAccessAdvisoryLockKey).Scan(&unlocked)
			if unlockErr == nil && unlocked {
				conn.Release()
			} else {
				if s.log != nil {
					s.log.Error("release service access advisory lock", zap.Bool("unlocked", unlocked), zap.Error(unlockErr))
				}
				rawConn := conn.Hijack()
				closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = rawConn.Close(closeCtx)
				closeCancel()
			}
			s.mutation.Unlock()
		})
	}, nil
}

func (s *Service) Put(ctx context.Context, actor Actor, subjectType, subjectID, mode string, serviceIDs []string) (Policy, error) {
	serviceIDs = NormalizeServiceIDs(serviceIDs)
	if mode != "all" && mode != "selected" {
		return Policy{}, fmt.Errorf("%w: mode must be all or selected", ErrInvalid)
	}
	if mode == "all" && len(serviceIDs) != 0 {
		return Policy{}, fmt.Errorf("%w: all mode requires an empty serviceIds list", ErrInvalid)
	}
	if actor.UserType != 1 && (actor.UserType != 2 || subjectType != "tenant") {
		return Policy{}, ErrForbidden
	}
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer unlock()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer tx.Rollback(ctx)

	before, err := getPolicy(ctx, tx, subjectType, subjectID, true)
	if err != nil {
		return Policy{}, err
	}
	if err := lockSubject(ctx, tx, subjectType, subjectID); err != nil {
		return Policy{}, err
	}
	if actor.UserType == 2 {
		operator, err := getPolicy(ctx, tx, "admin", actor.UserID, true)
		if err != nil {
			return Policy{}, ErrForbidden
		}
		if operator.Mode == "selected" {
			if mode == "all" || !isSubset(serviceIDs, operator.ServiceIDs) {
				return Policy{}, ErrForbidden
			}
		}
	}
	if err := validatePortalServices(ctx, tx, serviceIDs); err != nil {
		return Policy{}, err
	}

	if err := s.redis.Set(ctx, shared.FenceKey(subjectType, subjectID), "updating", 0).Err(); err != nil {
		return Policy{}, fmt.Errorf("%w: set update fence: %v", ErrUnavailable, err)
	}
	now := time.Now().UTC()
	var after Policy
	err = tx.QueryRow(ctx, `
		UPDATE gov_subject_service_access
		SET access_mode = $1, service_ids = $2, version = version + 1,
			updated_by = $3, updated_at = $4
		WHERE subject_type = $5 AND subject_id = $6
		RETURNING subject_type, subject_id, access_mode, service_ids, version, updated_at
	`, mode, serviceIDs, actor.UserID, now, subjectType, subjectID).Scan(
		&after.SubjectType, &after.SubjectID, &after.Mode, &after.ServiceIDs, &after.Version, &after.UpdatedAt)
	if err != nil {
		return Policy{}, err
	}
	if err := writePolicyAuditTx(ctx, tx, "service_access_update", "service_access_updated", actor.UserID, &before, &after, nil); err != nil {
		return Policy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	if err := s.writePolicySnapshot(ctx, after); err != nil {
		return Policy{}, err
	}
	if err := s.redis.Del(ctx, shared.FenceKey(subjectType, subjectID)).Err(); err != nil {
		return Policy{}, fmt.Errorf("%w: clear update fence: %v", ErrUnavailable, err)
	}
	return after, nil
}

func lockSubject(ctx context.Context, tx pgx.Tx, subjectType, subjectID string) error {
	table, idColumn := "iam_tenants", "tenant_id"
	if subjectType == "admin" {
		table, idColumn = "iam_admins", "user_id"
	} else if subjectType != "tenant" {
		return ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM `+table+` WHERE `+idColumn+` = $1 FOR UPDATE`, subjectID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func validatePortalServices(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT client_id FROM gov_clients WHERE portal_enabled AND client_id = ANY($1) ORDER BY client_id FOR KEY SHARE`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(ids) {
		return fmt.Errorf("%w: serviceIds must reference portal business services", ErrInvalid)
	}
	return nil
}

func (s *Service) ListCapabilities(ctx context.Context, userType int, userID, tenantID string) ([]string, error) {
	if userType == 1 {
		return listActivePortalServices(ctx, s.pool)
	}
	subjectType, subjectID := "tenant", tenantID
	if userType == 2 {
		subjectType, subjectID = "admin", userID
	} else if userType != 3 && userType != 4 {
		return []string{}, nil
	}
	p, err := s.Get(ctx, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	if p.Mode == "all" {
		return listActivePortalServices(ctx, s.pool)
	}
	rows, err := s.pool.Query(ctx, `SELECT client_id FROM gov_clients WHERE status = 'active' AND portal_enabled AND client_id = ANY($1) ORDER BY client_id`, p.ServiceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func listActivePortalServices(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT client_id FROM gov_clients WHERE status = 'active' AND portal_enabled ORDER BY client_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) Check(ctx context.Context, userType int, userID, tenantID, clientID string) error {
	return s.checker.Check(ctx, userType, userID, tenantID, clientID, clientID)
}

func (s *Service) ReconcileSubject(ctx context.Context, subjectType, subjectID string) error {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	p, err := s.Get(ctx, subjectType, subjectID)
	if err != nil {
		return err
	}
	if err := s.writePolicySnapshot(ctx, p); err != nil {
		return err
	}
	if err := s.redis.Del(ctx, shared.FenceKey(subjectType, subjectID)).Err(); err != nil {
		return redisUnavailable("clear subject reconciliation fence", err)
	}
	return nil
}

func (s *Service) DeleteSubjectSnapshot(ctx context.Context, subjectType, subjectID string) error {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := s.redis.Del(ctx, shared.SubjectKey(subjectType, subjectID), shared.FenceKey(subjectType, subjectID)).Err(); err != nil {
		return redisUnavailable("delete subject snapshot", err)
	}
	return nil
}

func (s *Service) SyncService(ctx context.Context, serviceID string) error {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.syncService(ctx, serviceID)
}

func (s *Service) syncService(ctx context.Context, serviceID string) error {
	var snapshot shared.ServiceSnapshot
	if err := s.pool.QueryRow(ctx, `SELECT client_id, status = 'active', portal_enabled FROM gov_clients WHERE client_id = $1`, serviceID).Scan(&snapshot.ServiceID, &snapshot.Active, &snapshot.PortalEnabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if err := s.redis.Del(ctx, shared.ServiceKey(serviceID), shared.ServiceFenceKey(serviceID)).Err(); err != nil {
				return redisUnavailable("delete missing service snapshot", err)
			}
			return nil
		}
		return err
	}
	raw, _ := json.Marshal(snapshot)
	if err := s.redis.Set(ctx, shared.ServiceKey(serviceID), raw, 0).Err(); err != nil {
		return fmt.Errorf("%w: write service snapshot: %v", ErrUnavailable, err)
	}
	if err := s.redis.Del(ctx, shared.ServiceFenceKey(serviceID)).Err(); err != nil {
		return fmt.Errorf("%w: clear service update fence: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *Service) UpdateService(ctx context.Context, serviceID, displayName, description, status string, portalEnabled bool) (bool, error) {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return false, err
	}
	defer unlock()

	if err := s.redis.Set(ctx, shared.ServiceFenceKey(serviceID), "updating", 0).Err(); err != nil {
		return false, fmt.Errorf("%w: set service update fence: %v", ErrUnavailable, err)
	}
	command, err := s.pool.Exec(ctx, `UPDATE gov_clients SET display_name = COALESCE(NULLIF($1, ''), display_name), description = NULLIF($2, ''), status = $3, portal_enabled = $4, updated_at = NOW() WHERE client_id = $5`, displayName, description, status, portalEnabled, serviceID)
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 0 {
		if err := s.redis.Del(ctx, shared.ServiceKey(serviceID), shared.ServiceFenceKey(serviceID)).Err(); err != nil {
			return false, fmt.Errorf("%w: clear missing service snapshot: %v", ErrUnavailable, err)
		}
		return false, nil
	}
	if err := s.syncService(ctx, serviceID); err != nil {
		return true, err
	}
	if !portalEnabled {
		if err := s.reconcile(ctx); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (s *Service) DeleteService(ctx context.Context, serviceID, actorID string) (bool, error) {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return false, err
	}
	defer unlock()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM gov_clients WHERE client_id = $1 FOR UPDATE`, serviceID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if err := s.redis.Set(ctx, shared.ServiceFenceKey(serviceID), "deleting", 0).Err(); err != nil {
		return false, fmt.Errorf("%w: set service update fence: %v", ErrUnavailable, err)
	}
	rows, err := tx.Query(ctx, `
		SELECT subject_type, subject_id, access_mode, service_ids, version, updated_at
		FROM gov_subject_service_access
		WHERE access_mode = 'selected' AND $1 = ANY(service_ids)
		FOR UPDATE
	`, serviceID)
	if err != nil {
		return false, err
	}
	policies := make([]Policy, 0)
	for rows.Next() {
		var policy Policy
		if err := rows.Scan(&policy.SubjectType, &policy.SubjectID, &policy.Mode, &policy.ServiceIDs, &policy.Version, &policy.UpdatedAt); err != nil {
			rows.Close()
			return false, err
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()
	for _, policy := range policies {
		if err := s.redis.Set(ctx, shared.FenceKey(policy.SubjectType, policy.SubjectID), "service-deleting", 0).Err(); err != nil {
			return false, fmt.Errorf("%w: set update fence: %v", ErrUnavailable, err)
		}
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE gov_subject_service_access SET service_ids = array_remove(service_ids, $1), version = version + 1, updated_by = $2, updated_at = $3 WHERE access_mode = 'selected' AND $1 = ANY(service_ids)`, serviceID, actorID, now); err != nil {
		return false, err
	}
	for _, before := range policies {
		after := before
		after.ServiceIDs = slices.DeleteFunc(append([]string{}, before.ServiceIDs...), func(id string) bool { return id == serviceID })
		after.Version++
		after.UpdatedAt = now
		if err := writePolicyAuditTx(ctx, tx, "service_access_service_delete", "service_access_service_removed", actorID, &before, &after, map[string]any{"serviceId": serviceID}); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM gov_service_instances WHERE service_id = $1`, serviceID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM gov_service_sources WHERE service_id = $1`, serviceID); err != nil {
		return false, err
	}
	command, err := tx.Exec(ctx, `DELETE FROM gov_clients WHERE client_id = $1`, serviceID)
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 0 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if err := s.redis.Del(ctx, shared.ServiceKey(serviceID), shared.ServiceFenceKey(serviceID)).Err(); err != nil {
		return true, redisUnavailable("delete service snapshot", err)
	}
	if err := s.reconcile(ctx); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) DeleteSubject(ctx context.Context, subjectType, subjectID string) (bool, error) {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return false, err
	}
	defer unlock()

	table, idColumn := "iam_tenants", "tenant_id"
	if subjectType == "admin" {
		table, idColumn = "iam_admins", "user_id"
	} else if subjectType != "tenant" {
		return false, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := lockSubject(ctx, tx, subjectType, subjectID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if err := s.redis.Set(ctx, shared.FenceKey(subjectType, subjectID), "deleting", 0).Err(); err != nil {
		return false, fmt.Errorf("%w: set update fence: %v", ErrUnavailable, err)
	}
	if err := DeleteSubjectTx(ctx, tx, subjectType, subjectID); err != nil {
		return false, err
	}
	command, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE `+idColumn+` = $1`, subjectID)
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 0 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if err := s.redis.Del(ctx, shared.SubjectKey(subjectType, subjectID), shared.FenceKey(subjectType, subjectID)).Err(); err != nil {
		return true, fmt.Errorf("%w: delete subject snapshot: %v", ErrUnavailable, err)
	}
	return true, nil
}

func (s *Service) writePolicySnapshot(ctx context.Context, p Policy) error {
	snapshot := shared.SubjectSnapshot{SubjectType: p.SubjectType, SubjectID: p.SubjectID, Mode: p.Mode, ServiceIDs: p.ServiceIDs, Version: p.Version}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err := s.redis.Set(ctx, shared.SubjectKey(p.SubjectType, p.SubjectID), raw, 0).Err(); err != nil {
		return fmt.Errorf("%w: write subject snapshot: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *Service) Reconcile(ctx context.Context) error {
	unlock, err := s.lockMutation(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.reconcile(ctx)
}

func (s *Service) reconcile(ctx context.Context) error {
	if err := s.redis.Set(ctx, shared.GlobalFenceKey(), "reconciling", 0).Err(); err != nil {
		return redisUnavailable("set global reconciliation fence", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		DELETE FROM gov_subject_service_access p
		WHERE (p.subject_type = 'admin' AND NOT EXISTS (SELECT 1 FROM iam_admins a WHERE a.user_id = p.subject_id))
		   OR (p.subject_type = 'tenant' AND NOT EXISTS (SELECT 1 FROM iam_tenants t WHERE t.tenant_id = p.subject_id));
		UPDATE gov_subject_service_access p
		SET service_ids = ARRAY(SELECT DISTINCT u.service_id FROM unnest(p.service_ids) AS u(service_id)
			WHERE EXISTS (SELECT 1 FROM gov_clients c WHERE c.client_id = u.service_id AND c.portal_enabled) ORDER BY u.service_id),
			version = version + 1, updated_at = now(), updated_by = 'reconciler'
		WHERE p.access_mode = 'selected' AND EXISTS (
			SELECT 1 FROM unnest(p.service_ids) AS u(service_id)
			WHERE NOT EXISTS (SELECT 1 FROM gov_clients c WHERE c.client_id = u.service_id AND c.portal_enabled));
		DELETE FROM gov_service_sources s WHERE NOT EXISTS (SELECT 1 FROM gov_clients c WHERE c.client_id = s.service_id);
		DELETE FROM gov_service_instances i WHERE NOT EXISTS (SELECT 1 FROM gov_clients c WHERE c.client_id = i.service_id);
	`)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	serviceKeys := make(map[string]struct{})
	rows, err := s.pool.Query(ctx, `SELECT client_id, status = 'active', portal_enabled FROM gov_clients`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var snapshot shared.ServiceSnapshot
		if err := rows.Scan(&snapshot.ServiceID, &snapshot.Active, &snapshot.PortalEnabled); err != nil {
			rows.Close()
			return err
		}
		raw, _ := json.Marshal(snapshot)
		if err := s.redis.Set(ctx, shared.ServiceKey(snapshot.ServiceID), raw, 0).Err(); err != nil {
			rows.Close()
			return redisUnavailable("write reconciled service snapshot", err)
		}
		serviceKeys[shared.ServiceKey(snapshot.ServiceID)] = struct{}{}
		if err := s.redis.Del(ctx, shared.ServiceFenceKey(snapshot.ServiceID)).Err(); err != nil {
			rows.Close()
			return redisUnavailable("clear reconciled service fence", err)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if err := s.deleteKeysNotIn(ctx, shared.ServiceKey("*"), serviceKeys); err != nil {
		return err
	}
	serviceFenceKeys := make(map[string]struct{}, len(serviceKeys))
	for key := range serviceKeys {
		serviceID := strings.TrimPrefix(key, shared.ServiceKey(""))
		serviceFenceKeys[shared.ServiceFenceKey(serviceID)] = struct{}{}
	}
	if err := s.deleteKeysNotIn(ctx, shared.ServiceFenceKey("*"), serviceFenceKeys); err != nil {
		return err
	}

	subjectKeys := make(map[string]struct{})
	subjectFenceKeys := make(map[string]struct{})
	rows, err = s.pool.Query(ctx, `SELECT subject_type, subject_id, access_mode, service_ids, version, updated_at FROM gov_subject_service_access`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.SubjectType, &p.SubjectID, &p.Mode, &p.ServiceIDs, &p.Version, &p.UpdatedAt); err != nil {
			return err
		}
		if err := s.writePolicySnapshot(ctx, p); err != nil {
			return err
		}
		subjectKeys[shared.SubjectKey(p.SubjectType, p.SubjectID)] = struct{}{}
		subjectFenceKeys[shared.FenceKey(p.SubjectType, p.SubjectID)] = struct{}{}
		if err := s.redis.Del(ctx, shared.FenceKey(p.SubjectType, p.SubjectID)).Err(); err != nil {
			return redisUnavailable("clear reconciled subject fence", err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := s.deleteKeysNotIn(ctx, shared.SubjectKey("*", "*"), subjectKeys); err != nil {
		return err
	}
	if err := s.deleteKeysNotIn(ctx, shared.FenceKey("*", "*"), subjectFenceKeys); err != nil {
		return err
	}
	if err := s.redis.Del(ctx, shared.GlobalFenceKey()).Err(); err != nil {
		return redisUnavailable("clear global reconciliation fence", err)
	}
	return nil
}

func redisUnavailable(operation string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrUnavailable, operation, err)
}

func (s *Service) deleteKeysNotIn(ctx context.Context, pattern string, keep map[string]struct{}) error {
	var cursor uint64
	for {
		keys, next, err := s.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("%w: scan Redis snapshots: %v", ErrUnavailable, err)
		}
		stale := make([]string, 0, len(keys))
		for _, key := range keys {
			if _, ok := keep[key]; !ok {
				stale = append(stale, key)
			}
		}
		if len(stale) != 0 {
			if err := s.redis.Del(ctx, stale...).Err(); err != nil {
				return fmt.Errorf("%w: delete stale Redis snapshots: %v", ErrUnavailable, err)
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (s *Service) AuditFailure(ctx context.Context, actor Actor, eventType, targetType, targetID string, cause error) {
	metadata, _ := json.Marshal(map[string]any{"targetType": targetType, "targetId": targetID, "error": cause.Error()})
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_audit_logs
		(event_type, principal_type, user_id, decision, reason_code, reason_message, metadata)
		VALUES ($1, 'admin', $2, 'error', $3, $4, $5::jsonb)
	`, eventType, actor.UserID, eventType+"_failed", cause.Error(), metadata)
	if err != nil && s.log != nil {
		s.log.Error("write service access failure audit", zap.Error(err))
	}
}
