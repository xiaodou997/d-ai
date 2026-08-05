<script setup lang="ts">
import { shallowRef } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import {
  PortalContentCard,
  PortalMetricGrid
} from "@dai/app-core";
import { DsFilterBar, DsPagination, DsTag } from "@dai/ui";
import { requestSourceOptions } from "@dai/app-core/ai/usage";

import type {
  AdminUsageRow,
  UsageFilterChip,
  UsageFilters,
  UsageHighlight,
  UsageMetric,
  UsagePagination
} from "../model";
import UsageExplorerTable from "./UsageExplorerTable.vue";

const props = defineProps<{
  filterChips: UsageFilterChip[];
  highlights: UsageHighlight[];
  isPlatformAdmin: boolean;
  loading: boolean;
  logs: AdminUsageRow[];
  metrics: UsageMetric[];
  pagination: UsagePagination;
  showOverview?: boolean;
  summaryNote: string;
}>();

const filters = defineModel<UsageFilters>("filters", { required: true });
const showUpstreamDetails = shallowRef(true);

const emit = defineEmits<{
  pageChange: [page: number];
  pageSizeChange: [size: number];
  refresh: [];
  resetFilters: [];
  search: [];
  selectRecord: [row: AdminUsageRow];
}>();

function selectRecord(row: AdminUsageRow) {
  emit("selectRecord", row);
}
</script>

<template>
  <div class="usage-explorer">
    <PortalContentCard
      v-if="showOverview !== false"
      title="观测上下文"
      :description="summaryNote"
      class="usage-explorer__brief"
    >
      <div class="usage-explorer__brief-grid">
        <article
          v-for="item in highlights"
          :key="item.label"
          class="usage-explorer__highlight"
        >
          <span class="usage-explorer__highlight-label">{{ item.label }}</span>
          <strong class="usage-explorer__highlight-value">{{ item.value }}</strong>
          <p class="usage-explorer__highlight-hint">{{ item.hint }}</p>
        </article>
      </div>
      <div class="usage-explorer__chips">
        <span class="usage-explorer__chips-label">已应用口径</span>
        <DsTag v-if="!filterChips.length" tone="neutral">未附加字段筛选</DsTag>
        <DsTag v-for="chip in filterChips" :key="chip.key" tone="accent">
          {{ chip.label }} · {{ chip.value }}
        </DsTag>
      </div>
    </PortalContentCard>

    <PortalMetricGrid v-if="showOverview !== false" :metrics="metrics" />

    <section class="usage-explorer__layout">
      <div class="usage-explorer__data">
        <div class="usage-explorer__filters">
          <DsFilterBar>
            <el-input
              v-if="isPlatformAdmin"
              v-model="filters.tenant_id"
              clearable
              placeholder="租户 ID"
              class="usage-filter"
            />
            <el-input v-model="filters.user_id" clearable placeholder="用户 ID" class="usage-filter" />
            <el-input v-model="filters.model_code" clearable placeholder="模型编码" class="usage-filter" />
            <el-select v-model="filters.request_status" clearable placeholder="状态" class="usage-filter">
              <el-option label="成功" value="success" />
              <el-option label="失败" value="failed" />
              <el-option label="拒绝" value="rejected" />
            </el-select>
            <el-select v-model="filters.request_source" clearable placeholder="来源" class="usage-filter">
              <el-option
                v-for="item in requestSourceOptions"
                :key="item.value"
                :label="item.label"
                :value="item.value"
              />
            </el-select>
            <template #actions>
              <el-switch
                v-model="showUpstreamDetails"
                inline-prompt
                active-text="显示上游信息"
                inactive-text="隐藏上游信息"
                class="usage-filter-switch"
              />
              <el-button type="primary" @click="emit('search')">应用筛选</el-button>
              <el-button @click="emit('resetFilters')">重置</el-button>
              <el-button :icon="Refresh" :loading="loading" @click="emit('refresh')">刷新</el-button>
            </template>
          </DsFilterBar>
          <div class="usage-explorer__filter-note">
            请求记录表保留完整筛选口径，点击整行进入请求详情页；表格本身只保留高密度摘要。
          </div>
        </div>

        <UsageExplorerTable
          :rows="logs"
          :loading="loading"
          :show-upstream-details="showUpstreamDetails"
          @select-record="selectRecord"
        />

        <div class="usage-explorer__pager">
          <DsPagination
            :page="pagination.page"
            :page-size="pagination.size"
            :total="pagination.total"
            :page-sizes="[10, 20, 50, 100]"
            @update:page="emit('pageChange', $event)"
            @update:page-size="emit('pageSizeChange', $event)"
          />
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.usage-explorer {
  display: grid;
  gap: 20px;
}

.usage-explorer__brief-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 14px;
}

.usage-explorer__highlight {
  display: grid;
  gap: 6px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 12%, var(--ds-line));
  border-radius: var(--ds-radius-panel);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--ds-accent-soft) 70%, transparent) 0%, var(--ds-panel) 100%);
  padding: 14px 16px;
}

.usage-explorer__highlight-label {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.usage-explorer__highlight-value {
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
}

.usage-explorer__highlight-hint {
  margin: 0;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.5;
}

.usage-explorer__chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
}

.usage-explorer__chips-label {
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}

.usage-explorer__layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}

.usage-explorer__data {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.usage-explorer__pager {
  display: flex;
  justify-content: flex-end;
}

.usage-explorer__filters {
  display: grid;
  gap: 14px;
}

.usage-filter {
  flex: 0 1 160px;
  width: min(160px, 100%);
}

.usage-explorer__filter-note {
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.usage-filter-switch {
  margin-right: 4px;
}

.usage-filter-switch:deep(.el-switch__label) {
  font-size: 12px;
}
</style>
