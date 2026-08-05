package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/subscription"
)

// SubscriptionRepo 是订阅领域的 postgres 适配器（sqlc + finalize 事务）。
type SubscriptionRepo struct {
	q    *dbgen.Queries
	pool *pgxpool.Pool
}

func NewSubscriptionRepo(q *dbgen.Queries, pool *pgxpool.Pool) *SubscriptionRepo {
	return &SubscriptionRepo{q: q, pool: pool}
}

var _ subscription.Repo = (*SubscriptionRepo)(nil)

// ---- pgtype ↔ 原生类型转换 ----

func ts(t pgtype.Timestamptz) time.Time { return t.Time } // !Valid ⇒ 零值
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}
func ptrInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}
func ptrInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}
func stringsToUUIDs(ids []string) []pgtype.UUID {
	out := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		out[i] = mustParseUUID(id)
	}
	return out
}
func txtStr(t pgtype.Text) string { return t.String }

// nargText 把空串映射为 SQL NULL（narg 可选筛选用）。
func nargText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ---- 行映射 ----

func toPlan(r dbgen.AiSubPlan) subscription.Plan {
	return subscription.Plan{
		ID:                 uuidToString(r.ID),
		TenantID:           r.TenantID,
		Name:               r.Name,
		Description:        r.Description,
		PriceCredits:       r.PriceCredits,
		DurationDays:       r.DurationDays,
		TotalLimitMicro:    r.TotalLimitMicro,
		Window5hLimitMicro: int8Ptr(r.Window5hLimitMicro),
		Window7dLimitMicro: int8Ptr(r.Window7dLimitMicro),
		Status:             r.Status,
		SortOrder:          r.SortOrder,
		SaleLimit:          int4Ptr(r.SaleLimit),
		SoldCount:          r.SoldCount,
		ReservedCount:      r.ReservedCount,
		CreatedBy:          txtStr(r.CreatedBy),
		CreatedAt:          ts(r.CreatedAt),
		UpdatedAt:          ts(r.UpdatedAt),
		PurchasePolicy:     subscription.DefaultPurchasePolicy(),
	}
}

func toPurchasePolicy(r dbgen.AiSubPlanPurchasePolicy) subscription.PurchasePolicy {
	return subscription.PurchasePolicy{
		LifetimeMaxPurchases: int4Ptr(r.LifetimeMaxPurchases),
		PeriodType:           r.PeriodType,
		PeriodMaxPurchases:   int4Ptr(r.PeriodMaxPurchases),
		RollingWindowHours:   int4Ptr(r.RollingWindowHours),
		CalendarUnit:         txtStr(r.CalendarUnit),
		CalendarTimezone:     txtStr(r.CalendarTimezone),
		AllowAdvancePurchase: r.AllowAdvancePurchase,
		Version:              r.Version,
		CreatedAt:            ts(r.CreatedAt),
		UpdatedAt:            ts(r.UpdatedAt),
	}
}

func marshalPurchasePolicy(policy subscription.PurchasePolicy) ([]byte, error) {
	return json.Marshal(subscription.NormalizePurchasePolicy(policy))
}

func parsePurchasePolicy(raw []byte) subscription.PurchasePolicy {
	var policy subscription.PurchasePolicy
	if len(raw) == 0 || json.Unmarshal(raw, &policy) != nil {
		return subscription.DefaultPurchasePolicy()
	}
	return subscription.NormalizePurchasePolicy(policy)
}

func toOrder(r dbgen.AiSubOrder) subscription.Order {
	return subscription.Order{
		ID:                                 uuidToString(r.ID),
		OrderNo:                            r.OrderNo,
		TenantID:                           r.TenantID,
		UserID:                             r.UserID,
		PlanID:                             uuidToString(r.PlanID),
		PlanNameSnapshot:                   r.PlanNameSnapshot,
		PriceCredits:                       r.PriceCredits,
		DurationDaysSnapshot:               r.DurationDaysSnapshot,
		TotalLimitMicroSnapshot:            r.TotalLimitMicroSnapshot,
		Window5hLimitMicroSnapshot:         int8Ptr(r.Window5hLimitMicroSnapshot),
		Window7dLimitMicroSnapshot:         int8Ptr(r.Window7dLimitMicroSnapshot),
		GroupQuotaDebitMultipliersSnapshot: parseGroupQuotaDebitMultipliers(r.GroupQuotaDebitMultipliersSnapshot),
		PurchasePolicyVersion:              r.PurchasePolicyVersion,
		PurchasePolicySnapshot:             parsePurchasePolicy(r.PurchasePolicySnapshot),
		InventoryReserved:                  r.InventoryReserved,
		Status:                             r.Status,
		URMEventID:                         txtStr(r.UrmEventID),
		SubscriptionID:                     uuidToString(r.SubscriptionID),
		FailReason:                         txtStr(r.FailReason),
		PaidAt:                             tsPtr(r.PaidAt),
		CreatedAt:                          ts(r.CreatedAt),
		UpdatedAt:                          ts(r.UpdatedAt),
	}
}

func purchasePolicyCreateParams(planID string, policy subscription.PurchasePolicy) dbgen.CreatePlanPurchasePolicyParams {
	policy = subscription.NormalizePurchasePolicy(policy)
	return dbgen.CreatePlanPurchasePolicyParams{
		PlanID:               mustParseUUID(planID),
		LifetimeMaxPurchases: ptrInt4(policy.LifetimeMaxPurchases),
		PeriodType:           policy.PeriodType,
		PeriodMaxPurchases:   ptrInt4(policy.PeriodMaxPurchases),
		RollingWindowHours:   ptrInt4(policy.RollingWindowHours),
		CalendarUnit:         nullableText(policy.CalendarUnit),
		CalendarTimezone:     nullableText(policy.CalendarTimezone),
		AllowAdvancePurchase: policy.AllowAdvancePurchase,
	}
}

func purchasePolicyUpdateParams(planID string, policy subscription.PurchasePolicy) dbgen.UpdatePlanPurchasePolicyParams {
	create := purchasePolicyCreateParams(planID, policy)
	return dbgen.UpdatePlanPurchasePolicyParams{
		PlanID:               create.PlanID,
		LifetimeMaxPurchases: create.LifetimeMaxPurchases,
		PeriodType:           create.PeriodType,
		PeriodMaxPurchases:   create.PeriodMaxPurchases,
		RollingWindowHours:   create.RollingWindowHours,
		CalendarUnit:         create.CalendarUnit,
		CalendarTimezone:     create.CalendarTimezone,
		AllowAdvancePurchase: create.AllowAdvancePurchase,
	}
}

func insertPurchasePolicyRevision(ctx context.Context, q *dbgen.Queries, planID, changedBy string, policy subscription.PurchasePolicy) error {
	raw, err := marshalPurchasePolicy(policy)
	if err != nil {
		return err
	}
	return q.InsertPlanPurchasePolicyRevision(ctx, dbgen.InsertPlanPurchasePolicyRevisionParams{
		PlanID:         mustParseUUID(planID),
		Version:        policy.Version,
		PolicySnapshot: raw,
		ChangedBy:      nullableText(changedBy),
	})
}

func loadPlanPolicies(ctx context.Context, q *dbgen.Queries, planIDs []string) (map[string]subscription.PurchasePolicy, error) {
	out := make(map[string]subscription.PurchasePolicy, len(planIDs))
	if len(planIDs) == 0 {
		return out, nil
	}
	ids := make([]pgtype.UUID, len(planIDs))
	for i, id := range planIDs {
		ids[i] = mustParseUUID(id)
	}
	rows, err := q.ListPlanPurchasePolicies(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[uuidToString(row.PlanID)] = toPurchasePolicy(row)
	}
	return out, nil
}

func toSub(r dbgen.AiSubSubscription) subscription.Subscription {
	return subscription.Subscription{
		ID:                         uuidToString(r.ID),
		TenantID:                   r.TenantID,
		UserID:                     r.UserID,
		PlanID:                     uuidToString(r.PlanID),
		OrderID:                    uuidToString(r.OrderID),
		PlanNameSnapshot:           r.PlanNameSnapshot,
		DurationDays:               r.DurationDays,
		TotalLimitMicro:            r.TotalLimitMicro,
		Window5hLimitMicro:         int8Ptr(r.Window5hLimitMicro),
		Window7dLimitMicro:         int8Ptr(r.Window7dLimitMicro),
		Status:                     r.Status,
		ActivatedAt:                tsPtr(r.ActivatedAt),
		ExpiresAt:                  tsPtr(r.ExpiresAt),
		TotalUsedMicro:             r.TotalUsedMicro,
		Win5hStart:                 tsPtr(r.Win5hStart),
		Win5hUsedMicro:             r.Win5hUsedMicro,
		Win7dStart:                 tsPtr(r.Win7dStart),
		Win7dUsedMicro:             r.Win7dUsedMicro,
		GroupQuotaDebitMultipliers: parseGroupQuotaDebitMultipliers(r.GroupQuotaDebitMultipliers),
		CreatedAt:                  ts(r.CreatedAt),
		UpdatedAt:                  ts(r.UpdatedAt),
	}
}

// parseGroupQuotaDebitMultipliers 解析订阅的分组权重快照 JSONB；空/空对象/损坏 → nil（gate 按不覆盖处理）。
func parseGroupQuotaDebitMultipliers(raw []byte) map[string]float64 {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]float64
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return nil
	}
	return m
}

// marshalGroupQuotaDebitMultipliers 把套餐分组序列化为 {group_id: quota_debit_multiplier} 快照（空 → {}）。
func marshalGroupQuotaDebitMultipliers(groups []subscription.PlanGroup) []byte {
	m := make(map[string]float64, len(groups))
	for _, g := range groups {
		m[g.GroupID] = g.QuotaDebitMultiplier
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

// loadPlanGroups 批量读若干套餐的分组绑定（含分组名与权重），按 plan_id 分组返回。
func (r *SubscriptionRepo) loadPlanGroups(ctx context.Context, planIDs []string) (map[string][]subscription.PlanGroup, error) {
	return loadPlanGroupsWithQueries(ctx, r.q, planIDs)
}

func loadPlanGroupsWithQueries(ctx context.Context, q *dbgen.Queries, planIDs []string) (map[string][]subscription.PlanGroup, error) {
	out := make(map[string][]subscription.PlanGroup, len(planIDs))
	if len(planIDs) == 0 {
		return out, nil
	}
	uuids := make([]pgtype.UUID, len(planIDs))
	for i, id := range planIDs {
		uuids[i] = mustParseUUID(id)
	}
	rows, err := q.ListPlanGroupsForPlans(ctx, uuids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		pid := uuidToString(row.PlanID)
		out[pid] = append(out[pid], subscription.PlanGroup{
			GroupID:              uuidToString(row.GroupID),
			Name:                 row.GroupName,
			QuotaDebitMultiplier: numericToFloat(row.QuotaDebitMultiplier),
			SortOrder:            row.SortOrder,
		})
	}
	return out, nil
}

func toSubs(rs []dbgen.AiSubSubscription) []subscription.Subscription {
	out := make([]subscription.Subscription, len(rs))
	for i, r := range rs {
		out[i] = toSub(r)
	}
	return out
}

// ---- 套餐 ----

func (r *SubscriptionRepo) CreatePlan(ctx context.Context, p subscription.CreatePlanParams) (*subscription.Plan, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	sortOrder, err := q.NextPlanSortOrder(ctx, p.TenantID)
	if err != nil {
		return nil, err
	}

	row, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		TenantID:           p.TenantID,
		Name:               p.Name,
		Description:        p.Description,
		PriceCredits:       p.PriceCredits,
		DurationDays:       p.DurationDays,
		TotalLimitMicro:    p.TotalLimitMicro,
		Window5hLimitMicro: ptrInt8(p.Window5hLimitMicro),
		Window7dLimitMicro: ptrInt8(p.Window7dLimitMicro),
		Status:             subscription.PlanDraft,
		SortOrder:          sortOrder,
		SaleLimit:          ptrInt4(p.SaleLimit),
		CreatedBy:          nullableText(p.CreatedBy),
	})
	if err != nil {
		return nil, err
	}
	if err := insertPlanGroups(ctx, q, uuidToString(row.ID), p.Groups); err != nil {
		return nil, err
	}
	policyRow, err := q.CreatePlanPurchasePolicy(ctx, purchasePolicyCreateParams(uuidToString(row.ID), p.PurchasePolicy))
	if err != nil {
		return nil, err
	}
	policy := toPurchasePolicy(policyRow)
	if err := insertPurchasePolicyRevision(ctx, q, uuidToString(row.ID), p.CreatedBy, policy); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	pl := toPlan(row)
	pl.Groups = p.Groups
	pl.PurchasePolicy = policy
	return &pl, nil
}

// insertPlanGroups 覆盖写套餐分组绑定（调用方已 DELETE 或新建）。
func insertPlanGroups(ctx context.Context, q *dbgen.Queries, planID string, groups []subscription.PlanGroup) error {
	for i, g := range groups {
		sortOrder := g.SortOrder
		if sortOrder == 0 {
			sortOrder = int32(i)
		}
		if err := q.InsertPlanGroup(ctx, dbgen.InsertPlanGroupParams{
			PlanID:               mustParseUUID(planID),
			GroupID:              mustParseUUID(g.GroupID),
			QuotaDebitMultiplier: floatToNumeric(g.QuotaDebitMultiplier),
			SortOrder:            sortOrder,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *SubscriptionRepo) GetPlan(ctx context.Context, id string) (*subscription.Plan, error) {
	row, err := r.q.GetPlan(ctx, mustParseUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, subscription.ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	pl := toPlan(row)
	byPlan, err := r.loadPlanGroups(ctx, []string{pl.ID})
	if err != nil {
		return nil, err
	}
	pl.Groups = byPlan[pl.ID]
	policies, err := loadPlanPolicies(ctx, r.q, []string{pl.ID})
	if err != nil {
		return nil, err
	}
	if policy, ok := policies[pl.ID]; ok {
		pl.PurchasePolicy = policy
	}
	return &pl, nil
}

// UpdatePlanByTenant 事务改套餐并覆盖写分组绑定（delete+insert）。改动只影响新购。
func (r *SubscriptionRepo) UpdatePlanByTenant(ctx context.Context, p subscription.UpdatePlanParams) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)

	n, err := q.UpdatePlanByTenant(ctx, dbgen.UpdatePlanByTenantParams{
		ID:                 mustParseUUID(p.ID),
		TenantID:           p.TenantID,
		Name:               p.Name,
		Description:        p.Description,
		PriceCredits:       p.PriceCredits,
		DurationDays:       p.DurationDays,
		TotalLimitMicro:    p.TotalLimitMicro,
		Window5hLimitMicro: ptrInt8(p.Window5hLimitMicro),
		Window7dLimitMicro: ptrInt8(p.Window7dLimitMicro),
		SortOrder:          p.SortOrder,
		SaleLimit:          ptrInt4(p.SaleLimit),
	})
	if err != nil {
		return false, err
	}
	if n == 0 { // 不属于本租户，不触碰分组。
		return false, nil
	}
	if err := q.DeletePlanGroups(ctx, mustParseUUID(p.ID)); err != nil {
		return false, err
	}
	if err := insertPlanGroups(ctx, q, p.ID, p.Groups); err != nil {
		return false, err
	}
	if p.PurchasePolicy != nil {
		policyRow, err := q.UpdatePlanPurchasePolicy(ctx, purchasePolicyUpdateParams(p.ID, *p.PurchasePolicy))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
		if err == nil {
			policy := toPurchasePolicy(policyRow)
			if err := insertPurchasePolicyRevision(ctx, q, p.ID, p.UpdatedBy, policy); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *SubscriptionRepo) SetPlanStatusByTenant(ctx context.Context, id, tenantID, status string) (bool, error) {
	n, err := r.q.SetPlanStatusByTenant(ctx, dbgen.SetPlanStatusByTenantParams{
		ID: mustParseUUID(id), TenantID: tenantID, Status: status,
	})
	return n > 0, err
}

func (r *SubscriptionRepo) ReorderPlansByTenant(ctx context.Context, tenantID string, planIDs []string) error {
	ids := stringsToUUIDs(planIDs)
	n, err := r.q.ReorderPlansByTenant(ctx, dbgen.ReorderPlansByTenantParams{
		TenantID: tenantID,
		Column2:  ids,
	})
	if err != nil {
		return err
	}
	if n != int64(len(ids)) {
		return subscription.ErrPlanReorderInvalid
	}
	return nil
}

func (r *SubscriptionRepo) ListPlans(ctx context.Context, f subscription.PlanFilter) ([]subscription.Plan, int64, error) {
	if f.OnSaleOnly {
		total, err := r.q.CountAvailableOnSalePlansPage(ctx, f.TenantID)
		if err != nil {
			return nil, 0, err
		}
		rows, err := r.q.ListAvailableOnSalePlansPage(ctx, dbgen.ListAvailableOnSalePlansPageParams{
			TenantID: f.TenantID, Limit: f.Limit, Offset: f.Offset,
		})
		if err != nil {
			return nil, 0, err
		}
		return r.plansWithGroups(ctx, rows, total)
	}
	status := f.Status
	total, err := r.q.CountPlansPage(ctx, dbgen.CountPlansPageParams{
		TenantID: nargText(f.TenantID), Status: nargText(status),
	})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListPlansPage(ctx, dbgen.ListPlansPageParams{
		TenantID: nargText(f.TenantID), Status: nargText(status), Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	return r.plansWithGroups(ctx, rows, total)
}

func (r *SubscriptionRepo) plansWithGroups(ctx context.Context, rows []dbgen.AiSubPlan, total int64) ([]subscription.Plan, int64, error) {
	out := make([]subscription.Plan, len(rows))
	ids := make([]string, len(rows))
	for i, row := range rows {
		out[i] = toPlan(row)
		ids[i] = out[i].ID
	}
	byPlan, err := r.loadPlanGroups(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	policies, err := loadPlanPolicies(ctx, r.q, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].Groups = byPlan[out[i].ID]
		if policy, ok := policies[out[i].ID]; ok {
			out[i].PurchasePolicy = policy
		}
	}
	return out, total, nil
}

func (r *SubscriptionRepo) ListPurchasePolicyRevisions(ctx context.Context, planID string) ([]subscription.PurchasePolicyRevision, error) {
	rows, err := r.q.ListPlanPurchasePolicyRevisions(ctx, mustParseUUID(planID))
	if err != nil {
		return nil, err
	}
	out := make([]subscription.PurchasePolicyRevision, 0, len(rows))
	for _, row := range rows {
		policy := parsePurchasePolicy(row.PolicySnapshot)
		out = append(out, subscription.PurchasePolicyRevision{
			PlanID: uuidToString(row.PlanID), Version: row.Version, Policy: policy,
			ChangedBy: txtStr(row.ChangedBy), ChangedAt: ts(row.ChangedAt),
		})
	}
	return out, nil
}

func (r *SubscriptionRepo) AcquirePlanLock(ctx context.Context, planID string) (func(), error) {
	return r.acquireAdvisoryLock(ctx, "subscription-plan:"+planID)
}

// ValidateGroupsForTenant 返回入参分组中 active 且对该租户可见的子集。
func (r *SubscriptionRepo) ValidateGroupsForTenant(ctx context.Context, tenantID string, groupIDs []string) ([]string, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	uuids := make([]pgtype.UUID, len(groupIDs))
	for i, id := range groupIDs {
		uuids[i] = mustParseUUID(id)
	}
	return r.q.ValidateGroupsForTenant(ctx, dbgen.ValidateGroupsForTenantParams{
		TenantID: tenantID, Column2: uuids,
	})
}

// GroupNames 批量取分组名（订阅快照只存 group_id，展示层据此补名）。
func (r *SubscriptionRepo) GroupNames(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	uuids := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		uuids[i] = mustParseUUID(id)
	}
	rows, err := r.q.ListGroupNames(ctx, uuids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}

// CountUserAccessiblePlanGroups 返回套餐分组 ∩ 用户可见分组的数量。
func (r *SubscriptionRepo) CountUserAccessiblePlanGroups(ctx context.Context, planID, tenantID, userID string) (int64, error) {
	return r.q.CountUserAccessiblePlanGroups(ctx, dbgen.CountUserAccessiblePlanGroupsParams{
		PlanID: mustParseUUID(planID), TenantID: tenantID, UserID: userID,
	})
}

// ---- 订单 ----

func (r *SubscriptionRepo) CreateOrder(ctx context.Context, orderNo string, p subscription.PurchaseParams, plan *subscription.Plan) (*subscription.Order, error) {
	return createOrderWithQueries(ctx, r.q, orderNo, p, plan, false)
}

func createOrderWithQueries(ctx context.Context, q *dbgen.Queries, orderNo string, p subscription.PurchaseParams, plan *subscription.Plan, inventoryReserved bool) (*subscription.Order, error) {
	policy := subscription.NormalizePurchasePolicy(plan.PurchasePolicy)
	policySnapshot, err := marshalPurchasePolicy(policy)
	if err != nil {
		return nil, err
	}
	row, err := q.CreateOrder(ctx, dbgen.CreateOrderParams{
		OrderNo:                            orderNo,
		TenantID:                           p.TenantID,
		UserID:                             p.UserID,
		PlanID:                             mustParseUUID(plan.ID),
		PlanNameSnapshot:                   plan.Name,
		PriceCredits:                       plan.PriceCredits,
		DurationDaysSnapshot:               plan.DurationDays,
		TotalLimitMicroSnapshot:            plan.TotalLimitMicro,
		Window5hLimitMicroSnapshot:         ptrInt8(plan.Window5hLimitMicro),
		Window7dLimitMicroSnapshot:         ptrInt8(plan.Window7dLimitMicro),
		GroupQuotaDebitMultipliersSnapshot: marshalGroupQuotaDebitMultipliers(plan.Groups),
		PurchasePolicyVersion:              policy.Version,
		PurchasePolicySnapshot:             policySnapshot,
		InventoryReserved:                  inventoryReserved,
	})
	if err != nil {
		return nil, err
	}
	o := toOrder(row)
	return &o, nil
}

type purchaseUserState struct {
	now         time.Time
	live        []subscription.Subscription
	openPlanIDs []string
	paidByPlan  map[string][]time.Time
}

func purchaseUserLockKey(tenantID, userID string) string {
	return "subscription-user:" + tenantID + ":" + userID
}

func advanceLiveSubscriptions(ctx context.Context, q *dbgen.Queries, tenantID, userID string) ([]subscription.Subscription, error) {
	rows, err := q.LockLiveSubsForUser(ctx, dbgen.LockLiveSubsForUserParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	active := -1
	pending := -1
	for i := range rows {
		switch rows[i].Status {
		case subscription.SubActive:
			active = i
		case subscription.SubPending:
			if pending < 0 {
				pending = i
			}
		}
	}
	changed := false
	if active >= 0 && rows[active].ExpiresAt.Valid {
		expired, err := q.ExpireSubscriptionIfDue(ctx, rows[active].ID)
		if err != nil {
			return nil, err
		}
		if expired > 0 {
			active = -1
			changed = true
		}
	}
	if active < 0 && pending >= 0 {
		activated, err := q.ActivateSubscription(ctx, rows[pending].ID)
		if err != nil {
			return nil, err
		}
		changed = changed || activated > 0
	}
	if changed {
		rows, err = q.LockLiveSubsForUser(ctx, dbgen.LockLiveSubsForUserParams{TenantID: tenantID, UserID: userID})
		if err != nil {
			return nil, err
		}
	}
	return toSubs(rows), nil
}

func loadPurchaseUserState(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, tenantID, userID string, planIDs []string) (purchaseUserState, error) {
	state := purchaseUserState{paidByPlan: make(map[string][]time.Time, len(planIDs))}
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&state.now); err != nil {
		return state, err
	}
	live, err := advanceLiveSubscriptions(ctx, q, tenantID, userID)
	if err != nil {
		return state, err
	}
	state.live = live

	rows, err := tx.Query(ctx, `
		SELECT plan_id::text
		FROM ai_sub_orders
		WHERE tenant_id=$1 AND user_id=$2 AND status IN ('created','deducting')
	`, tenantID, userID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var planID string
		if err := rows.Scan(&planID); err != nil {
			rows.Close()
			return state, err
		}
		state.openPlanIDs = append(state.openPlanIDs, planID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()

	if len(planIDs) == 0 {
		return state, nil
	}
	ids := make([]pgtype.UUID, len(planIDs))
	for i, planID := range planIDs {
		ids[i] = mustParseUUID(planID)
	}
	rows, err = tx.Query(ctx, `
		SELECT plan_id::text, created_at
		FROM ai_sub_orders
		WHERE tenant_id=$1 AND user_id=$2 AND status='paid'
		  AND plan_id = ANY($3::uuid[])
		ORDER BY created_at ASC
	`, tenantID, userID, ids)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var planID string
		var purchasedAt time.Time
		if err := rows.Scan(&planID, &purchasedAt); err != nil {
			return state, err
		}
		state.paidByPlan[planID] = append(state.paidByPlan[planID], purchasedAt)
	}
	return state, rows.Err()
}

func purchaseFactsForPlan(state purchaseUserState, planID string, maxQueue int) subscription.PurchaseFacts {
	facts := subscription.PurchaseFacts{
		Now:                   state.now,
		PaidOrderTimes:        state.paidByPlan[planID],
		OpenOrderCount:        len(state.openPlanIDs),
		LiveSubscriptionCount: len(state.live),
		MaxQueue:              maxQueue,
	}
	for _, openPlanID := range state.openPlanIDs {
		if openPlanID == planID {
			facts.OpenSamePlanOrder = true
			break
		}
	}
	for i := range state.live {
		if state.live[i].PlanID != planID {
			continue
		}
		switch state.live[i].Status {
		case subscription.SubPending:
			facts.PendingSamePlan = true
		case subscription.SubActive:
			facts.ActiveSamePlanExpiresAt = state.live[i].ExpiresAt
		}
	}
	return facts
}

// EvaluatePlansForUser returns storefront decisions using the same live-state
// advancement and policy evaluator as order reservation.
func (r *SubscriptionRepo) EvaluatePlansForUser(ctx context.Context, tenantID, userID string, plans []subscription.Plan, maxQueue int) (map[string]subscription.PurchaseDecision, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	if err := q.LockUserPurchaseSerial(ctx, purchaseUserLockKey(tenantID, userID)); err != nil {
		return nil, err
	}
	planIDs := make([]string, len(plans))
	for i := range plans {
		planIDs[i] = plans[i].ID
	}
	state, err := loadPurchaseUserState(ctx, tx, q, tenantID, userID, planIDs)
	if err != nil {
		return nil, err
	}
	decisions := make(map[string]subscription.PurchaseDecision, len(plans))
	for i := range plans {
		decision, err := subscription.EvaluatePurchaseEligibility(
			plans[i].PurchasePolicy,
			purchaseFactsForPlan(state, plans[i].ID, maxQueue),
		)
		if err != nil {
			return nil, err
		}
		decisions[plans[i].ID] = decision
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return decisions, nil
}

// ReservePurchase owns the complete local acceptance decision. Its transaction
// is intentionally committed before URM is called; the created order is the
// durable reservation observed by concurrent attempts and the janitor.
func (r *SubscriptionRepo) ReservePurchase(ctx context.Context, orderNo string, p subscription.PurchaseParams, maxQueue int) (*subscription.PurchaseReservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	if err := q.LockUserPurchaseSerial(ctx, purchaseUserLockKey(p.TenantID, p.UserID)); err != nil {
		return nil, err
	}

	existing, err := q.GetOrderByOrderNo(ctx, orderNo)
	if err == nil {
		if existing.TenantID != p.TenantID || existing.UserID != p.UserID || uuidToString(existing.PlanID) != p.PlanID {
			return nil, subscription.ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		order := toOrder(existing)
		return &subscription.PurchaseReservation{Order: &order, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtext($1))`, "subscription-plan:"+p.PlanID); err != nil {
		return nil, err
	}
	planRow, err := q.GetPlan(ctx, mustParseUUID(p.PlanID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, subscription.ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	plan := toPlan(planRow)
	if plan.TenantID != p.TenantID {
		return nil, subscription.ErrPlanForbidden
	}
	if plan.Status != subscription.PlanOnSale {
		return nil, subscription.ErrPlanNotOnSale
	}
	groupsByPlan, err := loadPlanGroupsWithQueries(ctx, q, []string{plan.ID})
	if err != nil {
		return nil, err
	}
	plan.Groups = groupsByPlan[plan.ID]
	policyRow, err := q.GetPlanPurchasePolicy(ctx, planRow.ID)
	if err != nil {
		return nil, err
	}
	plan.PurchasePolicy = toPurchasePolicy(policyRow)

	// Profit is measured from actual period revenue and winning-upstream tenant
	// charges. A pre-purchase group multiplier estimate is intentionally not a
	// sale gate because the two multiplier chains are independent.
	accessible, err := q.CountUserAccessiblePlanGroups(ctx, dbgen.CountUserAccessiblePlanGroupsParams{
		PlanID: planRow.ID, TenantID: p.TenantID, UserID: p.UserID,
	})
	if err != nil {
		return nil, err
	}
	if accessible == 0 {
		return nil, subscription.ErrPlanNotAccessible
	}

	state, err := loadPurchaseUserState(ctx, tx, q, p.TenantID, p.UserID, []string{p.PlanID})
	if err != nil {
		return nil, err
	}
	decision, err := subscription.EvaluatePurchaseEligibility(plan.PurchasePolicy, purchaseFactsForPlan(state, p.PlanID, maxQueue))
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		// Persist lazy expiry/activation even when another purchase rule blocks.
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, &subscription.PurchaseDeniedError{Decision: decision}
	}
	reserved, err := q.ReservePlanInventory(ctx, planRow.ID)
	if err != nil {
		return nil, err
	}
	if reserved == 0 {
		return nil, subscription.ErrPlanSoldOut
	}
	order, err := createOrderWithQueries(ctx, q, orderNo, p, &plan, true)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &subscription.PurchaseReservation{Order: order}, nil
}

func (r *SubscriptionRepo) GetOrder(ctx context.Context, id string) (*subscription.Order, error) {
	row, err := r.q.GetOrderByID(ctx, mustParseUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, subscription.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	o := toOrder(row)
	return &o, nil
}

func (r *SubscriptionRepo) GetOrderByOrderNo(ctx context.Context, orderNo string) (*subscription.Order, error) {
	var id string
	if err := r.pool.QueryRow(ctx, `SELECT id::text FROM ai_sub_orders WHERE order_no = $1`, orderNo).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return nil, subscription.ErrOrderNotFound
	} else if err != nil {
		return nil, err
	}
	return r.GetOrder(ctx, id)
}

func (r *SubscriptionRepo) ListOpenPurchaseOrderPlanIDs(ctx context.Context, tenantID, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT plan_id
		FROM ai_sub_orders
		WHERE tenant_id = $1 AND user_id = $2 AND status IN ('created', 'deducting')
	`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var planIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		planIDs = append(planIDs, id)
	}
	return planIDs, rows.Err()
}

func (r *SubscriptionRepo) AcquirePurchaseLock(ctx context.Context, tenantID, userID string) (func(), error) {
	return r.acquireAdvisoryLock(ctx, "subscription-purchase:"+tenantID+":"+userID)
}

func (r *SubscriptionRepo) acquireAdvisoryLock(ctx context.Context, key string) (func(), error) {
	for {
		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		var acquired bool
		err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&acquired)
		if err != nil {
			conn.Release()
			return nil, err
		}
		if acquired {
			return func() {
				unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtext($1))`, key)
				conn.Release()
			}, nil
		}
		conn.Release()
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *SubscriptionRepo) MarkOrderDeducting(ctx context.Context, id string) (bool, error) {
	n, err := r.q.MarkOrderDeducting(ctx, mustParseUUID(id))
	return n > 0, err
}

func (r *SubscriptionRepo) MarkOrderFailed(ctx context.Context, id, reason string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)
	order, err := q.GetOrderByID(ctx, mustParseUUID(id))
	if err != nil {
		return false, err
	}
	n, err := q.MarkOrderFailed(ctx, dbgen.MarkOrderFailedParams{ID: order.ID, FailReason: nullableText(reason)})
	if err != nil {
		return false, err
	}
	if n > 0 && order.InventoryReserved {
		released, err := q.ReleasePlanInventory(ctx, order.PlanID)
		if err != nil {
			return false, err
		}
		if released == 0 {
			return false, errors.New("failed order inventory reservation is missing")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *SubscriptionRepo) ListReconcileOrders(ctx context.Context, cutoff time.Time, limit int32) ([]subscription.Order, error) {
	rows, err := r.q.ListReconcileOrders(ctx, dbgen.ListReconcileOrdersParams{
		UpdatedAt: pgtype.Timestamptz{Time: cutoff, Valid: true}, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]subscription.Order, len(rows))
	for i, row := range rows {
		out[i] = toOrder(row)
	}
	return out, nil
}

func (r *SubscriptionRepo) ListOrders(ctx context.Context, f subscription.OrderFilter) ([]subscription.Order, int64, error) {
	total, err := r.q.CountOrdersPage(ctx, dbgen.CountOrdersPageParams{
		TenantID: nargText(f.TenantID), UserID: nargText(f.UserID), Status: nargText(f.Status),
	})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListOrdersPage(ctx, dbgen.ListOrdersPageParams{
		TenantID: nargText(f.TenantID), UserID: nargText(f.UserID), Status: nargText(f.Status),
		Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]subscription.Order, len(rows))
	for i, row := range rows {
		out[i] = toOrder(row)
	}
	return out, total, nil
}

// ---- 订阅 ----

func (r *SubscriptionRepo) GetLiveSubs(ctx context.Context, tenantID, userID string) ([]subscription.Subscription, error) {
	rows, err := r.q.GetLiveSubsForUser(ctx, dbgen.GetLiveSubsForUserParams{TenantID: tenantID, UserID: userID})
	if err != nil {
		return nil, err
	}
	return toSubs(rows), nil
}

func (r *SubscriptionRepo) ExpireIfDue(ctx context.Context, id string) (bool, error) {
	n, err := r.q.ExpireSubscriptionIfDue(ctx, mustParseUUID(id))
	return n > 0, err
}

func (r *SubscriptionRepo) Activate(ctx context.Context, id string) (bool, error) {
	n, err := r.q.ActivateSubscription(ctx, mustParseUUID(id))
	return n > 0, err
}

func (r *SubscriptionRepo) Debit(ctx context.Context, id string, micro int64) (int64, error) {
	total, err := r.q.DebitSubscription(ctx, dbgen.DebitSubscriptionParams{ID: mustParseUUID(id), Win5hUsedMicro: micro})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, subscription.ErrSubNotFound
	}
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *SubscriptionRepo) ExpireDue(ctx context.Context) (int64, error) {
	return r.q.ExpireDueSubscriptions(ctx)
}
func (r *SubscriptionRepo) GetSubscription(ctx context.Context, id string) (*subscription.Subscription, error) {
	row, err := r.q.GetSubscriptionByID(ctx, mustParseUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, subscription.ErrSubNotFound
	}
	if err != nil {
		return nil, err
	}
	s := toSub(row)
	return &s, nil
}

func (r *SubscriptionRepo) ListSubscriptions(ctx context.Context, f subscription.SubFilter) ([]subscription.Subscription, int64, error) {
	total, err := r.q.CountSubsPage(ctx, dbgen.CountSubsPageParams{
		TenantID: nargText(f.TenantID), UserID: nargText(f.UserID), Status: nargText(f.Status),
	})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListSubsPage(ctx, dbgen.ListSubsPageParams{
		TenantID: nargText(f.TenantID), UserID: nargText(f.UserID), Status: nargText(f.Status),
		Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	return toSubs(rows), total, nil
}

// FinalizeOrder 购买 finalize 事务：与预占共用用户级串行锁 → 幂等短路 → 判定 active/pending
// → 建订阅 → 订单置 paid。janitor 卡单重放走同一路径（订单已 paid 时返回既有订阅）。
func (r *SubscriptionRepo) FinalizeOrder(ctx context.Context, order *subscription.Order, eventID string) (*subscription.Subscription, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := r.q.WithTx(tx)

	cur, err := q.GetOrderByID(ctx, mustParseUUID(order.ID))
	if err != nil {
		return nil, err
	}
	// 按数据库中的订单归属串行化（含首购无 live 行的并发）。
	if err := q.LockUserPurchaseSerial(ctx, purchaseUserLockKey(cur.TenantID, cur.UserID)); err != nil {
		return nil, err
	}
	// 获取 advisory lock 期间订单可能已被另一补偿实例推进，锁后必须重读。
	cur, err = q.GetOrderByID(ctx, cur.ID)
	if err != nil {
		return nil, err
	}
	// 幂等：订单已 paid ⇒ 返回既有订阅（janitor 重放到此终止）。
	if cur.Status == subscription.OrderPaid {
		if !cur.SubscriptionID.Valid {
			return nil, errors.New("paid order missing subscription id")
		}
		sub, err := q.GetSubscriptionByID(ctx, cur.SubscriptionID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		s := toSub(sub)
		return &s, nil
	}

	// 判定是否已有 active ⇒ 决定新订阅 active(首个) 还是 pending(排队)。
	live, err := q.LockLiveSubsForUser(ctx, dbgen.LockLiveSubsForUserParams{TenantID: cur.TenantID, UserID: cur.UserID})
	if err != nil {
		return nil, err
	}
	status := subscription.SubActive
	for _, s := range live {
		if s.Status == subscription.SubActive {
			status = subscription.SubPending
			break
		}
	}

	sub, err := q.CreateSubscription(ctx, dbgen.CreateSubscriptionParams{
		TenantID:                   cur.TenantID,
		UserID:                     cur.UserID,
		PlanID:                     cur.PlanID,
		OrderID:                    cur.ID,
		PlanNameSnapshot:           cur.PlanNameSnapshot,
		DurationDays:               cur.DurationDaysSnapshot,
		TotalLimitMicro:            cur.TotalLimitMicroSnapshot,
		Window5hLimitMicro:         cur.Window5hLimitMicroSnapshot,
		Window7dLimitMicro:         cur.Window7dLimitMicroSnapshot,
		Status:                     status,
		GroupQuotaDebitMultipliers: cur.GroupQuotaDebitMultipliersSnapshot,
	})
	if err != nil {
		return nil, err
	}

	n, err := q.MarkOrderPaid(ctx, dbgen.MarkOrderPaidParams{
		ID: cur.ID, UrmEventID: nullableText(eventID), SubscriptionID: sub.ID,
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("finalize: mark order paid affected 0 rows")
	}
	if cur.InventoryReserved {
		committed, err := q.CommitPlanInventorySale(ctx, cur.PlanID)
		if err != nil {
			return nil, err
		}
		if committed == 0 {
			return nil, errors.New("finalize: inventory reservation is missing")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s := toSub(sub)
	return &s, nil
}
