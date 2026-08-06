<!--
  交易流水 — 1:1 搬运自 v1/platform/platform-admin/src/views/Finance/TransactionList.vue。
  保留多条件筛选、批量确认/免除/退款、单条退款/确认/免除、行展开 ops 时间线、结果弹窗。
  适配：account axios api → platformAdminApi（listTransactions/refund/manualConfirmEvent/adminDismissEvent/
       batchConfirmEvents/batchRefundEvents）；res.records → res.items；错误读 err.message。
  重构：迁移至新设计系统一体面板（PortalPagePanel：图标徽章+面包屑标题+描述同行，
       筛选/表格/分页同卡）；数据接入 useListPage；DsTable 使用 selectable + expandable，
       v-model:selection 接选中行；6 个业务弹窗与结果弹窗抽至 ./components，主文件只保留页面编排；
       请求参数与筛选语义保持不变，弹窗仍为 element-plus。
-->
<template>
  <div class="transactions-page">
    <PortalPagePanel
      :icon="ArrowLeftRight"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '交易流水' }]"
      description="按租户、用户、状态与时间范围检索平台交易流水，支持单条与批量的确认、免除、退款。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="租户名称">
            <el-input v-model="query.tenantName" placeholder="租户名称" clearable class="tx-input" @keyup.enter="search" />
          </DsFilterField>
          <DsFilterField label="用户名">
            <el-input v-model="query.username" placeholder="用户名" clearable class="tx-input" @keyup.enter="search" />
          </DsFilterField>
          <DsFilterField label="Client">
            <el-input v-model="query.clientName" placeholder="Client Name" clearable class="tx-input" @keyup.enter="search" />
          </DsFilterField>
          <DsFilterField label="状态">
            <el-select v-model="query.status" placeholder="全部状态" clearable class="tx-status-select">
              <el-option v-for="s in STATUS_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="交易时间">
            <el-date-picker
              v-model="query.timeRange"
              type="datetimerange"
              range-separator="至"
              start-placeholder="开始时间"
              end-placeholder="结束时间"
              class="tx-date-range"
            />
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="tx-button-icon" />筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="tx-button-icon" />重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <!-- 批量操作工具条：选中行时浮现在筛选条与表格之间 -->
      <transition name="slide-down">
        <div v-if="selection.length > 0" class="tx-batch-bar">
          <div class="tx-batch-bar__info">
            <span class="tx-batch-bar__count">已选 {{ selection.length }} 条</span>
            <span class="tx-batch-bar__sum">
              租户积分合计 <b>{{ selectedTenantTotal.toLocaleString() }}</b>
              &nbsp;·&nbsp;
              用户积分合计 <b>{{ selectedUserTotal.toLocaleString() }}</b>
            </span>
          </div>
          <div class="tx-batch-bar__actions">
            <template v-if="selectionAction === 'released'">
              <el-button type="primary" size="small" @click="openBatchConfirm">确认扣款（{{ selection.length }}条）</el-button>
              <el-button type="warning" size="small" @click="openBatchDismiss">免除扣费（{{ selection.length }}条）</el-button>
            </template>
            <template v-else-if="selectionAction === 'succeeded'">
              <el-button type="danger" size="small" @click="openBatchRefund">批量退款（{{ selection.length }}条）</el-button>
            </template>
            <template v-else>
              <span class="tx-batch-bar__hint">请选择相同状态的记录以执行批量操作</span>
            </template>
            <el-button size="small" class="tx-batch-bar__clear" @click="clearSelection">取消选择</el-button>
          </div>
        </div>
      </transition>

      <DsTable
        v-model:selection="selection"
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="eventId"
        :loading="loading"
        selectable
        expandable
        empty-title="暂无交易流水"
      >
        <template #expand="{ row }">
          <OpsTimeline :ops="parseOps(row.metadata)" :terminal-note="row.terminalNote" />
        </template>
        <template #cell-username="{ row }">
          <span v-if="row.username">{{ row.username }}</span>
          <span v-else class="tx-dash">—</span>
        </template>
        <template #cell-clientId="{ row }">
          <span v-if="row.clientId" class="tx-client-chip">{{ row.clientId }}</span>
          <span v-else class="tx-dash">—</span>
        </template>
        <template #cell-tenantCredits="{ row }">
          <span v-if="row.tenantCredits" class="tx-num tx-num--tenant">{{ formatCredits(row.tenantCredits) }}</span>
          <span v-else class="tx-dash">—</span>
        </template>
        <template #cell-userCredits="{ row }">
          <span v-if="row.userCredits" class="tx-num tx-num--user">{{ formatCredits(row.userCredits) }}</span>
          <span v-else class="tx-dash">—</span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="tx-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button v-if="row.status === 'succeeded'" link type="danger" class="font-bold" @click="openRefund(row)">退款</el-button>
          <el-button v-if="row.status === 'released'" link type="primary" class="font-bold" @click="openConfirm(row)">确认扣款</el-button>
          <el-button v-if="row.status === 'released'" link type="warning" class="font-bold" @click="openDismiss(row)">免除</el-button>
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

    <!-- 弹窗均为 element-plus（过渡期），业务逻辑封装在子组件内 -->
    <TransactionRefundDialog ref="refundDialogRef" @success="refresh" />
    <TransactionConfirmDialog ref="confirmDialogRef" @success="refresh" />
    <TransactionDismissDialog ref="dismissDialogRef" @success="refresh" />
    <TransactionBatchConfirmDialog ref="batchConfirmDialogRef" @done="handleBatchDone" />
    <TransactionBatchDismissDialog ref="batchDismissDialogRef" @done="handleBatchDone" />
    <TransactionBatchRefundDialog ref="batchRefundDialogRef" @done="handleBatchDone" />
    <TransactionBatchResultDialog ref="resultDialogRef" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RefreshRight, Search } from '@element-plus/icons-vue'
import { ArrowLeftRight } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import { formatCredits } from '@/platform/ai/utils'
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import OpsTimeline from './components/TransactionOpsTimeline.vue'
import TransactionRefundDialog from './components/TransactionRefundDialog.vue'
import TransactionConfirmDialog from './components/TransactionConfirmDialog.vue'
import TransactionDismissDialog from './components/TransactionDismissDialog.vue'
import TransactionBatchConfirmDialog from './components/TransactionBatchConfirmDialog.vue'
import TransactionBatchDismissDialog from './components/TransactionBatchDismissDialog.vue'
import TransactionBatchRefundDialog from './components/TransactionBatchRefundDialog.vue'
import TransactionBatchResultDialog from './components/TransactionBatchResultDialog.vue'

const STATUS_OPTIONS = [
  { value: 'pending', label: '进行中' },
  { value: 'succeeded', label: '成功' },
  { value: 'released', label: '已释放' },
  { value: 'cancelled', label: '取消' },
  { value: 'refunded', label: '已退款' }
]

// 状态语义沿用原 el-tag 配色：成功=positive、进行中=warning、已释放=danger、取消/已退款=info
const STATUS_TONE: Record<string, 'positive' | 'warning' | 'danger' | 'info' | 'neutral'> = {
  succeeded: 'positive',
  pending: 'warning',
  released: 'danger',
  cancelled: 'info',
  refunded: 'info'
}

const statusText = (s: string) => {
  const map: Record<string, string> = { pending: '进行中', succeeded: '成功', cancelled: '取消', refunded: '已退款', released: '已释放' }
  return map[s] ?? s ?? '—'
}
const statusTone = (s: string) => STATUS_TONE[s] ?? 'neutral'

const columns: DsTableColumn[] = [
  { key: 'eventId', title: '交易流水', width: 200, mono: true },
  { key: 'tenantName', title: '租户' },
  { key: 'username', title: '用户名' },
  { key: 'clientId', title: 'Client' },
  { key: 'description', title: '描述' },
  { key: 'tenantCredits', title: '租户积分', align: 'right' },
  { key: 'userCredits', title: '用户积分', align: 'right' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'createdTime', title: '交易时间', width: 170 },
  { key: 'actions', title: '操作', width: 190 }
]

interface TxQuery extends Record<string, unknown> {
  tenantName: string
  username: string
  clientName: string
  status: string
  timeRange: [Date, Date] | null
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
} = useListPage<TxQuery, any>({
  initialQuery: { tenantName: '', username: '', clientName: '', status: '', timeRange: null },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const res = await platformAdminApi.listTransactions({
        page: params.page,
        size: params.pageSize,
        tenantName: params.tenantName || undefined,
        username: params.username || undefined,
        clientName: params.clientName || undefined,
        status: params.status || undefined,
        timeFrom: params.timeRange?.[0] ? params.timeRange[0].getTime() : undefined,
        timeTo: params.timeRange?.[1] ? params.timeRange[1].getTime() : undefined
      })
      return { items: res.items || [], total: res.total || 0 }
    } catch {
      // 保留原行为：请求失败时静默清空列表，不弹错误提示
      return { items: [], total: 0 }
    }
  }
})

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : '—')

// 多选 & 批量操作栏
const selection = ref<any[]>([])
const clearSelection = () => { selection.value = [] }

const selectionAction = computed(() => {
  if (!selection.value.length) return null
  const statuses = new Set(selection.value.map((r) => r.status))
  if (statuses.size > 1) return 'mixed'
  const s = [...statuses][0]
  return s === 'released' || s === 'succeeded' ? s : 'other'
})

const selectedTenantTotal = computed(() => selection.value.reduce((sum, r) => sum + (r.tenantCredits || 0), 0))
const selectedUserTotal = computed(() => selection.value.reduce((sum, r) => sum + (r.userCredits || 0), 0))

const parseOps = (metadataStr?: string) => {
  try {
    const meta = JSON.parse(metadataStr || '{}')
    return meta.ops || []
  } catch {
    return []
  }
}

// 弹窗编排：子组件内部维护表单与提交，主文件只负责打开与收尾
const refundDialogRef = ref<InstanceType<typeof TransactionRefundDialog>>()
const confirmDialogRef = ref<InstanceType<typeof TransactionConfirmDialog>>()
const dismissDialogRef = ref<InstanceType<typeof TransactionDismissDialog>>()
const batchConfirmDialogRef = ref<InstanceType<typeof TransactionBatchConfirmDialog>>()
const batchDismissDialogRef = ref<InstanceType<typeof TransactionBatchDismissDialog>>()
const batchRefundDialogRef = ref<InstanceType<typeof TransactionBatchRefundDialog>>()
const resultDialogRef = ref<InstanceType<typeof TransactionBatchResultDialog>>()

const openRefund = (row: any) => refundDialogRef.value?.open(row)
const openConfirm = (row: any) => confirmDialogRef.value?.open(row)
const openDismiss = (row: any) => dismissDialogRef.value?.open(row)
const openBatchConfirm = () => batchConfirmDialogRef.value?.open(selection.value)
const openBatchDismiss = () => batchDismissDialogRef.value?.open(selection.value)
const openBatchRefund = () => batchRefundDialogRef.value?.open(selection.value)

// 批量操作收尾：清除选择 → 展示结果 → 刷新列表（顺序与原实现一致）
const handleBatchDone = (res: any) => {
  clearSelection()
  resultDialogRef.value?.open(res)
  refresh()
}
</script>

<style scoped>
.transactions-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 筛选控件宽度 */
.tx-input {
  width: min(180px, 100%);
}

.tx-status-select {
  width: 150px;
}

/* datetimerange 内容较宽，固定编辑器宽度，避免被 flex 撑满整行 */
.tx-date-range {
  width: 380px;
}

:deep(.tx-date-range .el-range-input) {
  font-size: 12px;
}

.tx-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

/* 批量操作工具条：accent-soft 浅底，浮现在筛选条与表格之间 */
.tx-batch-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin: 12px 16px;
  padding: 10px 14px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 18%, transparent);
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent-soft);
}

.tx-batch-bar__info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  min-width: 0;
}

.tx-batch-bar__count {
  font-size: 13px;
  font-weight: 700;
  color: var(--ds-accent-hover);
}

.tx-batch-bar__sum {
  font-size: 12px;
  color: var(--ds-muted);
}

.tx-batch-bar__sum b {
  color: var(--ds-ink-soft);
}

.tx-batch-bar__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.tx-batch-bar__hint {
  font-size: 12px;
  font-weight: 500;
  color: var(--ds-warning);
}

.tx-batch-bar__clear {
  margin-left: 4px;
}

/* Client 标签 */
.tx-client-chip {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 500;
}

.tx-dash {
  color: var(--ds-faint);
}

.tx-num {
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.tx-num--tenant {
  color: var(--ds-accent);
}

.tx-num--user {
  color: var(--ds-positive);
}

.tx-time {
  font-size: 12px;
  color: var(--ds-faint);
}

.slide-down-enter-active, .slide-down-leave-active { transition: all 0.2s ease; }
.slide-down-enter-from, .slide-down-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
