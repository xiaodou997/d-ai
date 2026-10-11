<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { ArrowUpRight, Landmark, ShoppingBag } from "lucide-vue-next";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import type { TenantOperationsDashboard } from "../composables/useTenantOperationsDashboard";
import type { TenantFinanceSummary } from "../composables/useTenantFinanceSummary";
import { sourceLabel, formatUSD, formatMicroUSD, formatTime, ledgerAmountClass, ledgerAmount, ledgerType, ledgerNote } from "../financePresentation";

const props = defineProps<{
  dashboard: Pick<TenantOperationsDashboard, "balanceLedger" | "ledgerLoading" | "recentConsumption" | "recentLoading">;
  finance: Pick<TenantFinanceSummary, "rangeLabel">;
}>();
const dashboard = props.dashboard;
const { rangeLabel } = props.finance;

const router = useRouter();

const recentUsageRows = computed(() =>
  dashboard.recentConsumption.value.slice(0, 6).map((record) => ({
    id: record.request_id,
    user: record.user_id || "未知用户",
    model: record.model || "未命名模型",
    source: sourceLabel(record.source),
    chargedUsd: (record.charge.user_charged_micro / 1_000_000) || 0,
    payableUsd: ((record.charge.tenant_charged_micro || 0) / 1_000_000) || 0,
    createdAt: record.created_at
  }))
);

const ledgerColumns: DsTableColumn[] = [
  { key: "txnType", title: "账务动作", width: 150 },
  { key: "amount", title: "变动金额", width: 150, align: "right" },
  { key: "balanceAfter", title: "变动后余额", width: 150, align: "right" },
  { key: "note", title: "备注", wrap: true },
  { key: "createdAt", title: "发生时间", width: 170 }
];

const usageColumns: DsTableColumn[] = [
  { key: "user", title: "用户", width: 150 },
  { key: "model", title: "模型" },
  { key: "source", title: "来源", width: 110 },
  { key: "charged", title: "用户扣款", width: 120, align: "right" },
  { key: "payable", title: "平台成本", width: 120, align: "right" },
  { key: "createdAt", title: "时间", width: 145 }
];

function goTo(path: string) {
  void router.push(path);
}
</script>

<template>
  <section class="finance-grid finance-grid--activity" aria-label="近期用户消费与账务流水">
    <article class="finance-card activity-card">
      <header class="finance-card__head">
        <div class="finance-card__title-wrap">
          <span class="finance-card__icon finance-card__icon--warning"><ShoppingBag :size="18" /></span>
          <div>
            <h2>最近用户消费</h2>
            <p>{{ rangeLabel }}最新成功 AI 请求，方便快速核对计费结果。</p>
          </div>
        </div>
        <button type="button" class="text-action" @click="goTo('/tenant/ai/usage')">查看全部 <ArrowUpRight :size="13" /></button>
      </header>
      <div class="finance-table-wrap">
        <DsTable
          :columns="usageColumns"
          :rows="recentUsageRows"
          row-key="id"
          :loading="dashboard.recentLoading.value"
          empty-title="暂无成功消费记录"
          empty-description="当前时间范围内没有可核对的用户 AI 消费。"
          aria-label="最近用户消费"
        >
          <template #cell-user="{ row }"><strong>{{ row.user }}</strong></template>
          <template #cell-source="{ row }"><DsTag tone="neutral">{{ row.source }}</DsTag></template>
          <template #cell-charged="{ row }"><strong class="amount-positive">{{ formatUSD(row.chargedUsd) }}</strong></template>
          <template #cell-payable="{ row }"><strong class="amount-negative">{{ formatUSD(row.payableUsd) }}</strong></template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
        </DsTable>
      </div>
    </article>

    <article class="finance-card activity-card">
      <header class="finance-card__head">
        <div class="finance-card__title-wrap">
          <span class="finance-card__icon finance-card__icon--info"><Landmark :size="18" /></span>
          <div>
            <h2>最近账务动作</h2>
            <p>充值、服务消费、余额调整和提现都在这里留痕。</p>
          </div>
        </div>
        <button type="button" class="text-action" @click="goTo('/tenant/account?tab=ledger')">完整流水 <ArrowUpRight :size="13" /></button>
      </header>
      <div class="finance-table-wrap">
        <DsTable
          :columns="ledgerColumns"
          :rows="dashboard.balanceLedger.value"
          row-key="txnId"
          :loading="dashboard.ledgerLoading.value"
          empty-title="暂无账务动作"
          empty-description="充值或产生服务消费后，账务动作会记录在这里。"
          aria-label="最近账务动作"
        >
          <template #cell-txnType="{ row }"><strong>{{ ledgerType(row) }}</strong></template>
          <template #cell-amount="{ row }"><strong :class="ledgerAmountClass(row.amountMicroUsd)">{{ ledgerAmount(row.amountMicroUsd) }}</strong></template>
          <template #cell-balanceAfter="{ row }">{{ formatMicroUSD(row.balanceAfterMicroUsd) }}</template>
          <template #cell-note="{ row }"><span class="table-note">{{ ledgerNote(row) }}</span></template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
        </DsTable>
      </div>
    </article>
  </section>
</template>
