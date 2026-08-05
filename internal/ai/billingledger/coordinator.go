package billingledger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"xiaodou/dai/internal/ai/urm"
)

type window struct {
	ID                   string
	OwnerType            string
	TenantID             string
	UserID               string
	WantTenant           bool
	WantUser             bool
	LeaseID              string
	LeaseVersion         int64
	RequestedTenantMicro int64
	RequestedUserMicro   int64
	GrantedTenantMicro   int64
	GrantedUserMicro     int64
	AccruedTenantMicro   int64
	AccruedUserMicro     int64
	State                string
	ExpiresAt            *time.Time
	GraceUntil           *time.Time
	MaxAgeAt             time.Time
}

func (c *Coordinator) Admit(ctx context.Context, intent Intent) (Admission, error) {
	if c == nil || c.pool == nil || c.port == nil {
		return Admission{}, ErrDependencyUnavailable
	}
	if intent.RequestTTL <= 0 {
		intent.RequestTTL = time.Minute
	}
	if err := validateIntent(intent); err != nil {
		return Admission{}, err
	}
	if !intent.WantTenant && !intent.WantUser {
		return Admission{RequestID: intent.RequestID}, nil
	}
	if existing, err := c.readAdmission(ctx, intent.RequestID); err == nil {
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Admission{}, err
	}

	for attempt := 0; attempt < 4; attempt++ {
		win, err := c.selectOrCreateWindow(ctx, intent)
		if err != nil {
			return Admission{}, err
		}
		switch win.State {
		case "opening":
			if err := c.activateWindow(ctx, win); err != nil {
				return Admission{}, err
			}
			continue
		case "active":
			if win.ExpiresAt == nil || win.ExpiresAt.Before(c.now().Add(c.config.AdmissionHeadroom)) {
				if err := c.renewWindow(ctx, win); err != nil {
					_ = c.beginDrain(context.WithoutCancel(ctx), win.ID, "lease_renew_failed", err.Error())
					continue
				}
				continue
			}
			admission, admitted, err := c.insertAdmission(ctx, win, intent)
			if err != nil {
				return Admission{}, err
			}
			if admitted {
				return admission, nil
			}
		default:
			continue
		}
	}
	return Admission{}, fmt.Errorf("%w: unable to obtain an active credit lease", ErrAdmissionConflict)
}

// Complete joins request completion to the usage/quota transaction supplied by
// the caller. No remote call occurs while the transaction is open.
func (c *Coordinator) Complete(ctx context.Context, tx pgx.Tx, completion Completion) error {
	if c == nil || c.pool == nil {
		return ErrDependencyUnavailable
	}
	if completion.RequestID == "" {
		return fmt.Errorf("%w: completion request_id is required", ErrProtocolViolation)
	}
	if completion.TenantMicro < 0 || completion.UserMicro < 0 {
		return fmt.Errorf("%w: negative completion amount", ErrProtocolViolation)
	}
	var windowID, leaseID, status string
	var recordedTenantMicro, recordedUserMicro *int64
	if err := tx.QueryRow(ctx, `
		SELECT window_id, lease_id, status, actual_tenant_micro, actual_user_micro
		FROM ai_billing_request_admissions
		WHERE request_id=$1
		FOR UPDATE
	`, completion.RequestID).Scan(&windowID, &leaseID, &status,
		&recordedTenantMicro, &recordedUserMicro); err != nil {
		if errors.Is(err, pgx.ErrNoRows) && completion.TenantMicro == 0 && completion.UserMicro == 0 {
			return nil
		}
		return fmt.Errorf("%w: request admission missing: %v", ErrProtocolViolation, err)
	}
	if completion.ExpectedLeaseID != "" && completion.ExpectedLeaseID != leaseID {
		return fmt.Errorf("%w: completion lease does not match request admission", ErrProtocolViolation)
	}
	if status == "completed" {
		if recordedTenantMicro == nil || recordedUserMicro == nil ||
			*recordedTenantMicro != completion.TenantMicro ||
			*recordedUserMicro != completion.UserMicro {
			return fmt.Errorf("%w: completed request amount changed", ErrProtocolViolation)
		}
		return nil
	}
	if status != "active" && status != "reconciling" {
		return fmt.Errorf("%w: admission has unsupported status %q", ErrProtocolViolation, status)
	}
	if completion.Source == "" {
		completion.Source = "runtime"
	}
	if completion.Source != "runtime" && completion.Source != "manual" {
		return fmt.Errorf("%w: unsupported completion source", ErrProtocolViolation)
	}
	now := c.now()
	tag, err := tx.Exec(ctx, `
		UPDATE ai_billing_request_admissions
		SET status='completed', actual_tenant_micro=$1, actual_user_micro=$2,
		    completion_source=$3, completion_note=NULLIF($4,''),
		    completed_at=$5, updated_at=$5
		WHERE request_id=$6 AND status IN ('active','reconciling')
	`, completion.TenantMicro, completion.UserMicro, completion.Source,
		completion.Note, now, completion.RequestID)
	if err != nil {
		return fmt.Errorf("complete billing admission: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: admission changed during completion", ErrAdmissionConflict)
	}
	tag, err = tx.Exec(ctx, `
		UPDATE ai_billing_windows
		SET accrued_tenant_micro=accrued_tenant_micro+$1,
		    accrued_user_micro=accrued_user_micro+$2,
		    state=CASE
		      WHEN state='reconciling' AND NOT EXISTS (
		        SELECT 1 FROM ai_billing_request_admissions
		        WHERE window_id=$4 AND status IN ('active','reconciling')
		      ) THEN 'draining'
		      WHEN state='active' AND (
		        max_age_at <= $3
		        OR (granted_tenant_micro > 0 AND accrued_tenant_micro+$1 >= (granted_tenant_micro*3)/4)
		        OR (granted_user_micro > 0 AND accrued_user_micro+$2 >= (granted_user_micro*3)/4)
		      ) THEN 'draining'
		      ELSE state
		    END,
		    updated_at=$3
		WHERE window_id=$4 AND lease_id=$5
		  AND state IN ('active', 'draining', 'reconciling')
	`, completion.TenantMicro, completion.UserMicro, now, windowID, leaseID)
	if err != nil {
		return fmt.Errorf("accrue billing window: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: billing window is no longer completable", ErrProtocolViolation)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_usage_logs
		SET billing_window_id=$1, urm_transaction_id=$2
		WHERE request_id=$3
	`, windowID, leaseID, completion.RequestID); err != nil {
		return fmt.Errorf("link usage to billing window: %w", err)
	}
	return nil
}

func (c *Coordinator) selectOrCreateWindow(ctx context.Context, intent Intent) (*window, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	ownerKey := intent.OwnerType + "\x1f" + intent.TenantID + "\x1f" + intent.UserID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, ownerKey); err != nil {
		return nil, err
	}
	win, err := findOpenWindow(ctx, tx, intent.OwnerType, intent.TenantID, intent.UserID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	now := c.now()
	if err == nil {
		// An opening window is a durable idempotency anchor for the remote
		// Acquire call. It must be activated (or explicitly failed) before any
		// later intent can rotate it; draining an opening window would create a
		// local row that can neither admit nor settle.
		if win.State == "opening" {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return win, nil
		}
		incompatible := (intent.WantTenant && !win.WantTenant) || (intent.WantUser && !win.WantUser)
		rotate := incompatible || win.MaxAgeAt.Before(now) || windowCapacityReached(win)
		if !rotate {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return win, nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ai_billing_windows
			SET state='draining', updated_at=$1
			WHERE window_id=$2 AND state IN ('opening', 'active')
		`, now, win.ID); err != nil {
			return nil, err
		}
	}

	win = &window{
		ID:                   "bw_" + uuid.NewString(),
		OwnerType:            intent.OwnerType,
		TenantID:             intent.TenantID,
		UserID:               intent.UserID,
		WantTenant:           intent.WantTenant,
		WantUser:             intent.WantUser,
		RequestedTenantMicro: boolAmount(intent.WantTenant, c.config.RequestedTenantMicro),
		RequestedUserMicro:   boolAmount(intent.WantUser, c.config.RequestedUserMicro),
		State:                "opening",
		MaxAgeAt:             now.Add(c.config.WindowMaxAge),
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_billing_windows (
			window_id, owner_type, tenant_id, user_id, want_tenant, want_user,
			requested_tenant_micro, requested_user_micro, state, max_age_at,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'opening',$9,$10,$10)
	`, win.ID, win.OwnerType, win.TenantID, win.UserID, win.WantTenant, win.WantUser,
		win.RequestedTenantMicro, win.RequestedUserMicro, win.MaxAgeAt, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return win, nil
}

func (c *Coordinator) activateWindow(ctx context.Context, win *window) error {
	res, err := c.port.AcquireCreditLease(ctx, urm.AcquireCreditLeaseRequest{
		ClientWindowID:       win.ID,
		TenantID:             win.TenantID,
		UserID:               win.UserID,
		Description:          fmt.Sprintf("ai billing window owner=%s window=%s", win.OwnerType, win.ID),
		RequestedTenantMicro: win.RequestedTenantMicro,
		RequestedUserMicro:   win.RequestedUserMicro,
		TTLSeconds:           int64(c.config.LeaseTTL / time.Second),
		GraceSeconds:         int64(c.config.LeaseGrace / time.Second),
	})
	if err != nil {
		_ = c.recordWindowError(context.WithoutCancel(ctx), win.ID, portErrorCode(err), err.Error())
		c.observeLeaseOperation("acquire", portErrorCode(err))
		return classifyPortError(err)
	}
	res, state, err := c.prepareOpeningLease(ctx, win, res)
	if err != nil {
		code := portErrorCode(err)
		if errors.Is(err, ErrProtocolViolation) {
			code = "protocol_violation"
		}
		_ = c.recordWindowError(context.WithoutCancel(ctx), win.ID, code, err.Error())
		c.observeLeaseOperation("acquire", code)
		return err
	}
	tag, err := c.pool.Exec(ctx, `
		UPDATE ai_billing_windows
		SET lease_id=$1, lease_version=$2,
		    granted_tenant_micro=$3, granted_user_micro=$4,
		    state=$5, expires_at=$6, grace_until=$7,
		    opened_at=COALESCE(opened_at,$8), updated_at=$8,
		    last_error_code=NULL, last_error_detail=NULL
		WHERE window_id=$9 AND state='opening'
	`, res.LeaseID, res.Version, res.GrantedTenantMicro, res.GrantedUserMicro,
		state, res.ExpiresAt, res.GraceUntil, c.now(), win.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		current, readErr := c.readWindow(ctx, win.ID)
		if readErr == nil && current.LeaseID == res.LeaseID && current.State == state {
			return nil
		}
		return fmt.Errorf("%w: provisional window changed during activation", ErrAdmissionConflict)
	}
	if state == "draining" {
		c.observeLeaseOperation("acquire", "recovered_released")
	} else {
		c.observeLeaseOperation("acquire", "success")
	}
	c.Trigger()
	return nil
}

func (c *Coordinator) prepareOpeningLease(
	ctx context.Context,
	win *window,
	res *urm.CreditLeaseResponse,
) (*urm.CreditLeaseResponse, string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if err := validateOpeningLeaseIdentity(win, res); err != nil {
			return nil, "", err
		}
		now := c.now()
		switch {
		case res.EscrowState == "released":
			return res, "draining", nil
		case res.EscrowState == "active" && res.ExpiresAt.After(now):
			return res, "active", nil
		case (res.EscrowState == "active" || res.EscrowState == "grace") &&
			res.GraceUntil.After(now):
			previousVersion := res.Version
			renewed, renewErr := c.port.RenewCreditLease(ctx, res.LeaseID, urm.RenewCreditLeaseRequest{
				Version:      previousVersion,
				TTLSeconds:   int64(c.config.LeaseTTL / time.Second),
				GraceSeconds: int64(c.config.LeaseGrace / time.Second),
			})
			if renewErr == nil {
				c.observeLeaseOperation("renew", "opening_recovery")
				res = renewed
				continue
			}
			current, getErr := c.port.GetCreditLease(ctx, res.LeaseID)
			if getErr != nil {
				c.observeLeaseOperation("renew", portErrorCode(renewErr))
				return nil, "", classifyPortError(errors.Join(renewErr, getErr))
			}
			if current.Version <= previousVersion &&
				current.EscrowState == res.EscrowState &&
				current.SettlementState == res.SettlementState {
				c.observeLeaseOperation("renew", portErrorCode(renewErr))
				return nil, "", classifyPortError(renewErr)
			}
			c.observeLeaseOperation("renew", "opening_reconciled")
			res = current
		default:
			return nil, "", fmt.Errorf("%w: opening credit lease is no longer recoverable", ErrProtocolViolation)
		}
	}
	return nil, "", fmt.Errorf("%w: opening credit lease recovery did not converge", ErrProtocolViolation)
}

func (c *Coordinator) renewWindow(ctx context.Context, win *window) error {
	res, err := c.port.RenewCreditLease(ctx, win.LeaseID, urm.RenewCreditLeaseRequest{
		Version:      win.LeaseVersion,
		TTLSeconds:   int64(c.config.LeaseTTL / time.Second),
		GraceSeconds: int64(c.config.LeaseGrace / time.Second),
	})
	if err != nil {
		reconciled, reconcileErr := c.reconcileRenewFailure(ctx, win)
		if reconcileErr == nil {
			res, err = reconciled, nil
		} else if isConflictResponse(err) {
			err = reconcileErr
		}
	}
	if err != nil {
		_ = c.recordWindowError(context.WithoutCancel(ctx), win.ID, portErrorCode(err), err.Error())
		c.observeLeaseOperation("renew", portErrorCode(err))
		return classifyPortError(err)
	}
	if err := validateRenewResponse(win, res, c.now()); err != nil {
		_ = c.recordWindowError(context.WithoutCancel(ctx), win.ID, "protocol_violation", err.Error())
		c.observeLeaseOperation("renew", "protocol_violation")
		return err
	}
	tag, err := c.pool.Exec(ctx, `
		UPDATE ai_billing_windows
		SET lease_version=$1, expires_at=$2, grace_until=$3, updated_at=$4,
		    last_error_code=NULL, last_error_detail=NULL
		WHERE window_id=$5 AND lease_id=$6 AND lease_version < $1
		  AND state IN ('active','draining')
	`, res.Version, res.ExpiresAt, res.GraceUntil, c.now(), win.ID, win.LeaseID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		current, readErr := c.readWindow(ctx, win.ID)
		if readErr == nil && current.LeaseID == res.LeaseID &&
			current.LeaseVersion >= res.Version && current.ExpiresAt != nil &&
			!current.ExpiresAt.Before(res.ExpiresAt) {
			c.observeLeaseOperation("renew", "concurrent_success")
			return nil
		}
		c.observeLeaseOperation("renew", "local_conflict")
		return ErrAdmissionConflict
	}
	c.observeLeaseOperation("renew", "success")
	return nil
}

func (c *Coordinator) reconcileRenewFailure(ctx context.Context, win *window) (*urm.CreditLeaseResponse, error) {
	current, err := c.port.GetCreditLease(ctx, win.LeaseID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.LeaseID != win.LeaseID || current.ClientWindowID != win.ID ||
		current.SettlementState != "unsettled" || current.EscrowState == "released" {
		return nil, fmt.Errorf("%w: remote lease cannot be renewed", ErrProtocolViolation)
	}
	if current.EscrowState == "active" && current.Version > win.LeaseVersion &&
		current.ExpiresAt.After(c.now().Add(c.config.AdmissionHeadroom)) {
		return current, nil
	}
	return c.port.RenewCreditLease(ctx, win.LeaseID, urm.RenewCreditLeaseRequest{
		Version:      current.Version,
		TTLSeconds:   int64(c.config.LeaseTTL / time.Second),
		GraceSeconds: int64(c.config.LeaseGrace / time.Second),
	})
}

func (c *Coordinator) insertAdmission(ctx context.Context, win *window, intent Intent) (Admission, bool, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return Admission{}, false, err
	}
	defer tx.Rollback(ctx)
	current, err := findWindowByID(ctx, tx, win.ID, true)
	if err != nil {
		return Admission{}, false, err
	}
	now := c.now()
	if current.State != "active" || current.LeaseID == "" || current.ExpiresAt == nil ||
		!current.ExpiresAt.After(now.Add(c.config.AdmissionHeadroom)) {
		return Admission{}, false, nil
	}
	expires := now.Add(intent.RequestTTL)
	tag, err := tx.Exec(ctx, `
		INSERT INTO ai_billing_request_admissions (
			request_id, window_id, lease_id, status, request_expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,'active',$4,$5,$5)
		ON CONFLICT (request_id) DO NOTHING
	`, intent.RequestID, current.ID, current.LeaseID, expires, now)
	if err != nil {
		return Admission{}, false, err
	}
	if tag.RowsAffected() == 0 {
		existing, err := readAdmissionWith(ctx, tx, intent.RequestID)
		if err != nil {
			return Admission{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Admission{}, false, err
		}
		return existing, true, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ai_billing_windows SET last_admitted_at=$1, updated_at=$1 WHERE window_id=$2
	`, now, current.ID); err != nil {
		return Admission{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Admission{}, false, err
	}
	return Admission{RequestID: intent.RequestID, WindowID: current.ID, LeaseID: current.LeaseID}, true, nil
}

func (c *Coordinator) readAdmission(ctx context.Context, requestID string) (Admission, error) {
	return readAdmissionWith(ctx, c.pool, requestID)
}

func readAdmissionWith(ctx context.Context, q querier, requestID string) (Admission, error) {
	var a Admission
	var status string
	err := q.QueryRow(ctx, `
		SELECT request_id, window_id, lease_id, status
		FROM ai_billing_request_admissions WHERE request_id=$1
	`, requestID).Scan(&a.RequestID, &a.WindowID, &a.LeaseID, &status)
	if err == nil && status != "active" {
		return Admission{}, fmt.Errorf("%w: request_id is not admissible (status=%s)", ErrAdmissionConflict, status)
	}
	return a, err
}

func validateIntent(intent Intent) error {
	if intent.RequestID == "" || intent.OwnerType == "" || intent.TenantID == "" {
		return fmt.Errorf("%w: request_id, owner_type and tenant_id are required", ErrProtocolViolation)
	}
	if intent.OwnerType != "tenant" && intent.OwnerType != "user" {
		return fmt.Errorf("%w: unsupported owner_type", ErrProtocolViolation)
	}
	if intent.WantUser && intent.UserID == "" {
		return fmt.Errorf("%w: user_id is required for user billing", ErrProtocolViolation)
	}
	return nil
}

func findOpenWindow(ctx context.Context, q querier, ownerType, tenantID, userID string) (*window, error) {
	return scanWindow(q.QueryRow(ctx, windowSelectSQL+`
		WHERE owner_type=$1 AND tenant_id=$2 AND user_id=$3
		  AND state IN ('opening','active')
		ORDER BY created_at DESC LIMIT 1
	`, ownerType, tenantID, userID))
}

func findWindowByID(ctx context.Context, q querier, windowID string, forUpdate bool) (*window, error) {
	query := windowSelectSQL + ` WHERE window_id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	return scanWindow(q.QueryRow(ctx, query, windowID))
}

func (c *Coordinator) readWindow(ctx context.Context, windowID string) (*window, error) {
	return findWindowByID(ctx, c.pool, windowID, false)
}

const windowSelectSQL = `
	SELECT window_id, owner_type, tenant_id, user_id, want_tenant, want_user,
	       COALESCE(lease_id,''), lease_version,
	       requested_tenant_micro, requested_user_micro,
	       granted_tenant_micro, granted_user_micro,
	       accrued_tenant_micro, accrued_user_micro, state,
	       expires_at, grace_until, max_age_at
	FROM ai_billing_windows`

func scanWindow(row pgx.Row) (*window, error) {
	var win window
	err := row.Scan(&win.ID, &win.OwnerType, &win.TenantID, &win.UserID,
		&win.WantTenant, &win.WantUser, &win.LeaseID, &win.LeaseVersion,
		&win.RequestedTenantMicro, &win.RequestedUserMicro,
		&win.GrantedTenantMicro, &win.GrantedUserMicro,
		&win.AccruedTenantMicro, &win.AccruedUserMicro, &win.State,
		&win.ExpiresAt, &win.GraceUntil, &win.MaxAgeAt)
	return &win, err
}

func (c *Coordinator) beginDrain(ctx context.Context, windowID, code, detail string) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE ai_billing_windows
		SET state='draining', last_error_code=NULLIF($1,''), last_error_detail=NULLIF($2,''), updated_at=$3
		WHERE window_id=$4 AND state='active'
	`, code, detail, c.now(), windowID)
	c.Trigger()
	return err
}

func (c *Coordinator) recordWindowError(ctx context.Context, windowID, code, detail string) error {
	_, err := c.pool.Exec(ctx, `
		UPDATE ai_billing_windows
		SET last_error_code=$1, last_error_detail=$2, updated_at=$3
		WHERE window_id=$4
	`, code, detail, c.now(), windowID)
	return err
}

func windowCapacityReached(win *window) bool {
	return (win.GrantedTenantMicro > 0 && win.AccruedTenantMicro >= (win.GrantedTenantMicro*3)/4) ||
		(win.GrantedUserMicro > 0 && win.AccruedUserMicro >= (win.GrantedUserMicro*3)/4)
}

func validateOpeningLeaseIdentity(win *window, res *urm.CreditLeaseResponse) error {
	if res == nil || res.LeaseID == "" || res.ClientWindowID != win.ID ||
		res.TenantID != win.TenantID || res.UserID != win.UserID ||
		res.Version <= 0 || res.SettlementState != "unsettled" ||
		(win.WantTenant && res.GrantedTenantMicro <= 0) ||
		(win.WantUser && res.GrantedUserMicro <= 0) ||
		(!win.WantTenant && res.GrantedTenantMicro != 0) ||
		(!win.WantUser && res.GrantedUserMicro != 0) ||
		res.GrantedTenantMicro > win.RequestedTenantMicro ||
		res.GrantedUserMicro > win.RequestedUserMicro ||
		!res.GraceUntil.After(res.ExpiresAt) {
		return fmt.Errorf("%w: invalid credit lease response", ErrProtocolViolation)
	}
	return nil
}

func validateRenewResponse(win *window, res *urm.CreditLeaseResponse, now time.Time) error {
	if res == nil || res.LeaseID != win.LeaseID || res.ClientWindowID != win.ID ||
		res.TenantID != win.TenantID || res.UserID != win.UserID ||
		res.GrantedTenantMicro != win.GrantedTenantMicro ||
		res.GrantedUserMicro != win.GrantedUserMicro ||
		res.Version <= win.LeaseVersion || res.EscrowState != "active" ||
		res.SettlementState != "unsettled" || !res.ExpiresAt.After(now) ||
		!res.GraceUntil.After(res.ExpiresAt) {
		return fmt.Errorf("%w: invalid credit lease renewal response", ErrProtocolViolation)
	}
	return nil
}

func boolAmount(enabled bool, amount int64) int64 {
	if enabled {
		return amount
	}
	return 0
}

func portErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrProtocolViolation):
		return "protocol_violation"
	case errors.Is(err, ErrAdmissionConflict):
		return "admission_conflict"
	case errors.Is(err, ErrInsufficientBalance):
		return "insufficient_balance"
	}
	var apiErr *urm.APIError
	if errors.As(err, &apiErr) && apiErr.Code != "" {
		return apiErr.Code
	}
	return "dependency_unavailable"
}

func isConflictResponse(err error) bool {
	var apiErr *urm.APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict
}
