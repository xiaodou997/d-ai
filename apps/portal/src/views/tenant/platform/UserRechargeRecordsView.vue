<!--
  租户端 — 用户充值明细:查看本租户下终端用户的 USD 余额充值明细,支持撤销充值。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡);el-table → DsTable(单号 mono、金额右对齐、DsTag 状态:
       有效=positive、已撤销=info),PortalFilterBar → DsFilterBar/DsFilterField,
       空态 → DsEmpty,text-slate-* 颜色类换 --ds-* token;数据接入 useListPage,
       请求参数与筛选语义保持不变;撤销充值弹窗仍为 element-plus。
-->
<template>
  <div class="page-container recharge-records-page">
    <PortalPagePanel
      fill
      :icon="CircleDollarSign"
      :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '用户充值明细' }]"
      description="查看本租户下终端用户的 USD 余额充值明细"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="用户名">
            <el-input v-model="query.username" placeholder="搜索用户名" clearable class="recharge-records-username-input" @keyup.enter="search" />
          </DsFilterField>
          <DsFilterField label="充值时间">
            <el-date-picker
              v-model="query.dateRange"
              type="daterange"
              range-separator="至"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
              value-format="YYYY-MM-DD"
              class="recharge-records-date-range"
            />
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="recharge-records-button-icon" />筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="recharge-records-button-icon" />重置
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
        empty-title="暂无充值记录"
      >
        <template #empty>
          <DsEmpty title="暂无充值记录" description="当前筛选条件下没有用户充值明细" />
        </template>
        <template #cell-paidAmount="{ row }">
          <span class="recharge-records-amount">${{ (row.paidAmountMinor / 100).toFixed(2) }}</span>
        </template>
        <template #cell-amount="{ row }">
          <span class="recharge-records-credits">+{{ formatDisplayUSD(row.amountUsd) }}</span>
        </template>
        <template #cell-rechargeMethod="{ row }">
          <DsTag :tone="isOnlineRecharge(row.orderType) ? 'accent' : 'neutral'">{{ rechargeMethodText(row.orderType) }}</DsTag>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusLabel(row.status) }}</DsTag>
        </template>
        <template #cell-note="{ row }">
          <span v-if="row.note">{{ row.note }}</span>
          <span v-else class="recharge-records-dash">—</span>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="recharge-records-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button v-if="row.status === 'active'" link type="danger" class="font-bold" @click="handleReverse(row)">撤销</el-button>
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

    <el-dialog v-model="reverseDialogVisible" title="确认撤销充值" width="480" :close-on-click-modal="false" :append-to-body="true">
      <div class="space-y-4">
        <el-alert type="danger" :closable="false" show-icon class="reverse-warning-alert" title="此操作将回收该充值对应额度包的剩余金额，请确认操作无误。" />
        <div v-if="reverseRow" class="space-y-2 text-sm">
          <div class="flex justify-between"><span class="recharge-records-dialog-label">充值单号</span><span class="font-mono">{{ reverseRow.orderId }}</span></div>
          <div class="flex justify-between"><span class="recharge-records-dialog-label">到账金额</span><span class="font-bold">{{ formatDisplayUSD(reverseRow.amountUsd) }}</span></div>
        </div>
        <el-form :model="reverseForm" label-position="top">
          <el-form-item label="撤销原因" required>
            <el-input v-model="reverseForm.reason" type="textarea" :rows="3" placeholder="请输入撤销原因" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button class="rounded-xl!" @click="reverseDialogVisible = false">取消</el-button>
        <el-button type="danger" :loading="reverseLoading" class="rounded-xl!" @click="confirmReverse">确认撤销</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { Search, RefreshRight } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { CircleDollarSign } from "lucide-vue-next";
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
import type { RechargeRecordItem } from "@/api/types/platformTenant";
import { formatDisplayUSD } from "@/shared/currency";
import { isOnlineRecharge, rechargeMethodText } from "@/utils/recharge";

const columns: DsTableColumn[] = [
  { key: "orderId", title: "充值单号", width: 200, mono: true },
  { key: "username", title: "用户名", width: 130 },
  { key: "paidAmount", title: "实付金额（USD）", width: 140, align: "right" },
  { key: "amount", title: "到账金额（USD）", width: 140, align: "right" },
  { key: "rechargeMethod", title: "充值方式", width: 110, align: "center" },
  { key: "status", title: "状态", width: 90 },
  { key: "note", title: "备注" },
  { key: "createdTime", title: "时间", width: 180 },
  { key: "actions", title: "操作", width: 80, align: "center" }
];

const statusLabel = (s: string) => ({ active: "有效", reversed: "已撤销" })[s] || s || "有效";
// 状态语义沿用原 el-tag 配色:有效=positive、已撤销=info
const statusTone = (s: string): "positive" | "info" => (s === "reversed" ? "info" : "positive");

interface RechargeQuery extends Record<string, unknown> {
  username: string;
  dateRange: string[];
}

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
} = useListPage<RechargeQuery, RechargeRecordItem>({
  initialQuery: { username: "", dateRange: [] },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const [start, end] = params.dateRange ?? [];
      const endDate = end ? new Date(`${end}T00:00:00`) : null;
      if (endDate) endDate.setDate(endDate.getDate() + 1);
      const res = await platformTenantApi.getUserRechargeRecords({
        page: params.page,
        size: params.pageSize,
        username: params.username || undefined,
        timeFrom: start ? new Date(`${start}T00:00:00`).getTime() : undefined,
        timeTo: endDate ? endDate.getTime() : undefined
      });
      return { items: res?.items ?? [], total: res?.total ?? 0 };
    } catch {
      return { items: [], total: 0 };
    }
  }
});

const reverseDialogVisible = ref(false);
const reverseLoading = ref(false);
const reverseRow = ref<RechargeRecordItem | null>(null);
const reverseForm = reactive({ reason: "" });

function formatTime(ts?: number | null) {
  return ts ? new Date(ts).toLocaleString("zh-CN") : "—";
}

function handleReverse(row: RechargeRecordItem) {
  reverseRow.value = row;
  reverseForm.reason = "";
  reverseDialogVisible.value = true;
}

async function confirmReverse() {
  if (!reverseForm.reason.trim()) {
    ElMessage.warning("请输入撤销原因");
    return;
  }
  if (!reverseRow.value) return;
  reverseLoading.value = true;
  try {
    const result = await platformTenantApi.reverseRecharge(reverseRow.value.orderId, { reason: reverseForm.reason });
    reverseDialogVisible.value = false;
    if (result?.status === "PARTIAL_REVERSAL") {
      ElMessage.warning({
        message: `部分撤销成功：回收 $${result.reversedAmountUsd.toLocaleString()}，已消耗 $${result.lostAmountUsd.toLocaleString()} 无法回收`,
        duration: 5000
      });
    } else {
      ElMessage.success("充值撤销成功");
    }
    refresh();
  } catch (err: any) {
    ElMessage.error(err?.message || "撤销失败");
  } finally {
    reverseLoading.value = false;
  }
}
</script>

<style scoped>
.recharge-records-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.recharge-records-username-input {
  width: 200px;
}

.recharge-records-date-range {
  width: 260px;
}

.recharge-records-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.recharge-records-dash {
  color: var(--ds-faint);
}

.recharge-records-amount {
  font-weight: 600;
  color: var(--ds-ink-soft);
}

.recharge-records-credits {
  font-weight: 700;
  color: var(--ds-positive);
}

.recharge-records-time {
  font-size: 12px;
  color: var(--ds-faint);
}

.recharge-records-dialog-label {
  color: var(--ds-muted);
}

.reverse-warning-alert {
  border-radius: var(--ds-radius-panel);
  border: 1px solid color-mix(in srgb, var(--ds-danger) 22%, transparent);
  background: var(--ds-danger-soft);
  padding: 14px 16px;
}

:deep(.reverse-warning-alert .el-alert__icon) {
  color: var(--ds-danger);
}

:deep(.reverse-warning-alert .el-alert__title) {
  color: var(--ds-danger);
  font-weight: 600;
}
</style>
