<script setup lang="ts">
import { FileClock } from "lucide-vue-next";
import { DsTable, DsTabs, DsTag, type DsTableColumn } from "@/shared/ui";
import type { TenantBalanceLedgerItem, TenantTopupOrderItem } from "@/api/types/tenant";
import type { RechargeRecordItem } from "@/api/types/platformTenant";
import { isOnlineRecharge, rechargeMethodText } from "@/utils/recharge";
import {
  balanceSourceText, balanceStatusText, balanceTransactionText, formatMicroUSD, formatTime,
  formatUSD,
  type AccountCenterPage, type AccountCenterTab
} from "../model";

type DsTagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";
defineProps<{
  activeTab: AccountCenterTab;
  rechargeRecords: AccountCenterPage<RechargeRecordItem>;
  pendingOrders: AccountCenterPage<TenantTopupOrderItem>;
  balanceLedger: AccountCenterPage<TenantBalanceLedgerItem>;
  loading: { recharges: boolean; pendingOrders: boolean; balanceLedger: boolean };
}>();
const emit = defineEmits<{ tab: [value: AccountCenterTab] }>();
const tabs = [
  { key: "ledger", label: "额度明细" },
  { key: "recharges", label: "充值记录" }
];
const pendingOrderColumns: DsTableColumn[] = [
  { key: "order", title: "订单" }, { key: "paid", title: "支付金额", width: 130, align: "right" },
  { key: "credited", title: "预计到账", width: 130, align: "right" }, { key: "status", title: "状态", width: 110 },
  { key: "createdAt", title: "创建时间", width: 180 }
];
const rechargeColumns: DsTableColumn[] = [
  { key: "source", title: "来源" }, { key: "rechargeMethod", title: "充值方式", width: 110 },
  { key: "paidAmount", title: "支付金额", width: 125, align: "right" },
  { key: "amount", title: "到账金额", width: 130, align: "right" }, { key: "status", title: "状态", width: 100 },
  { key: "note", title: "备注" }, { key: "createdTime", title: "时间", width: 180 }
];
const ledgerColumns: DsTableColumn[] = [
  { key: "txnType", title: "类型" }, { key: "amount", title: "变动金额", width: 140, align: "right" },
  { key: "balanceAfter", title: "变动后余额", width: 150, align: "right" }, { key: "note", title: "备注" },
  { key: "createdAt", title: "时间", width: 180 }
];
function statusTone(status: string): DsTagTone {
  if (status === "active" || status === "paid") return "positive";
  if (["pending", "approved", "created", "paying"].includes(status)) return "warning";
  if (["reversed", "rejected", "expired", "closed"].includes(status)) return "danger";
  return "info";
}
</script>

<template>
  <div class="account-records">
    <div class="account-records__tabs"><DsTabs :tabs="tabs" :model-value="activeTab" @update:model-value="emit('tab', $event as AccountCenterTab)" /></div>

    <div v-if="activeTab === 'recharges'" class="records-panel">
      <div v-if="pendingOrders.items.length" class="pending-orders">
        <div class="pending-orders__heading"><FileClock :size="16" /><strong>待支付订单</strong><span>支付完成后 USD 额度会自动到账</span></div>
        <DsTable :frame="false" :columns="pendingOrderColumns" :rows="pendingOrders.items" row-key="orderId" :loading="loading.pendingOrders" empty-title="暂无待支付订单">
          <template #cell-order="{ row }">{{ row.packageName || (row.topupMode === "package" ? "额度包" : "自定义充值") }}</template>
          <template #cell-paid="{ row }">${{ (row.paymentAmountMinor / 100).toFixed(2) }}</template>
          <template #cell-credited="{ row }">{{ formatMicroUSD(row.creditedAmountMicroUsd) }}</template>
          <template #cell-status="{ row }"><DsTag :tone="statusTone(row.status)">{{ row.status === "paying" ? "确认中" : "待支付" }}</DsTag></template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
        </DsTable>
      </div>
      <div class="records-panel__head"><span class="records-panel__title">额度充值记录</span><span class="records-panel__hint">包含平台发放、在线充值和用户充值收入</span></div>
      <div class="records-panel__table">
        <DsTable :frame="false" :columns="rechargeColumns" :rows="rechargeRecords.items" row-key="orderId" :loading="loading.recharges" empty-title="暂无充值记录">
          <template #cell-source="{ row }">{{ balanceSourceText(row.orderType) }}</template>
          <template #cell-rechargeMethod="{ row }"><DsTag :tone="isOnlineRecharge(row.orderType) ? 'accent' : 'neutral'">{{ rechargeMethodText(row.orderType) }}</DsTag></template>
          <template #cell-paidAmount="{ row }">{{ row.paidAmountMinor ? `$${(row.paidAmountMinor / 100).toFixed(2)}` : "—" }}</template>
          <template #cell-amount="{ row }"><span class="amount-positive">+{{ formatUSD(row.amountUsd) }}</span></template>
          <template #cell-status="{ row }"><DsTag :tone="statusTone(row.status)">{{ balanceStatusText(row.status) }}</DsTag></template>
          <template #cell-createdTime="{ row }">{{ formatTime(row.createdTime) }}</template>
        </DsTable>
      </div>
    </div>

    <div v-else-if="activeTab === 'ledger'" class="records-panel">
      <div class="records-panel__head"><span class="records-panel__title">额度明细</span><span class="records-panel__hint">充值、账户调整和管理员提现扣减记录在这里；AI 服务消费请查看使用记录</span></div>
      <div class="records-panel__table">
        <DsTable :frame="false" :columns="ledgerColumns" :rows="balanceLedger.items" row-key="txnId" :loading="loading.balanceLedger" empty-title="暂无额度明细">
          <template #cell-txnType="{ row }">{{ balanceTransactionText(row.txnType) }}</template>
          <template #cell-amount="{ row }"><span :class="row.amountMicroUsd >= 0 ? 'amount-positive' : 'amount-negative'">{{ row.amountMicroUsd >= 0 ? "+" : "-" }}{{ formatMicroUSD(Math.abs(row.amountMicroUsd)) }}</span></template>
          <template #cell-balanceAfter="{ row }">{{ formatMicroUSD(row.balanceAfterMicroUsd) }}</template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
        </DsTable>
      </div>
    </div>

  </div>
</template>

<style scoped>
.account-records { display:flex; min-width:0; flex:1; min-height:0; flex-direction:column; }.account-records__tabs { padding:16px 24px 14px; border-bottom:1px solid var(--ds-line); }
.records-panel { display:flex; min-width:0; flex:1; min-height:0; flex-direction:column; }.records-panel__head { display:flex; align-items:baseline; gap:10px; padding:14px 24px; border-bottom:1px solid var(--ds-line); }.records-panel__title { color:var(--ds-ink); font-size:13.5px; font-weight:700; }.records-panel__hint { color:var(--ds-muted); font-size:12px; }.records-panel__table { flex:1; min-height:0; overflow:auto; }
.pending-orders { border-bottom:1px solid var(--ds-line); background:var(--ds-warning-soft); }.pending-orders__heading { display:flex; align-items:center; gap:7px; padding:12px 24px 4px; color:var(--ds-warning); font-size:12px; }.pending-orders__heading span { color:var(--ds-muted); }
.amount-positive { color:var(--ds-positive); font-weight:700; }.amount-negative { color:var(--ds-danger); font-weight:700; }
@media (max-width:768px) { .account-records__tabs,.records-panel__head,.pending-orders__heading { padding-inline:16px; }.records-panel__hint { display:none; } }
</style>
