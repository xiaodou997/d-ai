<!--
  租户端 — 消费记录(交易流水):查看每次服务使用时扣除的积分。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡);el-table → DsTable(mono 单号、金额右对齐、DsTag 状态),
       PortalFilterBar → DsFilterBar/DsFilterField,空态 → DsEmpty;数据接入 useListPage,
       请求参数与筛选语义保持不变;状态点颜色由 DsTag tone 承接(成功=positive、
       进行中=warning、其余=danger)。
-->
<template>
  <div class="page-container transactions-page">
    <PortalPagePanel
      fill
      :icon="ArrowLeftRight"
      :breadcrumbs="[{ label: '租户运营' }, { label: '使用记录' }, { label: '消费记录' }]"
      description="查看每次服务使用时扣除的积分"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="用户名">
            <el-input v-model="query.username" placeholder="搜索用户名" clearable class="transactions-username-input" @keyup.enter="search" />
          </DsFilterField>
          <DsFilterField label="APP 名称">
            <el-input v-model="query.appName" placeholder="搜索 APP 名称" clearable class="transactions-appname-input" @keyup.enter="search" />
          </DsFilterField>
          <DsFilterField label="交易时间">
            <el-date-picker
              v-model="query.dateRange"
              type="daterange"
              range-separator="至"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
              value-format="YYYY-MM-DD"
              class="transactions-date-range"
            />
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="transactions-button-icon" />筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="transactions-button-icon" />重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="eventId"
        :loading="loading"
        empty-title="暂无消费记录"
      >
        <template #empty>
          <DsEmpty title="暂无消费记录" description="还没有服务消费的积分扣除记录" />
        </template>
        <template #cell-username="{ row }">
          <span v-if="row.username">{{ row.username }}</span>
          <span v-else class="transactions-dash">—</span>
        </template>
        <template #cell-appName="{ row }">
          <span v-if="row.appName || row.clientId" class="transactions-app-tag">{{ row.appName || row.clientId }}</span>
          <span v-else class="transactions-dash">—</span>
        </template>
        <template #cell-tenantCredits="{ row }">
          <span v-if="row.tenantCredits" class="transactions-num transactions-num--tenant">{{ (row.tenantCredits || 0).toLocaleString() }}</span>
          <span v-else class="transactions-dash">—</span>
        </template>
        <template #cell-userCredits="{ row }">
          <span v-if="row.userCredits" class="transactions-num transactions-num--user">{{ (row.userCredits || 0).toLocaleString() }}</span>
          <span v-else class="transactions-dash">—</span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="transactions-time">{{ formatTime(row.createdTime) }}</span>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { Search, RefreshRight } from "@element-plus/icons-vue";
import { ArrowLeftRight } from "lucide-vue-next";
import { PortalPagePanel, useListPage } from "@/platform";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";

import { platformTenantApi } from "@/api/platformTenant";
import type { AccountTransactionItem } from "@/api/types/platformTenant";

const columns: DsTableColumn[] = [
  { key: "eventId", title: "交易流水", width: 200, mono: true },
  { key: "username", title: "用户名" },
  { key: "appName", title: "APP 名称" },
  { key: "description", title: "描述" },
  { key: "tenantCredits", title: "我的积分消耗", align: "right" },
  { key: "userCredits", title: "用户积分消耗", align: "right" },
  { key: "status", title: "状态", width: 100 },
  { key: "createdTime", title: "交易时间", width: 170 }
];

// 状态语义沿用原状态点配色:成功=positive、进行中=warning、其余(取消/退款/释放/撤销)=danger
const statusText = (s: string) => {
  const map: Record<string, string> = {
    pending: "进行中",
    succeeded: "成功",
    cancelled: "取消",
    refunded: "已退款",
    released: "已释放",
    reversed: "已撤销"
  };
  return map[s] ?? s ?? "—";
};
const statusTone = (s: string): "positive" | "warning" | "danger" => {
  if (s === "succeeded") return "positive";
  if (s === "pending") return "warning";
  return "danger";
};

interface TransactionsQuery extends Record<string, unknown> {
  username: string;
  appName: string;
  dateRange: string[];
}

const {
  rows,
  total,
  loading,
  page,
  pageSize,
  query,
  search,
  resetQuery,
  handlePageChange,
  handlePageSizeChange
} = useListPage<TransactionsQuery, AccountTransactionItem>({
  initialQuery: { username: "", appName: "", dateRange: [] },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const [start, end] = params.dateRange ?? [];
      const endDate = end ? new Date(`${end}T00:00:00`) : null;
      if (endDate) endDate.setDate(endDate.getDate() + 1);
      const res = await platformTenantApi.getTransactions({
        page: params.page,
        size: params.pageSize,
        username: params.username || undefined,
        clientName: params.appName || undefined,
        timeFrom: start ? new Date(`${start}T00:00:00`).getTime() : undefined,
        timeTo: endDate ? endDate.getTime() : undefined
      });
      return { items: res?.items ?? [], total: res?.total ?? 0 };
    } catch (e) {
      console.error("获取流水列表失败:", e);
      return { items: [], total: 0 };
    }
  }
});

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}
</script>

<style scoped>
.transactions-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.transactions-username-input {
  width: 180px;
}

.transactions-appname-input {
  width: 200px;
}

.transactions-date-range {
  width: 260px;
}

.transactions-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.transactions-dash {
  color: var(--ds-faint);
}

.transactions-app-tag {
  display: inline-flex;
  font-size: 12px;
  font-weight: 600;
  color: var(--ds-accent-hover);
  background: var(--ds-accent-soft);
  padding: 2px 8px;
  border-radius: var(--ds-radius-control);
}

.transactions-num {
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.transactions-num--tenant {
  color: var(--ds-accent-hover);
}

.transactions-num--user {
  color: var(--ds-positive);
}

.transactions-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
