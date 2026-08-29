<script setup lang="ts">
import { DsEmpty } from "@/shared/ui";
import type { AdminTenantLimitForm, AdminTenantPolicySubject } from "../types";

defineProps<{
  selectedTenant: AdminTenantPolicySubject | null;
  loading: boolean;
  configured: boolean;
  summaryText: string;
  form: AdminTenantLimitForm;
}>();

const statusOptions = [
  { label: "启用", value: "active" },
  { label: "停用", value: "disabled" }
];
</script>

<template>
  <section v-loading="loading" class="limit-panel">
    <template v-if="selectedTenant">
      <div class="stats-grid">
        <div class="stat-card">
          <span class="stat-label">最大同时请求数</span>
          <strong class="stat-value">{{ form.concurrency_limit ?? "不限" }}</strong>
          <span class="stat-meta">租户整体并发</span>
        </div>
        <div class="stat-card">
          <span class="stat-label">策略状态</span>
          <strong class="stat-value">{{ configured ? (form.status === "active" ? "启用" : "停用") : "未配置" }}</strong>
          <span class="stat-meta">租户级限流</span>
        </div>
      </div>

      <el-alert :closable="false" type="info" class="summary-alert" :title="summaryText" />

      <el-form label-position="top" class="form-grid">
        <el-form-item label="最大同时请求数">
          <el-input-number v-model="form.concurrency_limit" :min="1" :step="1" :controls="false" class="full-field" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status" class="full-field">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
    </template>

    <DsEmpty v-else title="从左侧选择一个租户进行限流配置" />
  </section>
</template>

<style scoped>
.limit-panel {
  min-height: 260px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.stat-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: linear-gradient(180deg, var(--ds-panel) 0%, var(--ds-panel-muted) 100%);
}

.stat-label,
.stat-meta {
  color: var(--ds-muted);
  font-size: 12px;
}

.stat-value {
  color: var(--ds-ink);
  font-size: 20px;
  font-weight: 700;
}

.summary-alert {
  margin-bottom: 16px;
  border-radius: var(--ds-radius-panel);
  border: 1px solid color-mix(in srgb, var(--ds-info) 22%, transparent);
  background: var(--ds-info-soft);
  padding: 14px 16px;
}

:deep(.summary-alert .el-alert__icon) {
  color: var(--ds-info);
}

:deep(.summary-alert .el-alert__title) {
  color: var(--ds-info);
  font-weight: 600;
  font-size: 13px;
  line-height: 1.6;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.full-field {
  width: 100%;
}
</style>
