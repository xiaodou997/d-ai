package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"xiaodou/dai/internal/billing"
	shared "xiaodou/dai/internal/domain"
)

// 编译时接口检查
var _ billing.EventRepository = (*EventRepository)(nil)

type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// 可空的字符串列统一 COALESCE 为空串：BillingEvent 对应字段是非指针 string，直接
// Scan NULL 会报错——这会让 FindByIdempotencyKey 对 terminal_note/client_id 等为
// NULL 的事件解析失败，进而幂等短路失效、重复请求撞唯一约束返回 500。
const eventSelectCols = `
	SELECT id, event_id, COALESCE(idempotency_key, ''), tenant_id, COALESCE(user_id, ''),
	       COALESCE(description, ''), COALESCE(client_id, ''), tenant_credits, user_credits,
	       status, metadata::text, COALESCE(terminal_note, ''), created_at, finished_at
	FROM bill_events`

func scanEvent(scanner interface {
	Scan(dest ...any) error
}) (*billing.BillingEvent, error) {
	e := &billing.BillingEvent{}
	err := scanner.Scan(
		&e.ID, &e.EventID, &e.IdempotencyKey, &e.TenantID, &e.UserID,
		&e.Description, &e.ClientID, &e.TenantCredits, &e.UserCredits,
		&e.Status, &e.Metadata, &e.TerminalNote, &e.CreatedAt, &e.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Save 保存消费事件
func (r *EventRepository) Save(event *billing.BillingEvent) error {
	ctx := context.Background()

	if event.ID == 0 {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO bill_events
			(event_id, idempotency_key, tenant_id, user_id,
			 description, client_id, tenant_credits, user_credits,
			 status, metadata, terminal_note, created_at, finished_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, COALESCE($12, now()), $13)
			RETURNING id
		`, event.EventID, nullableString(event.IdempotencyKey), event.TenantID, nullableString(event.UserID),
			event.Description, nullableString(event.ClientID), event.TenantCredits, event.UserCredits,
			event.Status, emptyJSON(event.Metadata), nullableString(event.TerminalNote), event.CreatedAt, event.FinishedAt).Scan(&event.ID)
		return err
	}

	result, err := r.pool.Exec(ctx, `
		UPDATE bill_events
		SET tenant_credits = $1, user_credits = $2, status = $3,
		    terminal_note = $4, finished_at = $5
		WHERE id = $6
	`, event.TenantCredits, event.UserCredits, event.Status, nullableString(event.TerminalNote), event.FinishedAt, event.ID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return shared.ErrTransactionNotFound
	}
	return nil
}

// FindByEventID 根据事件ID查找
func (r *EventRepository) FindByEventID(eventID string) (*billing.BillingEvent, error) {
	ctx := context.Background()
	row := r.pool.QueryRow(ctx, eventSelectCols+` WHERE event_id = $1`, eventID)
	return scanEvent(row)
}

// FindByIdempotencyKey 根据幂等键查找（幂等校验）
func (r *EventRepository) FindByIdempotencyKey(key string) (*billing.BillingEvent, error) {
	ctx := context.Background()
	row := r.pool.QueryRow(ctx, eventSelectCols+` WHERE idempotency_key = $1`, key)
	return scanEvent(row)
}

// FindByUserID 查找终端用户的消费记录
func (r *EventRepository) FindByUserID(userID string, limit int) ([]*billing.BillingEvent, error) {
	ctx := context.Background()
	rows, err := r.pool.Query(ctx, eventSelectCols+`
		WHERE user_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*billing.BillingEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// CountToday 统计今日消费事件数
func (r *EventRepository) CountToday() (int64, error) {
	ctx := context.Background()
	var count int64
	todayStart := billing.NowUTC().Truncate(24 * time.Hour)
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bill_events
		WHERE created_at >= $1 AND event_type = 'charge'
	`, todayStart).Scan(&count)
	return count, err
}

// CountTodaySuccess 统计今日成功消费事件数
func (r *EventRepository) CountTodaySuccess() (int64, error) {
	ctx := context.Background()
	var count int64
	todayStart := billing.NowUTC().Truncate(24 * time.Hour)
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bill_events
		WHERE created_at >= $1 AND status = 'succeeded' AND event_type = 'charge'
	`, todayStart).Scan(&count)
	return count, err
}

// CountActivePreAuth 统计活跃预授权数
func (r *EventRepository) CountActivePreAuth() (int64, error) {
	ctx := context.Background()
	var count int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM bill_events WHERE status = 'pending'
	`).Scan(&count)
	return count, err
}

// FindStuckPreAuth 查找超过 timeoutMinutes 分钟仍为 pending 的预授权（调度器应已处理但未处理的）
func (r *EventRepository) FindStuckPreAuth(timeoutMinutes int) ([]*billing.BillingEvent, error) {
	ctx := context.Background()
	threshold := billing.NowUTC().Add(-time.Duration(timeoutMinutes) * time.Minute)
	rows, err := r.pool.Query(ctx, eventSelectCols+`
		WHERE status = 'pending' AND created_at < $1
		ORDER BY created_at ASC LIMIT 100
	`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*billing.BillingEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// FindReleasedInHours 查找最近N小时内被调度器超时释放的事件（服务已消耗但积分漏扣的真实漏单）
// cancelled = 业务请求失败→正常取消，不计入漏单
func (r *EventRepository) FindReleasedInHours(hours, limit int) ([]*billing.BillingEvent, error) {
	ctx := context.Background()
	cutoff := billing.NowUTC().Add(-time.Duration(hours) * time.Hour)
	rows, err := r.pool.Query(ctx, eventSelectCols+`
		WHERE status = 'released' AND created_at > $1
		ORDER BY created_at DESC LIMIT $2
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*billing.BillingEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func emptyJSON(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}
