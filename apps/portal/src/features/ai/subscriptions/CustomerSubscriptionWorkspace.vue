<!--
  用户端 AI 订阅套餐工作区：套餐商城 + 我的订阅（当前/排队/订单/历史）。
  重构：迁移至 DsUI 一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       页面级 Tab 统一 DsTabs,列表统一 DsTable、状态徽章统一 DsTag、空态统一 DsEmpty);
       业务逻辑/请求参数不变,购买确认弹窗与用量进度条仍保留 element-plus(过渡期)。
-->
<script setup lang="ts">
import {
  computed,
  onBeforeUnmount,
  onMounted,
  shallowRef
} from "vue";
import { useRouter } from "vue-router";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { Crown } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsEmpty, DsTable, DsTabs, DsTag, type DsTableColumn } from "@/shared/ui";
import { formatMultiplier } from "@/platform/ai/utils";
import { formatDisplayMicroUSD, formatDisplayUSD } from "@/shared/currency";

import { aiCustomerApi } from "@/api/aiCustomer";
import { platformCustomerApi } from "@/api/platformCustomer";
import SubscriptionPurchaseEligibility from "@/features/ai/subscriptions/SubscriptionPurchaseEligibility.vue";
import type {
  AiSubOrder,
  AiSubPlan,
  AiSubPurchaseBlockReason,
  AiSubPurchaseProblemMeta,
  AiSubWindow,
  AiSubscription
} from "@/api/types/aiCustomer";

const router = useRouter();

const activeTab = shallowRef<"shop" | "mine">("shop");

const subTabs = [
  { key: "shop", label: "套餐商城" },
  { key: "mine", label: "我的订阅" }
];

const queuedColumns: DsTableColumn[] = [
  { key: "plan_name", title: "套餐" },
  { key: "quota", title: "额度", align: "right" },
  { key: "duration", title: "时长", width: 90, align: "right" },
  { key: "created_at", title: "购买时间", width: 170 }
];

const orderColumns: DsTableColumn[] = [
  { key: "order_no", title: "订单号", mono: true },
  { key: "plan_name", title: "套餐" },
  { key: "price", title: "价格", width: 120, align: "right" },
  { key: "status", title: "状态", width: 100 },
  { key: "fail_reason", title: "失败原因" },
  { key: "created_at", title: "时间", width: 170 }
];

const historyColumns: DsTableColumn[] = [
  { key: "plan_name", title: "套餐" },
  { key: "status", title: "状态", width: 100 },
  { key: "activated_at", title: "生效", width: 170 },
  { key: "expires_at", title: "到期", width: 170 }
];

// ---- 数据 ----
const plans = shallowRef<AiSubPlan[]>([]);
const current = shallowRef<AiSubscription | null>(null);
const subscriptions = shallowRef<AiSubscription[]>([]);
const orders = shallowRef<AiSubOrder[]>([]);
const availableUsd = shallowRef(0);

const loadingShop = shallowRef(false);
const loadingMine = shallowRef(false);

// ---- 购买弹窗 ----
const buyVisible = shallowRef(false);
const buyPlan = shallowRef<AiSubPlan | null>(null);
const purchasing = shallowRef(false);
const purchaseHint = shallowRef("");
const PURCHASE_ATTEMPT_KEY_PREFIX = "doustack:customer:ai-subscription-purchase:";

// ---- 倒计时用的当前时间（每秒刷新）----
const now = shallowRef(Date.now());
let ticker: ReturnType<typeof setInterval> | null = null;

function usdLabel(v: number): string {
  return formatDisplayUSD(v);
}

function microUSD(v: number | null | undefined): string {
  if (v == null) return "不限";
  return formatDisplayMicroUSD(v);
}

const insufficient = computed(() => (buyPlan.value ? availableUsd.value * 1_000_000 < buyPlan.value.price_micro_usd : false));
const purchaseBlocked = computed(
  () => buyPlan.value?.sold_out === true || buyPlan.value?.purchase_eligibility?.allowed === false
);
const policyBlockCodes = new Set<AiSubPurchaseBlockReason>([
  "purchase_order_processing",
  "purchase_plan_already_queued",
  "subscription_queue_full",
  "advance_purchase_not_allowed",
  "purchase_lifetime_limit_reached",
  "purchase_rolling_limit_reached",
  "purchase_calendar_limit_reached"
]);

function subStatusLabel(status: string): string {
  return { pending: "排队中", active: "生效中", expired: "已过期", cancelled: "已取消" }[status] ?? status;
}
function subStatusTone(status: string): "positive" | "warning" | "neutral" {
  const map: Record<string, "positive" | "warning"> = { active: "positive", pending: "warning" };
  return map[status] ?? "neutral";
}
function orderStatusLabel(status: string): string {
  return { created: "已创建", deducting: "处理中", paid: "已完成", failed: "已失败" }[status] ?? status;
}
function orderStatusTone(status: string): "positive" | "warning" | "danger" | "neutral" {
  const map: Record<string, "positive" | "warning" | "danger"> = {
    paid: "positive",
    deducting: "warning",
    failed: "danger"
  };
  return map[status] ?? "neutral";
}

function fmtTime(value?: string | null): string {
  if (!value) return "-";
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}

// 窗口进度：返回百分比（used/limit），limit 为空返回 null（不限，不画进度）。
function windowPercent(w: AiSubWindow): number | null {
  if (w.limit_micro_usd == null || w.limit_micro_usd <= 0) return null;
  const pct = (w.used_micro_usd / w.limit_micro_usd) * 100;
  return Math.min(100, Math.max(0, Math.round(pct)));
}
function totalPercent(sub: AiSubscription): number {
  if (!sub.total_limit_micro_usd) return 0;
  return Math.min(100, Math.max(0, Math.round((sub.total_used_micro_usd / sub.total_limit_micro_usd) * 100)));
}
function progressColor(pct: number | null): string {
  if (pct == null) return "var(--ds-accent)";
  if (pct >= 95) return "var(--ds-danger)";
  if (pct >= 80) return "var(--ds-warning)";
  return "var(--ds-accent)";
}

function usageTone(pct: number | null): string {
  if (pct == null) return "normal";
  if (pct >= 95) return "danger";
  if (pct >= 80) return "warning";
  return "normal";
}

// 倒计时文案：reset_at 距 now 的剩余时长。
function countdown(resetAt?: string | null): string {
  if (!resetAt) return "";
  const diff = new Date(resetAt).getTime() - now.value;
  if (diff <= 0) return "即将重置";
  const totalMin = Math.floor(diff / 60000);
  const d = Math.floor(totalMin / 1440);
  const h = Math.floor((totalMin % 1440) / 60);
  const m = totalMin % 60;
  if (d > 0) return `${d}天${h}小时后重置`;
  if (h > 0) return `${h}小时${m}分后重置`;
  return `${m}分钟后重置`;
}

// ---- 拉数据 ----
async function loadShop() {
  loadingShop.value = true;
  try {
    const [planRes, cur, bal] = await Promise.all([
      aiCustomerApi.listSubscriptionPlans({ limit: 100 }),
      aiCustomerApi.getCurrentSubscription(),
      platformCustomerApi.getBalance(false).catch(() => null)
    ]);
    plans.value = planRes?.items ?? [];
    current.value = cur ?? null;
    if (bal) availableUsd.value = bal.availableUsd ?? 0;
  } finally {
    loadingShop.value = false;
  }
}

async function loadMine() {
  loadingMine.value = true;
  try {
    const [cur, subRes, orderRes] = await Promise.all([
      aiCustomerApi.getCurrentSubscription(),
      aiCustomerApi.listMySubscriptions({ limit: 50 }),
      aiCustomerApi.listSubscriptionOrders({ limit: 50 })
    ]);
    current.value = cur ?? null;
    subscriptions.value = subRes?.items ?? [];
    orders.value = orderRes?.items ?? [];
  } finally {
    loadingMine.value = false;
  }
}

function refresh() {
  if (activeTab.value === "shop") void loadShop();
  else void loadMine();
}

function handleTabChange(key: string) {
  activeTab.value = key as "shop" | "mine";
  refresh();
}

// 排队中订阅（非当前生效）。
const queued = computed(() => subscriptions.value.filter((s) => s.status === "pending"));
const historySubs = computed(() => subscriptions.value.filter((s) => s.status === "expired" || s.status === "cancelled"));

// ---- 购买 ----
function openBuy(plan: AiSubPlan) {
  if (plan.sold_out || plan.purchase_eligibility?.allowed === false) return;
  buyPlan.value = plan;
  purchaseHint.value = "";
  buyVisible.value = true;
  // 打开时刷新一次余额，避免过期。
  void platformCustomerApi
    .getBalance(false)
    .then((bal) => {
      if (bal) availableUsd.value = bal.availableUsd ?? 0;
    })
    .catch(() => undefined);
}

function goRecharge() {
  buyVisible.value = false;
  void router.push("/customer/account/topup");
}

// 202 处理中：轮询订单终态。
async function pollOrder(orderId: string): Promise<AiSubOrder | null> {
  const maxAttempts = 20;
  for (let i = 0; i < maxAttempts; i++) {
    await new Promise((r) => setTimeout(r, 1500));
    try {
      const order = await aiCustomerApi.getSubscriptionOrder(orderId);
      if (order.status === "paid" || order.status === "failed") return order;
    } catch {
      // 轮询期间的瞬时错误忽略，继续重试。
    }
  }
  return null;
}

function purchaseAttemptKey(planId: string): string {
  const storageKey = `${PURCHASE_ATTEMPT_KEY_PREFIX}${planId}`;
  const existing = sessionStorage.getItem(storageKey);
  if (existing) return existing;
  const created = crypto.randomUUID();
  sessionStorage.setItem(storageKey, created);
  return created;
}

function clearPurchaseAttemptKey(planId: string) {
  sessionStorage.removeItem(`${PURCHASE_ATTEMPT_KEY_PREFIX}${planId}`);
}

async function confirmBuy() {
  const plan = buyPlan.value;
  if (!plan || purchasing.value) return;
  purchasing.value = true;
  purchaseHint.value = "";
  const idempotencyKey = purchaseAttemptKey(plan.id);
  let terminal = false;
  try {
    const res = await aiCustomerApi.createSubscriptionOrder(plan.id, idempotencyKey);
    if (res.processing && res.order) {
      purchaseHint.value = "扣款处理中，正在确认结果…";
      const settled = await pollOrder(res.order.id);
      if (settled?.status === "paid") {
        terminal = true;
        ElMessage.success("订阅开通成功");
      } else if (settled?.status === "failed") {
        terminal = true;
        ElMessage.error(`购买失败：${settled.fail_reason || "扣款未成功"}`);
      } else {
        ElMessage.warning("扣款处理中，请稍后在“我的订阅”查看订单结果");
      }
    } else {
      terminal = true;
      ElMessage.success("订阅开通成功");
    }
    if (terminal) clearPurchaseAttemptKey(plan.id);
    buyVisible.value = false;
    await Promise.all([loadShop(), activeTab.value === "mine" ? loadMine() : Promise.resolve()]);
  } catch (err) {
    const e = err as {
      code?: string;
      detail?: string;
      message?: string;
      meta?: AiSubPurchaseProblemMeta;
    };
    if (e?.code === "insufficient_balance") {
      clearPurchaseAttemptKey(plan.id);
      purchaseHint.value = "USD 余额不足，无法购买。请先充值后再试。";
    } else if (
      e?.code &&
      policyBlockCodes.has(e.code as AiSubPurchaseBlockReason)
    ) {
      clearPurchaseAttemptKey(plan.id);
      const retryAt = e.meta?.retry_at;
      purchaseHint.value = retryAt
        ? `${e.detail || "当前不可购买"}，${fmtTime(retryAt)} 后可重试。`
        : e.detail || "当前不满足套餐购买条件。";
      await loadShop();
      buyPlan.value = plans.value.find((item) => item.id === plan.id) ?? plan;
    } else if (e?.code === "conflict") {
      clearPurchaseAttemptKey(plan.id);
      purchaseHint.value = e.detail || "订阅排队已满（最多 1 个生效 + 2 个排队）。";
      await loadShop();
      buyPlan.value = plans.value.find((item) => item.id === plan.id) ?? plan;
    } else if (e?.code === "plan_unavailable") {
      clearPurchaseAttemptKey(plan.id);
      purchaseHint.value = "套餐适用分组已变化，当前暂不可购买。";
      await loadShop();
    } else {
      ElMessage.error(e?.detail || e?.message || "购买失败");
    }
  } finally {
    purchasing.value = false;
  }
}

onMounted(() => {
  void loadShop();
  ticker = setInterval(() => {
    now.value = Date.now();
  }, 1000);
});
onBeforeUnmount(() => {
  if (ticker) clearInterval(ticker);
});
</script>

<template>
  <div class="page-container subscription-page">
    <PortalPagePanel
      :icon="Crown"
      :breadcrumbs="[{ label: '智能服务' }, { label: '我的服务' }, { label: '订阅套餐' }]"
      description="使用 USD 余额购买固定时长的 AI 额度套餐，订阅期内的用量优先扣套餐额度，额度用尽自动回落按量计费。"
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loadingShop || loadingMine" @click="refresh">刷新</el-button>
      </template>

      <div class="sub-body">
        <DsTabs :tabs="subTabs" :model-value="activeTab" @update:model-value="handleTabChange" />

        <!-- ===== 套餐商城 ===== -->
        <div v-show="activeTab === 'shop'" v-loading="loadingShop" class="sub-panel">
          <DsEmpty
            v-if="!plans.length && !loadingShop"
            title="暂无在售套餐"
            description="当前暂无在售套餐，请稍后刷新查看。"
          />
          <div v-else class="plan-grid">
            <article v-for="plan in plans" :key="plan.id" class="plan-card">
              <header class="plan-card__head">
                <h3 class="plan-card__name">{{ plan.name }}</h3>
                <div class="plan-card__price">
                  <strong>{{ microUSD(plan.price_micro_usd) }}</strong>
                  <span>USD</span>
                </div>
              </header>
              <p v-if="plan.description" class="plan-card__desc">{{ plan.description }}</p>
              <ul class="plan-card__specs">
                <li><span>有效期</span><b>{{ plan.duration_days }} 天</b></li>
                <li v-if="plan.sale_limit != null">
                  <span>销售情况</span>
                  <b :class="{ 'is-sold-out': plan.sold_out }">
                    {{ plan.sold_out ? "已售罄" : `剩余 ${plan.available_count ?? 0} 份` }}
                  </b>
                </li>
                <li class="plan-card__quota-row">
                  <span>总额度</span>
                  <b class="money-value">{{ microUSD(plan.total_limit_micro_usd) }}</b>
                  <small>{{ microUSD(plan.total_limit_micro_usd) }}</small>
                </li>
                <li class="plan-card__quota-row">
                  <span>5 小时窗口</span>
                  <b class="money-value">{{ microUSD(plan.window_5h_limit_micro_usd) }}</b>
                  <small>{{ microUSD(plan.window_5h_limit_micro_usd) }}</small>
                </li>
                <li class="plan-card__quota-row">
                  <span>7 天窗口</span>
                  <b class="money-value">{{ microUSD(plan.window_7d_limit_micro_usd) }}</b>
                  <small>{{ microUSD(plan.window_7d_limit_micro_usd) }}</small>
                </li>
              </ul>
              <div v-if="plan.groups && plan.groups.length" class="plan-card__groups">
                <div class="plan-card__groups-label">覆盖分组</div>
                <div class="plan-card__groups-tags">
                  <DsTag v-for="g in plan.groups" :key="g.id" tone="neutral">
                    {{ g.name }} ×{{ formatMultiplier(g.quota_debit_multiplier) }}
                  </DsTag>
                </div>
                <p class="plan-card__groups-hint">额度按命中分组的基准价 × 套餐扣额倍率计量</p>
              </div>
              <SubscriptionPurchaseEligibility :plan="plan" />
              <el-button
                type="primary"
                class="plan-card__buy"
                :disabled="plan.sold_out || plan.purchase_eligibility?.allowed === false"
                @click="openBuy(plan)"
              >{{ plan.sold_out ? "已售罄" : plan.purchase_eligibility?.allowed === false ? "暂不可购买" : "购买" }}</el-button>
            </article>
          </div>
        </div>

        <!-- ===== 我的订阅 ===== -->
        <div v-show="activeTab === 'mine'" v-loading="loadingMine" class="sub-panel mine-stack">
          <!-- 当前生效 -->
          <section class="mine-block">
            <h2 class="mine-block__title">当前订阅</h2>
            <div v-if="current" class="current-card">
              <div class="current-card__head">
                <div class="current-card__title">
                  <span class="current-card__name">{{ current.plan_name }}</span>
                  <small>到期：{{ fmtTime(current.expires_at) }}</small>
                </div>
                <DsTag :tone="subStatusTone(current.status)">
                  {{ subStatusLabel(current.status) }}
                </DsTag>
              </div>

              <div v-if="current.groups && current.groups.length" class="current-card__groups">
                <span class="current-card__groups-label">覆盖分组</span>
                <DsTag v-for="g in current.groups" :key="g.id" tone="neutral" class="current-card__group-tag">
                  {{ g.name }} ×{{ formatMultiplier(g.quota_debit_multiplier) }}
                </DsTag>
              </div>

              <div class="usage-grid">
                <section class="usage-card" :class="`is-${usageTone(totalPercent(current))}`">
                  <header class="usage-card__head">
                    <div>
                      <strong>总额度</strong>
                      <span>订阅周期内累计</span>
                    </div>
                    <b>{{ totalPercent(current) }}% used</b>
                  </header>
                  <div class="usage-card__money">
                    <strong>{{ microUSD(current.total_used_micro_usd) }}</strong>
                    <span>/ {{ microUSD(current.total_limit_micro_usd) }}</span>
                  </div>
                  <div class="usage-card__sub">
                    {{ microUSD(current.total_used_micro_usd) }} / {{ microUSD(current.total_limit_micro_usd) }}
                    <em>剩余 {{ microUSD(current.total_remaining_micro_usd) }}</em>
                  </div>
                  <el-progress :percentage="totalPercent(current)" :color="progressColor(totalPercent(current))" :stroke-width="9" />
                </section>

                <section
                  class="usage-card"
                  :class="`is-${usageTone(windowPercent(current.window_5h))}`"
                >
                  <header class="usage-card__head">
                    <div>
                      <strong>5 小时窗口</strong>
                      <span v-if="current.window_5h.reset_at">{{ countdown(current.window_5h.reset_at) }}</span>
                      <span v-else>窗口首次使用后开始</span>
                    </div>
                    <b v-if="windowPercent(current.window_5h) != null">{{ windowPercent(current.window_5h) }}% used</b>
                    <b v-else>不限</b>
                  </header>
                  <template v-if="current.window_5h.limit_micro_usd != null">
                    <div class="usage-card__money">
                      <strong>{{ microUSD(current.window_5h.used_micro_usd) }}</strong>
                      <span>/ {{ microUSD(current.window_5h.limit_micro_usd) }}</span>
                    </div>
                    <div class="usage-card__sub">
                      {{ microUSD(current.window_5h.used_micro_usd) }} / {{ microUSD(current.window_5h.limit_micro_usd) }}
                      <em v-if="current.window_5h.reset_at">重置：{{ fmtTime(current.window_5h.reset_at) }}</em>
                    </div>
                    <el-progress
                      :percentage="windowPercent(current.window_5h) as number"
                      :color="progressColor(windowPercent(current.window_5h))"
                      :stroke-width="9"
                    />
                  </template>
                  <span v-else class="usage-card__unlimited">该窗口不限</span>
                </section>

                <section
                  class="usage-card"
                  :class="`is-${usageTone(windowPercent(current.window_7d))}`"
                >
                  <header class="usage-card__head">
                    <div>
                      <strong>7 天窗口</strong>
                      <span v-if="current.window_7d.reset_at">{{ countdown(current.window_7d.reset_at) }}</span>
                      <span v-else>窗口首次使用后开始</span>
                    </div>
                    <b v-if="windowPercent(current.window_7d) != null">{{ windowPercent(current.window_7d) }}% used</b>
                    <b v-else>不限</b>
                  </header>
                  <template v-if="current.window_7d.limit_micro_usd != null">
                    <div class="usage-card__money">
                      <strong>{{ microUSD(current.window_7d.used_micro_usd) }}</strong>
                      <span>/ {{ microUSD(current.window_7d.limit_micro_usd) }}</span>
                    </div>
                    <div class="usage-card__sub">
                      {{ microUSD(current.window_7d.used_micro_usd) }} / {{ microUSD(current.window_7d.limit_micro_usd) }}
                      <em v-if="current.window_7d.reset_at">重置：{{ fmtTime(current.window_7d.reset_at) }}</em>
                    </div>
                    <el-progress
                      :percentage="windowPercent(current.window_7d) as number"
                      :color="progressColor(windowPercent(current.window_7d))"
                      :stroke-width="9"
                    />
                  </template>
                  <span v-else class="usage-card__unlimited">该窗口不限</span>
                </section>
              </div>
            </div>
            <DsEmpty
              v-else-if="!loadingMine"
              title="暂无生效中的订阅"
              description="可在「套餐商城」选购固定时长的 AI 额度套餐。"
            />
          </section>

          <!-- 排队中 -->
          <section v-if="queued.length" class="mine-block">
            <header class="mine-block__head">
              <h2 class="mine-block__title">排队中</h2>
              <span class="mine-block__count">共 {{ queued.length }} 条</span>
            </header>
            <DsTable
              :frame="false"
              :columns="queuedColumns"
              :rows="queued"
              row-key="id"
              :loading="loadingMine"
            >
              <template #cell-quota="{ row }">
                <span class="table-money">{{ microUSD(row.total_limit_micro_usd) }}</span>
                <small class="table-sub">{{ microUSD(row.total_limit_micro_usd) }}</small>
              </template>
              <template #cell-duration="{ row }">{{ row.duration_days }} 天</template>
              <template #cell-created_at="{ row }">{{ fmtTime(row.created_at) }}</template>
            </DsTable>
          </section>

          <!-- 订单历史 -->
          <section class="mine-block">
            <header class="mine-block__head">
              <h2 class="mine-block__title">订单记录</h2>
              <span class="mine-block__count">共 {{ orders.length }} 条</span>
            </header>
            <DsTable
              :frame="false"
              :columns="orderColumns"
              :rows="orders"
              row-key="id"
              :loading="loadingMine"
              empty-title="暂无订单"
            >
              <template #cell-price="{ row }">{{ microUSD(row.price_micro_usd) }}</template>
              <template #cell-status="{ row }">
                <DsTag :tone="orderStatusTone(row.status)">
                  {{ orderStatusLabel(row.status) }}
                </DsTag>
              </template>
              <template #cell-fail_reason="{ row }">{{ row.fail_reason || "-" }}</template>
              <template #cell-created_at="{ row }">{{ fmtTime(row.created_at) }}</template>
            </DsTable>
          </section>

          <!-- 历史订阅 -->
          <section v-if="historySubs.length" class="mine-block">
            <header class="mine-block__head">
              <h2 class="mine-block__title">历史订阅</h2>
              <span class="mine-block__count">共 {{ historySubs.length }} 条</span>
            </header>
            <DsTable
              :frame="false"
              :columns="historyColumns"
              :rows="historySubs"
              row-key="id"
              :loading="loadingMine"
            >
              <template #cell-status="{ row }">
                <DsTag :tone="subStatusTone(row.status)">{{ subStatusLabel(row.status) }}</DsTag>
              </template>
              <template #cell-activated_at="{ row }">{{ fmtTime(row.activated_at) }}</template>
              <template #cell-expires_at="{ row }">{{ fmtTime(row.expires_at) }}</template>
            </DsTable>
          </section>
        </div>
      </div>
    </PortalPagePanel>

    <!-- ===== 购买确认弹窗 ===== -->
    <el-dialog v-model="buyVisible" title="确认购买订阅" width="440px" append-to-body>
      <template v-if="buyPlan">
        <div class="buy-summary">
          <div class="buy-summary__row"><span>套餐</span><b>{{ buyPlan.name }}</b></div>
          <div class="buy-summary__row"><span>有效期</span><b>{{ buyPlan.duration_days }} 天</b></div>
          <div v-if="buyPlan.sale_limit != null" class="buy-summary__row">
            <span>剩余数量</span><b :class="{ 'is-sold-out': buyPlan.sold_out }">{{ buyPlan.sold_out ? "已售罄" : `${buyPlan.available_count ?? 0} 份` }}</b>
          </div>
          <div class="buy-summary__row"><span>总额度</span><b>{{ microUSD(buyPlan.total_limit_micro_usd) }}</b></div>
          <div class="buy-summary__row buy-summary__price">
            <span>需支付</span><b>{{ microUSD(buyPlan.price_micro_usd) }}</b>
          </div>
          <div class="buy-summary__row"><span>当前可用</span><b>{{ usdLabel(availableUsd) }}</b></div>
          <div v-if="buyPlan.groups && buyPlan.groups.length" class="buy-summary__row buy-summary__groups">
            <span>覆盖分组</span>
            <div class="buy-summary__groups-tags">
              <el-tag v-for="g in buyPlan.groups" :key="g.id" size="small" effect="plain">{{ g.name }} ×{{ formatMultiplier(g.quota_debit_multiplier) }}</el-tag>
            </div>
          </div>
        </div>

        <SubscriptionPurchaseEligibility :plan="buyPlan" class="buy-policy" />

        <el-alert type="warning" :closable="false" show-icon class="buy-alert">
          订阅一经购买 <b>不可退款</b>；订阅期内 AI 用量在<b>覆盖分组</b>内优先扣本套餐额度
          （按分组基准价 × 套餐扣额倍率计量），额度用尽或使用套餐外分组时自动回落按量计费。
        </el-alert>

        <el-alert v-if="insufficient" type="error" :closable="false" show-icon class="buy-alert">
          USD 余额不足，无法购买该套餐。
        </el-alert>
        <p v-if="purchaseHint" class="buy-hint">{{ purchaseHint }}</p>
      </template>

      <template #footer>
        <el-button @click="buyVisible = false">取消</el-button>
        <el-button v-if="insufficient" type="primary" @click="goRecharge">去充值</el-button>
        <el-button
          v-else
          type="primary"
          :disabled="purchaseBlocked"
          :loading="purchasing"
          @click="confirmBuy"
        >{{ buyPlan?.sold_out ? "已售罄" : purchaseBlocked ? "暂不可购买" : "确认购买" }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.subscription-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 面板 body 无内边距:24px 容器承载 Tab 与两个面板 */
.sub-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

.sub-panel {
  min-height: 200px;
}

/* 套餐卡片 */
.plan-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 16px;
}

.plan-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 18px 20px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.plan-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}

.plan-card__name {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 700;
}

.plan-card__price {
  display: flex;
  align-items: baseline;
  gap: 4px;
  color: var(--ds-accent-hover);
}

.plan-card__price strong {
  font-size: 24px;
  font-weight: 700;
}

.plan-card__price span {
  font-size: 12px;
  color: var(--ds-muted);
}

.plan-card__desc {
  margin: 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.5;
}

.plan-card__specs {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.plan-card__specs li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
}

.plan-card__specs .plan-card__quota-row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 2px 10px;
  align-items: center;
  padding: 8px 10px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 12%, var(--ds-line));
  border-radius: var(--ds-radius-shell);
  background: color-mix(in srgb, var(--ds-accent-soft) 32%, var(--ds-panel));
}

.plan-card__specs span {
  color: var(--ds-muted);
}

.plan-card__specs b {
  color: var(--ds-ink);
  font-weight: 650;
}

.plan-card__specs b.is-sold-out,
.buy-summary b.is-sold-out {
  color: var(--ds-danger);
}

.plan-card__specs .money-value {
  color: var(--ds-accent-hover);
  font-size: 15px;
  font-weight: 800;
}

.plan-card__quota-row small {
  grid-column: 2;
  color: var(--ds-muted);
  font-size: 11px;
  white-space: nowrap;
}

.plan-card__groups {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.plan-card__groups-label {
  font-size: 12px;
  color: var(--ds-muted);
}

.plan-card__groups-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.plan-card__groups-hint {
  margin: 2px 0 0;
  font-size: 11px;
  color: var(--ds-muted);
  line-height: 1.4;
}

.current-card__groups {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.current-card__groups-label {
  font-size: 12px;
  color: var(--ds-muted);
}

.current-card__group-tag {
  margin-right: 2px;
}

.buy-summary__groups {
  align-items: flex-start;
}

.buy-summary__groups-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
  max-width: 70%;
}

.plan-card__buy {
  margin-top: 4px;
}

/* 我的订阅 */
.mine-stack {
  display: flex;
  flex-direction: column;
  gap: 22px;
}

.mine-block__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin: 0 0 12px;
}

.mine-block__head .mine-block__title {
  margin: 0;
}

.mine-block__title {
  margin: 0 0 12px;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 700;
}

.mine-block__count {
  color: var(--ds-faint);
  font-size: 12.5px;
  white-space: nowrap;
}

.current-card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 18px 20px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.current-card__head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.current-card__title {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-right: auto;
}

.current-card__name {
  color: var(--ds-ink);
  font-size: 16px;
  font-weight: 700;
}

.current-card__title small {
  color: var(--ds-muted);
  font-size: 12px;
}

.usage-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.usage-card {
  display: flex;
  min-height: 150px;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-xl);
  background:
    radial-gradient(circle at top left, color-mix(in srgb, var(--ds-accent) 10%, transparent), transparent 36%),
    var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.usage-card.is-warning {
  border-color: color-mix(in srgb, var(--ds-warning) 28%, var(--ds-line));
  background:
    radial-gradient(circle at top left, color-mix(in srgb, var(--ds-warning) 16%, transparent), transparent 38%),
    var(--ds-panel);
}

.usage-card.is-danger {
  border-color: color-mix(in srgb, var(--ds-danger) 30%, var(--ds-line));
  background:
    radial-gradient(circle at top left, color-mix(in srgb, var(--ds-danger) 13%, transparent), transparent 38%),
    var(--ds-panel);
}

.usage-card__head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.usage-card__head div {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.usage-card__head strong {
  color: var(--ds-ink);
  font-size: 14px;
}

.usage-card__head span {
  color: var(--ds-muted);
  font-size: 12px;
}

.usage-card__head b {
  flex: 0 0 auto;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}

.usage-card.is-warning .usage-card__head b {
  color: var(--ds-warning);
}

.usage-card.is-danger .usage-card__head b {
  color: var(--ds-danger);
}

.usage-card__money {
  display: flex;
  align-items: baseline;
  gap: 5px;
}

.usage-card__money strong {
  color: var(--ds-accent-hover);
  font-size: 24px;
  font-weight: 850;
  letter-spacing: -0.02em;
}

.usage-card.is-warning .usage-card__money strong {
  color: var(--ds-warning);
}

.usage-card.is-danger .usage-card__money strong {
  color: var(--ds-danger);
}

.usage-card__money span {
  color: var(--ds-muted);
  font-size: 13px;
}

.usage-card__sub {
  display: flex;
  min-height: 32px;
  flex-direction: column;
  gap: 3px;
  color: var(--ds-muted);
  font-size: 12px;
}

.usage-card__sub em {
  color: var(--ds-ink-soft);
  font-style: normal;
}

.usage-card__unlimited {
  display: inline-flex;
  width: fit-content;
  margin-top: auto;
  padding: 6px 10px;
  border: 1px dashed var(--ds-line-strong);
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}

.table-money {
  display: block;
  color: var(--ds-accent-hover);
  font-size: 14px;
  font-weight: 800;
}

.table-sub {
  display: block;
  color: var(--ds-muted);
  font-size: 11px;
}

/* 购买弹窗 */
.buy-summary {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 14px;
}

.buy-summary__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
}

.buy-summary__row span {
  color: var(--ds-muted);
}

.buy-summary__row b {
  color: var(--ds-ink);
  font-weight: 650;
}

.buy-summary__price b {
  color: var(--ds-accent-hover);
  font-size: 18px;
}

.buy-summary__price small {
  margin-left: 6px;
  color: var(--ds-muted);
  font-size: 12px;
}

.buy-alert {
  margin-bottom: 10px;
}

.buy-policy {
  margin-bottom: 14px;
}

.buy-hint {
  margin: 8px 0 0;
  color: var(--ds-warning);
  font-size: 13px;
}

@media (max-width: 960px) {
  .usage-grid {
    grid-template-columns: 1fr;
  }
}
</style>
