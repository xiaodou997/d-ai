<!--
  充值记录 — 查看历史充值明细。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       el-table → DsTable(:frame="false",单号 mono、
       支付/到账金额右对齐),el-tag → DsTag;DsPagination 始终渲染(去掉 total>0 才显示)。
       业务逻辑与请求参数保持不变。
-->
<template>
  <div class="page-container recharge-page">
    <PortalPagePanel
      fill
      :icon="ReceiptText"
      :breadcrumbs="[{ label: '用户中心' }, { label: '充值与明细' }, { label: '充值记录' }]"
      description="查看我的历史充值明细"
    >
      <DsTable
        :frame="false"
        :columns="columns"
        :rows="list"
        row-key="orderId"
        :loading="loading"
      >
        <template #cell-paidAmount="{ row }">
          <span class="recharge-amount">${{ ((row.paidAmountMinor || 0) / 100).toFixed(2) }}</span>
        </template>
        <template #cell-amount="{ row }">
          <span class="recharge-credits">+${{ (row.amountUsd || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 }) }}</span>
        </template>
        <template #cell-rechargeMethod="{ row }">
          <DsTag :tone="isOnlineRecharge(row.orderType) ? 'accent' : 'neutral'">{{ rechargeMethodText(row.orderType) }}</DsTag>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="rechargeStatusTone(row.status)">
            {{ rechargeStatusText(row.status) }}
          </DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          {{ formatTime(row.createdTime) }}
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handleSizeChange"
        />
      </template>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from "vue";
import { ReceiptText } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { platformCustomerApi } from "@/api/platformCustomer";
import type { RechargeRecordItem } from "@/api/types/platformCustomer";
import { isOnlineRecharge, rechargeMethodText } from "@/utils/recharge";

const columns: DsTableColumn[] = [
  { key: "orderId", title: "充值单号", width: 200, mono: true },
  { key: "paidAmount", title: "实付金额（USD）", width: 150, align: "right" },
  { key: "amount", title: "到账金额（USD）", width: 160, align: "right" },
  { key: "rechargeMethod", title: "充值方式", width: 110, align: "center" },
  { key: "status", title: "状态", width: 110 },
  { key: "note", title: "备注" },
  { key: "createdTime", title: "充值时间", width: 180 }
];

const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const loading = ref(false);
const list = ref<RechargeRecordItem[]>([]);

function formatTime(ts?: number | string | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

function rechargeStatusTone(status: string): "positive" | "warning" | "danger" | "neutral" {
  const map: Record<string, "positive" | "warning" | "danger" | "neutral"> = {
    SUCCESS: "positive",
    PENDING: "warning",
    FAILED: "danger",
    "1": "positive",
    "0": "warning",
    "-1": "danger"
  };
  return map[status] || "neutral";
}

function rechargeStatusText(status: string) {
  const map: Record<string, string> = {
    SUCCESS: "成功",
    PENDING: "处理中",
    FAILED: "失败",
    "1": "成功",
    "0": "处理中",
    "-1": "失败"
  };
  return map[status] || String(status);
}

async function fetchList() {
  loading.value = true;
  try {
    const res = await platformCustomerApi.getRechargeRecords({ page: page.value, size: pageSize.value });
    list.value = res?.items ?? [];
    total.value = res?.total ?? list.value.length;
  } catch (e) {
    console.error("获取充值记录失败:", e);
    list.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

function handlePageChange(value: number) {
  page.value = value;
  fetchList();
}

function handleSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
  fetchList();
}

onMounted(() => {
  fetchList();
});
</script>

<style scoped>
.recharge-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.recharge-amount {
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.recharge-credits {
  font-weight: 700;
  color: var(--ds-positive);
}
</style>
