<!--
  账户全景 — 1:1 搬运自 v1/urm/urm-admin/src/views/Finance/AccountOverview.vue。
  适配：account axios → urmAdminApi.getAccountBalance。echarts 积分构成饼图保留。
  注：V1 的 balance 接口与 v4 完全一致（仅余额字段），维度卡/最近流水在 V1 中即按
     `?? 0/[]` 优雅降级，本页保持同样行为（非功能丢失，与 V1 一致）。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/结果同卡,最近流水作为卡内分区),弹窗仍为 element-plus。
-->
<template>
  <div class="account-overview-page">
    <PortalPagePanel
      :icon="WalletIcon"
      :breadcrumbs="[{ label: '用户中心' }, { label: '数据监控' }, { label: '账户全景' }]"
      description="按账户类型与 ID 查询单个租户或用户的资产、积分构成、维度信息与最近流水。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="账户类型">
            <el-select v-model="queryForm.accountType" class="acc-type-select" placeholder="账户类型">
              <el-option label="租户账户" :value="1" />
              <el-option label="用户账户" :value="2" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="账户 ID">
            <el-input
              v-model="queryForm.accountId"
              :placeholder="queryForm.accountType === 1 ? '输入租户 ID（如 T_xxx）' : '输入用户 ID（如 EU_xxx）'"
              clearable
              class="acc-id-input"
              @keyup.enter="handleSearch"
            />
          </DsFilterField>

          <template #actions>
            <el-button type="primary" :loading="loading" @click="handleSearch">
              <Search class="acc-btn-icon" />筛选
            </el-button>
            <el-button @click="handleReset">
              <RefreshRight class="acc-btn-icon" />重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <div v-if="info" class="acc-results">
        <div class="acc-grid">
          <!-- ① 账户余额卡 -->
          <div class="acc-card acc-balance">
            <div class="acc-balance__blob"></div>
            <p class="acc-card__eyebrow">当前可用积分</p>
            <h2 class="acc-balance__value" :class="{ 'is-negative': info.credits < 0 }">
              {{ (info.credits || 0).toLocaleString() }}
              <span class="acc-balance__unit">积分</span>
            </h2>
            <div class="acc-balance__tags">
              <DsTag :tone="info.status === 1 ? 'positive' : 'danger'">
                {{ info.statusDisplay }}
              </DsTag>
              <DsTag v-if="info.hasDebt" tone="danger">
                未结债务 {{ formatMicroCredits(info.outstandingDebtMicro) }} 积分
              </DsTag>
            </div>
            <div class="acc-balance__breakdown">
              <div class="acc-kv"><span>永久有效积分</span><b>{{ (info.permanentCredits || 0).toLocaleString() }}</b></div>
              <div class="acc-kv"><span>限时有效积分</span><b class="is-warning">{{ (info.temporaryCredits || 0).toLocaleString() }}</b></div>
              <div class="acc-kv"><span>近 30 天消耗</span><b class="is-danger">-{{ (info.recentConsumedCredits || 0).toLocaleString() }}</b></div>
              <div class="acc-kv"><span>历史总消耗</span><b class="is-muted">{{ (info.consumedCredits || 0).toLocaleString() }}</b></div>
            </div>
          </div>

          <!-- ② 积分构成饼图 -->
          <div class="acc-card">
            <p class="acc-card__eyebrow">积分构成</p>
            <div v-if="(info.permanentCredits || 0) + (info.temporaryCredits || 0) > 0">
              <div ref="pieRef" class="acc-pie-chart" />
              <div class="acc-pie-legend">
                <div class="acc-pie-legend__item">
                  <p class="acc-pie-legend__label">永久</p>
                  <p class="acc-pie-legend__val is-accent">{{ fmtNum(info.permanentCredits) }}</p>
                </div>
                <div class="acc-pie-legend__item">
                  <p class="acc-pie-legend__label">限时</p>
                  <p class="acc-pie-legend__val is-warning">{{ fmtNum(info.temporaryCredits) }}</p>
                </div>
              </div>
            </div>
            <div v-else class="acc-pie-empty">暂无有效积分</div>
          </div>

          <!-- ③ 维度信息（平面卡片，指标项使用 DsMetricCard） -->
          <div class="acc-card acc-dimension">
            <template v-if="info.accountType === 1">
              <p class="acc-card__eyebrow">租户全景</p>
              <div class="acc-dimension__grid">
                <DsMetricCard label="组织用户" :value="String(info.orgUserCount || 0)" hint="人" />
                <DsMetricCard label="终端用户" :value="String(info.endUserCount || 0)" hint="人" />
                <DsMetricCard label="接入应用" :value="String(info.appCount || 0)" hint="个" />
                <DsMetricCard label="用户积分总量" :value="fmtNum(info.userCreditsTotal)" hint="积分" />
              </div>
            </template>
            <template v-else>
              <p class="acc-card__eyebrow">用户全景</p>
              <div class="acc-dimension__stack">
                <div class="acc-dimension__row">
                  <p class="acc-dimension__label">所属租户</p>
                  <p class="acc-dimension__text">{{ info.tenantName || '—' }}</p>
                </div>
                <div class="acc-dimension__row">
                  <p class="acc-dimension__label">用户 ID</p>
                  <p class="acc-dimension__mono">{{ info.accountId }}</p>
                </div>
                <div class="acc-dimension__row">
                  <p class="acc-dimension__label">账户状态</p>
                  <p class="acc-dimension__text">{{ info.statusDisplay }}</p>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- 空状态 -->
      <DsEmpty
        v-else-if="searched"
        title="未找到该账户的资产信息"
        description="请检查账户 ID 或类型是否正确"
      >
        <template #icon>
          <WalletIcon :size="24" :stroke-width="1.5" />
        </template>
      </DsEmpty>

      <!-- 最近流水(后端内嵌最新 15 条,无分页;作为卡内分区) -->
      <div v-if="info" class="acc-tx-section">
        <p class="acc-tx-section__title">最近流水(最新 15 条)</p>
        <DsTable
          :frame="false"
          :columns="txColumns"
          :rows="info.recentTransactions || []"
          row-key="eventId"
          empty-title="该账户暂无交易记录"
        >
          <template #cell-transactionType="{ row }">
            <DsTag :tone="getTxTypeTone(row.transactionType)">{{ row.transactionTypeDisplay }}</DsTag>
          </template>
          <template #cell-credits="{ row }">
            <span class="acc-tx-amount" :class="row.transactionType === 5 ? 'is-in' : 'is-out'">
              {{ row.transactionType === 5 ? '+' : '-' }}{{ (row.credits || 0).toLocaleString() }}
            </span>
            <span class="acc-tx-unit">积分</span>
          </template>
          <template #cell-resourceTypeCode="{ row }">
            <span class="acc-tx-resource">{{ row.resourceTypeCode || '—' }}</span>
          </template>
          <template #cell-status="{ row }">
            <DsTag :tone="getTxStatusTone(row.status)">{{ row.statusDisplay }}</DsTag>
          </template>
          <template #cell-createdTime="{ row }">
            <span class="acc-tx-time">{{ formatTime(row.createdTime) }}</span>
          </template>
        </DsTable>
      </div>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, nextTick, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { RefreshRight, Search } from '@element-plus/icons-vue'
import { Wallet as WalletIcon } from 'lucide-vue-next'
import { PieChart } from 'echarts/charts'
import { TooltipComponent } from 'echarts/components'
import { init, use, type EChartsType } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PortalPagePanel } from '@/platform'
import { formatMicroCredits } from '@/platform/ai/usage'
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsMetricCard,
  DsTable,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { urmAdminApi } from '@/api/urmAdmin'

use([PieChart, TooltipComponent, CanvasRenderer])

const queryForm = reactive({ accountType: 1, accountId: '' })
const info = ref<any>(null)
const searched = ref(false)
const loading = ref(false)

const pieRef = ref<HTMLElement | null>(null)
let pieChart: EChartsType | null = null

const txColumns: DsTableColumn[] = [
  { key: 'eventId', title: '交易 ID', width: 190, mono: true },
  { key: 'transactionType', title: '类型', width: 110 },
  { key: 'credits', title: '变动积分', align: 'right', width: 140 },
  { key: 'resourceTypeCode', title: '资源类型', width: 130 },
  { key: 'status', title: '状态', width: 100 },
  { key: 'createdTime', title: '时间', width: 180 }
]

const getTxTypeTone = (type: number): 'danger' | 'warning' | 'info' | 'positive' | 'neutral' =>
  (({ 1: 'danger', 2: 'warning', 3: 'warning', 4: 'info', 5: 'positive' } as const)[type] ?? 'neutral')

const getTxStatusTone = (status: number): 'positive' | 'warning' | 'danger' =>
  (({ 1: 'positive', 0: 'warning' } as const)[status] ?? 'danger')

const fmtNum = (n?: number) => (n || 0).toLocaleString()

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : '—')

// echarts 无法直接消费 CSS 变量，运行时从 :root 解析 token 值，避免硬编码色值
const dsColor = (name: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(name).trim() || undefined

const renderPie = (data: any) => {
  nextTick(() => {
    if (!pieRef.value) return
    if (!pieChart) pieChart = init(pieRef.value)
    const permanent = data.permanentCredits || 0
    const temporary = data.temporaryCredits || 0
    pieChart.setOption({
      color: [dsColor('--ds-accent'), dsColor('--ds-warning')].filter(Boolean) as string[],
      tooltip: { trigger: 'item', formatter: '{b}: {c} 积分 ({d}%)' },
      series: [{
        type: 'pie',
        radius: ['45%', '75%'],
        avoidLabelOverlap: false,
        itemStyle: { borderRadius: 8, borderColor: dsColor('--ds-panel'), borderWidth: 3 },
        label: { show: false },
        data: [
          { name: '永久积分', value: permanent },
          { name: '限时积分', value: temporary }
        ].filter((d) => d.value > 0)
      }]
    })
  })
}

const normalizeAccount = (raw: any) => {
  if (!raw) return null
  return {
    ...raw,
    credits: raw.availableCredits ?? raw.credits ?? 0,
    temporaryCredits: raw.timedCredits ?? raw.temporaryCredits ?? 0,
    permanentCredits: raw.permanentCredits ?? 0,
    consumedCredits: raw.usedCredits ?? raw.consumedCredits ?? 0,
    recentConsumedCredits: raw.recentConsumedCredits ?? 0,
    accountType: queryForm.accountType,
    accountId: queryForm.accountId.trim(),
	 hasDebt: raw.serviceState === 'blocked_debt',
    status: raw.status ?? 1,
    statusDisplay: raw.statusDisplay ?? '正常',
    recentTransactions: raw.recentTransactions ?? []
  }
}

const handleSearch = async () => {
  if (!queryForm.accountId.trim()) return ElMessage.warning('请输入账户 ID')
  loading.value = true
  try {
    const response = await urmAdminApi.getAccountBalance({
      accountType: queryForm.accountType,
      accountId: queryForm.accountId.trim(),
      detail: true
    })
    const data = normalizeAccount(response)
    searched.value = true
    info.value = data
    if (data) renderPie(data)
  } catch {
    info.value = null
    searched.value = true
  } finally {
    loading.value = false
  }
}

const handleReset = () => {
  queryForm.accountId = ''
  info.value = null
  searched.value = false
}

watch(
  () => info.value,
  (val) => {
    if (!val) {
      pieChart?.dispose()
      pieChart = null
    }
  }
)
</script>

<style scoped>
.account-overview-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.acc-type-select {
  width: 150px;
}

.acc-id-input {
  width: min(360px, 100%);
}

.acc-btn-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

/* 查询结果区（面板 body 无内边距，这里补齐） */
.acc-results {
  padding: 24px;
}

/* 三卡网格 */
.acc-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
}

.acc-card {
  position: relative;
  overflow: hidden;
  padding: 24px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.acc-card__eyebrow {
  margin: 0 0 12px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ds-faint);
}

/* ① 余额卡 */
.acc-balance__blob {
  position: absolute;
  right: -32px;
  top: -32px;
  width: 144px;
  height: 144px;
  border-radius: 50%;
  background: color-mix(in srgb, var(--ds-accent) 6%, transparent);
}

.acc-balance__value {
  margin: 0 0 4px;
  font-size: 38px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--ds-ink);
}

.acc-balance__value.is-negative {
  color: var(--ds-danger);
}

.acc-balance__unit {
  margin-left: 4px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ds-faint);
}

.acc-balance__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0 20px;
}

.acc-balance__breakdown {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--ds-line);
}

.acc-kv {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
}

.acc-kv span {
  color: var(--ds-faint);
}

.acc-kv b {
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.acc-kv b.is-warning { color: var(--ds-warning); }
.acc-kv b.is-danger { color: var(--ds-danger); }
.acc-kv b.is-muted { color: var(--ds-muted); }

/* ② 饼图卡 */
.acc-pie-chart {
  width: 100%;
  height: 180px;
}

.acc-pie-legend {
  display: flex;
  justify-content: space-around;
  margin-top: 8px;
}

.acc-pie-legend__item {
  text-align: center;
}

.acc-pie-legend__label {
  margin: 0;
  font-size: 11px;
  font-weight: 600;
  color: var(--ds-faint);
}

.acc-pie-legend__val {
  margin: 2px 0 0;
  font-size: 14px;
  font-weight: 800;
}

.acc-pie-legend__val.is-accent { color: var(--ds-accent); }
.acc-pie-legend__val.is-warning { color: var(--ds-warning); }

.acc-pie-empty {
  height: 180px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--ds-faint);
  font-size: 13px;
}

/* ③ 维度卡（token 化平面卡片） */
.acc-dimension__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.acc-dimension__stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.acc-dimension__row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.acc-dimension__label {
  margin: 0;
  font-size: 11px;
  font-weight: 600;
  color: var(--ds-faint);
}

.acc-dimension__text {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.acc-dimension__mono {
  margin: 0;
  font-family: var(--ds-font-mono);
  font-size: 12px;
  font-weight: 700;
  color: var(--ds-ink-soft);
  word-break: break-all;
}

/* 最近流水 */
.acc-tx-amount {
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.acc-tx-amount.is-in { color: var(--ds-positive); }
.acc-tx-amount.is-out { color: var(--ds-danger); }

.acc-tx-unit {
  margin-left: 4px;
  font-size: 12px;
  color: var(--ds-faint);
}

.acc-tx-resource {
  font-size: 12px;
  color: var(--ds-muted);
}

.acc-tx-time {
  font-size: 12px;
  color: var(--ds-faint);
}

/* 最近流水分区(卡内,1px 分隔线) */
.acc-tx-section {
  border-top: 1px solid var(--ds-line);
}

.acc-tx-section__title {
  margin: 0;
  padding: 14px 24px;
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-ink-soft);
}

@media (max-width: 1024px) {
  .acc-grid {
    grid-template-columns: 1fr;
  }
}
</style>
