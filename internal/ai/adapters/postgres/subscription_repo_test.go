package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	dbgen "xiaodou/dai/internal/ai/db/gen"
	"xiaodou/dai/internal/ai/domain"
	"xiaodou/dai/internal/ai/subscription"
)

// 需要真实 Postgres（含 ai_sub_* schema）：设置 AI_TEST_DATABASE_URL 后运行，否则跳过。
func subTestRepo(t *testing.T) (*SubscriptionRepo, *pgxpool.Pool, context.Context, string, string) {
	t.Helper()
	dsn := os.Getenv("AI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AI_TEST_DATABASE_URL to run subscription DB tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("ping: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "t_" + suffix
	userID := "u_" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ai_sub_subscriptions WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_sub_orders WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_sub_plan_purchase_policy_revisions WHERE plan_id IN (SELECT id FROM ai_sub_plans WHERE tenant_id=$1)`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_sub_plan_purchase_policies WHERE plan_id IN (SELECT id FROM ai_sub_plans WHERE tenant_id=$1)`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_sub_plan_groups WHERE plan_id IN (SELECT id FROM ai_sub_plans WHERE tenant_id=$1)`, tenantID) // 补偿：原 FK CASCADE 已移除
		_, _ = pool.Exec(ctx, `DELETE FROM ai_sub_plans WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_user_groups WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_groups WHERE name LIKE 'grp_'||$1||'%'`, tenantID)
		pool.Close()
	})
	return NewSubscriptionRepo(dbgen.New(pool), pool), pool, ctx, tenantID, userID
}

func i64(v int64) *int64 { return &v }

// seedGroup 插一个属于该租户的 active 分组，返回 id（名字带 tenantID 便于清理）。
// user_default_visible=false：终端用户只有经 ai_user_groups 显式绑定后才可见，
// 购买交集校验（CountUserAccessiblePlanGroups）才有可断言的「未授权 → 授权」跃迁。
func seedGroup(t *testing.T, r *SubscriptionRepo, ctx context.Context, tenantID string) string {
	t.Helper()
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ai_groups (tenant_id, name, user_default_visible, retail_price_book_id, status)
		VALUES ($1, 'grp_'||$1||'_'||$2, false, gen_random_uuid(), 'active')
		RETURNING id::text`, tenantID, fmt.Sprint(time.Now().UnixNano())).Scan(&id)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	return id
}

// seeUserGroup 让某终端用户可见指定分组（购买交集校验需要）。
func seeUserGroup(t *testing.T, r *SubscriptionRepo, ctx context.Context, tenantID, userID, groupID string) {
	t.Helper()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ai_user_groups (tenant_id, user_id, group_id)
		VALUES ($1,$2,$3::uuid)
		ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING`, tenantID, userID, groupID)
	if err != nil {
		t.Fatalf("seed user group: %v", err)
	}
}

func mkPlan(t *testing.T, r *SubscriptionRepo, ctx context.Context, tenantID string, dur int32, total int64, w5h, w7d *int64) *subscription.Plan {
	t.Helper()
	gid := seedGroup(t, r, ctx, tenantID)
	p, err := r.CreatePlan(ctx, subscription.CreatePlanParams{
		TenantID: tenantID, Name: "plan-" + fmt.Sprint(time.Now().UnixNano()), PriceCredits: 200,
		DurationDays: dur, TotalLimitMicro: total, Window5hLimitMicro: w5h, Window7dLimitMicro: w7d,
		Groups: []subscription.PlanGroup{{GroupID: gid, QuotaDebitMultiplier: 1}},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return p
}

// TestDebitConcurrency：N goroutine 并发 Debit，断言总计数无丢失（行锁原子）。
func TestDebitConcurrency(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	plan := mkPlan(t, r, ctx, tenantID, 30, 1_000_000_000, nil, nil) // 无窗口，只累加 total
	order, err := r.CreateOrder(ctx, "SUB"+fmt.Sprint(time.Now().UnixNano()), subscription.PurchaseParams{TenantID: tenantID, UserID: userID, PlanID: plan.ID}, plan)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	sub, err := r.FinalizeOrder(ctx, order, "EV_seed")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if sub.Status != subscription.SubActive {
		t.Fatalf("seed sub should be active, got %s", sub.Status)
	}

	const n = 50
	const amt = int64(1000)
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.Debit(ctx, sub.ID, amt); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("concurrent debit error: %v", e)
	}
	got, err := r.GetSubscription(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalUsedMicro != n*amt {
		t.Fatalf("lost update: want %d got %d", n*amt, got.TotalUsedMicro)
	}
}

// TestWindowRollover：5h 窗过期后 Debit 应整窗重置（不累加），total 继续累加。
func TestWindowRollover(t *testing.T) {
	r, pool, ctx, tenantID, userID := subTestRepo(t)
	plan := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, i64(500_000), i64(900_000))
	order, _ := r.CreateOrder(ctx, "SUB"+fmt.Sprint(time.Now().UnixNano()), subscription.PurchaseParams{TenantID: tenantID, UserID: userID, PlanID: plan.ID}, plan)
	sub, err := r.FinalizeOrder(ctx, order, "EV_seed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Debit(ctx, sub.ID, 100); err != nil { // 开窗
		t.Fatal(err)
	}
	if _, err := r.Debit(ctx, sub.ID, 100); err != nil { // 累加 → win5h=200
		t.Fatal(err)
	}
	// 把 5h 窗起点拨到 6h 前，模拟窗口翻转
	if _, err := pool.Exec(ctx, `UPDATE ai_sub_subscriptions SET win5h_start=now()-interval '6 hours' WHERE id=$1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Debit(ctx, sub.ID, 100); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetSubscription(ctx, sub.ID)
	if got.Win5hUsedMicro != 100 {
		t.Fatalf("5h window should reset to 100, got %d", got.Win5hUsedMicro)
	}
	if got.Win7dUsedMicro != 300 { // 7d 未翻转，持续累加
		t.Fatalf("7d window should accumulate to 300, got %d", got.Win7dUsedMicro)
	}
	if got.TotalUsedMicro != 300 {
		t.Fatalf("total should be 300, got %d", got.TotalUsedMicro)
	}
}

// TestFinalizeOrderIdempotent：重复 FinalizeOrder 只开通一份订阅。
func TestFinalizeOrderIdempotent(t *testing.T) {
	r, pool, ctx, tenantID, userID := subTestRepo(t)
	plan := mkPlan(t, r, ctx, tenantID, 7, 1_000_000, nil, nil)
	order, _ := r.CreateOrder(ctx, "SUB"+fmt.Sprint(time.Now().UnixNano()), subscription.PurchaseParams{TenantID: tenantID, UserID: userID, PlanID: plan.ID}, plan)
	s1, err := r.FinalizeOrder(ctx, order, "EV_1")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := r.FinalizeOrder(ctx, order, "EV_1")
	if err != nil {
		t.Fatalf("second finalize should be idempotent: %v", err)
	}
	if s1.ID != s2.ID {
		t.Fatalf("finalize created duplicate subscription: %s vs %s", s1.ID, s2.ID)
	}
	var cnt int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_sub_subscriptions WHERE order_id=$1`, order.ID).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("expected exactly 1 subscription for order, got %d", cnt)
	}
}

func TestFinalizeOrderUsesImmutableOrderEntitlementSnapshot(t *testing.T) {
	r, pool, ctx, tenantID, userID := subTestRepo(t)
	w5h := int64(100_000)
	w7d := int64(500_000)
	plan := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, &w5h, &w7d)
	order, err := r.CreateOrder(ctx, "SUB"+fmt.Sprint(time.Now().UnixNano()), subscription.PurchaseParams{
		TenantID: tenantID, UserID: userID, PlanID: plan.ID,
	}, plan)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE ai_sub_plans
		SET duration_days=1, total_limit_micro=2000000,
		    window_5h_limit_micro=200000, window_7d_limit_micro=NULL
		WHERE id=$1`, plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE ai_sub_plan_groups SET quota_debit_multiplier=9 WHERE plan_id=$1`, plan.ID); err != nil {
		t.Fatal(err)
	}

	sub, err := r.FinalizeOrder(ctx, order, "EV_snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if sub.DurationDays != 30 || sub.TotalLimitMicro != 1_000_000 {
		t.Fatalf("subscription used mutable plan terms: duration=%d total=%d", sub.DurationDays, sub.TotalLimitMicro)
	}
	if sub.Window5hLimitMicro == nil || *sub.Window5hLimitMicro != w5h ||
		sub.Window7dLimitMicro == nil || *sub.Window7dLimitMicro != w7d {
		t.Fatalf("subscription used mutable window terms: 5h=%v 7d=%v", sub.Window5hLimitMicro, sub.Window7dLimitMicro)
	}
	if got := sub.GroupQuotaDebitMultipliers[plan.Groups[0].GroupID]; got != 1 {
		t.Fatalf("subscription used mutable group multiplier: got=%v want=1", got)
	}
}

// fakePurchaser：每个幂等键返回确定的 event，可注入失败。
type fakePurchaser struct {
	mu    sync.Mutex
	calls int
	fail  error
	last  subscription.DebitRequest
}

type blockingPurchaser struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingPurchaser) DebitStrict(context.Context, subscription.DebitRequest) (*subscription.DebitReceipt, error) {
	close(b.entered)
	<-b.release
	return &subscription.DebitReceipt{AuthorizationID: "EV_blocking"}, nil
}

func (f *fakePurchaser) DebitStrict(ctx context.Context, req subscription.DebitRequest) (*subscription.DebitReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = req
	if f.fail != nil {
		return nil, f.fail
	}
	return &subscription.DebitReceipt{AuthorizationID: "EV_" + req.IdempotencyKey}, nil
}

// TestPlanGroupsValidationAndSnapshot：分组绑定校验（缺组/权重/不可见）+ 购买交集 + 快照。
func TestPlanGroupsValidationAndSnapshot(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	svc := subscription.NewService(r, &fakePurchaser{}, zap.NewNop())

	base := subscription.CreatePlanParams{
		TenantID: tenantID, Name: "p-" + fmt.Sprint(time.Now().UnixNano()),
		PriceCredits: 100, DurationDays: 30, TotalLimitMicro: 1_000_000,
	}

	// 1) 无分组 ⇒ ErrPlanNeedsGroups
	if _, err := svc.CreatePlan(ctx, base); err != subscription.ErrPlanNeedsGroups {
		t.Fatalf("no groups: want ErrPlanNeedsGroups, got %v", err)
	}
	// 2) 权重 <=0 ⇒ ErrPlanGroupsInvalid
	gid := seedGroup(t, r, ctx, tenantID)
	bad := base
	bad.Groups = []subscription.PlanGroup{{GroupID: gid, QuotaDebitMultiplier: 0}}
	if _, err := svc.CreatePlan(ctx, bad); err != subscription.ErrPlanGroupsInvalid {
		t.Fatalf("bad weight: want ErrPlanGroupsInvalid, got %v", err)
	}
	// 3) 不存在的分组 ⇒ ErrPlanGroupsInvalid（对租户不可见）
	inv := base
	inv.Groups = []subscription.PlanGroup{{GroupID: "00000000-0000-0000-0000-000000000000", QuotaDebitMultiplier: 1}}
	if _, err := svc.CreatePlan(ctx, inv); err != subscription.ErrPlanGroupsInvalid {
		t.Fatalf("invisible group: want ErrPlanGroupsInvalid, got %v", err)
	}
	// 4) 合法：绑定权重 3，读回带分组
	ok := base
	ok.Groups = []subscription.PlanGroup{{GroupID: gid, QuotaDebitMultiplier: 3}}
	plan, err := svc.CreatePlan(ctx, ok)
	if err != nil {
		t.Fatalf("create ok: %v", err)
	}
	got, err := r.GetPlan(ctx, plan.ID)
	if err != nil || len(got.Groups) != 1 || got.Groups[0].QuotaDebitMultiplier != 3 {
		t.Fatalf("read back groups: %+v err=%v", got.Groups, err)
	}
	if ok2, err := svc.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok2 {
		t.Fatalf("on_sale: ok=%v err=%v", ok2, err)
	}

	// 5) 用户看不到分组 ⇒ 购买 ErrPlanNotAccessible
	pp := subscription.PurchaseParams{TenantID: tenantID, UserID: userID, PlanID: plan.ID, IdempotencyKey: "visibility-purchase"}
	if _, _, err := svc.Purchase(ctx, pp); err != subscription.ErrPlanNotAccessible {
		t.Fatalf("no visibility: want ErrPlanNotAccessible, got %v", err)
	}
	// 6) 授予可见后购买成功，订阅快照 {gid: 3}
	seeUserGroup(t, r, ctx, tenantID, userID, gid)
	_, sub, err := svc.Purchase(ctx, pp)
	if err != nil || sub == nil {
		t.Fatalf("purchase after visibility: sub=%v err=%v", sub, err)
	}
	if w := sub.GroupQuotaDebitMultipliers[gid]; w != 3 {
		t.Fatalf("snapshot weight for %s = %v, want 3 (weights=%v)", gid, w, sub.GroupQuotaDebitMultipliers)
	}
}

func TestPurchaseReservationSnapshotDoesNotBlockPlanMutation(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	purchaser := &blockingPurchaser{entered: make(chan struct{}), release: make(chan struct{})}
	svc := subscription.NewService(r, purchaser, zap.NewNop())
	plan := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)
	seeUserGroup(t, r, ctx, tenantID, userID, plan.Groups[0].GroupID)
	if ok, err := svc.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
		t.Fatalf("list plan: ok=%v err=%v", ok, err)
	}

	purchaseDone := make(chan error, 1)
	go func() {
		_, _, err := svc.Purchase(ctx, subscription.PurchaseParams{
			TenantID: tenantID, UserID: userID, PlanID: plan.ID, IdempotencyKey: "plan-lock-purchase",
		})
		purchaseDone <- err
	}()
	<-purchaser.entered

	statusDone := make(chan error, 1)
	go func() {
		_, err := svc.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOffSale)
		statusDone <- err
	}()
	select {
	case err := <-statusDone:
		if err != nil {
			close(purchaser.release)
			t.Fatalf("down-shelf during debit: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(purchaser.release)
		<-purchaseDone
		t.Fatal("plan mutation was blocked by the external debit")
	}
	close(purchaser.release)
	if err := <-purchaseDone; err != nil {
		t.Fatalf("purchase should finalize from its reservation snapshot: %v", err)
	}
}

// TestPurchaseQueueAndInsufficient：首购 active、续购排 pending、超限 QueueFull、余额不足 failed。
func TestPurchaseQueueAndInsufficient(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	fp := &fakePurchaser{}
	svc := subscription.NewService(r, fp, zap.NewNop())

	planA := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)
	planB := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)
	planC := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)
	for _, p := range []*subscription.Plan{planA, planB, planC} {
		seeUserGroup(t, r, ctx, tenantID, userID, p.Groups[0].GroupID)
		if ok, err := svc.SetPlanStatus(ctx, p.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
			t.Fatalf("on_sale: ok=%v err=%v", ok, err)
		}
	}
	// 让贫困测试用户也能看到套餐分组（购买交集校验前置）。
	seeUserGroup(t, r, ctx, tenantID, userID+"_poor", planA.Groups[0].GroupID)
	pp := subscription.PurchaseParams{TenantID: tenantID, UserID: userID, PlanID: planA.ID}

	// 1) 首购 A ⇒ active
	pp.IdempotencyKey = "purchase-1"
	_, s1, err := svc.Purchase(ctx, pp)
	if err != nil || s1 == nil || s1.Status != subscription.SubActive {
		t.Fatalf("purchase#1 should be active: s=%v err=%v", s1, err)
	}
	wantPriceMicro, ok := domain.CreditsToMicro(planA.PriceCredits)
	if !ok {
		t.Fatalf("test plan price is not representable: %d", planA.PriceCredits)
	}
	if fp.last.UserMicro != wantPriceMicro {
		t.Fatalf("subscription debit = %d micro, want %d", fp.last.UserMicro, wantPriceMicro)
	}
	// 2) 续购 A（active A + 买 A）⇒ pending
	pp.IdempotencyKey = "purchase-2"
	order2, s2, err := svc.Purchase(ctx, pp)
	if err != nil || s2 == nil || s2.Status != subscription.SubPending {
		t.Fatalf("purchase#2 should be pending: s=%v err=%v", s2, err)
	}
	// 3) pending 内同套餐去重：pending A 已存在 ⇒ 再买 A 拒绝
	pp.IdempotencyKey = "purchase-3"
	if _, _, err := svc.Purchase(ctx, pp); !errors.Is(err, subscription.ErrPlanAlreadyQueued) {
		t.Fatalf("purchase#3 should be ErrPlanAlreadyQueued, got %v", err)
	}
	// 3b) 同 Idempotency-Key 重放已成交订单 ⇒ 命中原订单，不触发去重
	pp.IdempotencyKey = "purchase-2"
	if o, _, err := svc.Purchase(ctx, pp); err != nil || o == nil || o.ID != order2.ID {
		t.Fatalf("purchase#2 replay should hit original order: o=%v err=%v", o, err)
	}
	// 4) 换套餐买 B ⇒ pending（1 active + 2 pending = 1+maxQueue(2)）
	if _, s, err := svc.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: userID, PlanID: planB.ID, IdempotencyKey: "purchase-4",
	}); err != nil || s == nil || s.Status != subscription.SubPending {
		t.Fatalf("purchase#4 should be pending: s=%v err=%v", s, err)
	}
	// 5) 超排队上限（买未排队的 C）⇒ QueueFull
	if _, _, err := svc.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: userID, PlanID: planC.ID, IdempotencyKey: "purchase-5",
	}); !errors.Is(err, subscription.ErrQueueFull) {
		t.Fatalf("purchase#5 should be ErrQueueFull, got %v", err)
	}

	// 余额不足：换个用户，purchaser 注入 ErrInsufficientBalance ⇒ ErrInsufficientBalance + 订单 failed
	fp2 := &fakePurchaser{fail: subscription.ErrInsufficientBalance}
	svc2 := subscription.NewService(r, fp2, zap.NewNop())
	pp2 := subscription.PurchaseParams{TenantID: tenantID, UserID: userID + "_poor", PlanID: planA.ID, IdempotencyKey: "poor-purchase"}
	if _, _, err := svc2.Purchase(ctx, pp2); err != subscription.ErrInsufficientBalance {
		t.Fatalf("poor purchase should be ErrInsufficientBalance, got %v", err)
	}
}

// TestPurchaseDedupOnOpenOrder：在途订单（deducting）里已有同套餐 ⇒ 再买同套餐拒绝。
func TestPurchaseDedupOnOpenOrder(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	svc := subscription.NewService(r, &fakePurchaser{}, zap.NewNop())
	plan := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)
	seeUserGroup(t, r, ctx, tenantID, userID, plan.Groups[0].GroupID)
	if ok, err := svc.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
		t.Fatalf("on_sale: ok=%v err=%v", ok, err)
	}
	pp := subscription.PurchaseParams{TenantID: tenantID, UserID: userID, PlanID: plan.ID}
	order, err := r.CreateOrder(ctx, "SUBD"+fmt.Sprint(time.Now().UnixNano()), pp, plan)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if ok, err := r.MarkOrderDeducting(ctx, order.ID); err != nil || !ok {
		t.Fatalf("mark deducting: ok=%v err=%v", ok, err)
	}
	pp.IdempotencyKey = "dedup-open-order"
	if _, _, err := svc.Purchase(ctx, pp); !errors.Is(err, subscription.ErrPlanAlreadyQueued) {
		t.Fatalf("purchase with open order should be ErrPlanAlreadyQueued, got %v", err)
	}
}

func TestPurchaseConcurrencyHonorsIdempotencyAndQueueLimit(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	plan := mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)
	gid := plan.Groups[0].GroupID
	fp := &fakePurchaser{}
	svc := subscription.NewService(r, fp, zap.NewNop())
	if ok, err := svc.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
		t.Fatalf("on_sale: ok=%v err=%v", ok, err)
	}
	seeUserGroup(t, r, ctx, tenantID, userID, gid)

	const retries = 8
	var wg sync.WaitGroup
	errs := make(chan error, retries)
	for i := 0; i < retries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := svc.Purchase(ctx, subscription.PurchaseParams{
				TenantID: tenantID, UserID: userID, PlanID: plan.ID, IdempotencyKey: "same-attempt",
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, subscription.ErrOrderProcessing) {
			t.Fatalf("idempotent concurrent purchase: %v", err)
		}
	}
	fp.mu.Lock()
	calls := fp.calls
	fp.mu.Unlock()
	if calls != 1 {
		t.Fatalf("billing debit calls = %d, want 1", calls)
	}
	subs, total, err := r.ListSubscriptions(ctx, subscription.SubFilter{TenantID: tenantID, UserID: userID, Limit: 20})
	if err != nil || total != 1 || len(subs) != 1 {
		t.Fatalf("idempotent subscriptions = %d/%d, %v", len(subs), total, err)
	}

	// 队列上限并发：3 个套餐轮流买（同套餐 pending 去重会先于 QueueFull 拦截，
	// 单套餐无法填满队列），成功恰好 3 次（1 active + 2 pending），其余被
	// QueueFull 或同套餐去重拒绝（顺序随机，两者都合法）。
	queueUser := userID + "_queue"
	plans := []*subscription.Plan{plan, mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil), mkPlan(t, r, ctx, tenantID, 30, 1_000_000, nil, nil)}
	for _, p := range plans[1:] {
		if ok, err := svc.SetPlanStatus(ctx, p.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
			t.Fatalf("on_sale: ok=%v err=%v", ok, err)
		}
	}
	for _, p := range plans {
		seeUserGroup(t, r, ctx, tenantID, queueUser, p.Groups[0].GroupID)
	}
	const buyers = 10
	results := make(chan error, buyers)
	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := svc.Purchase(ctx, subscription.PurchaseParams{
				TenantID: tenantID, UserID: queueUser, PlanID: plans[i%len(plans)].ID,
				IdempotencyKey: fmt.Sprintf("queue-attempt-%d", i),
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, subscription.ErrQueueFull), errors.Is(err, subscription.ErrPlanAlreadyQueued):
			rejected++
		default:
			t.Fatalf("unexpected concurrent queue error: %v", err)
		}
	}
	if succeeded != 3 || rejected != buyers-3 {
		t.Fatalf("queue outcomes succeeded/rejected = %d/%d, want 3/%d", succeeded, rejected, buyers-3)
	}
}

func TestPlanInventoryPreventsOversellAndReleasesFailedReservation(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	groupID := seedGroup(t, r, ctx, tenantID)
	saleLimit := int32(1)
	service := subscription.NewService(r, &fakePurchaser{}, zap.NewNop())
	plan, err := service.CreatePlan(ctx, subscription.CreatePlanParams{
		TenantID: tenantID, Name: "inventory-" + fmt.Sprint(time.Now().UnixNano()),
		PriceCredits: 100, DurationDays: 30, TotalLimitMicro: 1_000_000,
		SaleLimit: &saleLimit,
		Groups:    []subscription.PlanGroup{{GroupID: groupID, QuotaDebitMultiplier: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := service.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
		t.Fatalf("on sale: ok=%v err=%v", ok, err)
	}
	users := []string{userID + "_a", userID + "_b"}
	for _, id := range users {
		seeUserGroup(t, r, ctx, tenantID, id, groupID)
	}

	results := make(chan error, len(users))
	var wg sync.WaitGroup
	for i, id := range users {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			_, _, err := service.Purchase(ctx, subscription.PurchaseParams{
				TenantID: tenantID, UserID: id, PlanID: plan.ID,
				IdempotencyKey: fmt.Sprintf("inventory-race-%d", i),
			})
			results <- err
		}(i, id)
	}
	wg.Wait()
	close(results)
	succeeded, soldOut := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, subscription.ErrPlanSoldOut):
			soldOut++
		default:
			t.Fatalf("unexpected inventory race result: %v", err)
		}
	}
	if succeeded != 1 || soldOut != 1 {
		t.Fatalf("inventory race succeeded/sold-out = %d/%d", succeeded, soldOut)
	}
	stored, err := r.GetPlan(ctx, plan.ID)
	if err != nil || stored.SoldCount != 1 || stored.ReservedCount != 0 {
		t.Fatalf("inventory after race = sold %d reserved %d err=%v", stored.SoldCount, stored.ReservedCount, err)
	}

	failedPlan, err := service.CreatePlan(ctx, subscription.CreatePlanParams{
		TenantID: tenantID, Name: "inventory-release-" + fmt.Sprint(time.Now().UnixNano()),
		PriceCredits: 100, DurationDays: 30, TotalLimitMicro: 1_000_000,
		SaleLimit: &saleLimit,
		Groups:    []subscription.PlanGroup{{GroupID: groupID, QuotaDebitMultiplier: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := service.SetPlanStatus(ctx, failedPlan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
		t.Fatalf("on sale failed-plan: ok=%v err=%v", ok, err)
	}
	poorUser := userID + "_poor_inventory"
	buyerUser := userID + "_buyer_inventory"
	seeUserGroup(t, r, ctx, tenantID, poorUser, groupID)
	seeUserGroup(t, r, ctx, tenantID, buyerUser, groupID)
	failingService := subscription.NewService(r, &fakePurchaser{fail: subscription.ErrInsufficientBalance}, zap.NewNop())
	if _, _, err := failingService.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: poorUser, PlanID: failedPlan.ID, IdempotencyKey: "inventory-failed",
	}); !errors.Is(err, subscription.ErrInsufficientBalance) {
		t.Fatalf("failed purchase = %v", err)
	}
	afterFailure, err := r.GetPlan(ctx, failedPlan.ID)
	if err != nil || afterFailure.SoldCount != 0 || afterFailure.ReservedCount != 0 {
		t.Fatalf("inventory was not released: plan=%+v err=%v", afterFailure, err)
	}
	if _, _, err := service.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: buyerUser, PlanID: failedPlan.ID, IdempotencyKey: "inventory-after-failure",
	}); err != nil {
		t.Fatalf("released inventory could not be purchased: %v", err)
	}
}

func TestReorderPlansByTenantIsAtomicAndDeterministic(t *testing.T) {
	r, _, ctx, tenantID, _ := subTestRepo(t)
	service := subscription.NewService(r, &fakePurchaser{}, zap.NewNop())
	first := mkPlan(t, r, ctx, tenantID, 7, 1_000_000, nil, nil)
	second := mkPlan(t, r, ctx, tenantID, 7, 1_000_000, nil, nil)
	third := mkPlan(t, r, ctx, tenantID, 7, 1_000_000, nil, nil)
	want := []string{third.ID, first.ID, second.ID}
	if err := service.ReorderPlans(ctx, tenantID, want); err != nil {
		t.Fatal(err)
	}
	plans, total, err := r.ListPlans(ctx, subscription.PlanFilter{TenantID: tenantID, Limit: 20})
	if err != nil || total != 3 || len(plans) != 3 {
		t.Fatalf("listed plans=%d total=%d err=%v", len(plans), total, err)
	}
	for i, id := range want {
		if plans[i].ID != id || plans[i].SortOrder != int32((i+1)*10) {
			t.Fatalf("plan[%d]=%s order=%d, want %s order=%d", i, plans[i].ID, plans[i].SortOrder, id, (i+1)*10)
		}
	}
	invalid := []string{second.ID, "00000000-0000-0000-0000-000000000000", first.ID}
	if err := service.ReorderPlans(ctx, tenantID, invalid); !errors.Is(err, subscription.ErrPlanReorderInvalid) {
		t.Fatalf("invalid reorder = %v", err)
	}
	unchanged, _, err := r.ListPlans(ctx, subscription.PlanFilter{TenantID: tenantID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range want {
		if unchanged[i].ID != id || unchanged[i].SortOrder != int32((i+1)*10) {
			t.Fatalf("invalid reorder partially updated plan[%d]=%s order=%d", i, unchanged[i].ID, unchanged[i].SortOrder)
		}
	}
}

func TestPurchasePolicyRollingHistoryAndRevisions(t *testing.T) {
	r, _, ctx, tenantID, userID := subTestRepo(t)
	svc := subscription.NewService(r, &fakePurchaser{}, zap.NewNop())
	groupID := seedGroup(t, r, ctx, tenantID)
	policy := subscription.PurchasePolicy{
		LifetimeMaxPurchases: i32(2),
		PeriodType:           subscription.PurchasePeriodRolling,
		PeriodMaxPurchases:   i32(1),
		RollingWindowHours:   i32(24),
		AllowAdvancePurchase: true,
	}
	plan, err := svc.CreatePlan(ctx, subscription.CreatePlanParams{
		TenantID: tenantID, Name: "limited-" + fmt.Sprint(time.Now().UnixNano()),
		PriceCredits: 100, DurationDays: 30, TotalLimitMicro: 1_000_000,
		Groups:         []subscription.PlanGroup{{GroupID: groupID, QuotaDebitMultiplier: 1}},
		PurchasePolicy: policy, CreatedBy: "creator",
	})
	if err != nil {
		t.Fatalf("create limited plan: %v", err)
	}

	update := subscription.UpdatePlanParams{
		ID: plan.ID, TenantID: tenantID, Name: plan.Name, Description: plan.Description,
		PriceCredits: plan.PriceCredits, DurationDays: plan.DurationDays,
		TotalLimitMicro: plan.TotalLimitMicro, Groups: plan.Groups,
		PurchasePolicy: &policy, UpdatedBy: "editor",
	}
	if ok, err := svc.UpdatePlan(ctx, update); err != nil || !ok {
		t.Fatalf("same-policy update: ok=%v err=%v", ok, err)
	}
	revisions, err := svc.ListPurchasePolicyRevisions(ctx, plan.ID)
	if err != nil || len(revisions) != 1 || revisions[0].Version != 1 {
		t.Fatalf("unchanged policy revisions = %+v err=%v", revisions, err)
	}
	update.PurchasePolicy = nil
	if ok, err := svc.UpdatePlan(ctx, update); err != nil || !ok {
		t.Fatalf("omitted-policy update: ok=%v err=%v", ok, err)
	}
	unchanged, err := svc.GetPlan(ctx, plan.ID)
	if err != nil || unchanged.PurchasePolicy.AllowAdvancePurchase != policy.AllowAdvancePurchase ||
		unchanged.PurchasePolicy.PeriodType != policy.PeriodType {
		t.Fatalf("omitted update changed policy: plan=%+v err=%v", unchanged, err)
	}
	revisions, err = svc.ListPurchasePolicyRevisions(ctx, plan.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("omitted update revisions = %+v err=%v", revisions, err)
	}
	update.PurchasePolicy = &policy
	update.PurchasePolicy.AllowAdvancePurchase = false
	if ok, err := svc.UpdatePlan(ctx, update); err != nil || !ok {
		t.Fatalf("policy update: ok=%v err=%v", ok, err)
	}
	revisions, err = svc.ListPurchasePolicyRevisions(ctx, plan.ID)
	if err != nil || len(revisions) != 2 || revisions[0].Version != 2 || revisions[0].ChangedBy != "editor" {
		t.Fatalf("changed policy revisions = %+v err=%v", revisions, err)
	}
	// Restore advance purchase so the rolling and lifetime rules can be tested
	// independently from the active subscription rule.
	update.PurchasePolicy.AllowAdvancePurchase = true
	if ok, err := svc.UpdatePlan(ctx, update); err != nil || !ok {
		t.Fatalf("restore advance purchase: ok=%v err=%v", ok, err)
	}
	if ok, err := svc.SetPlanStatus(ctx, plan.ID, tenantID, subscription.PlanOnSale); err != nil || !ok {
		t.Fatalf("list limited plan: ok=%v err=%v", ok, err)
	}
	seeUserGroup(t, r, ctx, tenantID, userID, groupID)

	order1, _, err := svc.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: userID, PlanID: plan.ID, IdempotencyKey: "limited-1",
	})
	if err != nil {
		t.Fatalf("first limited purchase: %v", err)
	}
	plans, _, err := svc.ListPlansForUser(ctx, subscription.PlanFilter{
		TenantID: tenantID, OnSaleOnly: true, Limit: 20,
	}, userID)
	if err != nil || len(plans) != 1 || plans[0].PurchaseEligibility == nil {
		t.Fatalf("storefront eligibility: plans=%+v err=%v", plans, err)
	}
	decision := plans[0].PurchaseEligibility
	if decision.Allowed || decision.PrimaryReason != subscription.PurchaseRollingLimitReached || decision.RetryAt == nil {
		t.Fatalf("rolling decision = %+v", decision)
	}

	if _, err := r.pool.Exec(ctx, `UPDATE ai_sub_orders SET created_at=created_at-interval '25 hours' WHERE id=$1`, order1.ID); err != nil {
		t.Fatalf("age first order: %v", err)
	}
	plans, _, err = svc.ListPlansForUser(ctx, subscription.PlanFilter{
		TenantID: tenantID, OnSaleOnly: true, Limit: 20,
	}, userID)
	if err != nil || len(plans) != 1 || plans[0].PurchaseEligibility == nil || !plans[0].PurchaseEligibility.Allowed {
		t.Fatalf("eligibility after rolling boundary: plans=%+v err=%v", plans, err)
	}
	if _, _, err := svc.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: userID, PlanID: plan.ID, IdempotencyKey: "limited-2",
	}); err != nil {
		t.Fatalf("second limited purchase: %v", err)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE ai_sub_subscriptions SET status='cancelled' WHERE tenant_id=$1 AND user_id=$2`, tenantID, userID); err != nil {
		t.Fatalf("clear live subscriptions for lifetime assertion: %v", err)
	}
	_, _, err = svc.Purchase(ctx, subscription.PurchaseParams{
		TenantID: tenantID, UserID: userID, PlanID: plan.ID, IdempotencyKey: "limited-3",
	})
	var denied *subscription.PurchaseDeniedError
	if !errors.As(err, &denied) || denied.Decision.PrimaryReason != subscription.PurchaseLifetimeLimitReached {
		t.Fatalf("third limited purchase = %v, decision=%+v", err, denied)
	}
}

func i32(v int32) *int32 { return &v }
