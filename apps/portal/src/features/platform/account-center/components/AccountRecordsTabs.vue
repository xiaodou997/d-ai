<!--
  租户端账户中心 — 账户记录 Tab 工作区(积分记录/余额明细/提现记录)。
  重构:迁移至新设计系统,el-tabs → DsTabs、el-table → DsTable(:frame="false"),
       el-tag → DsTag(success 映射为 positive tone);
  二次重构:与密钥/应用管理页对齐 —— Tab 条与说明行各自带 24px 容器,表格通栏并撑满
       剩余高度内部滚动,分页不再跟在表格后面,改由外层面板的 #pagination 脚沉底。
-->
<script setup lang="ts">
import { FileClock } from "lucide-vue-next";
import { DsTable, DsTabs, DsTag, type DsTableColumn } from "@/shared/ui";

import type { TenantCashLedgerItem, TenantWithdrawal } from "@/api/types/tenant";
import type { RechargeRecordItem } from "@/api/types/platformTenant";
import {
  cashTransactionText,
  creditSourceText,
  creditStatusText,
  formatCents,
  formatCredits,
  formatTime,
  maskAccount,
  withdrawalStatusText,
  withdrawalStatusTone,
  type AccountCenterPage,
  type AccountCenterTab
} from "../model";

type PendingOrder = {
  orderId: string;
  status: string;
  amount: number;
  creditAmount: number;
  topupMode: "custom" | "package";
  packageName?: string;
  createdAt: number;
};

type DsTagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";

defineProps<{
  activeTab: AccountCenterTab;
  pointRecords: AccountCenterPage<RechargeRecordItem>;
  pendingOrders: AccountCenterPage<PendingOrder>;
  cashLedger: AccountCenterPage<TenantCashLedgerItem>;
  withdrawals: AccountCenterPage<TenantWithdrawal>;
  loading: { points: boolean; pendingOrders: boolean; cashLedger: boolean; withdrawals: boolean };
}>();

// 分页由外层面板的 #pagination 脚统一渲染,此处不再 emit page
const emit = defineEmits<{
  tab: [value: AccountCenterTab];
  cancelWithdrawal: [id: string];
}>();

// 条数不在 Tab 上重复,底部分页脚已有「共 N 条」
const tabs = [
  { key: "points", label: "积分记录" },
  { key: "balance", label: "余额明细" },
  { key: "withdrawals", label: "提现记录" }
];

const pendingOrderColumns: DsTableColumn[] = [
  { key: "order", title: "订单" },
  { key: "amount", title: "金额", width: 120, align: "right" },
  { key: "status", title: "状态", width: 110 },
  { key: "createdAt", title: "创建时间", width: 180 }
];

const pointColumns: DsTableColumn[] = [
  { key: "source", title: "来源" },
  { key: "paidAmount", title: "支付金额", width: 125, align: "right" },
  { key: "creditAmount", title: "到账积分", width: 130, align: "right" },
  { key: "status", title: "状态", width: 100 },
  { key: "note", title: "备注" },
  { key: "createdTime", title: "时间", width: 180 }
];

const cashColumns: DsTableColumn[] = [
  { key: "txnType", title: "类型" },
  { key: "amount", title: "变动金额", width: 130, align: "right" },
  { key: "balanceAfter", title: "变动后余额", width: 140, align: "right" },
  { key: "note", title: "备注" },
  { key: "createdAt", title: "时间", width: 180 }
];

const withdrawalColumns: DsTableColumn[] = [
  { key: "amount", title: "申请金额", width: 125, align: "right" },
  { key: "payoutAmount", title: "实际到账", width: 125, align: "right" },
  { key: "accountNo", title: "收款账号", mono: true },
  { key: "status", title: "状态", width: 130 },
  { key: "createdAt", title: "申请时间", width: 180 },
  { key: "actions", title: "操作", width: 90 }
];

function statusTone(status: string): DsTagTone {
  if (status === "active" || status === "paid") return "positive";
  if (status === "pending" || status === "approved" || status === "created" || status === "paying") return "warning";
  if (status === "reversed" || status === "rejected" || status === "expired") return "danger";
  return "info";
}

// model 里的 withdrawalStatusTone 仍按 el-tag 命名(success/warning/danger/info),映射为 DsTag tone
function withdrawalTone(status: string): DsTagTone {
  const tone = withdrawalStatusTone(status);
  return tone === "success" ? "positive" : tone;
}
</script>

<template>
  <div class="account-records">
    <div class="account-records__tabs">
      <DsTabs
        :tabs="tabs"
        :model-value="activeTab"
        @update:model-value="emit('tab', $event as AccountCenterTab)"
      />
    </div>

    <!-- 积分记录:待处理购买 + 积分到账记录 -->
    <div v-if="activeTab === 'points'" class="records-panel">
      <div v-if="pendingOrders.items.length" class="pending-orders">
        <div class="pending-orders__heading">
          <FileClock :size="16" /><strong>待处理购买</strong><span>完成微信支付后积分会自动到账</span>
        </div>
        <DsTable
          :frame="false"
          :columns="pendingOrderColumns"
          :rows="pendingOrders.items"
          row-key="orderId"
          :loading="loading.pendingOrders"
          empty-title="暂无待处理订单"
        >
          <template #cell-order="{ row }">
            {{ row.packageName || (row.topupMode === "package" ? "积分套餐" : "自定义金额") }}
          </template>
          <template #cell-amount="{ row }">¥{{ formatCents(row.amount) }}</template>
          <template #cell-status="{ row }">
            <DsTag :tone="statusTone(row.status)">{{ row.status === "paying" ? "确认中" : "待支付" }}</DsTag>
          </template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
        </DsTable>
      </div>

      <div class="records-panel__head">
        <span class="records-panel__title">积分到账记录</span>
        <span class="records-panel__hint">包含平台发放、微信购买和余额购买</span>
      </div>
      <div class="records-panel__table">
        <DsTable
          :frame="false"
          :columns="pointColumns"
          :rows="pointRecords.items"
          row-key="orderId"
          :loading="loading.points"
          empty-title="暂无积分记录"
        >
          <template #cell-source="{ row }">{{ creditSourceText(row.orderType) }}</template>
          <template #cell-paidAmount="{ row }">{{ row.paidAmount ? `¥${formatCents(row.paidAmount)}` : "—" }}</template>
          <template #cell-creditAmount="{ row }">
            <span class="amount-positive">+{{ formatCredits(row.creditAmount) }}</span>
          </template>
          <template #cell-status="{ row }">
            <DsTag :tone="statusTone(row.status)">{{ creditStatusText(row.status) }}</DsTag>
          </template>
          <template #cell-createdTime="{ row }">{{ formatTime(row.createdTime) }}</template>
        </DsTable>
      </div>
    </div>

    <!-- 余额明细 -->
    <div v-else-if="activeTab === 'balance'" class="records-panel">
      <div class="records-panel__head">
        <span class="records-panel__title">余额明细</span>
        <span class="records-panel__hint">用户充值到账、购买积分和提现都会记录在这里</span>
      </div>
      <div class="records-panel__table">
        <DsTable
          :frame="false"
          :columns="cashColumns"
          :rows="cashLedger.items"
          row-key="txnId"
          :loading="loading.cashLedger"
          empty-title="暂无余额明细"
        >
          <template #cell-txnType="{ row }">{{ cashTransactionText(row.txnType) }}</template>
          <template #cell-amount="{ row }">
            <span :class="row.amount >= 0 ? 'amount-positive' : 'amount-negative'">
              {{ row.amount >= 0 ? "+" : "-" }}¥{{ formatCents(Math.abs(row.amount)) }}
            </span>
          </template>
          <template #cell-balanceAfter="{ row }">¥{{ formatCents(row.balanceAfter) }}</template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
        </DsTable>
      </div>
    </div>

    <!-- 提现记录 -->
    <div v-else class="records-panel">
      <div class="records-panel__head">
        <span class="records-panel__title">提现记录</span>
        <span class="records-panel__hint">提现申请会先冻结金额，审核通过后打款</span>
      </div>
      <div class="records-panel__table">
        <DsTable
          :frame="false"
          :columns="withdrawalColumns"
          :rows="withdrawals.items"
          row-key="withdrawalId"
          :loading="loading.withdrawals"
          empty-title="暂无提现记录"
        >
          <template #cell-amount="{ row }">¥{{ formatCents(row.amount) }}</template>
          <template #cell-payoutAmount="{ row }">¥{{ formatCents(row.payoutAmount) }}</template>
          <template #cell-accountNo="{ row }">{{ maskAccount(row.accountNo) }}</template>
          <template #cell-status="{ row }">
            <DsTag :tone="withdrawalTone(row.status)">{{ withdrawalStatusText(row.status) }}</DsTag>
          </template>
          <template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
          <template #cell-actions="{ row }">
            <el-button
              v-if="row.status === 'pending'"
              type="danger"
              link
              size="small"
              @click="emit('cancelWithdrawal', row.withdrawalId)"
            >
              取消
            </el-button>
          </template>
        </DsTable>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 通栏结构:Tab 条/说明行各自带 24px 容器,表格铺满并内部滚动(分页在面板脚) */
.account-records {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

/* 分隔线画在容器上:容器是面板全宽的 block,border 才真正通栏(DsTabs 自身不再带线) */
.account-records__tabs {
  padding: 16px 24px 14px;
  border-bottom: 1px solid var(--ds-line);
}

.records-panel {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.records-panel__head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 14px 24px;
  border-bottom: 1px solid var(--ds-line);
}

.records-panel__title {
  color: var(--ds-ink);
  font-size: 13.5px;
  font-weight: 700;
}

.records-panel__hint {
  color: var(--ds-muted);
  font-size: 12px;
}

.records-panel__table {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.records-panel__table :deep(.ds-table) {
  display: flex;
  min-height: 100%;
  flex-direction: column;
}

.records-panel__table :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
}

.pending-orders {
  flex: 0 0 auto;
  margin: 14px 24px 0;
  border: 1px solid color-mix(in srgb, var(--ds-warning) 30%, var(--ds-line));
  border-radius: 8px;
  overflow: hidden;
}

.pending-orders__heading {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 12px 14px;
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
  font-size: 12px;
}

.pending-orders__heading span {
  color: var(--ds-muted);
}

.amount-positive {
  color: var(--ds-positive);
  font-weight: 750;
}

.amount-negative {
  color: var(--ds-danger);
  font-weight: 750;
}

@media (max-width: 768px) {
  .account-records__tabs,
  .records-panel__head {
    padding-inline: 16px;
  }

  .pending-orders {
    margin-inline: 16px;
  }
}

@media (max-width: 700px) {
  .pending-orders__heading {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .pending-orders__heading span {
    width: 100%;
    margin-left: 23px;
  }
}
</style>
