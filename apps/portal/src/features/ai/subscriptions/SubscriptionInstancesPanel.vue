<!--
  订阅管理「订阅实例」Tab 内容 — 终端用户订阅额度生效情况列表。
  重构:el-table 迁移至 DsTable(:frame="false"),筛选改为 DsFilterBar/DsFilterField,
       状态 el-tag 换成 DsTag(success→positive 等 tone 映射);业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { onMounted, shallowRef } from "vue";
import { Refresh, Search } from "@element-plus/icons-vue";
import { EMPTY_IDENTITY_INCLUDED, normalizeIdentityIncluded, PortalIdentityCell, resolveIdentityUserLabel, resolveIdentityUserMeta, type IdentityIncluded } from "@/platform/ai/identity";
import { DsFilterBar, DsFilterField, DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantSubscription } from "@/api/types/aiTenant";

const MICRO_USD = 1_000_000;
const userId = shallowRef("");
const statusFilter = shallowRef("");
const subscriptions = shallowRef<TenantSubscription[]>([]);
const total = shallowRef(0);
const page = shallowRef(1);
const pageSize = shallowRef(20);
const loading = shallowRef(false);
const identityIncluded = shallowRef<IdentityIncluded>(EMPTY_IDENTITY_INCLUDED);

const statusOptions = [
  { label: "全部状态", value: "" }, { label: "排队中", value: "pending" }, { label: "生效中", value: "active" }, { label: "已过期", value: "expired" }, { label: "已取消", value: "cancelled" }
];

const columns: DsTableColumn[] = [
  { key: "user", title: "用户" },
  { key: "plan_name", title: "套餐" },
  { key: "status", title: "状态", width: 100 },
  { key: "quota", title: "总额度" },
  { key: "activated_at", title: "生效", width: 170 },
  { key: "expires_at", title: "到期", width: 170 }
];

function statusLabel(status: string) { return { pending: "排队中", active: "生效中", expired: "已过期", cancelled: "已取消" }[status] ?? status; }
function statusTone(status: string): "positive" | "warning" | "neutral" {
  const map: Record<string, "positive" | "warning" | "neutral"> = { active: "positive", pending: "warning", expired: "neutral", cancelled: "neutral" };
  return map[status] ?? "neutral";
}
function micro(value?: number | null) { return value == null ? "不限" : `$${(value / MICRO_USD).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`; }
function fmtTime(value?: string | null) { return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "-"; }

async function fetchSubscriptions() {
  loading.value = true;
  try {
    const result = await aiTenantApi.listSubscriptions({ user_id: userId.value || undefined, status: statusFilter.value || undefined, limit: pageSize.value, offset: (page.value - 1) * pageSize.value });
    subscriptions.value = result?.items ?? [];
    total.value = result?.total ?? 0;
    identityIncluded.value = normalizeIdentityIncluded(result?.included);
  } finally { loading.value = false; }
}

function search() { page.value = 1; void fetchSubscriptions(); }
function reset() { userId.value = ""; statusFilter.value = ""; search(); }
onMounted(() => void fetchSubscriptions());
</script>

<template>
  <!-- 单根节点:父组件对本面板使用 v-show,多根会导致指令失效 -->
  <div class="sub-instances-panel">
    <DsFilterBar class="sub-instances-filters">
      <DsFilterField label="用户 ID">
        <el-input v-model="userId" clearable placeholder="按用户 ID 筛选" class="filter-input" @keyup.enter="search" />
      </DsFilterField>
      <DsFilterField label="状态">
        <el-select v-model="statusFilter" placeholder="全部状态" class="filter-select" @change="search">
          <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </DsFilterField>
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="fetchSubscriptions">刷新</el-button>
        <el-button type="primary" :icon="Search" @click="search">查询</el-button>
        <el-button plain @click="reset">重置</el-button>
      </template>
    </DsFilterBar>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="subscriptions"
      row-key="id"
      :loading="loading"
      empty-title="暂无订阅"
    >
      <template #cell-user="{ row }">
        <PortalIdentityCell :label="resolveIdentityUserLabel(row.user_id, identityIncluded)" :meta="resolveIdentityUserMeta(row.user_id, identityIncluded)" />
      </template>
      <template #cell-status="{ row }">
        <DsTag :tone="statusTone(row.status)">{{ statusLabel(row.status) }}</DsTag>
      </template>
      <template #cell-quota="{ row }">{{ micro(row.total_used_micro_usd) }} / {{ micro(row.total_limit_micro_usd) }}</template>
      <template #cell-activated_at="{ row }">{{ fmtTime(row.activated_at) }}</template>
      <template #cell-expires_at="{ row }">{{ fmtTime(row.expires_at) }}</template>
    </DsTable>

    <div class="sub-instances-pager">
      <DsPagination
        :page="page"
        :page-size="pageSize"
        :total="total"
        :page-sizes="[10, 20, 50]"
        @update:page="page = $event; fetchSubscriptions()"
        @update:page-size="pageSize = $event; page = 1; fetchSubscriptions()"
      />
    </div>
  </div>
</template>

<style scoped>
.sub-instances-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sub-instances-filters {
  margin-bottom: 16px;
  flex-shrink: 0;
}

.filter-input {
  width: min(220px, 100%);
}

/* DsTable 撑满剩余高度并内部滚动,空态纵向居中 */
.sub-instances-panel :deep(.ds-table) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.sub-instances-panel :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

.filter-select.el-select {
  width: 160px;
}

.sub-instances-pager {
  display: flex;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--ds-line);
  flex-shrink: 0;
}
</style>
