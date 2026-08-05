<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";

import { aiTenantApi } from "../../../../api/aiTenant";
import type { TenantAiLimitPolicy } from "../../../../types/aiTenant";
import type { TenantEndUserItem } from "../../../../types/tenant";

const props = defineProps<{
  user: TenantEndUserItem | null;
}>();

const statusOptions = [
  { label: "启用", value: "active" },
  { label: "停用", value: "disabled" }
];

const loading = shallowRef(false);
const saving = shallowRef(false);
const currentPolicy = shallowRef<TenantAiLimitPolicy | null>(null);
let loadGeneration = 0;
const form = reactive<{
  concurrencyLimit: number | null;
  status: "active" | "disabled";
}>({
  concurrencyLimit: null,
  status: "active"
});

const summaryText = computed(() => {
  if (!currentPolicy.value) return "未配置用户专属限流，当前只受租户层策略约束。";
  if (currentPolicy.value.status === "disabled") return "已保存用户限流策略，但当前停用中。";
  return "该用户会同时命中租户层策略与用户专属策略，任一层超限即拒绝。";
});

function applyPolicy(policy: TenantAiLimitPolicy | null) {
  currentPolicy.value = policy;
  form.concurrencyLimit = policy?.concurrency_limit ?? null;
  form.status = policy?.status ?? "active";
}

async function loadPolicy(userId: string) {
  const requestGeneration = ++loadGeneration;
  loading.value = true;
  try {
    const response = await aiTenantApi.listUserLimitPolicies(userId);
    if (requestGeneration !== loadGeneration) return;
    applyPolicy(response.items?.[0] ?? null);
  } catch (error) {
    if (requestGeneration !== loadGeneration) return;
    ElMessage.error(error instanceof Error ? error.message : "加载用户限流失败");
    applyPolicy(null);
  } finally {
    if (requestGeneration === loadGeneration) loading.value = false;
  }
}

async function savePolicy() {
  if (!props.user) return;
  saving.value = true;
  try {
    const policy = await aiTenantApi.upsertUserLimitPolicy(props.user.userId, {
      concurrency_limit: form.concurrencyLimit ?? undefined,
      status: form.status
    });
    applyPolicy(policy);
    ElMessage.success("用户限流策略已保存");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

watch(
  () => props.user?.userId,
  (userId) => {
    if (!userId) {
      applyPolicy(null);
      return;
    }
    void loadPolicy(userId);
  },
  { immediate: true }
);
</script>

<template>
  <section v-loading="loading" class="limit-policy-panel">
    <div class="stats-grid">
      <div class="stat-card">
        <span class="stat-label">最大同时请求数</span>
        <strong class="stat-value">{{ form.concurrencyLimit ?? "不限" }}</strong>
        <span class="stat-meta">用户整体并发</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">策略状态</span>
        <strong class="stat-value">{{ currentPolicy ? (currentPolicy.status === "active" ? "启用" : "停用") : "未配置" }}</strong>
        <span class="stat-meta">用户级策略</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">生效范围</span>
        <strong class="stat-value">租户 + 用户</strong>
        <span class="stat-meta">任一层超限即拒绝</span>
      </div>
    </div>

    <el-alert :closable="false" type="info" class="summary-alert" :title="summaryText" />

    <el-form label-position="top" class="form-grid">
      <el-form-item label="最大同时请求数">
        <el-input-number v-model="form.concurrencyLimit" :min="1" :step="1" :controls="false" class="full-field" />
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="form.status" class="full-field">
          <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </el-form-item>
    </el-form>

    <div class="form-actions">
      <el-button type="primary" :loading="saving" @click="savePolicy">保存调用限额</el-button>
    </div>
  </section>
</template>

<style scoped>
.limit-policy-panel { padding-top: 4px; }
.stats-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin-bottom: 12px; }
.stat-card { display: flex; flex-direction: column; gap: 6px; min-width: 0; padding: 2px 14px; border-left: 1px solid var(--ds-line); }
.stat-card:first-child { padding-left: 0; border-left: 0; }
.stat-label, .stat-meta { color: var(--ds-muted); font-size: 12px; }
.stat-value { color: var(--ds-ink); font-size: 20px; font-weight: 700; }
.summary-alert { margin-bottom: 16px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.full-field { width: 100%; }
.form-actions { display: flex; justify-content: flex-end; }
@media (max-width: 960px) { .stats-grid, .form-grid { grid-template-columns: 1fr; } }
</style>
