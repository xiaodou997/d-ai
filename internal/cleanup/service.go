// Package cleanup owns the database data lifecycle policy and its background
// execution. It deliberately excludes billing facts, ledgers, users, and
// configuration from the cleanup targets.
package cleanup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	TargetRequestBody     = "request_body"
	TargetRequestPayloads = "request_payloads"
	TargetNotifications   = "notifications"
	TargetModerationLogs  = "moderation_logs"
	TargetRiskEvents      = "risk_events"
	TargetAdminAuditLogs  = "admin_audit_logs"
	TargetAuditBlobs      = "audit_blobs"
	TargetUsageRollups    = "usage_rollups"

	ConfirmationPhrase = "CLEANUP_DATA"
	policyKey          = "data_cleanup"
	cleanupRunLimit    = 30
)

var (
	ErrAlreadyRunning = errors.New("data cleanup is already running")
	ErrInvalidTarget  = errors.New("invalid data cleanup target")
	archiveTableName  = regexp.MustCompile(`^ai_request_payloads_archive_[0-9]{4}_[0-9]{2}$`)
)

// Policy is stored in sys_settings.data_cleanup. All values are days except
// BatchSize, which is the maximum number of rows handled in one transaction.
type Policy struct {
	Enabled            bool `json:"enabled"`
	RequestBodyDays    int  `json:"requestBodyDays"`
	RequestPayloadDays int  `json:"requestPayloadDays"`
	NotificationDays   int  `json:"notificationDays"`
	ModerationDays     int  `json:"moderationDays"`
	RiskEventDays      int  `json:"riskEventDays"`
	AdminAuditDays     int  `json:"adminAuditDays"`
	AuditBlobDays      int  `json:"auditBlobDays"`
	UsageRollupDays    int  `json:"usageRollupDays"`
	BatchSize          int  `json:"batchSize"`
}

// PreviewItem describes how many rows the current policy would remove or
// shrink. Body cleanup is reported separately from full payload deletion.
type PreviewItem struct {
	Target        string    `json:"target"`
	Label         string    `json:"label"`
	RetentionDays int       `json:"retentionDays"`
	Cutoff        time.Time `json:"cutoff"`
	EligibleRows  int64     `json:"eligibleRows"`
}

type Preview struct {
	Policy      Policy        `json:"policy"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Items       []PreviewItem `json:"items"`
}

type Run struct {
	ID          string           `json:"id"`
	Trigger     string           `json:"trigger"`
	Status      string           `json:"status"`
	RequestedBy string           `json:"requestedBy,omitempty"`
	Targets     []string         `json:"targets"`
	Summary     map[string]int64 `json:"summary"`
	Error       string           `json:"error,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
	StartedAt   *time.Time       `json:"startedAt,omitempty"`
	CompletedAt *time.Time       `json:"completedAt,omitempty"`
}

type Service struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
	root   context.Context
}

func NewService(pool *pgxpool.Pool, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{pool: pool, logger: logger, root: context.Background()}
}

func DefaultPolicy() Policy {
	return Policy{
		Enabled:            true,
		RequestBodyDays:    30,
		RequestPayloadDays: 180,
		NotificationDays:   90,
		ModerationDays:     90,
		RiskEventDays:      365,
		AdminAuditDays:     365,
		AuditBlobDays:      180,
		UsageRollupDays:    730,
		BatchSize:          1000,
	}
}

func AllTargets() []string {
	return []string{
		TargetRequestBody,
		TargetRequestPayloads,
		TargetNotifications,
		TargetModerationLogs,
		TargetRiskEvents,
		TargetAdminAuditLogs,
		TargetAuditBlobs,
		TargetUsageRollups,
	}
}

func (s *Service) Start(ctx context.Context) {
	s.root = ctx
	go func() {
		if err := s.recoverStaleRuns(ctx); err != nil {
			s.logger.Warn("data cleanup: recover stale runs failed", zap.Error(err))
		}
		s.runAutomatic(ctx)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAutomatic(ctx)
			}
		}
	}()
}

func (s *Service) GetPolicy(ctx context.Context) (Policy, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM sys_settings WHERE key = $1`, policyKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, fmt.Errorf("read data cleanup policy: %w", err)
	}
	policy := DefaultPolicy()
	if err := json.Unmarshal(raw, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode data cleanup policy: %w", err)
	}
	return normalizePolicy(policy)
}

func (s *Service) UpdatePolicy(ctx context.Context, policy Policy, actor string) (Policy, error) {
	policy, err := normalizePolicy(policy)
	if err != nil {
		return Policy{}, err
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return Policy{}, fmt.Errorf("encode data cleanup policy: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO sys_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, NULLIF($3, ''), now())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
	`, policyKey, raw, actor); err != nil {
		return Policy{}, fmt.Errorf("save data cleanup policy: %w", err)
	}
	return policy, nil
}

func (s *Service) Preview(ctx context.Context) (Preview, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return Preview{}, err
	}
	now := time.Now().UTC()
	items := make([]PreviewItem, 0, len(AllTargets()))
	for _, target := range AllTargets() {
		retention := policy.retentionDays(target)
		cutoff := now.AddDate(0, 0, -retention)
		count, err := s.countTarget(ctx, target, cutoff)
		if err != nil {
			return Preview{}, fmt.Errorf("preview %s: %w", target, err)
		}
		items = append(items, PreviewItem{
			Target:        target,
			Label:         targetLabel(target),
			RetentionDays: retention,
			Cutoff:        cutoff,
			EligibleRows:  count,
		})
	}
	return Preview{Policy: policy, GeneratedAt: now, Items: items}, nil
}

func (s *Service) StartManual(targets []string, actor string) (Run, error) {
	targets, err := normalizeTargets(targets)
	if err != nil {
		return Run{}, err
	}
	run, err := s.queueRun("manual", targets, actor)
	if err != nil {
		return Run{}, err
	}
	go s.execute(context.WithoutCancel(s.root), run.ID, "manual", targets, actor)
	return run, nil
}

func (s *Service) ListRuns(ctx context.Context) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, trigger, status, COALESCE(requested_by, ''), targets, summary,
		       COALESCE(error, ''), created_at, started_at, completed_at
		FROM sys_data_cleanup_runs
		ORDER BY created_at DESC
		LIMIT $1
	`, cleanupRunLimit)
	if err != nil {
		return nil, fmt.Errorf("list data cleanup runs: %w", err)
	}
	defer rows.Close()
	var result []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (s *Service) runAutomatic(ctx context.Context) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		s.logger.Warn("data cleanup: read policy failed", zap.Error(err))
		return
	}
	if !policy.Enabled {
		s.logger.Debug("data cleanup: automatic cleanup is disabled")
		return
	}
	run, err := s.queueRun("automatic", AllTargets(), "")
	if errors.Is(err, ErrAlreadyRunning) {
		s.logger.Info("data cleanup: another run is already active")
		return
	}
	if err != nil {
		s.logger.Warn("data cleanup: queue automatic run failed", zap.Error(err))
		return
	}
	s.execute(ctx, run.ID, "automatic", run.Targets, "")
}

func (s *Service) queueRun(trigger string, targets []string, actor string) (Run, error) {
	rawTargets, err := json.Marshal(targets)
	if err != nil {
		return Run{}, err
	}
	var run Run
	var rawTargetBytes, rawSummary []byte
	err = s.pool.QueryRow(context.Background(), `
		INSERT INTO sys_data_cleanup_runs (trigger, status, requested_by, targets)
		VALUES ($1, 'queued', NULLIF($2, ''), $3::jsonb)
		RETURNING id, trigger, status, COALESCE(requested_by, ''), targets, summary,
		          COALESCE(error, ''), created_at, started_at, completed_at
	`, trigger, actor, rawTargets).Scan(
		&run.ID, &run.Trigger, &run.Status, &run.RequestedBy,
		&rawTargetBytes, &rawSummary, &run.Error, &run.CreatedAt, &run.StartedAt, &run.CompletedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Run{}, ErrAlreadyRunning
		}
		return Run{}, fmt.Errorf("queue data cleanup run: %w", err)
	}
	if err := json.Unmarshal(rawTargetBytes, &run.Targets); err != nil {
		return Run{}, fmt.Errorf("decode queued cleanup targets: %w", err)
	}
	if err := json.Unmarshal(rawSummary, &run.Summary); err != nil {
		return Run{}, fmt.Errorf("decode queued cleanup summary: %w", err)
	}
	return run, nil
}

func (s *Service) execute(ctx context.Context, runID, trigger string, targets []string, actor string) {
	started := time.Now().UTC()
	if _, err := s.pool.Exec(ctx, `
		UPDATE sys_data_cleanup_runs
		SET status = 'running', started_at = $2
		WHERE id = $1 AND status = 'queued'
	`, runID, started); err != nil {
		s.logger.Error("data cleanup: mark run running failed", zap.String("run_id", runID), zap.Error(err))
		s.finishRun(context.WithoutCancel(ctx), runID, trigger, actor, targets, nil, err)
		return
	}

	policy, err := s.GetPolicy(ctx)
	if err != nil {
		s.finishRun(ctx, runID, trigger, actor, targets, nil, err)
		return
	}
	summary := make(map[string]int64, len(targets))
	for _, target := range targets {
		count, cleanErr := s.cleanTarget(ctx, target, policy)
		if cleanErr != nil {
			s.finishRun(ctx, runID, trigger, actor, targets, summary, fmt.Errorf("%s: %w", target, cleanErr))
			return
		}
		summary[target] = count
	}
	s.finishRun(ctx, runID, trigger, actor, targets, summary, nil)
}

func (s *Service) finishRun(ctx context.Context, runID, trigger, actor string, targets []string, summary map[string]int64, runErr error) {
	status := "completed"
	errorText := ""
	if runErr != nil {
		status = "failed"
		errorText = runErr.Error()
	}
	rawSummary, _ := json.Marshal(summary)
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE sys_data_cleanup_runs
		SET status = $2, summary = $3::jsonb, error = NULLIF($4, ''), completed_at = $5
		WHERE id = $1
	`, runID, status, rawSummary, errorText, now)
	if err != nil {
		s.logger.Error("data cleanup: finish run failed", zap.String("run_id", runID), zap.Error(err))
		return
	}

	requestSummary, _ := json.Marshal(map[string]any{
		"runId":   runID,
		"trigger": trigger,
		"targets": targets,
		"summary": summary,
		"error":   errorText,
	})
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO ai_admin_audit_logs (actor, action, object_type, object_id, request_summary, result, http_status)
		VALUES (NULLIF($1, ''), 'data_cleanup_completed', 'data_cleanup_run', $2, $3::jsonb, $4, $5)
	`, actor, runID, requestSummary, status, func() int {
		if runErr != nil {
			return 500
		}
		return 200
	}()); err != nil {
		s.logger.Warn("data cleanup: write audit log failed", zap.String("run_id", runID), zap.Error(err))
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM sys_data_cleanup_runs
		WHERE id IN (
			SELECT id
			FROM sys_data_cleanup_runs
			WHERE status IN ('completed', 'failed')
			ORDER BY created_at DESC
			OFFSET $1
		)
	`, cleanupRunLimit); err != nil {
		s.logger.Warn("data cleanup: prune run history failed", zap.String("run_id", runID), zap.Error(err))
	}
	if runErr != nil {
		s.logger.Error("data cleanup: run failed", zap.String("run_id", runID), zap.Error(runErr))
	} else {
		s.logger.Info("data cleanup: run completed", zap.String("run_id", runID), zap.Any("summary", summary))
	}
}

func (s *Service) cleanTarget(ctx context.Context, target string, policy Policy) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -policy.retentionDays(target))
	switch target {
	case TargetRequestBody:
		return s.clearRequestBodies(ctx, cutoff, policy.BatchSize)
	case TargetRequestPayloads:
		return s.deleteRequestPayloads(ctx, cutoff, policy.BatchSize)
	case TargetNotifications:
		return s.deleteNotifications(ctx, cutoff, policy.BatchSize)
	case TargetModerationLogs:
		return s.deleteModerationLogs(ctx, cutoff, policy.BatchSize)
	case TargetRiskEvents:
		return s.deleteRiskEvents(ctx, cutoff, policy.BatchSize)
	case TargetAdminAuditLogs:
		return s.deleteAdminAuditLogs(ctx, cutoff, policy.BatchSize)
	case TargetAuditBlobs:
		return s.deleteUnreferencedBlobs(ctx, cutoff, policy.BatchSize)
	case TargetUsageRollups:
		return s.deleteUsageRollups(ctx, cutoff, policy.BatchSize)
	default:
		return 0, fmt.Errorf("%w: %s", ErrInvalidTarget, target)
	}
}

func (s *Service) countTarget(ctx context.Context, target string, cutoff time.Time) (int64, error) {
	var query string
	switch target {
	case TargetRequestBody:
		query = `SELECT COUNT(*) FROM ai_request_payloads WHERE created_at < $1 AND (request_messages IS NOT NULL OR request_params IS NOT NULL OR response_message IS NOT NULL OR internal_error_detail IS NOT NULL OR attempts_detail IS NOT NULL OR media_refs IS NOT NULL)`
	case TargetRequestPayloads:
		query = `SELECT COUNT(*) FROM ai_request_payloads WHERE created_at < $1`
	case TargetNotifications:
		query = `SELECT COUNT(*) FROM sys_notification_deliveries WHERE created_at < $1 AND status IN ('sent', 'failed')`
	case TargetModerationLogs:
		query = `SELECT COUNT(*) FROM ai_content_moderation_logs WHERE created_at < $1 AND flagged = FALSE`
	case TargetRiskEvents:
		query = `SELECT COUNT(*) FROM ai_risk_events WHERE created_at < $1 AND status IN ('resolved', 'dismissed')`
	case TargetAdminAuditLogs:
		query = `SELECT COUNT(*) FROM ai_admin_audit_logs WHERE created_at < $1`
	case TargetAuditBlobs:
		return s.countUnreferencedBlobs(ctx, cutoff)
	case TargetUsageRollups:
		query = `SELECT COUNT(*) FROM ai_usage_rollups_hourly WHERE bucket_start < $1`
	default:
		return 0, fmt.Errorf("%w: %s", ErrInvalidTarget, target)
	}
	var count int64
	if err := s.pool.QueryRow(ctx, query, cutoff).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) clearRequestBodies(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	var total int64
	for {
		result, err := s.pool.Exec(ctx, `
			UPDATE ai_request_payloads
			SET request_messages = NULL,
			    request_params = NULL,
			    response_message = NULL,
			    internal_error_detail = NULL,
			    attempts_detail = NULL,
			    media_refs = NULL
			WHERE id IN (
				SELECT id FROM ai_request_payloads
				WHERE created_at < $1
				  AND (request_messages IS NOT NULL OR request_params IS NOT NULL OR response_message IS NOT NULL OR internal_error_detail IS NOT NULL OR attempts_detail IS NOT NULL OR media_refs IS NOT NULL)
				ORDER BY created_at
				LIMIT $2
			)
		`, cutoff, batchSize)
		if err != nil {
			return total, err
		}
		changed := result.RowsAffected()
		total += changed
		if changed < int64(batchSize) {
			return total, nil
		}
	}
}

func (s *Service) deleteRequestPayloads(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, `
			DELETE FROM ai_request_payloads
			WHERE id IN (
				SELECT id FROM ai_request_payloads
				WHERE created_at < $1
				ORDER BY created_at
				LIMIT $2
			)
		`, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) deleteNotifications(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, `
			DELETE FROM sys_notification_deliveries
			WHERE id IN (
				SELECT id FROM sys_notification_deliveries
				WHERE created_at < $1 AND status IN ('sent', 'failed')
				ORDER BY created_at
				LIMIT $2
			)
		`, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) deleteModerationLogs(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, `
			DELETE FROM ai_content_moderation_logs
			WHERE id IN (
				SELECT id FROM ai_content_moderation_logs
				WHERE created_at < $1 AND flagged = FALSE
				ORDER BY created_at
				LIMIT $2
			)
		`, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) deleteRiskEvents(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, `
			DELETE FROM ai_risk_events
			WHERE id IN (
				SELECT id FROM ai_risk_events
				WHERE created_at < $1 AND status IN ('resolved', 'dismissed')
				ORDER BY created_at
				LIMIT $2
			)
		`, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) deleteAdminAuditLogs(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, `
			DELETE FROM ai_admin_audit_logs
			WHERE id IN (
				SELECT id FROM ai_admin_audit_logs
				WHERE created_at < $1
				ORDER BY created_at
				LIMIT $2
			)
		`, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) deleteUsageRollups(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, `
			DELETE FROM ai_usage_rollups_hourly
			WHERE (bucket_start, tenant_id, user_id, api_key_id, request_source, model_code, provider_code, request_status, billable_unit_type) IN (
				SELECT bucket_start, tenant_id, user_id, api_key_id, request_source, model_code, provider_code, request_status, billable_unit_type
				FROM ai_usage_rollups_hourly
				WHERE bucket_start < $1
				ORDER BY bucket_start
				LIMIT $2
			)
		`, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) deleteInBatches(ctx context.Context, operation func() (int64, error)) (int64, error) {
	var total int64
	for {
		changed, err := operation()
		if err != nil {
			return total, err
		}
		total += changed
		if changed == 0 {
			return total, nil
		}
	}
}

func (s *Service) countUnreferencedBlobs(ctx context.Context, cutoff time.Time) (int64, error) {
	filter, err := s.unreferencedBlobFilter(ctx, "candidate")
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM ai_audit_blobs candidate
		WHERE candidate.created_at < $1 AND %s
	`, filter)
	var count int64
	if err := s.pool.QueryRow(ctx, query, cutoff).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) deleteUnreferencedBlobs(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	filter, err := s.unreferencedBlobFilter(ctx, "candidate")
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`
		DELETE FROM ai_audit_blobs blob
		WHERE blob.sha256 IN (
			SELECT candidate.sha256
			FROM ai_audit_blobs candidate
			WHERE candidate.created_at < $1 AND %s
			ORDER BY candidate.created_at
			LIMIT $2
		)
	`, filter)
	return s.deleteInBatches(ctx, func() (int64, error) {
		result, err := s.pool.Exec(ctx, query, cutoff, batchSize)
		return result.RowsAffected(), err
	})
}

func (s *Service) unreferencedBlobFilter(ctx context.Context, candidateAlias string) (string, error) {
	tables, err := s.archiveTables(ctx)
	if err != nil {
		return "", err
	}
	clauses := []string{
		fmt.Sprintf("NOT EXISTS (SELECT 1 FROM ai_request_payloads p WHERE p.media_refs IS NOT NULL AND p.media_refs::text LIKE '%%' || %s.sha256 || '%%')", candidateAlias),
	}
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		clauses = append(clauses, fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s p WHERE p.media_refs IS NOT NULL AND p.media_refs::text LIKE '%%' || %s.sha256 || '%%')", quoted, candidateAlias))
	}
	return strings.Join(clauses, " AND "), nil
}

func (s *Service) archiveTables(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = current_schema()
		  AND tablename LIKE 'ai_request_payloads_archive_%'
		ORDER BY tablename
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		if archiveTableName.MatchString(table) {
			tables = append(tables, table)
		}
	}
	return tables, rows.Err()
}

func (s *Service) recoverStaleRuns(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sys_data_cleanup_runs
		SET status = 'failed', error = 'server restart interrupted cleanup', completed_at = now()
		WHERE status IN ('queued', 'running')
		  AND created_at < now() - INTERVAL '10 minutes'
	`)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (Run, error) {
	var run Run
	var rawTargets, rawSummary []byte
	if err := row.Scan(
		&run.ID, &run.Trigger, &run.Status, &run.RequestedBy,
		&rawTargets, &rawSummary, &run.Error, &run.CreatedAt, &run.StartedAt, &run.CompletedAt,
	); err != nil {
		return Run{}, err
	}
	if err := json.Unmarshal(rawTargets, &run.Targets); err != nil {
		return Run{}, fmt.Errorf("decode cleanup targets: %w", err)
	}
	if err := json.Unmarshal(rawSummary, &run.Summary); err != nil {
		return Run{}, fmt.Errorf("decode cleanup summary: %w", err)
	}
	return run, nil
}

func normalizePolicy(policy Policy) (Policy, error) {
	defaults := DefaultPolicy()
	if policy.RequestBodyDays == 0 {
		policy.RequestBodyDays = defaults.RequestBodyDays
	}
	if policy.RequestPayloadDays == 0 {
		policy.RequestPayloadDays = defaults.RequestPayloadDays
	}
	if policy.NotificationDays == 0 {
		policy.NotificationDays = defaults.NotificationDays
	}
	if policy.ModerationDays == 0 {
		policy.ModerationDays = defaults.ModerationDays
	}
	if policy.RiskEventDays == 0 {
		policy.RiskEventDays = defaults.RiskEventDays
	}
	if policy.AdminAuditDays == 0 {
		policy.AdminAuditDays = defaults.AdminAuditDays
	}
	if policy.AuditBlobDays == 0 {
		policy.AuditBlobDays = defaults.AuditBlobDays
	}
	if policy.UsageRollupDays == 0 {
		policy.UsageRollupDays = defaults.UsageRollupDays
	}
	if policy.BatchSize == 0 {
		policy.BatchSize = defaults.BatchSize
	}
	if policy.RequestPayloadDays < policy.RequestBodyDays {
		return Policy{}, errors.New("requestPayloadDays must be greater than or equal to requestBodyDays")
	}
	if policy.RequestBodyDays < 7 || policy.RequestPayloadDays < 30 || policy.NotificationDays < 7 || policy.ModerationDays < 7 || policy.RiskEventDays < 30 || policy.AdminAuditDays < 30 || policy.AuditBlobDays < 30 || policy.UsageRollupDays < 365 {
		return Policy{}, errors.New("cleanup retention is below the supported minimum")
	}
	if policy.BatchSize < 100 || policy.BatchSize > 5000 {
		return Policy{}, errors.New("cleanup batchSize must be between 100 and 5000")
	}
	return policy, nil
}

func normalizeTargets(targets []string) ([]string, error) {
	if len(targets) == 0 {
		targets = AllTargets()
	}
	known := make(map[string]struct{}, len(AllTargets()))
	for _, target := range AllTargets() {
		known[target] = struct{}{}
	}
	seen := make(map[string]struct{}, len(targets))
	result := make([]string, 0, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if _, ok := known[target]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidTarget, target)
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		result = append(result, target)
	}
	if len(result) == 0 {
		return nil, errors.New("at least one cleanup target is required")
	}
	return result, nil
}

func (p Policy) retentionDays(target string) int {
	switch target {
	case TargetRequestBody:
		return p.RequestBodyDays
	case TargetRequestPayloads:
		return p.RequestPayloadDays
	case TargetNotifications:
		return p.NotificationDays
	case TargetModerationLogs:
		return p.ModerationDays
	case TargetRiskEvents:
		return p.RiskEventDays
	case TargetAdminAuditLogs:
		return p.AdminAuditDays
	case TargetAuditBlobs:
		return p.AuditBlobDays
	case TargetUsageRollups:
		return p.UsageRollupDays
	default:
		return 0
	}
}

func targetLabel(target string) string {
	switch target {
	case TargetRequestBody:
		return "请求正文与错误详情"
	case TargetRequestPayloads:
		return "过期请求记录"
	case TargetNotifications:
		return "已发送/失败通知"
	case TargetModerationLogs:
		return "未命中审核记录"
	case TargetRiskEvents:
		return "已解决风险事件"
	case TargetAdminAuditLogs:
		return "管理审计日志"
	case TargetAuditBlobs:
		return "未引用媒体 Blob"
	case TargetUsageRollups:
		return "小时级用量汇总"
	default:
		return target
	}
}
