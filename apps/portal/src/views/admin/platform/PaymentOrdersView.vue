<!-- 支付订单 — 在线充值订单列表 + 手动查单同步（mock 模式下即仿真支付成功入口）。
     重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
     筛选/表格/分页同卡）;数据接入 useListPage,请求参数与筛选语义保持不变。 -->
<template>
  <div class="payment-orders-page">
    <PortalPagePanel
      :icon="ShoppingCart"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '支付订单' }]"
      description="查看在线充值订单，mock 模式下可用「同步」按钮仿真支付成功；真实商户号下用于运维兜底查单。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="场景">
            <el-select v-model="query.scene" placeholder="全部场景" clearable class="orders-select">
              <el-option label="用户充值" value="user_topup" />
              <el-option label="租户充值" value="tenant_topup" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="状态">
            <el-select v-model="query.status" placeholder="全部状态" clearable class="orders-select">
              <el-option label="待支付" value="created" />
              <el-option label="支付确认中" value="paying" />
              <el-option label="已到账" value="paid" />
              <el-option label="已关闭" value="closed" />
              <el-option label="已过期" value="expired" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="租户 ID">
            <el-input
              v-model="query.tenantId"
              placeholder="租户 ID"
              clearable
              class="orders-tenant-input"
              @keyup.enter="search"
            />
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="orders-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="orders-button-icon" />
              重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="orderId"
        :loading="loading"
        empty-title="暂无支付订单"
      >
        <template #cell-tenantName="{ row }">
          {{ row.tenantName || "—" }}
        </template>
        <template #cell-username="{ row }">
          {{ row.username || "—" }}
        </template>
        <template #cell-scene="{ row }">
          {{ sceneText(row.scene) }}
        </template>
        <template #cell-topupMode="{ row }">
          {{ topupModeText(row) }}
        </template>
        <template #cell-amount="{ row }">
          <span class="orders-num">${{ (row.paymentAmountMinor / 100).toFixed(2) }}</span>
        </template>
        <template #cell-grossAmount="{ row }">
          <span class="orders-num">{{ formatMicroUSD(row.grossAmountMicroUsd) }}</span>
        </template>
        <template #cell-feeAmount="{ row }">
          <span class="orders-num">{{ formatMicroUSD(row.feeAmountMicroUsd) }}</span>
        </template>
        <template #cell-giftAmount="{ row }"><span class="orders-num">{{ formatMicroUSD(row.giftAmountMicroUsd) }}</span></template>
        <template #cell-creditedAmount="{ row }">
          <span class="orders-num">+{{ formatMicroUSD(row.creditedAmountMicroUsd) }}</span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
        </template>
        <template #cell-transactionId="{ row }">
          {{ row.transactionId || "—" }}
        </template>
        <template #cell-createdAt="{ row }">
          <span class="orders-time">{{ formatTime(row.createdAt) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button v-if="row.status === 'created' || row.status === 'paying'" link type="primary" @click="syncOrder(row.orderId)">
            同步
          </el-button>
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
import { ElMessage } from "element-plus";
import { RefreshRight, Search } from "@element-plus/icons-vue";
import { ShoppingCart } from "lucide-vue-next";
import { PortalPagePanel, useListPage } from "@/platform";
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";
import { platformAdminApi } from "@/api/platformAdmin";
import type { PaymentOrderItem } from "@/api/types/admin";

const columns: DsTableColumn[] = [
  { key: "orderId", title: "订单号", width: 200, mono: true },
  { key: "tenantName", title: "租户名称", width: 150 },
  { key: "username", title: "用户名", width: 130 },
  { key: "scene", title: "充值对象", width: 110 },
  { key: "topupMode", title: "充值方式", width: 150 },
  { key: "amount", title: "支付金额", width: 120, align: "right" },
  { key: "grossAmount", title: "充值金额", width: 130, align: "right" },
  { key: "feeAmount", title: "手续费", width: 120, align: "right" },
  { key: "giftAmount", title: "赠送", width: 120, align: "right" },
  { key: "creditedAmount", title: "到账", width: 130, align: "right" },
  { key: "status", title: "状态", width: 120 },
  { key: "transactionId", title: "微信交易号", width: 180, mono: true },
  { key: "createdAt", title: "创建时间", width: 180 },
  { key: "actions", title: "操作", width: 100 }
];

const {
  rows,
  total,
  loading,
  page,
  pageSize,
  query,
  refresh,
  search,
  resetQuery,
  handlePageChange,
  handlePageSizeChange
} = useListPage<{ scene: string; status: string; tenantId: string }, PaymentOrderItem>({
  initialQuery: { scene: "", status: "", tenantId: "" },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const res = await platformAdminApi.listPaymentOrders({
        scene: params.scene || undefined,
        status: params.status || undefined,
        tenantId: params.tenantId || undefined,
        page: params.page,
        size: params.pageSize
      });
      return { items: res.items || [], total: res.total || 0 };
    } catch (error) {
      ElMessage.error("获取列表失败");
      throw error;
    }
  }
});

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}
function formatMicroUSD(value: number) { return `$${(Number(value ?? 0) / 1_000_000).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`; }

type StatusTone = "positive" | "warning" | "danger" | "info" | "neutral";

function statusTone(status: string): StatusTone {
  const map: Record<string, StatusTone> = {
    paid: "positive",
    created: "warning",
    paying: "warning",
    closed: "info",
    expired: "danger"
  };
  return map[status] || "neutral";
}

function statusText(status: string) {
  const map: Record<string, string> = {
    paid: "已到账",
    created: "待支付",
    paying: "支付确认中",
    closed: "已关闭",
    expired: "已过期"
  };
  return map[status] || status;
}

function sceneText(scene?: string) {
  const map: Record<string, string> = {
    user_topup: "用户充值",
    tenant_topup: "租户充值"
  };
  return scene ? map[scene] || scene : "—";
}

function topupModeText(row: PaymentOrderItem) {
  if (row.topupMode === "package") return row.packageName || "快捷套餐";
  return "自定义金额";
}

async function syncOrder(orderId: string) {
  try {
    const result = await platformAdminApi.syncPaymentOrder(orderId);
    ElMessage.success(`同步完成，当前状态：${statusText(result.status)}`);
    refresh();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "同步失败");
  }
}
</script>

<style scoped>
.payment-orders-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.orders-select {
  width: 160px;
}

.orders-tenant-input {
  width: 160px;
}

.orders-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.orders-num {
  font-variant-numeric: tabular-nums;
}

.orders-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
