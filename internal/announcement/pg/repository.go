package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/announcement"
	"xiaodou/dai/internal/auth"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateDraft(ctx context.Context, item announcement.Announcement) (announcement.Announcement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return announcement.Announcement{}, fmt.Errorf("begin announcement draft: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO ann_announcements
			(announcement_id, publisher_type, publisher_tenant_id, title, content_markdown,
			 category, severity, display_mode, status, starts_at, ends_at, created_by, updated_by)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, 'draft', $9, $10, $11, $11)
		RETURNING created_at, updated_at
	`, item.ID, item.PublisherType, item.PublisherTenantID, item.Title, item.ContentMarkdown,
		item.Category, item.Severity, item.DisplayMode, item.StartsAt, item.EndsAt, item.CreatedBy,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return announcement.Announcement{}, fmt.Errorf("insert announcement draft: %w", err)
	}
	if err := insertAudiences(ctx, tx, item.ID, item.Audiences); err != nil {
		return announcement.Announcement{}, err
	}
	if err := insertAudit(ctx, tx, item.ID, "created", announcement.Actor{
		UserType: auth.UserType(item.CreatedByUserType), UserID: auth.UserID(item.CreatedBy), TenantID: auth.TenantID(item.PublisherTenantID),
	}); err != nil {
		return announcement.Announcement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return announcement.Announcement{}, fmt.Errorf("commit announcement draft: %w", err)
	}
	return item, nil
}

func (r *Repository) GetManaged(ctx context.Context, actor announcement.Actor, id string) (announcement.Announcement, error) {
	where, args, err := managedWhere(actor, id)
	if err != nil {
		return announcement.Announcement{}, err
	}
	item, err := scanAnnouncement(r.pool.QueryRow(ctx, baseAnnouncementSelect+" WHERE "+where, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return announcement.Announcement{}, announcement.ErrNotFound
	}
	if err != nil {
		return announcement.Announcement{}, fmt.Errorf("get managed announcement: %w", err)
	}
	item.Audiences, err = r.listAudiences(ctx, id)
	if err != nil {
		return announcement.Announcement{}, err
	}
	return item, nil
}

func (r *Repository) UpdateDraft(ctx context.Context, actor announcement.Actor, item announcement.Announcement) (announcement.Announcement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return announcement.Announcement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	where, args, err := managedWhere(actor, item.ID)
	if err != nil {
		return announcement.Announcement{}, err
	}
	args = append(args, item.Title, item.ContentMarkdown, item.Category, item.Severity, item.DisplayMode, item.StartsAt, item.EndsAt, string(actor.UserID))
	base := len(args) - 7
	query := fmt.Sprintf(`UPDATE ann_announcements SET
		title=$%d, content_markdown=$%d, category=$%d, severity=$%d, display_mode=$%d,
		starts_at=$%d, ends_at=$%d, updated_by=$%d, updated_at=now()
		WHERE %s AND status='draft' RETURNING updated_at`,
		base, base+1, base+2, base+3, base+4, base+5, base+6, base+7, where)
	if err := tx.QueryRow(ctx, query, args...).Scan(&item.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return announcement.Announcement{}, announcement.ErrInvalidTransition
	} else if err != nil {
		return announcement.Announcement{}, fmt.Errorf("update announcement draft: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM ann_audiences WHERE announcement_id=$1`, item.ID); err != nil {
		return announcement.Announcement{}, fmt.Errorf("replace announcement audiences: %w", err)
	}
	if err := insertAudiences(ctx, tx, item.ID, item.Audiences); err != nil {
		return announcement.Announcement{}, err
	}
	if err := insertAudit(ctx, tx, item.ID, "updated", actor); err != nil {
		return announcement.Announcement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return announcement.Announcement{}, err
	}
	return item, nil
}

func (r *Repository) Publish(ctx context.Context, actor announcement.Actor, id string, now time.Time) (announcement.Announcement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return announcement.Announcement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	where, args, err := managedWhere(actor, id)
	if err != nil {
		return announcement.Announcement{}, err
	}
	item, err := scanAnnouncement(tx.QueryRow(ctx, baseAnnouncementSelect+" WHERE "+where+" FOR UPDATE", args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return announcement.Announcement{}, announcement.ErrNotFound
	}
	if err != nil {
		return announcement.Announcement{}, err
	}
	if item.Status != announcement.StatusDraft {
		return announcement.Announcement{}, announcement.ErrInvalidTransition
	}
	var missingTenant bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM ann_audiences aa
			WHERE aa.announcement_id=$1 AND aa.scope_type='tenant'
			  AND NOT EXISTS (SELECT 1 FROM iam_tenants t WHERE t.tenant_id=aa.tenant_id)
		)
	`, id).Scan(&missingTenant); err != nil {
		return announcement.Announcement{}, err
	}
	if missingTenant {
		return announcement.Announcement{}, announcement.ErrInvalidAudience
	}
	audienceSize, err := countAudience(ctx, tx, id)
	if err != nil {
		return announcement.Announcement{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE ann_announcements
		SET status='published', published_at=$2, audience_size_at_publish=$3, updated_by=$4, updated_at=$2
		WHERE announcement_id=$1 AND status='draft'
		RETURNING published_at, audience_size_at_publish, updated_at
	`, id, now, audienceSize, string(actor.UserID)).Scan(&item.PublishedAt, &item.AudienceSizeAtPublish, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return announcement.Announcement{}, announcement.ErrInvalidTransition
	}
	if err != nil {
		return announcement.Announcement{}, err
	}
	if err := insertAudit(ctx, tx, id, "published", actor); err != nil {
		return announcement.Announcement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return announcement.Announcement{}, err
	}
	item.Status = announcement.StatusPublished
	item.UpdatedBy = string(actor.UserID)
	item.Audiences, err = r.listAudiences(ctx, id)
	return item, err
}

func (r *Repository) ListInbox(ctx context.Context, principal announcement.Principal, query announcement.InboxQuery) (announcement.InboxPage, error) {
	kind, err := audienceKind(int(principal.UserType))
	if err != nil {
		return announcement.InboxPage{}, err
	}
	where := `a.status='published'
		AND (a.starts_at IS NULL OR a.starts_at <= now())
		AND (a.ends_at IS NULL OR a.ends_at > now())
		AND EXISTS (
			SELECT 1 FROM ann_audiences aa
			WHERE aa.announcement_id=a.announcement_id AND aa.audience_kind=$1
			  AND (aa.scope_type='all' OR (aa.scope_type='tenant' AND aa.tenant_id=$2))
		)`
	args := []any{kind, string(principal.TenantID), int(principal.UserType), string(principal.UserID)}
	if query.UnreadOnly {
		where += ` AND r.read_at IS NULL`
	}
	if query.DisplayMode != "" {
		args = append(args, query.DisplayMode)
		where += fmt.Sprintf(" AND a.display_mode=$%d", len(args))
	}
	from := ` FROM ann_announcements a
		LEFT JOIN ann_receipts r ON r.announcement_id=a.announcement_id AND r.user_type=$3 AND r.user_id=$4`
	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*)"+from+" WHERE "+where, args...).Scan(&total); err != nil {
		return announcement.InboxPage{}, err
	}
	var unread int64
	unreadWhere := strings.Replace(where, " AND r.read_at IS NULL", "", 1) + " AND r.read_at IS NULL"
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*)"+from+" WHERE "+unreadWhere, args...).Scan(&unread); err != nil {
		return announcement.InboxPage{}, err
	}
	offset := (query.Page - 1) * query.Size
	listArgs := append(append([]any{}, args...), query.Size, offset)
	rows, err := r.pool.Query(ctx, inboxSelect+from+" WHERE "+where+
		fmt.Sprintf(" ORDER BY a.published_at DESC, a.announcement_id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2), listArgs...)
	if err != nil {
		return announcement.InboxPage{}, err
	}
	defer rows.Close()
	items := make([]announcement.InboxItem, 0)
	for rows.Next() {
		item, readAt, err := scanInbox(rows)
		if err != nil {
			return announcement.InboxPage{}, err
		}
		items = append(items, announcement.InboxItem{Announcement: item, ReadAt: readAt})
	}
	if err := rows.Err(); err != nil {
		return announcement.InboxPage{}, err
	}
	return announcement.InboxPage{Items: items, Total: total, UnreadCount: unread, Page: query.Page, Size: query.Size}, nil
}

func (r *Repository) MarkRead(ctx context.Context, principal announcement.Principal, id string, now time.Time) error {
	kind, err := audienceKind(int(principal.UserType))
	if err != nil {
		return err
	}
	var marked int
	err = r.pool.QueryRow(ctx, `
		INSERT INTO ann_receipts (announcement_id, user_type, user_id, tenant_id, read_at)
		SELECT a.announcement_id, $2, $3, NULLIF($4, ''), $5
		FROM ann_announcements a
		WHERE a.announcement_id=$1 AND a.status='published'
		  AND (a.starts_at IS NULL OR a.starts_at <= now())
		  AND (a.ends_at IS NULL OR a.ends_at > now())
		  AND EXISTS (
			SELECT 1 FROM ann_audiences aa
			WHERE aa.announcement_id=a.announcement_id AND aa.audience_kind=$6
			  AND (aa.scope_type='all' OR (aa.scope_type='tenant' AND aa.tenant_id=$4))
		  )
		ON CONFLICT (announcement_id, user_type, user_id)
		DO UPDATE SET read_at=ann_receipts.read_at
		RETURNING 1
	`, id, int(principal.UserType), string(principal.UserID), string(principal.TenantID), now, kind).Scan(&marked)
	if errors.Is(err, pgx.ErrNoRows) {
		return announcement.ErrNotFound
	}
	return err
}

func (r *Repository) GetVisible(ctx context.Context, principal announcement.Principal, id string) (announcement.InboxItem, error) {
	kind, err := audienceKind(int(principal.UserType))
	if err != nil {
		return announcement.InboxItem{}, err
	}
	row := r.pool.QueryRow(ctx, inboxSelect+` FROM ann_announcements a
		LEFT JOIN ann_receipts r ON r.announcement_id=a.announcement_id AND r.user_type=$3 AND r.user_id=$4
		WHERE a.announcement_id=$5 AND a.status='published'
		  AND (a.starts_at IS NULL OR a.starts_at <= now())
		  AND (a.ends_at IS NULL OR a.ends_at > now())
		  AND EXISTS (
			SELECT 1 FROM ann_audiences aa WHERE aa.announcement_id=a.announcement_id
			AND aa.audience_kind=$1 AND (aa.scope_type='all' OR (aa.scope_type='tenant' AND aa.tenant_id=$2))
		  )`, kind, string(principal.TenantID), int(principal.UserType), string(principal.UserID), id)
	item, readAt, err := scanInbox(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return announcement.InboxItem{}, announcement.ErrNotFound
	}
	if err != nil {
		return announcement.InboxItem{}, err
	}
	return announcement.InboxItem{Announcement: item, ReadAt: readAt}, nil
}

func (r *Repository) Archive(ctx context.Context, actor announcement.Actor, id string, now time.Time) (announcement.Announcement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return announcement.Announcement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	where, args, err := managedWhere(actor, id)
	if err != nil {
		return announcement.Announcement{}, err
	}
	status, err := lockManagedStatus(ctx, tx, where, args)
	if err != nil {
		return announcement.Announcement{}, err
	}
	if status != announcement.StatusPublished {
		return announcement.Announcement{}, announcement.ErrInvalidTransition
	}
	args = append(args, now, string(actor.UserID))
	query := fmt.Sprintf(`UPDATE ann_announcements
		SET status='archived', archived_at=$%d, updated_by=$%d, updated_at=$%d
		WHERE %s AND status='published'`, len(args)-1, len(args), len(args)-1, where)
	result, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return announcement.Announcement{}, err
	}
	if result.RowsAffected() != 1 {
		return announcement.Announcement{}, announcement.ErrInvalidTransition
	}
	if err := insertAudit(ctx, tx, id, "archived", actor); err != nil {
		return announcement.Announcement{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return announcement.Announcement{}, err
	}
	return r.GetManaged(ctx, actor, id)
}

func (r *Repository) GetStats(ctx context.Context, actor announcement.Actor, id string) (announcement.Stats, error) {
	item, err := r.GetManaged(ctx, actor, id)
	if err != nil {
		return announcement.Stats{}, err
	}
	current, err := countAudience(ctx, r.pool, id)
	if err != nil {
		return announcement.Stats{}, err
	}
	var readCount int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ann_receipts WHERE announcement_id=$1`, id).Scan(&readCount); err != nil {
		return announcement.Stats{}, err
	}
	var atPublish int64
	if item.AudienceSizeAtPublish != nil {
		atPublish = *item.AudienceSizeAtPublish
	}
	return announcement.Stats{AudienceSizeAtPublish: atPublish, CurrentAudienceSize: current, ReadCount: readCount}, nil
}

func (r *Repository) Delete(ctx context.Context, actor announcement.Actor, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	where, args, err := managedWhere(actor, id)
	if err != nil {
		return err
	}
	status, err := lockManagedStatus(ctx, tx, where, args)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, "DELETE FROM ann_announcements WHERE "+where, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return announcement.ErrInvalidTransition
	}
	event := "deleted"
	if status == announcement.StatusDraft {
		event = "draft_deleted"
	}
	if err := insertAudit(ctx, tx, id, event, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeleteDraft is kept as a compatibility alias for the old repository port.
func (r *Repository) DeleteDraft(ctx context.Context, actor announcement.Actor, id string) error {
	return r.Delete(ctx, actor, id)
}

func (r *Repository) ListManaged(ctx context.Context, actor announcement.Actor, query announcement.ManageQuery) (announcement.ManagedPage, error) {
	where, args, err := managedListWhere(actor)
	if err != nil {
		return announcement.ManagedPage{}, err
	}
	if query.Status != "" {
		args = append(args, query.Status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		where += fmt.Sprintf(" AND (title ILIKE $%d OR content_markdown ILIKE $%d)", len(args), len(args))
	}
	var total int64
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM ann_announcements WHERE "+where, args...).Scan(&total); err != nil {
		return announcement.ManagedPage{}, err
	}
	offset := (query.Page - 1) * query.Size
	listArgs := append(append([]any{}, args...), query.Size, offset)
	rows, err := r.pool.Query(ctx, baseAnnouncementSelect+" WHERE "+where+
		fmt.Sprintf(" ORDER BY created_at DESC, announcement_id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2), listArgs...)
	if err != nil {
		return announcement.ManagedPage{}, err
	}
	defer rows.Close()
	items := make([]announcement.Announcement, 0)
	for rows.Next() {
		item, err := scanAnnouncement(rows)
		if err != nil {
			return announcement.ManagedPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return announcement.ManagedPage{}, err
	}
	for i := range items {
		items[i].Audiences, err = r.listAudiences(ctx, items[i].ID)
		if err != nil {
			return announcement.ManagedPage{}, err
		}
	}
	return announcement.ManagedPage{Items: items, Total: total, Page: query.Page, Size: query.Size}, nil
}

func (r *Repository) ListRecipients(ctx context.Context, actor announcement.Actor, id string, query announcement.RecipientQuery) (announcement.RecipientPage, error) {
	if _, err := r.GetManaged(ctx, actor, id); err != nil {
		return announcement.RecipientPage{}, err
	}
	args := []any{id}
	where := ""
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		where = fmt.Sprintf(" WHERE (recipients.username ILIKE $%d OR recipients.email ILIKE $%d)", len(args), len(args))
	}
	cte := recipientCTE
	var total int64
	if err := r.pool.QueryRow(ctx, cte+" SELECT COUNT(*) FROM recipients"+where, args...).Scan(&total); err != nil {
		return announcement.RecipientPage{}, err
	}
	offset := (query.Page - 1) * query.Size
	listArgs := append(append([]any{}, args...), query.Size, offset)
	rows, err := r.pool.Query(ctx, cte+` SELECT recipients.user_type, recipients.user_id, recipients.tenant_id,
		recipients.username, recipients.email, r.read_at
		FROM recipients
		LEFT JOIN ann_receipts r ON r.announcement_id=$1 AND r.user_type=recipients.user_type AND r.user_id=recipients.user_id`+
		where+fmt.Sprintf(" ORDER BY recipients.user_type, recipients.username LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2), listArgs...)
	if err != nil {
		return announcement.RecipientPage{}, err
	}
	defer rows.Close()
	items := make([]announcement.Recipient, 0)
	for rows.Next() {
		var item announcement.Recipient
		if err := rows.Scan(&item.UserType, &item.UserID, &item.TenantID, &item.Username, &item.Email, &item.ReadAt); err != nil {
			return announcement.RecipientPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return announcement.RecipientPage{}, err
	}
	return announcement.RecipientPage{Items: items, Total: total, Page: query.Page, Size: query.Size}, nil
}

func insertAudiences(ctx context.Context, tx pgx.Tx, id string, audiences []announcement.AudienceRule) error {
	for _, audience := range audiences {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ann_audiences (announcement_id, audience_kind, scope_type, tenant_id)
			VALUES ($1, $2, $3, NULLIF($4, ''))
		`, id, audience.Kind, audience.ScopeType, audience.TenantID); err != nil {
			return fmt.Errorf("insert announcement audience: %w", err)
		}
	}
	return nil
}

func insertAudit(ctx context.Context, tx pgx.Tx, id, event string, actor announcement.Actor) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ann_audit_events
			(announcement_id, event_type, actor_user_type, actor_user_id, actor_tenant_id)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
	`, id, event, int(actor.UserType), string(actor.UserID), string(actor.TenantID))
	if err != nil {
		return fmt.Errorf("insert announcement audit: %w", err)
	}
	return nil
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func countAudience(ctx context.Context, tx rowQuerier, id string) (int64, error) {
	var count int64
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM iam_accounts u
		WHERE u.status = 'active' AND EXISTS (
			SELECT 1 FROM ann_audiences aa
			WHERE aa.announcement_id = $1 AND (
				(u.user_type IN (1, 2) AND aa.audience_kind = 'admin' AND aa.scope_type = 'all')
				OR (u.user_type = 3 AND aa.audience_kind = 'tenant_user' AND (aa.scope_type = 'all' OR aa.tenant_id = u.tenant_id))
				OR (u.user_type = 4 AND aa.audience_kind = 'end_user' AND (aa.scope_type = 'all' OR aa.tenant_id = u.tenant_id))
			)
		)
	`, id).Scan(&count)
	return count, err
}

func (r *Repository) listAudiences(ctx context.Context, id string) ([]announcement.AudienceRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT audience_kind, scope_type, COALESCE(tenant_id, '')
		FROM ann_audiences WHERE announcement_id=$1 ORDER BY id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]announcement.AudienceRule, 0)
	for rows.Next() {
		var rule announcement.AudienceRule
		if err := rows.Scan(&rule.Kind, &rule.ScopeType, &rule.TenantID); err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func managedWhere(actor announcement.Actor, id string) (string, []any, error) {
	if actor.Has(auth.CapabilityPlatformAdmin) {
		return "announcement_id=$1 AND publisher_type='platform'", []any{id}, nil
	}
	if actor.Has(auth.CapabilityTenantSelf) {
		if actor.TenantID == "" {
			return "", nil, announcement.ErrForbidden
		}
		return "announcement_id=$1 AND publisher_type='tenant' AND publisher_tenant_id=$2", []any{id, string(actor.TenantID)}, nil
	}
	return "", nil, announcement.ErrForbidden
}

func lockManagedStatus(ctx context.Context, tx pgx.Tx, where string, args []any) (announcement.Status, error) {
	var status announcement.Status
	err := tx.QueryRow(ctx, "SELECT status FROM ann_announcements WHERE "+where+" FOR UPDATE", args...).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", announcement.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return status, nil
}

func managedListWhere(actor announcement.Actor) (string, []any, error) {
	if actor.Has(auth.CapabilityPlatformAdmin) {
		return "publisher_type='platform'", nil, nil
	}
	if actor.Has(auth.CapabilityTenantSelf) {
		if actor.TenantID == "" {
			return "", nil, announcement.ErrForbidden
		}
		return "publisher_type='tenant' AND publisher_tenant_id=$1", []any{string(actor.TenantID)}, nil
	}
	return "", nil, announcement.ErrForbidden
}

func audienceKind(userType int) (announcement.AudienceKind, error) {
	switch userType {
	case 1, 2:
		return announcement.AudienceAdmin, nil
	case 3:
		return announcement.AudienceTenantUser, nil
	case 4:
		return announcement.AudienceEndUser, nil
	default:
		return "", announcement.ErrForbidden
	}
}

const baseAnnouncementSelect = `SELECT announcement_id, publisher_type, COALESCE(publisher_tenant_id, ''),
	title, content_markdown, category, severity, display_mode, status, starts_at, ends_at,
	published_at, archived_at, audience_size_at_publish, created_by, updated_by, created_at, updated_at
	FROM ann_announcements`

const inboxSelect = `SELECT a.announcement_id, a.publisher_type, COALESCE(a.publisher_tenant_id, ''),
	a.title, a.content_markdown, a.category, a.severity, a.display_mode, a.status, a.starts_at, a.ends_at,
	a.published_at, a.archived_at, a.audience_size_at_publish, a.created_by, a.updated_by, a.created_at, a.updated_at,
	r.read_at`

const recipientCTE = `WITH recipients AS (
	SELECT u.user_type, u.user_id, COALESCE(u.tenant_id, '') AS tenant_id,
		u.username, COALESCE(u.email, '') AS email
	FROM iam_accounts u
	WHERE u.status = 'active' AND EXISTS (
		SELECT 1 FROM ann_audiences aa
		WHERE aa.announcement_id = $1 AND (
			(u.user_type IN (1, 2) AND aa.audience_kind = 'admin' AND aa.scope_type = 'all')
			OR (u.user_type = 3 AND aa.audience_kind = 'tenant_user' AND (aa.scope_type = 'all' OR aa.tenant_id = u.tenant_id))
			OR (u.user_type = 4 AND aa.audience_kind = 'end_user' AND (aa.scope_type = 'all' OR aa.tenant_id = u.tenant_id))
		)
	)
)`

type scanner interface {
	Scan(dest ...any) error
}

func scanAnnouncement(row scanner) (announcement.Announcement, error) {
	var item announcement.Announcement
	err := row.Scan(&item.ID, &item.PublisherType, &item.PublisherTenantID,
		&item.Title, &item.ContentMarkdown, &item.Category, &item.Severity, &item.DisplayMode, &item.Status,
		&item.StartsAt, &item.EndsAt, &item.PublishedAt, &item.ArchivedAt, &item.AudienceSizeAtPublish,
		&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanInbox(row scanner) (announcement.Announcement, *time.Time, error) {
	var item announcement.Announcement
	var readAt *time.Time
	err := row.Scan(&item.ID, &item.PublisherType, &item.PublisherTenantID,
		&item.Title, &item.ContentMarkdown, &item.Category, &item.Severity, &item.DisplayMode, &item.Status,
		&item.StartsAt, &item.EndsAt, &item.PublishedAt, &item.ArchivedAt, &item.AudienceSizeAtPublish,
		&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt, &readAt)
	return item, readAt, err
}
