<!--
  AI 工作台异常与质量信号区:最近请求错误 + 样本用户消耗 Top（dashboard feature）。
  重构:迁移至 DsUI——TenantWorkbenchSection(自研卡片)→ AiWorkbenchSection 分区,
       错误列表 el-table → DsTable(子面板内 :frame="false"),状态 el-tag → DsTag。
-->
<script setup lang="ts">
import { ArrowRight } from "@element-plus/icons-vue";
import { useRouter } from "vue-router";

import AiWorkbenchSection from "./AiWorkbenchSection.vue";
import AiUsageUserInsight from "./AiUsageUserInsight.vue";

interface UserInsightItem {
  key: string;
  userLabel: string;
  totalAmountUSD: number;
  amountText: string;
  requestCount: number;
  errorRateText: string;
  lastActiveText: string;
}

const props = defineProps<{
  userInsights: UserInsightItem[];
  usersLoading: boolean;
  rangeLabel: string;
}>();

const router = useRouter();

</script>

<template>
  <AiWorkbenchSection
    title="调用主体"
    :description="`查看 ${props.rangeLabel} 的主要调用主体，区分终端用户、外部标识与租户自身调用。`"
  >
    <template #actions>
      <el-button text type="primary" class="!text-xs font-bold" @click="router.push('/tenant/ai/usage')">
        查看更多信号 <el-icon class="ml-1"><ArrowRight /></el-icon>
      </el-button>
    </template>

    <div class="ai-quality-grid">
      <AiUsageUserInsight :loading="usersLoading" :items="userInsights" />
    </div>
  </AiWorkbenchSection>
</template>

<style scoped>
.ai-quality-grid {
  display: grid;
  grid-template-columns: repeat(1, minmax(0, 1fr));
  gap: 16px;
}

@media (min-width: 1280px) {
  .ai-quality-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.qs-panel {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.qs-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--ds-line);
  padding: 18px 24px;
}

.qs-panel__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 650;
}

.qs-panel__desc {
  margin: 2px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
}

.qs-panel__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 0;
}

.qs-panel__spinner {
  color: var(--ds-faint);
}
</style>
