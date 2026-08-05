<script setup lang="ts">
import { computed, onMounted, shallowRef } from "vue";
import { DsTag } from "@dai/ui";

import { formatMultiplier as formatMultiplierValue } from "../utils";
import type { PortalVisibleGroupRecord, PortalVisibleGroupsApi } from "./types";

const props = withDefaults(
  defineProps<{
    api: PortalVisibleGroupsApi;
    title: string;
    description: string;
    eyebrow: string;
    emptyHint?: string;
    impactMessage?: string;
    refreshIcon?: unknown;
    notifyError?: (message: string) => void;
  }>(),
  {
    emptyHint: "当前账号还没有开放分组，请联系管理员处理。",
    impactMessage: "分组会决定你可访问的模型范围、API Key 可绑定的路由，以及实际计费倍率。"
  }
);

const loading = shallowRef(false);
const hasLoaded = shallowRef(false);
const errorMessage = shallowRef("");
const groups = shallowRef<PortalVisibleGroupRecord[]>([]);

const totalCount = computed(() => groups.value.length);
const activeCount = computed(() => groups.value.filter((group) => group.status === "active").length);
const previewGroups = computed(() => groups.value.slice(0, 6));
const showInitialLoadingState = computed(() => loading.value && !hasLoaded.value);
const showErrorBanner = computed(() => Boolean(errorMessage.value));
const showErrorState = computed(() => Boolean(errorMessage.value) && groups.value.length === 0);
const summaryLabel = computed(() => {
  if (showInitialLoadingState.value) return "正在加载分组";
  if (showErrorState.value) return "分组加载失败";
  return `${totalCount.value} 个可用分组`;
});
const previewHint = computed(() => {
  if (showInitialLoadingState.value) return "正在同步当前账号的分组权限...";
  if (showErrorState.value) return errorMessage.value;
  return props.emptyHint;
});
const tableEmptyText = computed(() => {
  if (showInitialLoadingState.value) return "正在加载分组，请稍候...";
  if (showErrorState.value) return errorMessage.value || "分组加载失败，请重试。";
  return props.emptyHint;
});

function formatMultiplier(value?: number | null) {
  return `x${formatMultiplierValue(value)}`;
}

function statValue(value: number) {
  if (showInitialLoadingState.value) return "...";
  if (showErrorState.value) return "--";
  return String(value);
}

function statusLabel(value?: string) {
  if (value === "active") return "启用";
  if (value === "disabled") return "停用";
  return value || "-";
}

function statusTone(value?: string) {
  if (value === "active") return "positive";
  if (value === "disabled") return "neutral";
  return "warning";
}

async function fetchGroups() {
  loading.value = true;
  errorMessage.value = "";
  try {
    const res = await props.api.listGroups();
    groups.value = res.items || [];
    hasLoaded.value = true;
  } catch (error) {
    if (!hasLoaded.value) {
      groups.value = [];
    }
    errorMessage.value = (error as Error).message || "加载分组失败，请稍后重试。";
  } finally {
    loading.value = false;
  }
}

onMounted(fetchGroups);
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div class="page-copy">
        <p class="page-eyebrow">{{ eyebrow }}</p>
        <h1 class="page-heading">{{ title }}</h1>
        <p class="page-description">{{ description }}</p>
      </div>

      <div class="header-actions">
        <slot name="actions" />
        <el-button :icon="refreshIcon" :loading="loading" @click="fetchGroups">刷新</el-button>
      </div>
    </header>

    <section class="impact-banner">
      <div class="impact-copy">
        <span class="impact-kicker">Routing Scope</span>
        <p class="impact-text">{{ impactMessage }}</p>
      </div>

      <div class="impact-preview">
        <span class="impact-preview-label">{{ summaryLabel }}</span>
        <div class="impact-tags">
          <DsTag v-for="group in previewGroups" :key="group.id" class="impact-tag">
            {{ group.name }}
          </DsTag>
          <span v-if="!previewGroups.length" class="impact-empty">{{ previewHint }}</span>
        </div>
      </div>
    </section>

    <section v-if="showErrorBanner" class="status-banner">
      <div class="status-copy">
        <strong class="status-title">分组信息暂时不可用</strong>
        <p class="status-text">{{ errorMessage }}</p>
      </div>
      <el-button plain @click="fetchGroups">重试</el-button>
    </section>

    <section class="stats-grid">
      <article class="stat-card">
        <span class="stat-label">可用分组</span>
        <strong class="stat-value">{{ statValue(totalCount) }}</strong>
        <span class="stat-note">当前账号已开放</span>
      </article>

      <article class="stat-card">
        <span class="stat-label">启用分组</span>
        <strong class="stat-value">{{ statValue(activeCount) }}</strong>
        <span class="stat-note">当前仍可正常使用</span>
      </article>
    </section>

    <section class="table-panel">
      <div class="panel-header">
        <div class="panel-copy">
          <h2 class="panel-title">分组清单</h2>
          <p class="panel-subtitle">这里仅展示当前账号已获得使用权限的分组。</p>
        </div>
      </div>

      <el-table v-loading="loading" :data="groups" stripe class="groups-table">
        <el-table-column prop="name" label="分组" min-width="180" show-overflow-tooltip />

        <el-table-column label="生效倍率" width="120">
          <template #default="{ row }">
            <span class="table-mono">{{ formatMultiplier(row.effective_user_multiplier) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <DsTag :tone="statusTone(row.status)">
              {{ statusLabel(row.status) }}
            </DsTag>
          </template>
        </el-table-column>

        <el-table-column label="说明" min-width="280">
          <template #default="{ row }">
            <span v-if="row.description" class="description-text">{{ row.description }}</span>
            <span v-else class="description-empty">暂未填写说明</span>
          </template>
        </el-table-column>

        <template #empty>
          <span class="table-empty">{{ tableEmptyText }}</span>
        </template>
      </el-table>
    </section>
  </div>
</template>

<style scoped>
.page-container {
  --groups-ink: var(--ds-ink);
  --groups-muted: var(--ds-muted);
  --groups-border: var(--ds-line);
  --groups-surface: var(--ds-panel);
  --groups-surface-soft: linear-gradient(135deg, var(--ds-accent-soft) 0%, var(--ds-panel-muted) 60%, var(--ds-panel) 100%);
  --groups-shadow: var(--ds-shadow-panel);

  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 24px;
  border: 1px solid var(--ds-line);
  border-radius: 20px;
  background: var(--groups-surface);
  box-shadow: var(--groups-shadow);
}

.page-copy {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: 760px;
}

.page-eyebrow {
  margin: 0;
  color: var(--ds-accent);
  font-size: 11px;
  font-weight: 900;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.page-heading {
  margin: 0;
  color: var(--groups-ink);
  font-size: 28px;
  font-weight: 900;
  line-height: 1.05;
}

.page-description {
  margin: 0;
  color: var(--groups-muted);
  font-size: 14px;
  line-height: 1.7;
}

.header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 12px;
}

.impact-banner {
  display: grid;
  grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr);
  gap: 18px;
  padding: 22px 24px;
  border: 1px solid var(--groups-border);
  border-radius: 20px;
  background: var(--groups-surface-soft);
}

.impact-copy {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.impact-kicker {
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 900;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.impact-text {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 700;
  line-height: 1.7;
}

.impact-preview {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px 18px;
  border-radius: 18px;
  background: color-mix(in srgb, var(--ds-panel) 90%, transparent);
  border: 1px solid color-mix(in srgb, var(--ds-line) 90%, transparent);
}

.impact-preview-label {
  color: var(--groups-ink);
  font-size: 13px;
  font-weight: 800;
}

.impact-tags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.impact-tag {
  border-radius: 999px;
}

.impact-empty {
  color: var(--groups-muted);
  font-size: 13px;
  line-height: 1.6;
}

.status-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 20px;
  border: 1px solid color-mix(in srgb, var(--ds-danger) 30%, transparent);
  border-radius: 18px;
  background: var(--ds-danger-soft);
}

.status-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.status-title {
  color: var(--ds-danger);
  font-size: 14px;
  font-weight: 900;
}

.status-text {
  margin: 0;
  color: var(--ds-danger);
  font-size: 13px;
  line-height: 1.6;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.stat-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 20px;
  border: 1px solid var(--ds-line);
  border-radius: 18px;
  background: var(--groups-surface);
  box-shadow: var(--groups-shadow);
}

.stat-label {
  color: var(--groups-muted);
  font-size: 12px;
  font-weight: 700;
}

.stat-value {
  color: var(--groups-ink);
  font-size: 30px;
  font-weight: 900;
  line-height: 1;
}

.stat-note {
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.6;
}

.table-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  border: 1px solid var(--ds-line);
  border-radius: 20px;
  background: var(--groups-surface);
  box-shadow: var(--groups-shadow);
}

.panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.panel-copy {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.panel-title {
  margin: 0;
  color: var(--groups-ink);
  font-size: 18px;
  font-weight: 900;
}

.panel-subtitle {
  margin: 0;
  color: var(--groups-muted);
  font-size: 13px;
  line-height: 1.6;
}

.groups-table {
  width: 100%;
}

.table-mono {
  color: var(--groups-ink);
  font-variant-numeric: tabular-nums;
  font-weight: 800;
}

.description-text {
  color: var(--ds-ink-soft);
  line-height: 1.7;
}

.description-empty {
  color: var(--ds-faint);
}

.table-empty {
  color: var(--ds-faint);
  font-size: 13px;
}

@media (max-width: 1100px) {
  .impact-banner {
    grid-template-columns: 1fr;
  }

  .stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .page-container {
    padding: 16px;
  }

  .page-header {
    flex-direction: column;
    padding: 20px;
  }

  .page-heading {
    font-size: 24px;
  }

  .header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .stats-grid {
    grid-template-columns: 1fr;
  }

  .status-banner {
    flex-direction: column;
    align-items: flex-start;
  }

  .table-panel {
    padding: 18px;
  }
}
</style>
