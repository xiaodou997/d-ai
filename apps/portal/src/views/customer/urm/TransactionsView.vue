<!--
  积分明细 — 查看积分扣费与充值流水。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       GuideHelpLink 收进 #actions);el-table → DsTable(:frame="false",流水号 mono、
       扣减积分右对齐),el-tag → DsTag;DsPagination 始终渲染(去掉 total>0 才显示)。
       业务逻辑与请求参数保持不变。
-->
<template>
  <div class="page-container transactions-page">
    <PortalPagePanel
      fill
      :icon="ArrowLeftRight"
      :breadcrumbs="[{ label: '用户中心' }, { label: '充值与明细' }, { label: '积分明细' }]"
      description="查看我的积分扣费与充值明细"
    >
      <template #actions>
        <GuideHelpLink to="/help/account/transactions" />
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="list"
        row-key="eventId"
        :loading="loading"
      >
        <template #cell-userCredits="{ row }">
          <span class="transactions-deduction">-{{ (row.userCredits || 0).toLocaleString() }}</span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
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
import { ArrowLeftRight } from "lucide-vue-next";
import { PortalPagePanel } from "@dai/app-core";
import { GuideHelpLink } from "@dai/app-core/guide";
import { DsPagination, DsTable, DsTag, type DsTableColumn } from "@dai/ui";
import { urmCustomerApi } from "../../api/urmCustomer";
import type { AccountTransactionItem } from "../../types/urmCustomer";

const columns: DsTableColumn[] = [
  { key: "eventId", title: "流水 ID", mono: true },
  { key: "userCredits", title: "扣减积分", width: 140, align: "right" },
  { key: "description", title: "描述" },
  { key: "status", title: "状态", width: 100 },
  { key: "createdTime", title: "时间", width: 180 }
];

const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const loading = ref(false);
const list = ref<AccountTransactionItem[]>([]);

function formatTime(ts?: number | string | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

function statusTone(status: string): "positive" | "warning" | "danger" | "neutral" {
  const map: Record<string, "positive" | "warning" | "danger" | "neutral"> = {
    succeeded: "positive",
    pending: "warning",
    cancelled: "neutral",
    refunded: "neutral",
    released: "danger"
  };
  return map[status] || "neutral";
}

function statusText(status: string) {
  const map: Record<string, string> = {
    succeeded: "成功",
    pending: "进行中",
    cancelled: "取消",
    refunded: "已退款",
    released: "已释放"
  };
  return map[status] || "未知";
}

async function fetchList() {
  loading.value = true;
  try {
    const res = await urmCustomerApi.getTransactions({ page: page.value, size: pageSize.value });
    list.value = res?.items ?? [];
    total.value = res?.total ?? list.value.length;
  } catch (e) {
    console.error("获取流水列表失败:", e);
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
.transactions-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.transactions-deduction {
  font-weight: 700;
  color: var(--ds-danger);
}
</style>
