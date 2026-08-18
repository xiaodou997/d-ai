<!--
  租户余额总览 — 管理员可查看任意租户的统一 USD 余额与流水。
  重构：迁移至新设计系统一体面板（PortalPagePanel：图标徽章+面包屑标题+描述同行，
       筛选/表格/分页同卡）；数据接入 useListPage，分页始终渲染并支持切换页大小。
       listCashAccounts 仅支持 page/size 参数（无筛选字段），故筛选栏只保留 查询/重置 操作；
       流水弹窗仍为 element-plus 弹壳，弹内表格已统一为 DsTable。
-->
<template>
  <div class="cash-accounts-page">
    <PortalPagePanel
      :icon="Landmark"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '租户余额' }]"
      description="充值、服务消费、用户充值收入和管理员提现扣减均归集到租户统一额度。"
    >
      <template #filters>
        <DsFilterBar>
          <!-- 账户余额列表接口无筛选参数，筛选栏仅提供查询/重置（语义等同刷新） -->
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
          <span class="cash-accounts-num">{{ formatMicroUSD(row.balanceMicroUsd) }}</span>
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

    <el-dialog v-model="ledgerVisible" :title="`余额流水 · ${activeTenantId}`" width="720px" append-to-body>
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
          <span :class="row.amountMicroUsd >= 0 ? 'amount-positive' : 'amount-negative'">
            {{ row.amountMicroUsd >= 0 ? "+" : "" }}{{ formatMicroUSD(row.amountMicroUsd) }}
          </span>
        </template>
        <template #cell-balanceAfter="{ row }">
          <span class="cash-accounts-num">{{ formatMicroUSD(row.balanceAfterMicroUsd) }}</span>
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
import type { BalanceLedgerItem, TenantBalanceItem } from "@/api/types/admin";

const columns: DsTableColumn[] = [
  { key: "tenantId", title: "租户 ID", width: 200, mono: true },
  { key: "tenantName", title: "租户名称" },
  { key: "balance", title: "余额", align: "right" },
  { key: "actions", title: "操作", width: 100 }
];

// 流水弹窗固定取前 50 条(API 限制,不做翻页)
const ledgerColumns: DsTableColumn[] = [
  { key: "txnType", title: "类型", width: 120 },
  { key: "amount", title: "金额", width: 130, align: "right" },
  { key: "balanceAfter", title: "变动后余额", width: 160, align: "right" },
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
} = useListPage<Record<string, never>, TenantBalanceItem>({
  initialQuery: {},
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const res = await platformAdminApi.listTenantBalances({ page: params.page, size: params.pageSize });
      return { items: res.items || [], total: res.total || 0 };
    } catch (e) {
      console.error("获取账户余额总览失败:", e);
      throw e;
    }
  }
});

const ledgerVisible = ref(false);
const ledgerLoading = ref(false);
const ledger = ref<BalanceLedgerItem[]>([]);
const activeTenantId = ref("");

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}
function formatMicroUSD(value: number) {
  return `$${(Number(value ?? 0) / 1_000_000).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`;
}

function txnTypeText(type: string) {
  const map: Record<string, string> = {
    topup_income: "在线充值入账",
    refund_reversal: "退款收入冲正",
    consumption: "服务消费",
    withdraw: "提现",
    adjust: "人工调整"
  };
  return map[type] || type;
}

async function openLedger(row: TenantBalanceItem) {
  activeTenantId.value = row.tenantId;
  ledgerVisible.value = true;
  ledgerLoading.value = true;
  try {
    const res = await platformAdminApi.listBalanceLedger({ tenantId: row.tenantId, page: 1, size: 50 });
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
