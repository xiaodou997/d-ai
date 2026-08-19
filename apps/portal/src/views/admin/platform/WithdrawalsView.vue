<!--
  提现记录 — 管理员按租户创建提现记录并直接扣减额度。
  重构：迁移至新设计系统一体面板（PortalPagePanel：图标徽章+面包屑标题+描述同行，
       筛选/表格/分页同卡）；数据接入 useListPage，请求参数与筛选语义保持不变，弹窗仍为 element-plus。
-->
<template>
  <div class="withdrawals-page">
    <PortalPagePanel
      :icon="Banknote"
      :breadcrumbs="[{ label: '资金中心' }, { label: '提现记录' }]"
      description="租户联系管理员后，由管理员创建提现记录；创建时原子扣减租户额度。"
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
            <el-button type="primary" @click="openCreate">
              创建提现记录
            </el-button>
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
          <span class="withdrawals-num">{{ formatMicroUSD(row.amountMicroUsd) }}</span>
        </template>
        <template #cell-feeAmount="{ row }">
          <span class="withdrawals-num">{{ formatMicroUSD(row.feeAmountMicroUsd) }}</span>
        </template>
        <template #cell-payoutAmount="{ row }">
          <span class="withdrawals-num withdrawals-payout">{{ formatMicroUSD(row.payoutAmountMicroUsd) }}</span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
        </template>
        <template #cell-createdAt="{ row }">
          <span class="withdrawals-time">{{ formatTime(row.createdAt) }}</span>
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

    <el-dialog v-model="createVisible" title="创建提现记录" width="520px" append-to-body>
      <el-alert type="warning" :closable="false" show-icon title="提交后立即扣减租户额度，不再经过租户申请、审核或冻结流程。" />
      <el-form label-position="top" class="create-form">
        <el-form-item label="租户" required>
          <el-select v-model="createForm.tenantId" filterable placeholder="选择租户" class="create-form__full">
            <el-option v-for="tenant in tenantOptions" :key="tenant.tenantId" :label="`${tenant.tenantName || '未命名租户'} · ${tenant.tenantId}`" :value="tenant.tenantId" />
          </el-select>
        </el-form-item>
        <el-form-item label="扣减金额" required>
          <el-input-number v-model="createForm.amountUsd" :min="0" :precision="6" :controls="false" class="create-form__full" />
        </el-form-item>
        <div class="create-form__grid">
          <el-form-item label="收款户名"><el-input v-model="createForm.accountName" /></el-form-item>
          <el-form-item label="开户行"><el-input v-model="createForm.bankName" /></el-form-item>
        </div>
        <el-form-item label="收款账号"><el-input v-model="createForm.accountNo" /></el-form-item>
        <el-form-item label="打款凭证号"><el-input v-model="createForm.paymentRef" placeholder="选填，可填写银行流水号" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="createForm.note" type="textarea" :rows="2" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="createSubmitting" @click="submitCreate">确认创建并扣减</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { RefreshRight, Search } from "@element-plus/icons-vue";
import { Banknote } from "lucide-vue-next";
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
import type { WithdrawalItem } from "@/api/types/admin";
import { formatDisplayMicroUSD as formatMicroUSD } from "@/shared/currency";

const columns: DsTableColumn[] = [
  { key: "withdrawalId", title: "记录单号", width: 200, mono: true },
  { key: "amount", title: "扣减金额", width: 150, align: "right" },
  { key: "feeAmount", title: "手续费", width: 130, align: "right" },
  { key: "payoutAmount", title: "应付金额", width: 140, align: "right" },
  { key: "accountName", title: "收款户名", width: 120 },
  { key: "bankName", title: "开户行", width: 140 },
  { key: "accountNo", title: "账号", width: 180, mono: true },
  { key: "status", title: "状态", width: 130 },
  { key: "createdAt", title: "创建时间", width: 180 }
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
      const res = await platformAdminApi.listWithdrawals({
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

const createVisible = ref(false);
const createSubmitting = ref(false);
const tenantOptions = ref<Array<{ tenantId: string; tenantName?: string }>>([]);
const createForm = reactive({ tenantId: "", amountUsd: null as number | null, accountName: "", bankName: "", accountNo: "", note: "", paymentRef: "" });

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
    paid: "已记录（已扣减）",
    rejected: "已驳回",
    cancelled: "已取消"
  };
  return map[s] || s;
}

async function openCreate() {
  createForm.tenantId = "";
  createForm.amountUsd = null;
  createForm.accountName = "";
  createForm.bankName = "";
  createForm.accountNo = "";
  createForm.note = "";
  createForm.paymentRef = "";
  try {
    const res = await platformAdminApi.listTenants({ page: 1, size: 100, status: 1 });
    tenantOptions.value = res.items || [];
    createVisible.value = true;
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "租户列表加载失败");
  }
}

async function submitCreate() {
  const amountMicroUsd = Math.round(Number(createForm.amountUsd ?? 0) * 1_000_000);
  if (!createForm.tenantId || amountMicroUsd <= 0) {
    ElMessage.warning("请选择租户并填写大于 0 的扣减金额");
    return;
  }
  createSubmitting.value = true;
  try {
    await platformAdminApi.createWithdrawal({
      tenantId: createForm.tenantId,
      amountMicroUsd,
      accountName: createForm.accountName.trim() || undefined,
      bankName: createForm.bankName.trim() || undefined,
      accountNo: createForm.accountNo.trim() || undefined,
      note: createForm.note.trim() || undefined,
      paymentRef: createForm.paymentRef.trim() || undefined
    });
    ElMessage.success("提现记录已创建，租户额度已扣减");
    createVisible.value = false;
    refresh();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "创建提现记录失败");
  } finally {
    createSubmitting.value = false;
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

.create-form { margin-top: 18px; }
.create-form__full { width: 100%; }
.create-form__grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 560px) { .create-form__grid { grid-template-columns: 1fr; gap: 0; } }
</style>
