<!--
  租户列表面板 — 嵌在 AccessView 一体面板左栏(父级提供内边距与分隔线)。
  重构:去掉 PortalContentCard 外壳,el-tag/el-empty/el-pagination 换为 DsTag/DsEmpty/DsPagination。
-->
<script setup lang="ts">
import { computed } from "vue";
import { RefreshRight } from "@element-plus/icons-vue";

import { DsEmpty, DsPagination, DsTag } from "@dai/ui";
import type { TenantListItem } from "../../../../../types/admin";

const props = defineProps<{
  loading: boolean;
  tenants: TenantListItem[];
  total: number;
  page: number;
  size: number;
  keyword: string;
  selectedTenantId: string;
}>();

const emit = defineEmits<{
  (e: "update:keyword", value: string): void;
  (e: "change-page", value: number): void;
  (e: "change-size", value: number): void;
  (e: "select-tenant", tenant: TenantListItem): void;
  (e: "refresh"): void;
}>();

const pageSummary = computed(() => {
  if (!props.total) return "暂无租户";
  const start = (props.page - 1) * props.size + 1;
  const end = Math.min(props.page * props.size, props.total);
  return `${start}-${end} / ${props.total}`;
});

function statusTone(status: number): "positive" | "info" {
  return status === 1 ? "positive" : "info";
}

function statusLabel(tenant: TenantListItem) {
  return tenant.statusDisplay || (tenant.status === 1 ? "启用" : "停用");
}

function updateKeyword(value: string) {
  emit("update:keyword", value);
}

function updatePage(value: number) {
  emit("change-page", value);
}

function updatePageSize(value: number) {
  emit("change-size", value);
}
</script>

<template>
  <section v-loading="loading" class="tenant-panel">
    <header class="tenant-panel__head">
      <div>
        <span class="card-title">租户列表</span>
        <p class="card-desc">选择一个租户，维护平台侧容量保护边界。</p>
      </div>
      <el-button size="small" :icon="RefreshRight" :loading="loading" @click="$emit('refresh')">刷新</el-button>
    </header>

    <div class="filters">
      <el-input
        :model-value="keyword"
        placeholder="搜索租户名称 / ID"
        clearable
        @update:model-value="updateKeyword"
        @keyup.enter="$emit('refresh')"
      />
    </div>

    <div class="tenant-summary">
      <span>{{ pageSummary }}</span>
      <span class="tenant-summary-note">右侧仅对当前选中租户生效</span>
    </div>

    <div class="tenant-list">
      <button
        v-for="tenant in tenants"
        :key="tenant.tenantId"
        type="button"
        class="tenant-item"
        :class="{ 'is-active': selectedTenantId === tenant.tenantId }"
        @click="$emit('select-tenant', tenant)"
      >
        <div class="tenant-main">
          <span class="tenant-name">{{ tenant.tenantName }}</span>
          <DsTag :tone="statusTone(tenant.status)">{{ statusLabel(tenant) }}</DsTag>
        </div>
        <span class="tenant-sub">{{ tenant.tenantId }}</span>
      </button>
      <DsEmpty v-if="!tenants.length" title="当前筛选下没有租户" />
    </div>

    <div class="tenant-pagination">
      <DsPagination
        :page="page"
        :page-size="size"
        :total="total"
        @update:page="updatePage"
        @update:page-size="updatePageSize"
      />
    </div>
  </section>
</template>

<style scoped>
.tenant-panel {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.tenant-panel__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.card-title {
  font-weight: 700;
  color: var(--ds-ink);
}

.card-desc {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
}

.filters {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
  margin-bottom: 10px;
}

.tenant-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
  color: var(--ds-muted);
  font-size: 12px;
}

.tenant-summary-note {
  color: var(--ds-faint);
}

.tenant-list {
  flex: 1;
  overflow-y: auto;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
}

.tenant-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  padding: 12px 14px;
  border: none;
  border-bottom: 1px solid var(--ds-line);
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background-color 140ms ease, box-shadow 140ms ease;
}

.tenant-item:last-child {
  border-bottom: none;
}

.tenant-item:hover {
  background: var(--ds-panel-muted);
}

.tenant-item.is-active {
  background: var(--ds-accent-soft);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--ds-accent) 40%, var(--ds-line));
}

.tenant-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.tenant-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-ink);
}

.tenant-sub {
  color: var(--ds-faint);
  font-size: 12px;
}

.tenant-pagination {
  margin-top: 12px;
}
</style>
