<!--
  订阅管理「套餐管理」Tab 内容 — 套餐列表、上下架、展示顺序调整。
  重构:el-table 迁移至 DsTable(:frame="false"),筛选改为 DsFilterBar/DsFilterField,
       状态/分组 el-tag 换成 DsTag(tone 映射不变),DsPagination 始终渲染
       (排序模式下翻页事件被忽略,与原隐藏分页的行为一致);套餐编辑弹窗与限制记录
       抽屉仍为 element-plus,业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { computed, onMounted, shallowRef } from "vue";
import { ArrowDown, ArrowUp, Check, Close, Plus, Rank, Refresh } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { formatCredits, formatWholeCredits } from "@/platform/ai/usage";
import { formatMultiplier } from "@/platform/ai/utils";
import { DsEmpty, DsFilterBar, DsFilterField, DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantAiVisibleGroup, TenantSubPlan } from "@/api/types/aiTenant";
import SubscriptionPlanDialog from "./SubscriptionPlanDialog.vue";
import SubscriptionPurchasePolicyHistoryDrawer from "./SubscriptionPurchasePolicyHistoryDrawer.vue";
import { subscriptionPurchasePolicyLabel } from "./subscriptionPurchasePolicy";
import { estimateSubscriptionPaygValue, type SubscriptionPricingGroup } from "./subscriptionPricing";

const MICRO_PER_CREDIT = 10_000;
const DEFAULT_CREDITS_PER_USD = 100;

const loading = shallowRef(false);
const plans = shallowRef<TenantSubPlan[]>([]);
const total = shallowRef(0);
const statusFilter = shallowRef("");
const page = shallowRef(1);
const pageSize = shallowRef(20);
const visibleGroups = shallowRef<TenantAiVisibleGroup[]>([]);
const creditsPerUsd = shallowRef(DEFAULT_CREDITS_PER_USD);
const dialogVisible = shallowRef(false);
const editingPlan = shallowRef<TenantSubPlan | null>(null);
const historyPlan = shallowRef<TenantSubPlan | null>(null);
const historyVisible = shallowRef(false);
const ordering = shallowRef(false);
const orderingSaving = shallowRef(false);
const statusBeforeOrdering = shallowRef("");

const statusOptions = [
  { label: "全部状态", value: "" },
  { label: "草稿", value: "draft" },
  { label: "已上架", value: "on_sale" },
  { label: "已下架", value: "off_sale" }
];

// 排序模式下隐藏 总额度/窗口额度/购买限制/适用分组/操作 列,新增「顺序」列(与原 v-if 列一致)
const columns = computed<DsTableColumn[]>(() => {
  const base: DsTableColumn[] = [
    { key: "name", title: "套餐名称" },
    { key: "price", title: "售价", width: 145 }
  ];
  if (ordering.value) {
    return [
      { key: "order", title: "顺序", width: 112 },
      ...base,
      { key: "duration", title: "有效期", width: 86 },
      { key: "sales", title: "销售情况" },
      { key: "status", title: "状态", width: 100 }
    ];
  }
  return [
    ...base,
    { key: "quota", title: "总额度" },
    { key: "duration", title: "有效期", width: 86 },
    { key: "sales", title: "销售情况" },
    { key: "windows", title: "窗口额度" },
    { key: "policy", title: "购买限制" },
    { key: "groups", title: "适用分组" },
    { key: "status", title: "状态", width: 100 },
    { key: "actions", title: "操作", width: 230 }
  ];
});

const visibleGroupById = computed(() => new Map(visibleGroups.value.map((group) => [group.id, group])));

function statusLabel(status: string) {
  return { draft: "草稿", on_sale: "已上架", off_sale: "已下架" }[status] ?? status;
}

function statusTone(status: string): "neutral" | "positive" | "warning" {
  const map: Record<string, "neutral" | "positive" | "warning"> = { draft: "neutral", on_sale: "positive", off_sale: "warning" };
  return map[status] ?? "neutral";
}

function creditsLabel(micro?: number | null) {
  return micro == null ? "无额外限制" : `${formatCredits(micro / MICRO_PER_CREDIT)} 积分`;
}

function yuanHintFromCredits(credits: number) {
  return `按积分汇率约 ¥${(credits / creditsPerUsd.value).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function paygValueHint(plan: TenantSubPlan) {
  const groups: SubscriptionPricingGroup[] = [];
  for (const binding of plan.groups ?? []) {
    const group = visibleGroupById.value.get(binding.id);
    if (group?.status !== "active") return "当前分组不可用";
    groups.push({
      groupId: binding.id,
      paygMultiplier: group.default_user_multiplier,
      quotaDebitMultiplier: binding.quota_debit_multiplier
    });
  }
  const valueMicro = estimateSubscriptionPaygValue(plan.total_limit_micro, groups);
  if (valueMicro == null) return "选择可售分组后显示按量价值";
  const valueCredits = valueMicro / MICRO_PER_CREDIT;
  const yuan = valueCredits / creditsPerUsd.value;
  return `最低按量价值 ${valueCredits.toLocaleString("zh-CN", { maximumFractionDigits: 4 })} 积分（约 ¥${yuan.toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}）`;
}

function inventoryLabel(plan: TenantSubPlan) {
  if (plan.sale_limit == null) return "不限量";
  if (plan.sold_out) return `已售罄 · ${plan.sold_count}/${plan.sale_limit}`;
  return `已售 ${plan.sold_count}/${plan.sale_limit} · 剩余 ${plan.available_count ?? 0}`;
}

async function fetchPlans() {
  loading.value = true;
  try {
    const result = await aiTenantApi.listSubscriptionPlans({
      status: statusFilter.value || undefined,
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value
    });
    plans.value = result?.items ?? [];
    total.value = result?.total ?? 0;
  } finally {
    loading.value = false;
  }
}

async function fetchGroups() {
  try {
    const result = await aiTenantApi.listMyGroups();
    visibleGroups.value = (result?.items ?? []).filter((group) => group.status === "active");
    const firstGroupId = visibleGroups.value[0]?.id;
    if (!firstGroupId) return;
    const pricing = await aiTenantApi.getMyGroupEffectivePrices(firstGroupId);
    if (pricing?.credits_per_usd > 0) creditsPerUsd.value = pricing.credits_per_usd;
  } catch {
    visibleGroups.value = [];
  }
}

function filterPlans() {
  page.value = 1;
  void fetchPlans();
}

function handlePageChange(nextPage: number) {
  // 排序模式加载的是全量列表,翻页会打断排序(与原隐藏分页的行为一致)
  if (ordering.value) return;
  page.value = nextPage;
  void fetchPlans();
}

function handlePageSizeChange(nextSize: number) {
  if (ordering.value) return;
  pageSize.value = nextSize;
  page.value = 1;
  void fetchPlans();
}

function openCreate() {
  editingPlan.value = null;
  dialogVisible.value = true;
}

function openEdit(plan: TenantSubPlan) {
  editingPlan.value = plan;
  dialogVisible.value = true;
}

function openHistory(plan: TenantSubPlan) {
  historyPlan.value = plan;
  historyVisible.value = true;
}

async function beginOrdering() {
  loading.value = true;
  try {
    const result = await aiTenantApi.listSubscriptionPlans({ limit: 200, offset: 0 });
    if ((result?.total ?? 0) > 200) {
      ElMessage.warning("套餐超过 200 个，暂时无法在页面中统一排序");
      return;
    }
    statusBeforeOrdering.value = statusFilter.value;
    statusFilter.value = "";
    plans.value = result?.items ?? [];
    total.value = result?.total ?? 0;
    ordering.value = true;
  } finally {
    loading.value = false;
  }
}

function movePlan(index: number, offset: -1 | 1) {
  const target = index + offset;
  if (target < 0 || target >= plans.value.length) return;
  const next = [...plans.value];
  const [moved] = next.splice(index, 1);
  if (!moved) return;
  next.splice(target, 0, moved);
  plans.value = next;
}

async function finishOrdering(save: boolean) {
  if (save) {
    orderingSaving.value = true;
    try {
      await aiTenantApi.reorderSubscriptionPlans(plans.value.map((plan) => plan.id));
      ElMessage.success("展示顺序已保存");
    } catch (error) {
      const detail = error as { detail?: string; message?: string };
      ElMessage.error(detail.detail || detail.message || "保存顺序失败");
      return;
    } finally {
      orderingSaving.value = false;
    }
  }
  ordering.value = false;
  statusFilter.value = statusBeforeOrdering.value;
  page.value = 1;
  await fetchPlans();
}

async function toggleStatus(plan: TenantSubPlan, status: "on_sale" | "off_sale") {
  const action = status === "on_sale" ? "上架" : "下架";
  try {
    await ElMessageBox.confirm(`确认${action}套餐「${plan.name}」？`, `${action}套餐`, { type: "warning" });
    await aiTenantApi.setSubscriptionPlanStatus(plan.id, status);
    ElMessage.success(`已${action}`);
    await fetchPlans();
  } catch (error) {
    if (error === "cancel" || error === "close") return;
    const detail = error as { detail?: string; message?: string };
    ElMessage.error(detail.detail || detail.message || `${action}失败`);
  }
}

onMounted(() => {
  void fetchPlans();
  void fetchGroups();
});
</script>

<template>
  <!-- 单根节点:父组件对本面板使用 v-show,多根会导致指令失效 -->
  <div class="sub-plans-panel">
    <DsFilterBar class="sub-plans-filters">
      <DsFilterField label="状态">
        <el-select v-model="statusFilter" :disabled="ordering" class="filter-select" @change="filterPlans">
          <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
      </DsFilterField>
      <template #actions>
        <template v-if="ordering">
          <el-button :icon="Close" @click="finishOrdering(false)">取消调整</el-button>
          <el-button type="primary" :icon="Check" :loading="orderingSaving" @click="finishOrdering(true)">保存顺序</el-button>
        </template>
        <template v-else>
          <el-button :icon="Refresh" :loading="loading" @click="fetchPlans">刷新</el-button>
          <el-button :icon="Rank" @click="beginOrdering">调整顺序</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">新建套餐</el-button>
        </template>
      </template>
    </DsFilterBar>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="plans"
      row-key="id"
      :loading="loading"
      empty-title="暂无套餐"
    >
      <template #empty>
        <DsEmpty title="暂无套餐" description="还没有订阅套餐,先新建一个吧">
          <template v-if="!ordering" #action>
            <el-button type="primary" :icon="Plus" @click="openCreate">新建套餐</el-button>
          </template>
        </DsEmpty>
      </template>
      <template #cell-order="{ index }">
        <div class="order-controls">
          <span>{{ index + 1 }}</span>
          <el-tooltip content="上移" placement="top">
            <el-button circle size="small" :icon="ArrowUp" :disabled="index === 0" @click="movePlan(index, -1)" />
          </el-tooltip>
          <el-tooltip content="下移" placement="top">
            <el-button circle size="small" :icon="ArrowDown" :disabled="index === plans.length - 1" @click="movePlan(index, 1)" />
          </el-tooltip>
        </div>
      </template>
      <template #cell-price="{ row }">
        <div class="value-stack">
          <strong>{{ formatWholeCredits(row.price_credits) }} 积分</strong>
          <span>{{ yuanHintFromCredits(row.price_credits) }}</span>
        </div>
      </template>
      <template #cell-quota="{ row }">
        <div class="value-stack">
          <strong>{{ creditsLabel(row.total_limit_micro) }}</strong>
          <span>{{ paygValueHint(row) }}</span>
        </div>
      </template>
      <template #cell-duration="{ row }">{{ row.duration_days }} 天</template>
      <template #cell-sales="{ row }">
        <span class="inventory-state" :class="{ 'is-sold-out': row.sold_out }">{{ inventoryLabel(row) }}</span>
      </template>
      <template #cell-windows="{ row }">
        <div class="window-values">
          <span>5h {{ creditsLabel(row.window_5h_limit_micro) }}</span>
          <span>7d {{ creditsLabel(row.window_7d_limit_micro) }}</span>
        </div>
      </template>
      <template #cell-policy="{ row }">
        <span class="policy-summary">{{ subscriptionPurchasePolicyLabel(row.purchase_policy) }}</span>
      </template>
      <template #cell-groups="{ row }">
        <div v-if="row.groups?.length" class="group-tags">
          <DsTag v-for="group in row.groups" :key="group.id">{{ group.name }} ×{{ formatMultiplier(group.quota_debit_multiplier) }}</DsTag>
        </div>
        <span v-else class="muted">未设置</span>
      </template>
      <template #cell-status="{ row }">
        <DsTag :tone="statusTone(row.status)">{{ statusLabel(row.status) }}</DsTag>
      </template>
      <template #cell-actions="{ row }">
        <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
        <el-button link @click="openHistory(row)">限制记录</el-button>
        <el-button v-if="row.status === 'on_sale'" link type="warning" @click="toggleStatus(row, 'off_sale')">下架</el-button>
        <el-button v-else link type="success" @click="toggleStatus(row, 'on_sale')">上架</el-button>
      </template>
    </DsTable>

    <div class="sub-plans-pager">
      <DsPagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50]"
        @update:page="handlePageChange"
        @update:page-size="handlePageSizeChange"
      />
    </div>

    <SubscriptionPlanDialog v-model="dialogVisible" :plan="editingPlan" :groups="visibleGroups" :credits-per-usd="creditsPerUsd" @saved="fetchPlans" />
    <SubscriptionPurchasePolicyHistoryDrawer v-model="historyVisible" :plan="historyPlan" />
  </div>
</template>

<style scoped>
.sub-plans-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sub-plans-filters {
  margin-bottom: 16px;
  flex-shrink: 0;
}

/* DsTable 撑满剩余高度并内部滚动,空态纵向居中 */
.sub-plans-panel :deep(.ds-table) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sub-plans-panel :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

.filter-select.el-select {
  width: 160px;
}

.sub-plans-pager {
  display: flex;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--ds-line);
  flex-shrink: 0;
}

.value-stack, .window-values { display: flex; flex-direction: column; gap: 2px; }
.value-stack strong { color: var(--ds-ink); font-size: 13px; }
.value-stack span, .window-values span, .policy-summary, .muted { color: var(--ds-muted); font-size: 12px; }
.group-tags { display: flex; flex-wrap: wrap; gap: 4px; }
.inventory-state { color: var(--ds-ink-soft); font-size: 12px; }
.inventory-state.is-sold-out { color: var(--ds-danger); font-weight: 600; }
.order-controls { display: grid; grid-template-columns: 20px 28px 28px; align-items: center; gap: 4px; }
.order-controls span { color: var(--ds-muted); font-size: 12px; text-align: center; }
</style>
