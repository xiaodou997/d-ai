<script setup lang="ts">
import { useRouter } from "vue-router";
import { ArrowRight, ArrowUpRight, Banknote, CircleAlert, RefreshCw } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsButton } from "@/shared/ui";
import TenantWorkbenchRangeTabs from "@/components/workbench/TenantWorkbenchRangeTabs.vue";
import { WORKBENCH_RANGE_OPTIONS } from "@/components/workbench/workbenchRanges";
import { useTenantOperationsDashboard } from "./composables/useTenantOperationsDashboard";
import { useTenantFinanceSummary } from "./composables/useTenantFinanceSummary";
import TenantFinanceOverview from "./components/TenantFinanceOverview.vue";
import TenantFinanceBalances from "./components/TenantFinanceBalances.vue";
import TenantFinanceUsers from "./components/TenantFinanceUsers.vue";
import TenantFinanceSignals from "./components/TenantFinanceSignals.vue";
import TenantFinanceStructure from "./components/TenantFinanceStructure.vue";
import TenantFinanceActivity from "./components/TenantFinanceActivity.vue";

import TenantFinanceSection from "./components/TenantFinanceSection.vue";

const router = useRouter();
const dashboard = useTenantOperationsDashboard();
const finance = useTenantFinanceSummary(dashboard);

function goTo(path: string) {
  void router.push(path);
}

function handleRangeChange(rangeId: string) {
  void dashboard.selectRange(rangeId);
}
</script>

<template>
  <div class="page-container finance-workbench">
    <PortalPagePanel
      fill
      :icon="Banknote"
      :breadcrumbs="[{ label: '概览' }, { label: '经营分析' }]"
      description="把收入、平台成本、用户余额与账务风险放在同一个租户经营视图里。"
    >
      <template #actions>
        <TenantWorkbenchRangeTabs
          :model-value="dashboard.selectedRangeId.value"
          :options="WORKBENCH_RANGE_OPTIONS"
          :loading="dashboard.loading.value"
          aria-label="经营分析时间范围"
          @update:model-value="handleRangeChange"
        />
        <DsButton variant="ghost" size="sm" @click="goTo('/tenant/account?tab=ledger')">
          <template #icon><ArrowUpRight :size="14" /></template>
          账户流水
        </DsButton>
        <DsButton variant="primary" size="sm" :loading="dashboard.loading.value" @click="dashboard.refresh">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </DsButton>
      </template>

      <div class="finance-body" :aria-busy="dashboard.loading.value">
        <TenantFinanceSection title="经营结论" :errors="dashboard.failedSources('overview', 'summary', 'balance')" :loading="dashboard.loading.value" @retry="dashboard.refresh">
          <TenantFinanceOverview :dashboard="dashboard" :finance="finance" />
        </TenantFinanceSection>

        <TenantFinanceSection title="额度与资金流" :errors="dashboard.failedSources('balance', 'overview', 'summary')" :loading="dashboard.loading.value" @retry="dashboard.refresh">
          <TenantFinanceBalances :dashboard="dashboard" :finance="finance" />
        </TenantFinanceSection>

        <section class="finance-grid finance-grid--secondary" aria-label="用户资金健康与经营信号">
          <TenantFinanceSection title="用户资金健康" :errors="dashboard.failedSources('overview', 'users', 'ranking')" :loading="dashboard.loading.value" @retry="dashboard.refresh">
            <TenantFinanceUsers :dashboard="dashboard" :finance="finance" />
          </TenantFinanceSection>

          <TenantFinanceSection title="经营信号" :errors="dashboard.failedSources('overview', 'summary', 'balance')" :loading="dashboard.loading.value" @retry="dashboard.refresh">
            <TenantFinanceSignals :finance="finance" />
          </TenantFinanceSection>
        </section>

        <TenantFinanceSection title="成本结构" :errors="dashboard.failedSources('apps', 'models')" :loading="dashboard.loading.value" @retry="dashboard.refresh">
          <TenantFinanceStructure :dashboard="dashboard" />
        </TenantFinanceSection>

        <TenantFinanceSection title="近期消费与账务记录" :errors="dashboard.failedSources('recent', 'ledger')" :loading="dashboard.loading.value" @retry="dashboard.refresh">
          <TenantFinanceActivity :dashboard="dashboard" :finance="finance" />
        </TenantFinanceSection>

        <footer class="finance-footnote">
          <span class="finance-footnote__icon"><CircleAlert :size="14" /></span>
          <span>口径说明：D-AI 可归因收入来自系统内实际扣款，平台服务成本来自租户结算；外部 API 和租户自有平台的收入不在本页统计。</span>
          <button type="button" @click="goTo('/tenant/account?tab=ledger')">查看原始流水 <ArrowRight :size="13" /></button>
        </footer>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.finance-workbench {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.finance-body {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: 26px;
  padding: 24px;
  background: var(--ds-paper);
}

:deep(.finance-section) {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 14px;
}

:deep(.section-heading) {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 16px;
}

:deep(.section-heading__eyebrow) {
  display: block;
  margin-bottom: 4px;
  color: var(--ds-accent);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.13em;
}

:deep(.section-heading h2) {
  margin: 0;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 750;
  letter-spacing: -0.02em;
}

:deep(.section-heading__note) {
  color: var(--ds-ink-soft);
  font-size: 12px;
}

:deep(.finance-card__icon) {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

:deep(.finance-card__icon--positive) {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

:deep(.finance-card__icon--warning) {
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
}

:deep(.finance-card__icon--info) {
  background: var(--ds-info-soft);
  color: var(--ds-info);
}

:deep(.finance-card__period) {
  color: var(--ds-faint);
  font-size: 11px;
  white-space: nowrap;
}

:deep(.finance-grid) {
  display: grid;
  min-width: 0;
  gap: 16px;
}

:deep(.finance-grid--primary) {
  grid-template-columns: minmax(0, 1.1fr) minmax(420px, 0.9fr);
}

.finance-grid--secondary {
  grid-template-columns: minmax(0, 1.15fr) minmax(340px, 0.85fr);
}

:deep(.finance-grid--structure),
:deep(.finance-grid--activity) {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

:deep(.finance-card) {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

:deep(.finance-card__head) {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  border-bottom: 1px solid var(--ds-line);
  padding: 17px 20px;
}

:deep(.finance-card__title-wrap) {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 11px;
}

:deep(.finance-card__title-wrap > div) {
  min-width: 0;
}

:deep(.finance-card__title-wrap h2) {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 750;
}

:deep(.finance-card__title-wrap p) {
  margin: 4px 0 0;
  color: var(--ds-muted);
  font-size: 11.5px;
  line-height: 1.5;
}

:deep(.finance-card__footer-actions) {
  display: flex;
  align-items: center;
  gap: 8px;
  border-top: 1px solid var(--ds-line);
  padding: 14px 20px;
}

:deep(.finance-card__head-meta) {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

:deep(.text-action),
.finance-footnote button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: var(--ds-panel);
  color: var(--ds-accent-hover);
  font-size: 11.5px;
  font-weight: 750;
  white-space: nowrap;
  cursor: pointer;
}

:deep(.text-action:hover),
.finance-footnote button:hover {
  color: var(--ds-accent);
  text-decoration: underline;
}

:deep(.finance-table-wrap) {
  min-width: 0;
  overflow-x: auto;
  padding: 6px 10px 10px;
}

:deep(.table-primary) {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

:deep(.table-primary strong) {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.table-primary small),
:deep(.table-note) {
  overflow: hidden;
  color: var(--ds-ink-soft);
  font-size: 10.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.ds-tag--neutral) {
  color: var(--ds-ink-soft);
  background: var(--ds-panel-muted);
}

:deep(.ds-tag--positive) {
  color: var(--ds-positive);
  background: var(--ds-positive-soft);
}

.finance-footnote {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  padding: 11px 14px;
  color: var(--ds-faint);
  font-size: 10.5px;
  line-height: 1.55;
}

.finance-footnote__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  color: var(--ds-muted);
}

.finance-footnote button {
  margin-left: auto;
  flex: 0 0 auto;
  padding: 0;
  color: var(--ds-muted);
  font-size: 10.5px;
}

:deep(.amount-positive) {
  color: var(--ds-positive);
}

:deep(.amount-negative) {
  color: var(--ds-danger);
}

@media (max-width: 1240px) {

  :deep(.finance-grid--primary),
  .finance-grid--secondary {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 900px) {

  :deep(.finance-grid--structure),
  :deep(.finance-grid--activity) {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .finance-body {
    gap: 20px;
    padding: 16px;
  }

  :deep(.section-heading) {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }

  :deep(.finance-card__head) {
    padding-inline: 16px;
  }

  :deep(.finance-card__footer-actions) {
    align-items: flex-start;
    flex-direction: column;
    padding-inline: 16px;
  }

  .finance-footnote {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .finance-footnote button {
    margin-left: 22px;
  }
}
</style>
