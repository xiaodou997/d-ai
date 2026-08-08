<!--
  订阅管理「订单记录」Tab 内容 — 套餐购买订单列表。
  重构:el-table 迁移至 DsTable(:frame="false"),筛选改为 DsFilterBar/DsFilterField,
       状态 el-tag 换成 DsTag(success→positive 等 tone 映射),订单号列 mono、价格列右对齐;
       业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { onMounted, shallowRef } from "vue";
import { Refresh, Search } from "@element-plus/icons-vue";
import { EMPTY_IDENTITY_INCLUDED, normalizeIdentityIncluded, PortalIdentityCell, resolveIdentityUserLabel, resolveIdentityUserMeta, type IdentityIncluded } from "@/platform/ai/identity";
import { DsFilterBar, DsFilterField, DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantSubOrder } from "@/api/types/aiTenant";

const userId = shallowRef("");
const statusFilter = shallowRef("");
const orders = shallowRef<TenantSubOrder[]>([]);
const total = shallowRef(0);
const page = shallowRef(1);
const pageSize = shallowRef(20);
const loading = shallowRef(false);
const identityIncluded = shallowRef<IdentityIncluded>(EMPTY_IDENTITY_INCLUDED);

const statusOptions = [
  { label: "全部状态", value: "" }, { label: "已创建", value: "created" }, { label: "处理中", value: "deducting" }, { label: "已完成", value: "paid" }, { label: "已失败", value: "failed" }
];

const columns: DsTableColumn[] = [
  { key: "order_no", title: "订单号", width: 180, mono: true },
  { key: "user", title: "用户" },
  { key: "plan_name", title: "套餐" },
  { key: "price", title: "价格", width: 110, align: "right" },
  { key: "status", title: "状态", width: 100 },
  { key: "fail_reason", title: "失败原因" },
  { key: "created_at", title: "时间", width: 170 }
];

function statusLabel(status: string) { return { created: "已创建", deducting: "处理中", paid: "已完成", failed: "已失败" }[status] ?? status; }
function statusTone(status: string): "positive" | "warning" | "danger" | "neutral" {
  const map: Record<string, "positive" | "warning" | "danger" | "neutral"> = { paid: "positive", deducting: "warning", failed: "danger", created: "neutral" };
  return map[status] ?? "neutral";
}
function fmtTime(value?: string | null) { return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "-"; }
function formatMicroUSD(value: number) { return `$${(value / 1_000_000).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`; }

async function fetchOrders() {
  loading.value = true;
  try {
    const result = await aiTenantApi.listSubscriptionOrders({ user_id: userId.value || undefined, status: statusFilter.value || undefined, limit: pageSize.value, offset: (page.value - 1) * pageSize.value });
    orders.value = result?.items ?? [];
    total.value = result?.total ?? 0;
    identityIncluded.value = normalizeIdentityIncluded(result?.included);
  } finally { loading.value = false; }
}

function search() { page.value = 1; void fetchOrders(); }
function reset() { userId.value = ""; statusFilter.value = ""; search(); }
onMounted(() => void fetchOrders());
</script>

<template>
  <!-- 单根节点:父组件对本面板使用 v-show,多根会导致指令失效 -->
  <div class="sub-orders-panel">
    <DsFilterBar class="sub-orders-filters">
      <DsFilterField label="用户 ID">
        <el-input v-model="userId" clearable placeholder="按用户 ID 筛选" class="filter-input" @keyup.enter="search" />
      </DsFilterField>
      <DsFilterField label="状态">
        <el-select v-model="statusFilter" placeholder="全部状态" class="filter-select" @change="search">
          <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </DsFilterField>
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="fetchOrders">刷新</el-button>
        <el-button type="primary" :icon="Search" @click="search">查询</el-button>
        <el-button plain @click="reset">重置</el-button>
      </template>
    </DsFilterBar>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="orders"
      row-key="id"
      :loading="loading"
      empty-title="暂无订单"
    >
      <template #cell-user="{ row }">
        <PortalIdentityCell :label="resolveIdentityUserLabel(row.user_id, identityIncluded)" :meta="resolveIdentityUserMeta(row.user_id, identityIncluded)" />
      </template>
      <template #cell-price="{ row }">{{ formatMicroUSD(row.price_micro_usd) }}</template>
      <template #cell-status="{ row }">
        <DsTag :tone="statusTone(row.status)">{{ statusLabel(row.status) }}</DsTag>
      </template>
      <template #cell-fail_reason="{ row }">{{ row.fail_reason || "-" }}</template>
      <template #cell-created_at="{ row }">{{ fmtTime(row.created_at) }}</template>
    </DsTable>

    <div class="sub-orders-pager">
      <DsPagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50]"
        @update:page="page = $event; fetchOrders()"
        @update:page-size="pageSize = $event; page = 1; fetchOrders()"
      />
    </div>
  </div>
</template>

<style scoped>
.sub-orders-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sub-orders-filters {
  margin-bottom: 16px;
  flex-shrink: 0;
}

.filter-input {
  width: min(220px, 100%);
}

/* DsTable 撑满剩余高度并内部滚动,空态纵向居中 */
.sub-orders-panel :deep(.ds-table) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sub-orders-panel :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

.filter-select.el-select {
  width: 160px;
}

.sub-orders-pager {
  display: flex;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--ds-line);
  flex-shrink: 0;
}
</style>
