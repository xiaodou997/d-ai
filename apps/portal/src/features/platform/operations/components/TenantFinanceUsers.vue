<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { ArrowUpRight, UsersRound } from "lucide-vue-next";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import type { TenantOperationsDashboard } from "../composables/useTenantOperationsDashboard";
import type { TenantFinanceSummary } from "../composables/useTenantFinanceSummary";
import { type FinanceUserRow, buildUserRow, userStatusWeight, formatUSD, formatCount } from "../financePresentation";

const props = defineProps<{
  dashboard: Pick<TenantOperationsDashboard, "consumptionRanking" | "rankingLoading" | "users" | "usersLoading">;
  finance: Pick<TenantFinanceSummary, "lowBalanceCount" | "balance">;
}>();
const dashboard = props.dashboard;
const { lowBalanceCount, balance } = props.finance;

const router = useRouter();

const consumptionByUser = computed(
  () => new Map(dashboard.consumptionRanking.value.map((item) => [String(item.userId), item]))
);

const userHealthRows = computed<FinanceUserRow[]>(() => {
  const rows = dashboard.users.value.map((user) => {
    const consumption = consumptionByUser.value.get(String(user.userId));
    return buildUserRow({
      userId: String(user.userId),
      username: user.username || user.nickname || user.email || String(user.userId),
      balanceUsd: Number.isFinite(Number(user.balanceUsd)) ? Number(user.balanceUsd) : null,
      consumptionUsd: Number(consumption?.amountUsd) || 0,
      transactionCount: Number(consumption?.transactionCount) || 0,
      percentage: consumption?.percentage || "0"
    });
  });

  const knownIds = new Set(rows.map((row) => row.userId));
  for (const item of dashboard.consumptionRanking.value) {
    if (knownIds.has(String(item.userId))) continue;
    rows.push(
      buildUserRow({
        userId: String(item.userId),
        username: item.username || String(item.userId),
        balanceUsd: null,
        consumptionUsd: Number(item.amountUsd) || 0,
        transactionCount: Number(item.transactionCount) || 0,
        percentage: item.percentage || "0"
      })
    );
  }

  return rows
    .sort(
      (left, right) =>
        userStatusWeight(right) - userStatusWeight(left) || right.consumptionUsd - left.consumptionUsd
    )
    .slice(0, 8);
});

const userColumns: DsTableColumn[] = [
  { key: "user", title: "用户", width: 180 },
  { key: "status", title: "余额状态", width: 120 },
  { key: "balance", title: "当前余额", width: 120, align: "right" },
  { key: "consumption", title: "本期消费", width: 120, align: "right" },
  { key: "transactions", title: "消费次数", width: 100, align: "right" },
  { key: "percentage", title: "消费占比", width: 100, align: "right" }
];

function goTo(path: string) {
  void router.push(path);
}
</script>

<template>
  <article class="finance-card user-health-card">
    <header class="finance-card__head">
      <div class="finance-card__title-wrap">
        <span class="finance-card__icon finance-card__icon--warning"><UsersRound :size="18" /></span>
        <div>
          <h2>用户资金健康</h2>
          <p>展示部分用户与本期高消耗用户；低余额人数按全量启用用户统计（余额 ≤ $1）。</p>
        </div>
      </div>
      <div class="finance-card__head-meta">
        <DsTag tone="warning">低余额 {{ formatCount(lowBalanceCount) }}</DsTag>
        <button type="button" class="text-action" @click="goTo('/tenant/users/directory')">用户管理 <ArrowUpRight :size="13" /></button>
      </div>
    </header>
    <div class="finance-table-wrap">
      <DsTable
        :columns="userColumns"
        :rows="userHealthRows"
        row-key="userId"
        :loading="dashboard.usersLoading.value || dashboard.rankingLoading.value"
        empty-title="暂无用户资金数据"
        empty-description="创建终端用户并产生充值或消费后，这里会显示资金健康状态。"
        aria-label="用户资金健康"
      >
        <template #cell-user="{ row }">
          <div class="table-primary">
            <strong>{{ row.username }}</strong>
            <small>{{ row.userId }}</small>
          </div>
        </template>
        <template #cell-status="{ row }"><DsTag :tone="row.statusTone">{{ row.statusLabel }}</DsTag></template>
        <template #cell-balance="{ row }"><strong>{{ row.balanceUsd === null ? "—" : formatUSD(row.balanceUsd) }}</strong></template>
        <template #cell-consumption="{ row }"><strong class="amount-positive">{{ formatUSD(row.consumptionUsd) }}</strong></template>
        <template #cell-transactions="{ row }">{{ formatCount(row.transactionCount) }}</template>
        <template #cell-percentage="{ row }">{{ row.percentage }}%</template>
      </DsTable>
    </div>
  </article>
</template>
