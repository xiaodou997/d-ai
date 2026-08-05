package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"xiaodou/dai/internal/billing"
	billingpg "xiaodou/dai/internal/billing/pg"
	shared "xiaodou/dai/internal/domain"
)

const (
	LeaseEscrowActive   = "active"
	LeaseEscrowGrace    = "grace"
	LeaseEscrowReleased = "released"

	LeaseSettlementUnsettled = "unsettled"
	LeaseSettlementSettled   = "settled"

	defaultLeaseTTL   = 5 * time.Minute
	defaultLeaseGrace = 15 * time.Minute
	minimumLeaseTTL   = 30 * time.Second
	maximumLeaseTTL   = 30 * time.Minute
	minimumLeaseGrace = time.Minute
	maximumLeaseGrace = 24 * time.Hour
)

// CreditLeaseService owns the complete escrow lifecycle. Callers do not need
// to understand account locking, partial grants, late settlement, or reaping.
type CreditLeaseService struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
	now    func() time.Time
}

func NewCreditLeaseService(pool *pgxpool.Pool, logger *zap.Logger) *CreditLeaseService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CreditLeaseService{pool: pool, logger: logger, now: billing.NowUTC}
}

type AcquireLeaseParams struct {
	ClientID             string
	ClientWindowID       string
	TenantID             string
	UserID               string
	Description          string
	RequestedTenantMicro int64
	RequestedUserMicro   int64
	TTL                  time.Duration
	Grace                time.Duration
}

type RenewLeaseParams struct {
	LeaseID  string
	ClientID string
	Version  int64
	TTL      time.Duration
	Grace    time.Duration
}

type SettleLeaseParams struct {
	LeaseID           string
	ClientID          string
	SettlementID      string
	ActualTenantMicro int64
	ActualUserMicro   int64
}

type CreditLease struct {
	LeaseID            string
	ClientID           string
	ClientWindowID     string
	TenantID           string
	UserID             string
	GrantedTenantMicro int64
	GrantedUserMicro   int64
	EscrowState        string
	SettlementState    string
	Version            int64
	ExpiresAt          time.Time
	GraceUntil         time.Time
	SettlementID       string
	ActualTenantMicro  *int64
	ActualUserMicro    *int64
	SettledEventID     string
	SettledAt          *time.Time
	AccountState       string
	AllowFurtherUsage  bool
	TenantDeducted     int64
	UserDeducted       int64
	TenantDebtAdded    int64
	UserDebtAdded      int64
}

type leaseRow struct {
	CreditLease
	RequestedTenantMicro int64
	RequestedUserMicro   int64
	Description          string
	ReleasedAt           *time.Time
}

func (s *CreditLeaseService) Acquire(ctx context.Context, params AcquireLeaseParams) (*CreditLease, error) {
	params.TTL, params.Grace = normalizeLeaseDurations(params.TTL, params.Grace)
	if err := validateAcquireLease(params); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Serialise a client window before checking idempotency. This avoids
	// freezing the same account twice under concurrent retries.
	lockKey := params.ClientID + "\x1f" + params.ClientWindowID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return nil, fmt.Errorf("lock credit lease idempotency key: %w", err)
	}
	if existing, err := findLeaseByWindow(ctx, tx, params.ClientID, params.ClientWindowID, false); err == nil {
		if !sameAcquireLease(existing, params) {
			return nil, shared.NewErrorWithDetail(shared.ErrCreditLeaseSettlement.Code,
				shared.ErrCreditLeaseSettlement.Message, "client_window_id belongs to a different lease request")
		}
		return &existing.CreditLease, nil
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	if err := validateLeaseSubject(ctx, tx, params.ClientID, params.TenantID, params.UserID); err != nil {
		return nil, err
	}

	now := s.now()
	grantedTenant := params.RequestedTenantMicro
	grantedUser := params.RequestedUserMicro
	if grantedTenant > 0 {
		available, _, err := billingpg.GetTenantAvailableBalance(ctx, tx, params.TenantID, now)
		if err != nil {
			return nil, err
		}
		if current, err := currentTenantDebt(ctx, tx, params.TenantID); err != nil {
			return nil, err
		} else if current > 0 {
			return nil, shared.ErrTenantInOverdraft
		}
		if available < grantedTenant {
			grantedTenant = available
		}
		if grantedTenant <= 0 {
			return nil, shared.ErrTenantInsufficientBalance
		}
		if err := billingpg.AddTenantFrozen(ctx, tx, params.TenantID, grantedTenant); err != nil {
			return nil, err
		}
	}
	if grantedUser > 0 {
		available, _, err := billingpg.GetUserAvailableBalance(ctx, tx, params.UserID, now)
		if err != nil {
			return nil, err
		}
		if current, err := currentUserDebt(ctx, tx, params.UserID); err != nil {
			return nil, err
		} else if current > 0 {
			return nil, shared.ErrUserInOverdraft
		}
		if available < grantedUser {
			grantedUser = available
		}
		if grantedUser <= 0 {
			return nil, shared.ErrUserInsufficientBalance
		}
		if err := billingpg.AddUserFrozen(ctx, tx, params.UserID, grantedUser); err != nil {
			return nil, err
		}
	}

	accountState, allowFurtherUsage, err := currentEventState(ctx, tx, params.TenantID, params.UserID,
		grantedTenant > 0, grantedUser > 0)
	if err != nil {
		return nil, err
	}
	lease := CreditLease{
		LeaseID:            "CL_" + uuid.NewString(),
		ClientID:           params.ClientID,
		ClientWindowID:     params.ClientWindowID,
		TenantID:           params.TenantID,
		UserID:             params.UserID,
		GrantedTenantMicro: grantedTenant,
		GrantedUserMicro:   grantedUser,
		EscrowState:        LeaseEscrowActive,
		SettlementState:    LeaseSettlementUnsettled,
		Version:            1,
		ExpiresAt:          now.Add(params.TTL),
		GraceUntil:         now.Add(params.TTL + params.Grace),
		AccountState:       accountState,
		AllowFurtherUsage:  allowFurtherUsage,
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger_credit_leases (
			lease_id, client_id, client_window_id, tenant_id, user_id, description,
			requested_tenant_micro, requested_user_micro,
			granted_tenant_micro, granted_user_micro,
			escrow_state, settlement_state, version, expires_at, grace_until,
			account_state, allow_further_usage, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''),
			$7, $8, $9, $10, 'active', 'unsettled', 1, $11, $12, $13, $14, $15, $15
		)
	`, lease.LeaseID, lease.ClientID, lease.ClientWindowID, lease.TenantID, lease.UserID,
		params.Description, params.RequestedTenantMicro, params.RequestedUserMicro,
		lease.GrantedTenantMicro, lease.GrantedUserMicro, lease.ExpiresAt, lease.GraceUntil,
		lease.AccountState, lease.AllowFurtherUsage, now)
	if err != nil {
		return nil, fmt.Errorf("insert credit lease: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.logger.Info("credit lease acquired", zap.String("lease_id", lease.LeaseID),
		zap.String("tenant_id", lease.TenantID), zap.String("user_id", lease.UserID))
	return &lease, nil
}

func (s *CreditLeaseService) Renew(ctx context.Context, params RenewLeaseParams) (*CreditLease, error) {
	params.TTL, params.Grace = normalizeLeaseDurations(params.TTL, params.Grace)
	if params.LeaseID == "" || params.ClientID == "" || params.Version <= 0 {
		return nil, shared.NewErrorWithDetail(shared.ErrBadRequest.Code, shared.ErrBadRequest.Message,
			"lease_id, client_id and positive version are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	row, err := findLeaseByID(ctx, tx, params.LeaseID, true)
	if err == pgx.ErrNoRows {
		return nil, shared.ErrCreditLeaseNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.ClientID != params.ClientID {
		return nil, shared.ErrForbidden
	}
	if row.Version != params.Version {
		return nil, shared.ErrCreditLeaseVersion
	}
	now := s.now()
	if row.SettlementState == LeaseSettlementSettled || row.EscrowState == LeaseEscrowReleased || !now.Before(row.GraceUntil) {
		return nil, shared.ErrCreditLeaseNotRenewable
	}
	row.EscrowState = LeaseEscrowActive
	row.Version++
	row.ExpiresAt = now.Add(params.TTL)
	row.GraceUntil = row.ExpiresAt.Add(params.Grace)
	tag, err := tx.Exec(ctx, `
		UPDATE ledger_credit_leases
		SET escrow_state='active', version=$1, expires_at=$2, grace_until=$3, updated_at=$4
		WHERE lease_id=$5 AND version=$6 AND escrow_state IN ('active', 'grace')
		  AND settlement_state='unsettled'
	`, row.Version, row.ExpiresAt, row.GraceUntil, now, row.LeaseID, params.Version)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, shared.ErrCreditLeaseVersion
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &row.CreditLease, nil
}

func (s *CreditLeaseService) Settle(ctx context.Context, params SettleLeaseParams) (*CreditLease, error) {
	if params.LeaseID == "" || params.ClientID == "" || params.SettlementID == "" {
		return nil, shared.NewErrorWithDetail(shared.ErrBadRequest.Code, shared.ErrBadRequest.Message,
			"lease_id, client_id and settlement_id are required")
	}
	if params.ActualTenantMicro < 0 || params.ActualUserMicro < 0 {
		return nil, shared.ErrInvalidAmount
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// settlement_id is the global idempotency key, not merely a per-lease
	// attribute. Serialising it here turns concurrent cross-lease reuse into a
	// deterministic protocol conflict instead of a database unique violation.
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"credit-lease-settlement\x1f"+params.SettlementID); err != nil {
		return nil, fmt.Errorf("lock credit lease settlement key: %w", err)
	}
	row, err := findLeaseByID(ctx, tx, params.LeaseID, true)
	if err == pgx.ErrNoRows {
		return nil, shared.ErrCreditLeaseNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.ClientID != params.ClientID {
		return nil, shared.ErrForbidden
	}
	var settlementLeaseID string
	err = tx.QueryRow(ctx, `
		SELECT lease_id FROM ledger_credit_leases WHERE settlement_id=$1
	`, params.SettlementID).Scan(&settlementLeaseID)
	if err == nil && settlementLeaseID != params.LeaseID {
		return nil, shared.NewErrorWithDetail(shared.ErrCreditLeaseSettlement.Code,
			shared.ErrCreditLeaseSettlement.Message, "settlement_id already belongs to another lease")
	}
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	if row.SettlementState == LeaseSettlementSettled {
		if row.SettlementID != params.SettlementID || row.ActualTenantMicro == nil ||
			row.ActualUserMicro == nil || *row.ActualTenantMicro != params.ActualTenantMicro ||
			*row.ActualUserMicro != params.ActualUserMicro {
			return nil, shared.ErrCreditLeaseSettlement
		}
		return &row.CreditLease, nil
	}
	if params.ActualTenantMicro > 0 && row.GrantedTenantMicro == 0 {
		return nil, shared.NewErrorWithDetail(shared.ErrCreditLeaseSettlement.Code,
			shared.ErrCreditLeaseSettlement.Message, "tenant amount was not admitted by this lease")
	}
	if params.ActualUserMicro > 0 && row.GrantedUserMicro == 0 {
		return nil, shared.NewErrorWithDetail(shared.ErrCreditLeaseSettlement.Code,
			shared.ErrCreditLeaseSettlement.Message, "user amount was not admitted by this lease")
	}

	now := s.now()
	escrowWasReleased := row.EscrowState == LeaseEscrowReleased
	if !escrowWasReleased {
		if row.GrantedTenantMicro > 0 {
			if err := billingpg.ReduceTenantFrozen(ctx, tx, row.TenantID, row.GrantedTenantMicro); err != nil {
				return nil, err
			}
		}
		if row.GrantedUserMicro > 0 {
			if err := billingpg.ReduceUserFrozen(ctx, tx, row.UserID, row.GrantedUserMicro); err != nil {
				return nil, err
			}
		}
	}

	result := &row.CreditLease
	result.TenantDeducted, result.TenantDebtAdded, err = settleLeaseAmount(
		ctx, tx, billing.PackageTypeTenant, row.TenantID, "", params.ActualTenantMicro, now)
	if err != nil {
		return nil, err
	}
	result.UserDeducted, result.UserDebtAdded, err = settleLeaseAmount(
		ctx, tx, billing.PackageTypeUser, row.TenantID, row.UserID, params.ActualUserMicro, now)
	if err != nil {
		return nil, err
	}

	eventID := ""
	if params.ActualTenantMicro > 0 || params.ActualUserMicro > 0 {
		eventID = "EV_" + uuid.NewString()
		metadata, _ := json.Marshal(settlementMetadata{
			Mode:               "credit_lease",
			TenantDeducted:     result.TenantDeducted,
			UserDeducted:       result.UserDeducted,
			TenantOverdraftAdd: result.TenantDebtAdded,
			UserOverdraftAdd:   result.UserDebtAdded,
		})
		_, err = tx.Exec(ctx, `
			INSERT INTO bill_events (
				event_id, idempotency_key, tenant_id, user_id, client_id, description,
				event_type, tenant_credits, user_credits, status, metadata, created_at, finished_at
			) VALUES (
				$1, $2, $3, NULLIF($4, ''), $5, 'Credit lease settlement',
				'charge', NULLIF($6, 0), NULLIF($7, 0), 'succeeded', $8::jsonb, $9, $9
			)
		`, eventID, "credit-lease:"+params.SettlementID, row.TenantID, row.UserID,
			row.ClientID, params.ActualTenantMicro, params.ActualUserMicro, metadata, now)
		if err != nil {
			return nil, fmt.Errorf("insert lease settlement event: %w", err)
		}
	}

	result.AccountState, result.AllowFurtherUsage, err = currentEventState(ctx, tx, row.TenantID, row.UserID,
		row.GrantedTenantMicro > 0, row.GrantedUserMicro > 0)
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE ledger_credit_leases
		SET escrow_state='released', settlement_state='settled', version=version+1,
		    settlement_id=$1, actual_tenant_micro=$2, actual_user_micro=$3,
		    settled_event_id=NULLIF($4, ''), settled_at=$5,
		    tenant_deducted_micro=$6, user_deducted_micro=$7,
		    tenant_debt_added_micro=$8, user_debt_added_micro=$9,
		    account_state=$10, allow_further_usage=$11,
		    released_at=COALESCE(released_at, $5), updated_at=$5
		WHERE lease_id=$12 AND settlement_state='unsettled'
	`, params.SettlementID, params.ActualTenantMicro, params.ActualUserMicro, eventID, now,
		result.TenantDeducted, result.UserDeducted, result.TenantDebtAdded, result.UserDebtAdded,
		result.AccountState, result.AllowFurtherUsage, row.LeaseID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, shared.ErrCreditLeaseSettlement
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	result.EscrowState = LeaseEscrowReleased
	result.SettlementState = LeaseSettlementSettled
	result.Version = row.Version + 1
	result.SettlementID = params.SettlementID
	result.ActualTenantMicro = int64Ptr(params.ActualTenantMicro)
	result.ActualUserMicro = int64Ptr(params.ActualUserMicro)
	result.SettledEventID = eventID
	result.SettledAt = &now
	s.logger.Info("credit lease settled", zap.String("lease_id", row.LeaseID),
		zap.String("settlement_id", params.SettlementID), zap.String("event_id", eventID),
		zap.Bool("late_after_escrow_release", escrowWasReleased),
		zap.Int64("tenant_debt_added_micro", result.TenantDebtAdded),
		zap.Int64("user_debt_added_micro", result.UserDebtAdded))
	return result, nil
}

func (s *CreditLeaseService) Get(ctx context.Context, leaseID, clientID string) (*CreditLease, error) {
	row, err := findLeaseByID(ctx, s.pool, leaseID, false)
	if err == pgx.ErrNoRows {
		return nil, shared.ErrCreditLeaseNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.ClientID != clientID {
		return nil, shared.ErrForbidden
	}
	return &row.CreditLease, nil
}

// ReapExpired advances active leases to grace and releases escrow only after
// grace expires. Settlement remains legal after release.
func (s *CreditLeaseService) ReapExpired(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	now := s.now()
	tag, err := s.pool.Exec(ctx, `
		UPDATE ledger_credit_leases
		SET escrow_state='grace', version=version+1, updated_at=$1
		WHERE lease_id IN (
			SELECT lease_id FROM ledger_credit_leases
			WHERE escrow_state='active' AND settlement_state='unsettled' AND expires_at <= $1
			ORDER BY expires_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
	`, now, limit)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() > 0 {
		s.logger.Info("credit leases entered grace",
			zap.Int64("count", tag.RowsAffected()))
	}

	rows, err := s.pool.Query(ctx, `
		SELECT lease_id FROM ledger_credit_leases
		WHERE escrow_state='grace' AND settlement_state='unsettled' AND grace_until <= $1
		ORDER BY grace_until
		LIMIT $2
	`, now, limit)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	released := 0
	for _, id := range ids {
		ok, err := s.releaseExpired(ctx, id, now)
		if err != nil {
			return released, err
		}
		if ok {
			released++
		}
	}
	return released, nil
}

func (s *CreditLeaseService) releaseExpired(ctx context.Context, leaseID string, now time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	row, err := findLeaseByID(ctx, tx, leaseID, true)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.EscrowState != LeaseEscrowGrace || row.SettlementState != LeaseSettlementUnsettled || now.Before(row.GraceUntil) {
		return false, nil
	}
	if row.GrantedTenantMicro > 0 {
		if err := billingpg.ReduceTenantFrozen(ctx, tx, row.TenantID, row.GrantedTenantMicro); err != nil {
			return false, err
		}
	}
	if row.GrantedUserMicro > 0 {
		if err := billingpg.ReduceUserFrozen(ctx, tx, row.UserID, row.GrantedUserMicro); err != nil {
			return false, err
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE ledger_credit_leases
		SET escrow_state='released', version=version+1, released_at=$1, updated_at=$1
		WHERE lease_id=$2 AND escrow_state='grace' AND settlement_state='unsettled' AND version=$3
	`, now, leaseID, row.Version)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	return true, tx.Commit(ctx)
}

type leaseQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func findLeaseByID(ctx context.Context, q leaseQuerier, leaseID string, forUpdate bool) (*leaseRow, error) {
	query := leaseSelectSQL + ` WHERE lease_id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanLease(q.QueryRow(ctx, query, leaseID))
}

func findLeaseByWindow(ctx context.Context, q leaseQuerier, clientID, windowID string, forUpdate bool) (*leaseRow, error) {
	query := leaseSelectSQL + ` WHERE client_id=$1 AND client_window_id=$2`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanLease(q.QueryRow(ctx, query, clientID, windowID))
}

const leaseSelectSQL = `
	SELECT lease_id, client_id, client_window_id, tenant_id, COALESCE(user_id, ''),
	       COALESCE(description, ''), requested_tenant_micro, requested_user_micro,
	       granted_tenant_micro, granted_user_micro, escrow_state, settlement_state,
	       version, expires_at, grace_until, COALESCE(settlement_id, ''),
	       actual_tenant_micro, actual_user_micro, COALESCE(settled_event_id, ''),
	       settled_at, released_at, tenant_deducted_micro, user_deducted_micro,
	       tenant_debt_added_micro, user_debt_added_micro, account_state,
	       allow_further_usage
	FROM ledger_credit_leases`

func scanLease(row pgx.Row) (*leaseRow, error) {
	var result leaseRow
	err := row.Scan(&result.LeaseID, &result.ClientID, &result.ClientWindowID,
		&result.TenantID, &result.UserID, &result.Description,
		&result.RequestedTenantMicro, &result.RequestedUserMicro,
		&result.GrantedTenantMicro, &result.GrantedUserMicro,
		&result.EscrowState, &result.SettlementState, &result.Version,
		&result.ExpiresAt, &result.GraceUntil, &result.SettlementID,
		&result.ActualTenantMicro, &result.ActualUserMicro, &result.SettledEventID,
		&result.SettledAt, &result.ReleasedAt, &result.TenantDeducted,
		&result.UserDeducted, &result.TenantDebtAdded, &result.UserDebtAdded,
		&result.AccountState, &result.AllowFurtherUsage)
	return &result, err
}

func validateAcquireLease(params AcquireLeaseParams) error {
	if params.ClientID == "" || params.ClientWindowID == "" || params.TenantID == "" {
		return shared.NewErrorWithDetail(shared.ErrBadRequest.Code, shared.ErrBadRequest.Message,
			"client_id, client_window_id and tenant_id are required")
	}
	if params.RequestedTenantMicro < 0 || params.RequestedUserMicro < 0 ||
		(params.RequestedTenantMicro == 0 && params.RequestedUserMicro == 0) {
		return shared.ErrInvalidAmount
	}
	if params.RequestedUserMicro > 0 && params.UserID == "" {
		return shared.NewErrorWithDetail(shared.ErrBadRequest.Code, shared.ErrBadRequest.Message,
			"user_id is required when requested_user_micro is positive")
	}
	return nil
}

func validateLeaseSubject(ctx context.Context, tx pgx.Tx, clientID, tenantID, userID string) error {
	var tenantStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(status, 'active') FROM iam_tenants WHERE tenant_id=$1`, tenantID).Scan(&tenantStatus); err != nil {
		return shared.ErrAccountNotFound
	}
	if tenantStatus != "active" {
		return shared.ErrTenantSuspended
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `
		SELECT c.status='active' AND c.portal_enabled
		   AND (p.access_mode='all' OR (p.access_mode='selected' AND c.client_id=ANY(p.service_ids)))
		FROM gov_subject_service_access p
		JOIN gov_clients c ON c.client_id=$2
		WHERE p.subject_type='tenant' AND p.subject_id=$1
	`, tenantID, clientID).Scan(&allowed); err != nil || !allowed {
		return shared.ErrForbidden
	}
	if userID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM iam_users
				WHERE user_id=$1 AND tenant_id=$2 AND status='active'
			)
		`, userID, tenantID).Scan(&exists); err != nil || !exists {
			return shared.ErrAccountNotFound
		}
	}
	return nil
}

func settleLeaseAmount(ctx context.Context, tx pgx.Tx, packageType, tenantID, userID string, amount int64, now time.Time) (int64, int64, error) {
	if amount <= 0 {
		return 0, 0, nil
	}
	// The lease's own escrow has already been released by Settle. The frozen
	// balance that remains belongs to other leases and must not be consumed by
	// this settlement, including a late settlement after escrow expiry.
	shortfall, err := billingpg.DeductFIFOPartialPreservingFrozen(
		ctx, tx, packageType, tenantID, userID, amount, 0, now)
	if err != nil {
		return 0, 0, err
	}
	deducted := amount - shortfall
	if shortfall == 0 {
		return deducted, 0, nil
	}
	if packageType == billing.PackageTypeTenant {
		err = billingpg.IncreaseTenantOverdraft(ctx, tx, tenantID, shortfall)
	} else {
		err = billingpg.IncreaseUserOverdraft(ctx, tx, userID, shortfall)
	}
	return deducted, shortfall, err
}

func currentTenantDebt(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error) {
	var debt int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(current_overdraft, 0) FROM iam_tenants WHERE tenant_id=$1 FOR UPDATE`, tenantID).Scan(&debt)
	return debt, err
}

func currentUserDebt(ctx context.Context, tx pgx.Tx, userID string) (int64, error) {
	var debt int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(current_overdraft, 0) FROM iam_users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&debt)
	return debt, err
}

func normalizeLeaseDurations(ttl, grace time.Duration) (time.Duration, time.Duration) {
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	if ttl < minimumLeaseTTL {
		ttl = minimumLeaseTTL
	}
	if ttl > maximumLeaseTTL {
		ttl = maximumLeaseTTL
	}
	if grace <= 0 {
		grace = defaultLeaseGrace
	}
	if grace < minimumLeaseGrace {
		grace = minimumLeaseGrace
	}
	if grace > maximumLeaseGrace {
		grace = maximumLeaseGrace
	}
	return ttl, grace
}

func sameAcquireLease(existing *leaseRow, params AcquireLeaseParams) bool {
	return existing.ClientID == params.ClientID &&
		existing.ClientWindowID == params.ClientWindowID &&
		existing.TenantID == params.TenantID &&
		existing.UserID == params.UserID &&
		existing.Description == params.Description &&
		existing.RequestedTenantMicro == params.RequestedTenantMicro &&
		existing.RequestedUserMicro == params.RequestedUserMicro
}

func int64Ptr(v int64) *int64 { return &v }
