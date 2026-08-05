<!--
  提现审核 — 租户现金余额提现申请审核 + 线下打款核销。
  重构：迁移至新设计系统一体面板（PortalPagePanel：图标徽章+面包屑标题+描述同行，
       筛选/表格/分页同卡）；数据接入 useListPage，请求参数与筛选语义保持不变，弹窗仍为 element-plus。
-->
<template>
  <div class="withdrawals-page">
    <PortalPagePanel
      :icon="Banknote"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '提现审核' }]"
      description="审核租户现金余额提现申请，线下打款后回填凭证号核销。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="状态">
            <el-select v-model="query.status" placeholder="全部状态" clearable class="withdrawals-status-select">
              <el-option label="待审核" value="pending" />
              <el-option label="已通过待打款" value="approved" />
              <el-option label="已打款" value="paid" />
              <el-option label="已驳回" value="rejected" />
              <el-option label="已取消" value="cancelled" />
            </el-select>
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="withdrawals-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="withdrawals-button-icon" />
              重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="withdrawalId"
        :loading="loading"
        empty-title="暂无数据"
      >
        <template #cell-amount="{ row }">
          <span class="withdrawals-num">¥{{ (row.amount / 100).toFixed(2) }}</span>
        </template>
        <template #cell-feeAmount="{ row }">
          <span class="withdrawals-num">¥{{ (row.feeAmount / 100).toFixed(2) }}</span>
        </template>
        <template #cell-payoutAmount="{ row }">
          <span class="withdrawals-num withdrawals-payout">¥{{ (row.payoutAmount / 100).toFixed(2) }}</span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
        </template>
        <template #cell-createdAt="{ row }">
          <span class="withdrawals-time">{{ formatTime(row.createdAt) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button v-if="row.status === 'pending'" link type="primary" @click="openReview(row)">审核</el-button>
          <el-button v-if="row.status === 'approved'" link type="success" @click="openSettle(row)">核销</el-button>
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

    <el-dialog v-model="reviewVisible" title="审核提现申请" width="420px" append-to-body>
      <div v-if="activeRow" class="review-summary">
        <div class="review-row"><span>申请金额</span><strong>¥{{ (activeRow.amount / 100).toFixed(2) }}</strong></div>
        <div class="review-row"><span>手续费</span><strong>¥{{ (activeRow.feeAmount / 100).toFixed(2) }}</strong></div>
        <div class="review-row"><span>应打款</span><strong>¥{{ (activeRow.payoutAmount / 100).toFixed(2) }}</strong></div>
        <div class="review-row"><span>收款户名</span><span>{{ activeRow.accountName }}</span></div>
        <div class="review-row"><span>开户行</span><span>{{ activeRow.bankName }}</span></div>
        <div class="review-row"><span>账号</span><span>{{ activeRow.accountNo }}</span></div>
      </div>
      <el-form label-position="top">
        <el-form-item label="审核意见">
          <el-input v-model="reviewNote" type="textarea" :rows="2" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button type="danger" plain :loading="reviewSubmitting" @click="submitReview(false)">驳回</el-button>
        <el-button type="primary" :loading="reviewSubmitting" @click="submitReview(true)">通过</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="settleVisible" title="线下打款核销" width="420px" append-to-body>
      <div v-if="activeRow" class="review-summary">
        <div class="review-row"><span>应打款</span><strong>¥{{ (activeRow.payoutAmount / 100).toFixed(2) }}</strong></div>
        <div class="review-row"><span>收款户名</span><span>{{ activeRow.accountName }}</span></div>
        <div class="review-row"><span>开户行</span><span>{{ activeRow.bankName }}</span></div>
        <div class="review-row"><span>账号</span><span>{{ activeRow.accountNo }}</span></div>
      </div>
      <el-form label-position="top">
        <el-form-item label="打款凭证号" required>
          <el-input v-model="paymentRef" placeholder="银行流水号/凭证编号" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settleVisible = false">取消</el-button>
        <el-button type="primary" :loading="settleSubmitting" @click="submitSettle">确认已打款</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ElMessage } from "element-plus";
import { RefreshRight, Search } from "@element-plus/icons-vue";
import { Banknote } from "lucide-vue-next";
import { PortalPagePanel, useListPage } from "@dai/app-core";
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@dai/ui";
import { urmAdminApi } from "../../api/urmAdmin";
import type { WithdrawalItem } from "../../types/admin";

const columns: DsTableColumn[] = [
  { key: "withdrawalId", title: "申请单号", width: 200, mono: true },
  { key: "amount", title: "金额（元）", width: 120, align: "right" },
  { key: "feeAmount", title: "手续费（元）", width: 120, align: "right" },
  { key: "payoutAmount", title: "应打款（元）", width: 130, align: "right" },
  { key: "accountName", title: "收款户名", width: 120 },
  { key: "bankName", title: "开户行", width: 140 },
  { key: "accountNo", title: "账号", width: 180, mono: true },
  { key: "status", title: "状态", width: 130 },
  { key: "createdAt", title: "申请时间", width: 180 },
  { key: "actions", title: "操作", width: 160 }
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
} = useListPage<{ status: string }, WithdrawalItem>({
  initialQuery: { status: "" },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const res = await urmAdminApi.listWithdrawals({
        status: params.status || undefined,
        page: params.page,
        size: params.pageSize
      });
      return { items: res.items || [], total: res.total || 0 };
    } catch (e) {
      console.error("获取提现列表失败:", e);
      throw e;
    }
  }
});

const reviewVisible = ref(false);
const reviewSubmitting = ref(false);
const reviewNote = ref("");
const settleVisible = ref(false);
const settleSubmitting = ref(false);
const paymentRef = ref("");
const activeRow = ref<WithdrawalItem | null>(null);

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

type DsTagTone = "neutral" | "accent" | "positive" | "warning" | "danger" | "info";

function statusTone(s: string): DsTagTone {
  const map: Record<string, DsTagTone> = {
    pending: "warning",
    approved: "warning",
    paid: "positive",
    rejected: "danger",
    cancelled: "neutral"
  };
  return map[s] || "neutral";
}

function statusText(s: string) {
  const map: Record<string, string> = {
    pending: "待审核",
    approved: "已通过，待打款",
    paid: "已打款",
    rejected: "已驳回",
    cancelled: "已取消"
  };
  return map[s] || s;
}

function openReview(row: WithdrawalItem) {
  activeRow.value = row;
  reviewNote.value = "";
  reviewVisible.value = true;
}

async function submitReview(approve: boolean) {
  if (!activeRow.value) return;
  reviewSubmitting.value = true;
  try {
    await urmAdminApi.reviewWithdrawal(activeRow.value.withdrawalId, { approve, note: reviewNote.value || undefined });
    ElMessage.success(approve ? "已通过" : "已驳回");
    reviewVisible.value = false;
    refresh();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "操作失败");
  } finally {
    reviewSubmitting.value = false;
  }
}

function openSettle(row: WithdrawalItem) {
  activeRow.value = row;
  paymentRef.value = "";
  settleVisible.value = true;
}

async function submitSettle() {
  if (!activeRow.value) return;
  if (!paymentRef.value.trim()) {
    ElMessage.warning("请输入打款凭证号");
    return;
  }
  settleSubmitting.value = true;
  try {
    await urmAdminApi.settleWithdrawal(activeRow.value.withdrawalId, { paymentRef: paymentRef.value.trim() });
    ElMessage.success("已核销");
    settleVisible.value = false;
    refresh();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "核销失败");
  } finally {
    settleSubmitting.value = false;
  }
}
</script>

<style scoped>
.withdrawals-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.withdrawals-status-select {
  width: 160px;
}

.withdrawals-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.withdrawals-num {
  font-variant-numeric: tabular-nums;
}

.withdrawals-payout {
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.withdrawals-time {
  font-size: 12px;
  color: var(--ds-faint);
}

.review-summary {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
  padding: 12px;
  border-radius: 8px;
  background: var(--ds-panel-muted);
  font-size: 13px;
}

.review-row {
  display: flex;
  justify-content: space-between;
  color: var(--ds-muted);
}

.review-row strong {
  color: var(--ds-ink);
}
</style>
