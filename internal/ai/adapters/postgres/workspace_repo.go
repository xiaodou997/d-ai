package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"xiaodou/dai/internal/ai/core/identity"
	"xiaodou/dai/internal/ai/core/surface"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/imageassets"
	"xiaodou/dai/internal/ai/workspace"
)

type WorkspaceRepo struct {
	pool           *pgxpool.Pool
	grantChecker   *GroupAccessReader
	routeInspector *RouteInspector
}

func NewWorkspaceRepo(pool *pgxpool.Pool, grantChecker *GroupAccessReader, routeInspector *RouteInspector) *WorkspaceRepo {
	return &WorkspaceRepo{
		pool:           pool,
		grantChecker:   grantChecker,
		routeInspector: routeInspector,
	}
}

func (r *WorkspaceRepo) ListChatSessions(ctx context.Context, owner workspace.Owner, limit int) ([]workspace.ChatSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id::text,
		       t.title,
		       t.target_kind,
		       COALESCE(t.app_id::text, ''),
		       COALESCE(a.name, ''),
		       t.variables_json,
		       CASE
		         WHEN t.target_kind = 'model' THEN t.target_model_code
		         ELSE COALESCE(a.model_code, '')
		       END AS model_code,
		       COALESCE(t.selected_group_id::text, ''),
		       COALESCE(t.selected_group_name_snapshot, ''),
		       COALESCE(t.selected_effective_user_multiplier_snapshot::float8, 0),
		       COALESCE(NULLIF(t.selected_group_name_snapshot, '') || ' · ' || trim(to_char(t.selected_effective_user_multiplier_snapshot, 'FM999999990.9999')) || 'x', ''),
		       COALESCE(t.selected_surface, ''),
		       COALESCE(t.selected_route_id::text, ''),
		       t.status,
		       t.created_at,
		       t.updated_at
		FROM ai_workspace_threads t
		LEFT JOIN ai_apps a ON a.id = t.app_id
		WHERE t.tenant_id = $1
		  AND t.owner_scope = $2
		  AND t.user_id = $3
		  AND t.target_kind = 'model'
		  AND t.status <> 'deleted'
		ORDER BY t.updated_at DESC
		LIMIT $4
	`, owner.TenantID, ownerTypeFromScope(owner.Scope), ownerUserID(owner), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]workspace.ChatSession, 0)
	for rows.Next() {
		item, err := scanWorkspaceChatSession(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *WorkspaceRepo) GetChatSession(ctx context.Context, owner workspace.Owner, sessionID string) (workspace.ChatSession, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT t.id::text,
		       t.title,
		       t.target_kind,
		       COALESCE(t.app_id::text, ''),
		       COALESCE(a.name, ''),
		       t.variables_json,
		       CASE
		         WHEN t.target_kind = 'model' THEN t.target_model_code
		         ELSE COALESCE(a.model_code, '')
		       END AS model_code,
		       COALESCE(t.selected_group_id::text, ''),
		       COALESCE(t.selected_group_name_snapshot, ''),
		       COALESCE(t.selected_effective_user_multiplier_snapshot::float8, 0),
		       COALESCE(NULLIF(t.selected_group_name_snapshot, '') || ' · ' || trim(to_char(t.selected_effective_user_multiplier_snapshot, 'FM999999990.9999')) || 'x', ''),
		       COALESCE(t.selected_surface, ''),
		       COALESCE(t.selected_route_id::text, ''),
		       t.status,
		       t.created_at,
		       t.updated_at
		FROM ai_workspace_threads t
		LEFT JOIN ai_apps a ON a.id = t.app_id
		WHERE t.id = $1
		  AND t.tenant_id = $2
		  AND t.owner_scope = $3
		  AND t.user_id = $4
		  AND t.target_kind = 'model'
		  AND t.status <> 'deleted'
	`, sessionID, owner.TenantID, ownerTypeFromScope(owner.Scope), ownerUserID(owner))
	item, err := scanWorkspaceChatSession(row.Scan)
	if err != nil {
		if err == pgx.ErrNoRows {
			return workspace.ChatSession{}, domain.ErrNotFound
		}
		return workspace.ChatSession{}, err
	}
	return item, nil
}

func (r *WorkspaceRepo) ListChatMessages(ctx context.Context, owner workspace.Owner, sessionID string) ([]workspace.ChatMessage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id::text,
		       m.role,
		       m.content_text,
		       COALESCE(m.client_surface, ''),
		       COALESCE(m.upstream_surface, ''),
		       m.route_snapshot,
		       m.usage_json,
		       m.error_json,
		       m.created_at
		FROM ai_workspace_messages m
		JOIN ai_workspace_threads t ON t.id = m.thread_id
		WHERE t.id = $1
		  AND t.tenant_id = $2
		  AND t.owner_scope = $3
		  AND t.user_id = $4
		  AND t.target_kind = 'model'
		  AND t.status <> 'deleted'
		ORDER BY m.created_at ASC
	`, sessionID, owner.TenantID, ownerTypeFromScope(owner.Scope), ownerUserID(owner))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]workspace.ChatMessage, 0)
	for rows.Next() {
		item, err := scanWorkspaceChatMessage(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *WorkspaceRepo) ListImageJobs(ctx context.Context, owner workspace.Owner, limit int) ([]workspace.ImageJob, error) {
	query := `
		SELECT id::text,
		       task_type,
		       model_code,
		       status,
		       input_payload,
		       result_payload,
		       caller_charge,
		       COALESCE(error_message, ''),
		       created_at,
		       completed_at
		FROM ai_async_tasks
		WHERE task_type IN ('console.images.generation', 'console.images.edit')
		  AND tenant_id = $1
		  AND COALESCE(input_payload->>'agent_id', '') = ''
		  AND (expires_at IS NULL OR expires_at > now())`
	args := []any{owner.TenantID}
	if owner.Scope == identity.ScopeUser {
		query += ` AND COALESCE(user_id, '') = $2`
		args = append(args, owner.UserID)
	}
	query += ` ORDER BY created_at DESC LIMIT $` + limitParamIndex(args)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]workspace.ImageJob, 0)
	for rows.Next() {
		item, err := scanWorkspaceImageJob(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *WorkspaceRepo) CreateChatSession(ctx context.Context, owner workspace.Owner, input workspace.CreateChatSessionInput) (string, error) {
	groupID := strings.TrimSpace(input.GroupID)
	groupName := ""
	effectiveUserMultiplier := 0.0
	targetModelCode := strings.TrimSpace(input.ModelCode)
	if groupID != "" {
		meta, err := r.getWorkspaceGroupSelectionSnapshot(ctx, owner, groupID)
		if err != nil {
			return "", err
		}
		groupName = meta.Name
		effectiveUserMultiplier = meta.EffectiveUserMultiplier
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO ai_workspace_threads (
		  owner_scope, tenant_id, user_id, target_kind, target_model_code, selected_group_id,
		  selected_group_name_snapshot, selected_effective_user_multiplier_snapshot, app_id, title, variables_json, status
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, $7, $8::numeric, NULLIF($9, '')::uuid, $10, $11::jsonb, 'active')
		RETURNING id::text
	`, string(owner.Scope), owner.TenantID, ownerUserID(owner), string(workspace.ThreadTargetModel), targetModelCode, groupID, groupName, effectiveUserMultiplier, "", input.Title, []byte(`{}`))
	var sessionID string
	if err := row.Scan(&sessionID); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (r *WorkspaceRepo) DeleteChatSession(ctx context.Context, owner workspace.Owner, sessionID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		DELETE FROM ai_workspace_messages m
		USING ai_workspace_threads t
		WHERE m.thread_id = t.id
		  AND t.id = $1
		  AND t.tenant_id = $2
		  AND t.owner_scope = $3
		  AND t.user_id = $4
	`, sessionID, owner.TenantID, string(owner.Scope), ownerUserID(owner)); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM ai_workspace_threads
		WHERE id = $1 AND tenant_id = $2 AND owner_scope = $3 AND user_id = $4
		  AND target_kind = 'model'
	`, sessionID, owner.TenantID, string(owner.Scope), ownerUserID(owner))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (r *WorkspaceRepo) CreateChatMessage(ctx context.Context, owner workspace.Owner, sessionID string, input workspace.ChatMessageWriteInput) (string, error) {
	routeRaw := []byte(`{}`)
	if input.RouteID != "" {
		routeRaw, _ = json.Marshal(map[string]any{"route_id": input.RouteID})
	}
	var surfaceValue any
	if input.ClientSurface != "" {
		surfaceValue = string(input.ClientSurface)
	}
	errorRaw := []byte(`{}`)
	if input.StreamStatus != "" {
		errorRaw, _ = json.Marshal(map[string]any{"stream_status": string(input.StreamStatus)})
	}
	var messageID string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_workspace_messages (
			thread_id, role, content_text, client_surface, upstream_surface, route_snapshot, usage_json, error_json
		)
		SELECT t.id, $5, $6, $7, $7, $8, '{}'::jsonb, $9::jsonb
		FROM ai_workspace_threads t
		WHERE t.id = $1
		  AND t.tenant_id = $2
		  AND t.owner_scope = $3
		  AND t.user_id = $4
		  AND t.target_kind = 'model'
		  AND t.status <> 'deleted'
		RETURNING id::text`,
		sessionID, owner.TenantID, string(owner.Scope), ownerUserID(owner), string(input.Role), input.Content, surfaceValue, routeRaw, errorRaw,
	).Scan(&messageID)
	if err == pgx.ErrNoRows {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return messageID, nil
}

func (r *WorkspaceRepo) UpdateChatMessageContent(ctx context.Context, owner workspace.Owner, messageID string, content string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_workspace_messages m
		SET content_text = $2
		FROM ai_workspace_threads t
		WHERE m.id = $1
		  AND t.id = m.thread_id
		  AND t.tenant_id = $3
		  AND t.owner_scope = $4
		  AND t.user_id = $5
		  AND t.target_kind = 'model'
		  AND t.status <> 'deleted'`,
		messageID, content, owner.TenantID, string(owner.Scope), ownerUserID(owner))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *WorkspaceRepo) UpdateChatMessageRoute(ctx context.Context, owner workspace.Owner, messageID string, input workspace.ChatMessageRouteUpdate) error {
	routeRaw := []byte(`{}`)
	if input.RouteID != "" {
		routeRaw, _ = json.Marshal(map[string]any{"route_id": input.RouteID})
	}
	errorRaw := []byte(`{}`)
	if input.StreamStatus != "" {
		errorRaw, _ = json.Marshal(map[string]any{"stream_status": string(input.StreamStatus)})
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_workspace_messages m
		SET client_surface = $2,
		    upstream_surface = $2,
		    route_snapshot = $3,
		    error_json = $4::jsonb
		FROM ai_workspace_threads t
		WHERE m.id = $1
		  AND t.id = m.thread_id
		  AND t.tenant_id = $5
		  AND t.owner_scope = $6
		  AND t.user_id = $7
		  AND t.status <> 'deleted'`,
		messageID, string(input.ClientSurface), routeRaw, errorRaw, owner.TenantID, string(owner.Scope), ownerUserID(owner))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *WorkspaceRepo) UpdateChatSessionRoute(ctx context.Context, owner workspace.Owner, sessionID string, clientSurface surface.ID, routeID string) error {
	var route any
	if routeID != "" {
		route = routeID
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE ai_workspace_threads
		SET selected_surface = $2,
		    selected_route_id = $3,
		    updated_at = now()
		WHERE id = $1
		  AND tenant_id = $4
		  AND owner_scope = $5
		  AND user_id = $6
		  AND target_kind = 'model'
		  AND status <> 'deleted'`,
		sessionID, string(clientSurface), route, owner.TenantID, string(owner.Scope), ownerUserID(owner))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type workspaceGroupSelectionSnapshot struct {
	Name                    string
	EffectiveUserMultiplier float64
}

func (r *WorkspaceRepo) getWorkspaceGroupSelectionSnapshot(ctx context.Context, owner workspace.Owner, groupID string) (workspaceGroupSelectionSnapshot, error) {
	var out workspaceGroupSelectionSnapshot
	if err := r.pool.QueryRow(ctx, `
		SELECT
		  g.name,
		  COALESCE(ug.user_multiplier_override, g.default_user_multiplier)::float8 AS effective_user_multiplier
		FROM ai_groups g
		LEFT JOIN ai_user_groups ug
		  ON ug.group_id = g.id AND ug.tenant_id = $2 AND ug.user_id = $3
		WHERE g.id = $1
		  AND g.tenant_id = $2
		  AND g.status = 'active'
		  AND ($3::text = '' OR g.user_default_visible OR ug.id IS NOT NULL)
	`, groupID, owner.TenantID, owner.UserID).Scan(&out.Name, &out.EffectiveUserMultiplier); err != nil {
		return workspaceGroupSelectionSnapshot{}, err
	}
	return out, nil
}

func scanWorkspaceChatSession(scan func(dest ...any) error) (workspace.ChatSession, error) {
	var (
		item               workspace.ChatSession
		varsRaw            []byte
		targetKind         string
		selectedSurfaceRaw string
	)
	if err := scan(
		&item.ID,
		&item.Title,
		&targetKind,
		&item.AgentID,
		&item.AgentName,
		&varsRaw,
		&item.ModelCode,
		&item.GroupID,
		&item.GroupName,
		&item.EffectiveUserMultiplier,
		&item.BillingGroupLabel,
		&selectedSurfaceRaw,
		&item.SelectedRouteID,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return workspace.ChatSession{}, err
	}
	item.Variables = decodeWorkspaceStringMap(varsRaw)
	item.TargetType = workspace.SessionTypeFromTargetKind(workspace.ThreadTargetKind(targetKind))
	item.SelectedProtocol = workspace.ProtocolFromSurface(surface.ID(selectedSurfaceRaw))
	return item, nil
}

func scanWorkspaceChatMessage(scan func(dest ...any) error) (workspace.ChatMessage, error) {
	var (
		item               workspace.ChatMessage
		clientSurfaceRaw   string
		upstreamSurfaceRaw string
		routeRaw           []byte
		usageRaw           []byte
		errorRaw           []byte
	)
	if err := scan(
		&item.ID,
		&item.Role,
		&item.Content,
		&clientSurfaceRaw,
		&upstreamSurfaceRaw,
		&routeRaw,
		&usageRaw,
		&errorRaw,
		&item.CreatedAt,
	); err != nil {
		return workspace.ChatMessage{}, err
	}
	item.Protocol = workspace.ProtocolFromSurface(surface.ID(strings.TrimSpace(upstreamSurfaceRaw)))
	if item.Protocol == "" {
		item.Protocol = workspace.ProtocolFromSurface(surface.ID(strings.TrimSpace(clientSurfaceRaw)))
	}
	item.RouteID = extractWorkspaceRouteID(routeRaw)
	item.Usage = decodeWorkspaceJSONObject(usageRaw)
	item.Error = decodeWorkspaceJSONObject(errorRaw)
	return item, nil
}

func scanWorkspaceImageJob(scan func(dest ...any) error) (workspace.ImageJob, error) {
	var (
		item          workspace.ImageJob
		taskType      string
		inputPayload  []byte
		resultPayload []byte
		completedAt   pgtype.Timestamptz
	)
	if err := scan(
		&item.ID,
		&taskType,
		&item.ModelCode,
		&item.Status,
		&inputPayload,
		&resultPayload,
		&item.CallerChargeMicro,
		&item.ErrorMessage,
		&item.CreatedAt,
		&completedAt,
	); err != nil {
		return workspace.ImageJob{}, err
	}
	item.Operation = workspaceImageOperation(taskType)
	item.StoragePolicy = "summary_only"
	item.RawImageRetained = false
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	fillWorkspaceImageJobInput(&item, inputPayload)
	fillWorkspaceImageJobSummary(&item, resultPayload)
	return item, nil
}

func ownerTypeFromScope(scope identity.Scope) string {
	if scope == identity.ScopeUser {
		return string(domain.OwnerUser)
	}
	return string(domain.OwnerTenant)
}

func ownerUserID(owner workspace.Owner) string {
	if owner.Scope == identity.ScopeUser {
		return owner.UserID
	}
	return ""
}

func limitParamIndex(args []any) string {
	return fmt.Sprintf("%d", len(args)+1)
}

func decodeWorkspaceStringMap(raw []byte) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]string{}
	}
	norm := make(map[string]string, len(out))
	for key, value := range out {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		norm[key] = strings.TrimSpace(value)
	}
	return norm
}

func decodeWorkspaceJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func extractWorkspaceRouteID(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if routeID, ok := payload["route_id"].(string); ok {
		return strings.TrimSpace(routeID)
	}
	return ""
}

func workspaceImageOperation(taskType string) string {
	if taskType == "console.images.edit" {
		return "edit"
	}
	return "generation"
}

func fillWorkspaceImageJobInput(item *workspace.ImageJob, raw []byte) {
	if item == nil || len(raw) == 0 {
		return
	}
	var payload struct {
		Operation      string `json:"operation"`
		AgentID        string `json:"agent_id"`
		AgentName      string `json:"agent_name"`
		Prompt         string `json:"prompt"`
		N              int    `json:"n"`
		Size           string `json:"size"`
		Quality        string `json:"quality"`
		Style          string `json:"style"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	if payload.Operation != "" {
		item.Operation = payload.Operation
	}
	item.AgentID = payload.AgentID
	item.AgentName = payload.AgentName
	item.Prompt = strings.TrimSpace(payload.Prompt)
	if payload.N > 0 {
		item.RequestedOutputCount = payload.N
	} else {
		item.RequestedOutputCount = domain.DefaultImageOutputCount
	}
	if item.AgentName != "" {
		if item.Prompt != "" {
			item.Prompt = item.AgentName + " · " + item.Prompt
		} else {
			item.Prompt = item.AgentName
		}
	}
	item.Size = payload.Size
	item.Quality = payload.Quality
	item.Style = payload.Style
	item.ResponseFormat = payload.ResponseFormat
}

func fillWorkspaceImageJobSummary(item *workspace.ImageJob, raw []byte) {
	if item == nil || len(raw) == 0 {
		return
	}
	var payload struct {
		ImageCount       int                    `json:"image_count"`
		InlineCount      int                    `json:"inline_count"`
		URLCount         int                    `json:"url_count"`
		StoragePolicy    string                 `json:"storage_policy"`
		RawImageRetained *bool                  `json:"raw_image_retained"`
		Assets           []workspace.ImageAsset `json:"assets"`
		Items            []struct {
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	item.ImageCount = payload.ImageCount
	item.InlineCount = payload.InlineCount
	item.URLCount = payload.URLCount
	if strings.TrimSpace(payload.StoragePolicy) != "" {
		item.StoragePolicy = strings.TrimSpace(payload.StoragePolicy)
	}
	if payload.RawImageRetained != nil {
		item.RawImageRetained = *payload.RawImageRetained
	}
	item.Assets = sanitizeWorkspaceImageAssets(payload.Assets)
	item.RevisedPrompts = item.RevisedPrompts[:0]
	for _, row := range payload.Items {
		if text := strings.TrimSpace(row.RevisedPrompt); text != "" {
			item.RevisedPrompts = append(item.RevisedPrompts, text)
		}
	}
}

func sanitizeWorkspaceImageAssets(items []workspace.ImageAsset) []workspace.ImageAsset {
	if len(items) == 0 {
		return nil
	}
	out := make([]workspace.ImageAsset, 0, len(items))
	for _, item := range items {
		item.PreviewURL, item.DisplayURL, item.OriginalURL = imageassets.NormalizeAssetURLs(
			item.PreviewURL,
			item.DisplayURL,
			item.OriginalURL,
		)
		if item.PreviewURL == "" && item.DisplayURL == "" && item.OriginalURL == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}
