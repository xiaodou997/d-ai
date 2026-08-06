<!--
  现金账户总览 — 租户在线充值净收入现金账户，管理员可查看任意租户的现金流水。
  重构：迁移至新设计系统一体面板（PortalPagePanel：图标徽章+面包屑标题+描述同行，
       筛选/表格/分页同卡）；数据接入 useListPage，分页始终渲染并支持切换页大小。
       listCashAccounts 仅支持 page/size 参数（无筛选字段），故筛选栏只保留 查询/重置 操作；
       流水弹窗仍为 element-plus 弹壳，弹内表格已统一为 DsTable。
-->
<template>
  <div class="cash-accounts-page">
    <PortalPagePanel
      :icon="Landmark"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '现金账户' }]"
      description="用户在线充值扣除平台手续费后的净额进入租户现金账户，可用于租户购买积分或提现。"
    >
      <template #filters>
        <DsFilterBar>
          <!-- 现金账户列表接口无筛选参数，筛选栏仅提供 查询/重置（语义等同刷新） -->
          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="cash-accounts-button-icon" />
              查询
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="cash-accounts-button-icon" />
              重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="tenantId"
        :loading="loading"
        empty-title="暂无数据"
      >
        <template #cell-tenantName="{ row }">
          {{ row.tenantName || "—" }}
        </template>
        <template #cell-balance="{ row }">
          <span class="cash-accounts-num">¥{{ (row.balance / 100).toFixed(2) }}</span>
        </template>
        <template #cell-frozen="{ row }">
          <span class="cash-accounts-num">¥{{ (row.frozen / 100).toFixed(2) }}</span>
        </template>
        <template #cell-available="{ row }">
          <span class="cash-accounts-num">¥{{ (row.available / 100).toFixed(2) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button link type="primary" @click="openLedger(row)">查看流水</el-button>
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

    <el-dialog v-model="ledgerVisible" :title="`现金流水 · ${activeTenantId}`" width="720px" append-to-body>
      <DsTable
        :columns="ledgerColumns"
        :rows="ledger"
        row-key="txnId"
        :loading="ledgerLoading"
        empty-title="暂无数据"
      >
        <template #cell-txnType="{ row }">
          {{ txnTypeText(row.txnType) }}
        </template>
        <template #cell-amount="{ row }">
          <span :class="row.amount >= 0 ? 'amount-positive' : 'amount-negative'">
            {{ row.amount >= 0 ? "+" : "" }}{{ (row.amount / 100).toFixed(2) }}
          </span>
        </template>
        <template #cell-balanceAfter="{ row }">
          <span class="cash-accounts-num">¥{{ (row.balanceAfter / 100).toFixed(2) }}</span>
        </template>
        <template #cell-note="{ row }">
          {{ row.note || "—" }}
        </template>
        <template #cell-createdAt="{ row }">
          {{ formatTime(row.createdAt) }}
        </template>
      </DsTable>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { RefreshRight, Search } from "@element-plus/icons-vue";
import { Landmark } from "lucide-vue-next";
import { PortalPagePanel, useListPage } from "@/platform";
import {
  DsFilterBar,
  DsPagination,
  DsTable,
  type DsTableColumn
} from "@/shared/ui";
import { platformAdminApi } from "@/api/platformAdmin";
import type { CashAccountItem, CashLedgerItem } from "@/api/types/admin";

const columns: DsTableColumn[] = [
  { key: "tenantId", title: "租户 ID", width: 200, mono: true },
  { key: "tenantName", title: "租户名称" },
  { key: "balance", title: "余额（元）", align: "right" },
  { key: "frozen", title: "冻结（元）", align: "right" },
  { key: "available", title: "可用（元）", align: "right" },
  { key: "actions", title: "操作", width: 100 }
];

// 流水弹窗固定取前 50 条(API 限制,不做翻页)
const ledgerColumns: DsTableColumn[] = [
  { key: "txnType", title: "类型", width: 120 },
  { key: "amount", title: "金额（元）", width: 120, align: "right" },
  { key: "balanceAfter", title: "变动后余额（元）", width: 150, align: "right" },
  { key: "note", title: "备注" },
  { key: "createdAt", title: "时间", width: 170 }
];

const {
  rows,
  total,
  loading,
  page,
  pageSize,
  search,
  resetQuery,
  handlePageChange,
  handlePageSizeChange
} = useListPage<Record<string, never>, CashAccountItem>({
  initialQuery: {},
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const res = await platformAdminApi.listCashAccounts({ page: params.page, size: params.pageSize });
      return { items: res.items || [], total: res.total || 0 };
    } catch (e) {
      console.error("获取现金账户总览失败:", e);
      throw e;
    }
  }
});

const ledgerVisible = ref(false);
const ledgerLoading = ref(false);
const ledger = ref<CashLedgerItem[]>([]);
const activeTenantId = ref("");

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

function txnTypeText(type: string) {
  const map: Record<string, string> = {
    topup_income: "在线充值入账",
    buy_credits: "购买积分",
    withdraw: "提现",
    adjust: "人工调整"
  };
  return map[type] || type;
}

async function openLedger(row: CashAccountItem) {
  activeTenantId.value = row.tenantId;
  ledgerVisible.value = true;
  ledgerLoading.value = true;
  try {
    const res = await platformAdminApi.listCashLedger({ tenantId: row.tenantId, page: 1, size: 50 });
    ledger.value = res.items || [];
  } catch (e) {
    console.error("获取现金流水失败:", e);
  } finally {
    ledgerLoading.value = false;
  }
}
</script>

<style scoped>
.cash-accounts-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.cash-accounts-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.cash-accounts-num {
  font-variant-numeric: tabular-nums;
}

.amount-positive {
  color: var(--ds-positive);
  font-weight: 700;
}

.amount-negative {
  color: var(--ds-muted);
  font-weight: 700;
}
</style>
