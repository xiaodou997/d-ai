<template>
  <div v-loading="loading" class="debt-status">
    <div class="debt-status__main" :class="{ 'is-blocked': blocked }">
      <div>
        <p class="debt-status__label">未结债务</p>
        <p class="debt-status__value">{{ formatMicroUSD(status?.outstanding_debt_micro_usd ?? 0) }}</p>
      </div>
      <el-tag :type="blocked ? 'danger' : 'success'" effect="light">
        {{ blocked ? '服务已停止' : '服务正常' }}
      </el-tag>
    </div>

    <div class="debt-status__meta">
      <span>{{ status?.account_id || accountId }}</span>
      <el-button :icon="Refresh" circle text aria-label="刷新债务状态" @click="fetchStatus" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { platformAdminApi } from "../api/platformAdmin";
import type { DebtStatusOutputBody } from "@/api/types/admin";
import { formatDisplayMicroUSD as formatMicroUSD } from "@/shared/currency";

const props = defineProps<{
  ownerType: "tenant" | "user";
  accountId: string;
}>();

const loading = ref(false);
const status = ref<DebtStatusOutputBody | null>(null);
const blocked = computed(() => status.value?.service_state === "blocked_debt");
async function fetchStatus() {
  if (!props.accountId) return;
  loading.value = true;
  try {
    status.value = await platformAdminApi.getDebtStatus(props.ownerType, props.accountId);
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "查询债务状态失败");
  } finally {
    loading.value = false;
  }
}

watch(() => [props.ownerType, props.accountId], fetchStatus);
onMounted(fetchStatus);

defineExpose({ refresh: fetchStatus });
</script>

<style scoped>
.debt-status {
  min-height: 148px;
}

.debt-status__main {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 20px;
  border-left: 3px solid var(--ds-positive);
  background: var(--ds-panel-muted);
}

.debt-status__main.is-blocked {
  border-left-color: var(--ds-danger);
  background: var(--ds-danger-soft);
}

.debt-status__label,
.debt-status__meta {
  color: var(--ds-muted);
  font-size: 12px;
}

.debt-status__value {
  margin: 4px 0 0;
  color: var(--ds-ink);
  font-size: 24px;
  font-weight: 700;
  line-height: 1.2;
}

.debt-status__meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px 0 20px;
}
</style>
