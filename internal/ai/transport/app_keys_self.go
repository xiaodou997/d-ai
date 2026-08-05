package transport

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"

	pgadapter "xiaodou/dai/internal/ai/adapters/postgres"
	"xiaodou/dai/internal/ai/appkey"
	"xiaodou/dai/internal/ai/application"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/secret"
)

// runKeyDTO is the app key wire shape. Every app key is bound to exactly one
// published app — there is no direct-model target.
type runKeyDTO struct {
	ID               string  `json:"id"`
	OwnerType        string  `json:"owner_type"`
	TenantID         string  `json:"tenant_id"`
	UserID           string  `json:"user_id,omitempty"`
	Name             string  `json:"name"`
	LastFour         string  `json:"last_four"`
	Status           string  `json:"status"`
	AppID            string  `json:"agent_id"`
	AppName          string  `json:"agent_name,omitempty"`
	AppOwnerType     string  `json:"agent_owner_type,omitempty"`
	AppOwnerTenantID string  `json:"agent_owner_tenant_id,omitempty"`
	ExpiresAt        *int64  `json:"expires_at,omitempty"`
	CreatedBy        *string `json:"created_by,omitempty"`
	CreatedAt        *int64  `json:"created_at,omitempty"`
	UpdatedAt        *int64  `json:"updated_at,omitempty"`
}

type runKeysOutput struct {
	Body struct {
		Items    []runKeyDTO         `json:"items"`
		Total    int                 `json:"total"`
		Included IdentityIncludedDTO `json:"included"`
	}
}

type runKeyOutput struct {
	Body runKeyDTO
}

type runKeyCreatedOutput struct {
	Body struct {
		PlaintextKey string    `json:"plaintext_key"`
		Key          runKeyDTO `json:"key"`
	}
}

type runKeyRevealOutput struct {
	Body struct {
		PlaintextKey string `json:"plaintext_key"`
	}
}

type runKeyDeleteOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

type visibleAppsOutput struct {
	Body struct {
		Items []consumerAppDTO `json:"items"`
		Total int              `json:"total"`
	}
}

type runKeyWriteRequest struct {
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
	AppID     string `json:"agent_id"`
	ExpiresAt *int64 `json:"expires_at,omitempty"`
}

type tenantRunKeyCreateInput struct {
	Body runKeyWriteRequest
}

type tenantRunKeyUpdateInput struct {
	RunKeyID string `path:"runKeyID"`
	Body     struct {
		Name      *string `json:"name,omitempty"`
		Status    *string `json:"status,omitempty"`
		AppID     *string `json:"agent_id,omitempty"`
		ExpiresAt *int64  `json:"expires_at,omitempty"`
	}
}

type tenantRunKeyIDInput struct {
	RunKeyID string `path:"runKeyID"`
}

func registerTenantSelfRunKeys(api huma.API, d AIDeps) {
	registerRunKeyEndpoints(api, d, domain.OwnerTenant)
}

func registerUserSelfRunKeys(api huma.API, d AIDeps) {
	registerRunKeyEndpoints(api, d, domain.OwnerUser)
}

func registerRunKeyEndpoints(api huma.API, d AIDeps, ownerType domain.OwnerType) {
	pathBase := "/api/v1/tenants/me"
	summaryOwner := "租户"
	if ownerType == domain.OwnerUser {
		pathBase = "/api/v1/users/me"
		summaryOwner = "终端用户"
	}

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-visible-public-apps-" + string(ownerType),
		Method:      http.MethodGet,
		Path:        pathBase + "/app-agents",
		Summary:     summaryOwner + "可见应用列表",
		Tags:        []string{"app-agents"},
	}, func(ctx context.Context, _ *struct{}) (*visibleAppsOutput, error) {
		items, err := listVisibleApps(ctx, d, appViewerForOwner(ctx, ownerType))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &visibleAppsOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-list-app-keys-" + string(ownerType),
		Method:      http.MethodGet,
		Path:        pathBase + "/app-keys",
		Summary:     summaryOwner + "应用密钥列表",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, _ *struct{}) (*runKeysOutput, error) {
		items, err := listRunKeys(ctx, d, ownerType)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeysOutput{}
		out.Body.Items = items
		out.Body.Total = len(items)
		out.Body.Included = emptyIdentityIncluded()
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "ai-create-app-key-" + string(ownerType),
		Method:        http.MethodPost,
		Path:          pathBase + "/app-keys",
		Summary:       "创建" + summaryOwner + "应用密钥",
		Tags:          []string{"app-keys"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *tenantRunKeyCreateInput) (*runKeyCreatedOutput, error) {
		write, err := normalizeRunKeyCreate(in.Body)
		if err != nil {
			return nil, mapServiceError(err)
		}
		created, err := createRunKey(ctx, d, ownerType, write)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeyCreatedOutput{}
		out.Body.PlaintextKey = created.plaintext
		out.Body.Key = created.item
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-update-app-key-" + string(ownerType),
		Method:      http.MethodPatch,
		Path:        pathBase + "/app-keys/{runKeyID}",
		Summary:     "更新" + summaryOwner + "应用密钥",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, in *tenantRunKeyUpdateInput) (*runKeyOutput, error) {
		patch, err := normalizeRunKeyPatch(in)
		if err != nil {
			return nil, mapServiceError(err)
		}
		item, err := updateRunKey(ctx, d, ownerType, strings.TrimSpace(in.RunKeyID), patch)
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeyOutput{}
		out.Body = item
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-reveal-app-key-" + string(ownerType),
		Method:      http.MethodPost,
		Path:        pathBase + "/app-keys/{runKeyID}/reveal",
		Summary:     "查看" + summaryOwner + "应用密钥明文",
		Description: "解密返回该应用密钥的完整明文，可反复调用查看/复制。",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, in *tenantRunKeyIDInput) (*runKeyRevealOutput, error) {
		plaintext, err := revealRunKey(ctx, d, ownerType, strings.TrimSpace(in.RunKeyID))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeyRevealOutput{}
		out.Body.PlaintextKey = plaintext
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-rotate-app-key-" + string(ownerType),
		Method:      http.MethodPost,
		Path:        pathBase + "/app-keys/{runKeyID}/rotate",
		Summary:     "轮换" + summaryOwner + "应用密钥",
		Description: "生成一把新密钥并立即使旧密钥失效，应用绑定与其它元数据保持不变。",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, in *tenantRunKeyIDInput) (*runKeyCreatedOutput, error) {
		created, err := rotateRunKey(ctx, d, ownerType, strings.TrimSpace(in.RunKeyID))
		if err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeyCreatedOutput{}
		out.Body.PlaintextKey = created.plaintext
		out.Body.Key = created.item
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ai-delete-app-key-" + string(ownerType),
		Method:      http.MethodDelete,
		Path:        pathBase + "/app-keys/{runKeyID}",
		Summary:     "删除" + summaryOwner + "应用密钥",
		Tags:        []string{"app-keys"},
	}, func(ctx context.Context, in *tenantRunKeyIDInput) (*runKeyDeleteOutput, error) {
		if err := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres).DeleteInvokeKey(ctx, string(ownerType), tenantIDForOwner(ctx), userIDForOwner(ctx, ownerType), strings.TrimSpace(in.RunKeyID)); err != nil {
			return nil, mapServiceError(err)
		}
		out := &runKeyDeleteOutput{}
		out.Body.Deleted = true
		return out, nil
	})
}

type runKeyRow struct {
	ID               string
	OwnerType        string
	TenantID         string
	UserID           string
	Name             string
	LastFour         string
	Status           string
	AppID            string
	AppName          string
	AppOwnerType     string
	AppOwnerTenantID string
	ExpiresAt        *int64
	CreatedBy        *string
	CreatedAt        *int64
	UpdatedAt        *int64
}

func listRunKeys(ctx context.Context, d AIDeps, ownerType domain.OwnerType) ([]runKeyDTO, error) {
	return listRunKeysByScope(ctx, d, string(ownerType), tenantIDForOwner(ctx), userIDForOwner(ctx, ownerType))
}

type runKeyCreate struct {
	Name      string
	Status    string
	AppID     string
	ExpiresAt *time.Time
}

type createdRunKey struct {
	item      runKeyDTO
	plaintext string
}

func normalizeRunKeyCreate(in runKeyWriteRequest) (runKeyCreate, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return runKeyCreate{}, domain.NewValidationError("name", "is required")
	}
	status, err := normalizePromptStatus(in.Status)
	if err != nil {
		return runKeyCreate{}, err
	}
	appID := strings.TrimSpace(in.AppID)
	if appID == "" {
		return runKeyCreate{}, domain.NewValidationError("agent_id", "is required")
	}
	var expiresAt *time.Time
	if in.ExpiresAt != nil {
		t := time.UnixMilli(*in.ExpiresAt)
		expiresAt = &t
	}
	return runKeyCreate{
		Name:      name,
		Status:    status,
		AppID:     appID,
		ExpiresAt: expiresAt,
	}, nil
}

func createRunKey(ctx context.Context, d AIDeps, ownerType domain.OwnerType, write runKeyCreate) (createdRunKey, error) {
	if err := ensureVisibleActiveApp(ctx, d, appViewerForOwner(ctx, ownerType), write.AppID); err != nil {
		return createdRunKey{}, err
	}
	plaintext, err := appkey.Generate()
	if err != nil {
		return createdRunKey{}, err
	}
	ciphertext, err := secret.EncryptProviderKey(d.ProviderKeyMaster, plaintext)
	if err != nil {
		return createdRunKey{}, err
	}
	repo := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres)
	item, err := repo.CreateInvokeKey(ctx, application.InvokeKeyWrite{
		OwnerScope:    string(ownerType),
		TenantID:      tenantIDForOwner(ctx),
		UserID:        userIDForOwner(ctx, ownerType),
		Name:          write.Name,
		KeyHash:       appkey.Hash(plaintext),
		KeyCiphertext: ciphertext,
		LastFour:      appkey.LastFour(plaintext),
		Status:        application.Status(write.Status),
		AppID:         write.AppID,
		ExpiresAt:     write.ExpiresAt,
		CreatedBy:     claimsUserID(ctx),
	})
	if err != nil {
		return createdRunKey{}, err
	}
	row := invokeKeyToRunKeyRow(item)
	populateRunKeyAppFields(ctx, d, &row)
	return createdRunKey{item: runKeyRowToDTO(row), plaintext: plaintext}, nil
}

type runKeyPatch struct {
	Name      *string
	Status    *string
	AppID     *string
	ExpiresAt **time.Time
}

func normalizeRunKeyPatch(in *tenantRunKeyUpdateInput) (runKeyPatch, error) {
	var out runKeyPatch
	if in.Body.Name != nil {
		value := strings.TrimSpace(*in.Body.Name)
		if value == "" {
			return out, domain.NewValidationError("name", "cannot be empty")
		}
		out.Name = &value
	}
	if in.Body.Status != nil {
		value, err := normalizePromptStatus(*in.Body.Status)
		if err != nil {
			return out, err
		}
		out.Status = &value
	}
	if in.Body.AppID != nil {
		value := strings.TrimSpace(*in.Body.AppID)
		if value == "" {
			return out, domain.NewValidationError("agent_id", "cannot be empty")
		}
		out.AppID = &value
	}
	if in.Body.ExpiresAt != nil {
		t := time.UnixMilli(*in.Body.ExpiresAt)
		tPtr := &t
		out.ExpiresAt = &tPtr
	}
	if out.Name == nil && out.Status == nil && out.AppID == nil && out.ExpiresAt == nil {
		return out, domain.NewValidationError("", "no fields to update")
	}
	return out, nil
}

func updateRunKey(ctx context.Context, d AIDeps, ownerType domain.OwnerType, runKeyID string, patch runKeyPatch) (runKeyDTO, error) {
	current, err := getRunKey(ctx, d, ownerType, runKeyID)
	if err != nil {
		return runKeyDTO{}, err
	}
	name := current.Name
	if patch.Name != nil {
		name = *patch.Name
	}
	status := current.Status
	if patch.Status != nil {
		status = *patch.Status
	}
	appID := current.AppID
	if patch.AppID != nil {
		appID = *patch.AppID
	}
	var expiresAtValue *time.Time
	if current.ExpiresAt != nil {
		t := time.UnixMilli(*current.ExpiresAt)
		expiresAtValue = &t
	}
	if patch.ExpiresAt != nil {
		expiresAtValue = *patch.ExpiresAt
	}
	if err := ensureVisibleActiveApp(ctx, d, appViewerForOwner(ctx, ownerType), appID); err != nil {
		return runKeyDTO{}, err
	}
	repo := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres)
	item, err := repo.UpdateInvokeKey(ctx, runKeyID, application.InvokeKeyWrite{
		OwnerScope: string(ownerType),
		TenantID:   tenantIDForOwner(ctx),
		UserID:     userIDForOwner(ctx, ownerType),
		Name:       name,
		Status:     application.Status(status),
		AppID:      appID,
		ExpiresAt:  expiresAtValue,
	})
	if err != nil {
		return runKeyDTO{}, err
	}
	row := invokeKeyToRunKeyRow(item)
	populateRunKeyAppFields(ctx, d, &row)
	return runKeyRowToDTO(row), nil
}

func getRunKey(ctx context.Context, d AIDeps, ownerType domain.OwnerType, runKeyID string) (runKeyDTO, error) {
	item, err := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres).GetInvokeKeyByID(ctx, string(ownerType), tenantIDForOwner(ctx), userIDForOwner(ctx, ownerType), runKeyID)
	if err != nil {
		return runKeyDTO{}, mapInvokeKeyRepoError(err)
	}
	row := invokeKeyToRunKeyRow(item)
	populateRunKeyAppFields(ctx, d, &row)
	return runKeyRowToDTO(row), nil
}

func revealRunKey(ctx context.Context, d AIDeps, ownerType domain.OwnerType, runKeyID string) (string, error) {
	ciphertext, err := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres).RevealInvokeKey(ctx, string(ownerType), tenantIDForOwner(ctx), userIDForOwner(ctx, ownerType), runKeyID)
	if err != nil {
		return "", mapInvokeKeyRepoError(err)
	}
	if strings.TrimSpace(ciphertext) == "" {
		return "", domain.ErrNotFound
	}
	return secret.DecryptProviderKey(d.ProviderKeyMaster, ciphertext)
}

func rotateRunKey(ctx context.Context, d AIDeps, ownerType domain.OwnerType, runKeyID string) (createdRunKey, error) {
	plaintext, err := appkey.Generate()
	if err != nil {
		return createdRunKey{}, err
	}
	ciphertext, err := secret.EncryptProviderKey(d.ProviderKeyMaster, plaintext)
	if err != nil {
		return createdRunKey{}, err
	}
	repo := pgadapter.NewApplicationInvokeKeyRepo(d.Postgres)
	item, err := repo.RotateInvokeKey(ctx, runKeyID, application.InvokeKeyRotate{
		OwnerScope:    string(ownerType),
		TenantID:      tenantIDForOwner(ctx),
		UserID:        userIDForOwner(ctx, ownerType),
		KeyHash:       appkey.Hash(plaintext),
		KeyCiphertext: ciphertext,
		LastFour:      appkey.LastFour(plaintext),
	})
	if err != nil {
		return createdRunKey{}, mapInvokeKeyRepoError(err)
	}
	row := invokeKeyToRunKeyRow(item)
	populateRunKeyAppFields(ctx, d, &row)
	return createdRunKey{item: runKeyRowToDTO(row), plaintext: plaintext}, nil
}

func ensureVisibleActiveApp(ctx context.Context, d AIDeps, viewer pgadapter.AppViewer, appID string) error {
	if strings.TrimSpace(appID) == "" {
		return domain.NewValidationError("agent_id", "app is not available")
	}
	if _, err := pgadapter.NewApplicationAppRepo(d.Postgres).GetVisibleAgentByID(ctx, viewer, appID, []string{"chat", "image_generation", "image_edit"}); err != nil {
		if err == pgx.ErrNoRows {
			return domain.NewValidationError("agent_id", "app is not available")
		}
		return err
	}
	return nil
}

func runKeyRowToDTO(row runKeyRow) runKeyDTO {
	return runKeyDTO{
		ID:               row.ID,
		OwnerType:        row.OwnerType,
		TenantID:         row.TenantID,
		UserID:           row.UserID,
		Name:             row.Name,
		LastFour:         row.LastFour,
		Status:           row.Status,
		AppID:            row.AppID,
		AppName:          row.AppName,
		AppOwnerType:     row.AppOwnerType,
		AppOwnerTenantID: row.AppOwnerTenantID,
		ExpiresAt:        row.ExpiresAt,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func invokeKeyToRunKeyRow(item application.InvokeKey) runKeyRow {
	var expiresAt *int64
	if item.ExpiresAt != nil {
		v := item.ExpiresAt.UnixMilli()
		expiresAt = &v
	}
	return runKeyRow{
		ID:        item.ID,
		OwnerType: string(item.OwnerScope),
		TenantID:  item.TenantID,
		UserID:    item.UserID,
		Name:      item.Name,
		LastFour:  item.LastFour,
		Status:    string(item.Status),
		AppID:     item.AppID,
		ExpiresAt: expiresAt,
		CreatedBy: stringPtrOrNil(item.CreatedBy),
		CreatedAt: timePtrToMillis(&item.CreatedAt),
		UpdatedAt: timePtrToMillis(&item.UpdatedAt),
	}
}

func mapInvokeKeyRepoError(err error) error {
	if err == pgx.ErrNoRows {
		return domain.ErrNotFound
	}
	return err
}

func populateRunKeyAppFields(ctx context.Context, d AIDeps, row *runKeyRow) {
	if row == nil || strings.TrimSpace(row.AppID) == "" {
		return
	}
	viewer := pgadapter.AppViewer{TenantID: row.TenantID}
	if row.OwnerType == string(domain.OwnerUser) {
		viewer.UserID = row.UserID
	}
	agent, err := pgadapter.NewApplicationAppRepo(d.Postgres).GetVisibleAgentByID(ctx, viewer, row.AppID, []string{"chat", "image_generation", "image_edit"})
	if err != nil {
		return
	}
	row.AppName = agent.Name
	row.AppOwnerType = agent.OwnerType
	row.AppOwnerTenantID = agent.OwnerTenantID
}

func appViewerForOwner(ctx context.Context, ownerType domain.OwnerType) pgadapter.AppViewer {
	return pgadapter.AppViewer{
		TenantID: tenantIDForOwner(ctx),
		UserID:   userIDForOwner(ctx, ownerType),
	}
}

func tenantIDForOwner(ctx context.Context) string {
	return tenantIDFromContext(ctx)
}

func userIDForOwner(ctx context.Context, ownerType domain.OwnerType) string {
	if ownerType == domain.OwnerUser {
		return userIDFromContext(ctx)
	}
	return ""
}
