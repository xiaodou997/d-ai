<!-- 充值记录 — 1:1 搬运自 v1/platform/platform-admin/src/views/Finance/RechargeRecords.vue（api → platformAdminApi，data.items）
     重构：迁移至新设计系统一体面板（PortalPagePanel：图标徽章+面包屑标题+描述同行，
          筛选/表格/分页同卡），数据接入 useListPage；请求参数与筛选语义保持不变，撤销弹窗仍为 element-plus。-->
<template>
  <div class="recharge-records-page">
    <PortalPagePanel
      :icon="ReceiptText"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '充值记录' }]"
      description="查看平台与租户的充值到账记录，可按租户与类型筛选，并对有效记录执行撤销。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="租户名称">
            <el-input
              v-model="query.tenantName"
              placeholder="搜索租户名称"
              clearable
              class="recharge-records-search-input"
              @keyup.enter="search"
            >
              <template #prefix>
                <Search class="recharge-records-search-input__icon" />
              </template>
            </el-input>
          </DsFilterField>
          <DsFilterField label="类型">
            <el-select v-model="query.orderType" placeholder="全部类型" clearable class="recharge-records-type-select">
              <el-option label="平台→租户" value="platform_to_tenant" />
              <el-option label="租户→用户" value="tenant_to_user" />
            </el-select>
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="recharge-records-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="recharge-records-button-icon" />
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
        empty-title="暂无充值记录"
      >
        <template #cell-username="{ row }">
          <span v-if="row.username">{{ row.username }}</span>
          <span v-else class="recharge-records-placeholder">—</span>
        </template>
        <template #cell-paidAmount="{ row }">
          <span class="recharge-records-num recharge-records-amount">${{ (row.paidAmountMinor / 100).toFixed(2) }}</span>
        </template>
        <template #cell-amount="{ row }">
          <span class="recharge-records-num recharge-records-credits">+${{ (row.amountUsd || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 }) }}</span>
        </template>
        <template #cell-orderType="{ row }">
          <DsTag :tone="row.orderType === 'platform_to_tenant' ? 'accent' : 'positive'">
            {{ row.orderType === 'platform_to_tenant' ? '平台→租户' : '租户→用户' }}
          </DsTag>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">
            {{ statusLabel(row.status) }}
          </DsTag>
        </template>
        <template #cell-note="{ row }">
          <span v-if="!row.note" class="recharge-records-placeholder">—</span>
          <span v-else>{{ row.note }}</span>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="recharge-records-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button v-if="row.status === 'active'" link type="danger" @click="handleReverse(row)">
            撤销
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

    <!-- 撤销充值对话框 -->
    <el-dialog v-model="reverseDialogVisible" title="确认撤销充值" width="480" :close-on-click-modal="false" :append-to-body="true">
      <div class="space-y-4">
        <div class="recharge-reverse-alert">
          <p class="recharge-reverse-alert__text">此操作将回收该充值对应额度包的剩余金额，请确认操作无误。</p>
        </div>
        <div v-if="reverseRow" class="space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-slate-500">充值单号</span><span class="font-mono">{{ reverseRow.orderId }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">到账金额</span><span class="font-bold">${{ (reverseRow.amountUsd || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 }) }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">当前状态</span><span>{{ statusLabel(reverseRow.status) }}</span></div>
        </div>
        <el-form :model="reverseForm" label-position="top">
          <el-form-item label="撤销原因" required>
            <el-input v-model="reverseForm.reason" type="textarea" :rows="3" placeholder="请输入撤销原因" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="reverseDialogVisible = false" class="rounded-xl!">取消</el-button>
        <el-button type="danger" :loading="reverseLoading" @click="confirmReverse" class="rounded-xl!">确认撤销</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { RefreshRight, Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { ReceiptText } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'

const columns: DsTableColumn[] = [
  { key: 'orderId', title: '充值单号', width: 200, mono: true },
  { key: 'tenantName', title: '租户名称' },
  { key: 'username', title: '用户名' },
  { key: 'paidAmount', title: '实付金额（USD）', align: 'right' },
  { key: 'amount', title: '到账金额（USD）', align: 'right' },
  { key: 'orderType', title: '类型', align: 'center' },
  { key: 'status', title: '状态', align: 'center' },
  { key: 'note', title: '备注' },
  { key: 'createdTime', title: '充值时间', width: 170 },
  { key: 'actions', title: '操作', width: 90, align: 'center' }
]

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
} = useListPage<{ tenantName: string; orderType: string }, any>({
  initialQuery: { tenantName: '', orderType: '' },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const data = await platformAdminApi.listRechargeRecords({
        page: params.page,
        size: params.pageSize,
        tenantName: params.tenantName || undefined,
        rechargeType:
          params.orderType === 'platform_to_tenant' ? '1' : params.orderType === 'tenant_to_user' ? '2' : undefined
      })
      return { items: data.items || [], total: data.total || 0 }
    } catch (error) {
      ElMessage.error('获取列表失败')
      throw error
    }
  }
})

const reverseDialogVisible = ref(false)
const reverseLoading = ref(false)
const reverseRow = ref<any>(null)
const reverseForm = reactive({ reason: '' })

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : '—')

const statusLabel = (s: string) => (({ active: '有效', reversed: '已撤销' } as any)[s] || s || '有效')
const statusTone = (s: string): 'positive' | 'info' =>
  (({ active: 'positive', reversed: 'info' } as const)[s] ?? 'positive')

const handleReverse = (row: any) => {
  reverseRow.value = row
  reverseForm.reason = ''
  reverseDialogVisible.value = true
}

const confirmReverse = async () => {
  if (!reverseForm.reason.trim()) return ElMessage.warning('请输入撤销原因')
  reverseLoading.value = true
  try {
    const result = await platformAdminApi.reverseRecharge(reverseRow.value.orderId, { reason: reverseForm.reason })
    reverseDialogVisible.value = false
    if (result.status === 'PARTIAL_REVERSAL') {
      ElMessage.warning({
        message: `部分撤销成功：回收 $${result.reversedAmountUsd.toLocaleString()}，已消耗 $${result.lostAmountUsd.toLocaleString()} 无法回收`,
        duration: 5000
      })
    } else {
      ElMessage.success('充值撤销成功')
    }
    refresh()
  } catch (err: any) {
    ElMessage.error(err?.message || '撤销失败')
  } finally {
    reverseLoading.value = false
  }
}
</script>

<style scoped>
.recharge-records-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.recharge-records-search-input {
  width: min(260px, 100%);
}

.recharge-records-type-select {
  width: 160px;
}

.recharge-records-search-input__icon {
  width: 16px;
  height: 16px;
  color: var(--ds-faint);
}

.recharge-records-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.recharge-records-num {
  font-variant-numeric: tabular-nums;
}

.recharge-records-amount {
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.recharge-records-credits {
  font-weight: 700;
  color: var(--ds-positive);
}

.recharge-records-credits-unit {
  margin-left: 4px;
  font-size: 12px;
  color: var(--ds-faint);
}

.recharge-records-placeholder {
  color: var(--ds-line-strong);
}

.recharge-records-time {
  font-size: 12px;
  color: var(--ds-faint);
}

/* 撤销弹窗警示块：替代原 Tailwind 红色工具类 */
.recharge-reverse-alert {
  background: var(--ds-danger-soft);
  border: 1px solid var(--ds-danger);
  border-radius: var(--ds-radius-shell);
  padding: 16px;
}

.recharge-reverse-alert__text {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--ds-danger);
}
</style>
