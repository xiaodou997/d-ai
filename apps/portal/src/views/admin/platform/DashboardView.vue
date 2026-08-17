<!--
  控制概览 — 1:1 搬运自 v1/platform/platform-admin/src/views/Dashboard/index.vue。
  适配：analytics axios api → platformAdminApi（getGlobalStats/getConsumptionTrend/
       getResourceStatistics/getDashboardAlerts）。echarts 趋势图 + 消耗分布饼图保留。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       看板内容置于同卡 body 内 24px 容器排布），指标卡统一 DsMetricCard，区块统一
       PortalContentCard 承载；图表颜色在挂载时由 --ds-* token 解析，无硬编码色值；
       数据获取逻辑与请求参数保持不变。
-->
<template>
  <div class="dashboard-page">
    <PortalPagePanel
      :icon="LayoutDashboard"
      :breadcrumbs="[{ label: '用户中心' }, { label: '数据监控' }, { label: '控制概览' }]"
      description="实时资产计费监控看板。"
    >
      <template #actions>
        <div class="dash-countdown">
          <span class="dash-countdown__dot"></span>
          <span class="dash-countdown__text">{{ countdown }}s</span>
        </div>
        <div class="dash-segmented">
          <button
            v-for="opt in WORKBENCH_RANGE_OPTIONS"
            :key="opt.id"
            class="dash-segmented__btn"
            :class="{ 'is-active': selectedRangeId === opt.id }"
            @click="handleRangeChange(opt.id)"
          >{{ opt.label }}</button>
        </div>
        <el-button type="primary" class="rounded-2xl! px-5! font-bold" :loading="loading" @click="fetchAll">
          <el-icon class="mr-1"><Refresh /></el-icon>刷新
        </el-button>
      </template>

      <!-- 看板主体:body 无内边距,用 24px 容器承载原有栅格与卡片 -->
      <div class="dashboard-body">
    <!-- ① 平台 → 租户 -->
    <PortalContentCard
      title="平台 → 租户"
      :description="`充值类数据为${periodLabel}汇总，余额为当前实时值`"
    >
      <PortalMetricGrid>
        <DsMetricCard
          label="租户充值金额"
          :value="fmtMinorUSD(stats.tenantRechargePaidMinor)"
          :hint="`${periodLabel}平台向租户实收`"
        />
        <DsMetricCard
          label="租户到账金额"
          :value="fmtUSD(stats.tenantRechargeAmountUsd)"
          :hint="`${periodLabel}向租户入账`"
        />
        <DsMetricCard
          label="活跃租户数"
          :value="`${stats.activeTenants} 个`"
          hint="当前状态正常的租户"
        />
        <DsMetricCard
          label="租户余额"
          :value="fmtUSD(stats.tenantTotalBalanceUsd)"
          hint="所有租户当前余额合计"
        />
      </PortalMetricGrid>
    </PortalContentCard>

    <!-- ② 租户 → 用户 -->
    <PortalContentCard
      title="租户 → 用户"
      :description="`充值类数据为${periodLabel}汇总，余额为当前实时值`"
    >
      <PortalMetricGrid>
        <DsMetricCard
          label="用户充值金额"
          :value="fmtMinorUSD(stats.userRechargePaidMinor)"
          :hint="`${periodLabel}租户向用户实收`"
        />
        <DsMetricCard
          label="用户到账金额"
          :value="fmtUSD(stats.userRechargeAmountUsd)"
          :hint="`${periodLabel}向用户入账`"
        />
        <DsMetricCard
          label="新增终端用户"
          :value="`${fmtNum(stats.newUsers)} 名`"
          :hint="`${periodLabel}注册用户数`"
        />
        <DsMetricCard
          label="用户余额"
          :value="fmtUSD(stats.userTotalBalanceUsd)"
          hint="所有用户当前余额合计"
        />
      </PortalMetricGrid>
    </PortalContentCard>

    <!-- ③ 消费趋势 + 运营告警 -->
    <div class="dash-grid">
      <PortalContentCard
        title="资产消费趋势"
        :description="`${periodLabel}业务流水扣减曲线`"
        class="dash-trend-card"
      >
        <template #actions>
          <el-select v-model="trendAccountType" style="width:120px" size="small" @change="fetchTrend">
            <el-option label="全部" value="" />
            <el-option label="租户" value="1" />
            <el-option label="终端用户" value="2" />
          </el-select>
        </template>
        <div v-loading="trendLoading">
          <div ref="trendChartRef" style="width:100%;height:320px" />
        </div>
      </PortalContentCard>

      <AlertList
        class="dash-alerts"
        :failed-transactions="alerts.failedTransactions"
      />
    </div>

    <!-- ④ 业务系统消耗分布 -->
    <PortalContentCard
      title="业务系统消耗分布"
      :description="`${periodLabel}各业务系统消耗分布`"
    >
      <div v-loading="resourceLoading" class="dash-resource">
        <div ref="resourceChartRef" class="dash-resource__chart" />
        <div class="dash-resource__legend">
          <div v-for="(item, index) in resourceData?.resources || []" :key="item.clientId" class="dash-resource__item">
            <div class="dash-resource__name">
              <span class="dash-resource__dot" :style="{ backgroundColor: chartPalette[index % chartPalette.length] }"></span>
              <span>{{ item.clientName || item.clientId || '—' }}</span>
            </div>
            <div class="dash-resource__val">
              <p class="dash-resource__credits">${{ fmtNum(item.amountUsd) }}</p>
              <p class="dash-resource__pct">{{ item.percentage }}%</p>
            </div>
          </div>
          <div v-if="!(resourceData?.resources || []).length" class="dash-resource__empty">暂无消耗数据</div>
        </div>
      </div>
    </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, reactive, computed, onMounted, onUnmounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { LineChart, PieChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { graphic, init, use, type EChartsType } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LayoutDashboard } from 'lucide-vue-next'
import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from '@/platform'
import { DsMetricCard } from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import type { ResourceStatItem } from '@/api/types/admin'
import AlertList from '@/components/AlertList.vue'
import {
  buildWorkbenchRangeWindow,
  DEFAULT_WORKBENCH_RANGE_ID,
  getWorkbenchRangeOption,
  WORKBENCH_RANGE_OPTIONS,
  type WorkbenchRangeId
} from '@/components/workbench/workbenchRanges'

use([LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

const selectedRangeId = ref<WorkbenchRangeId>(DEFAULT_WORKBENCH_RANGE_ID)
const trendAccountType = ref('')
const loading = ref(false)
const trendLoading = ref(false)
const resourceLoading = ref(false)

const stats = reactive({
  currency: 'USD',
  tenantRechargePaidMinor: 0,
  tenantRechargeAmountUsd: 0,
  activeTenants: 0,
  tenantTotalBalanceUsd: 0,
  userRechargePaidMinor: 0,
  userRechargeAmountUsd: 0,
  newUsers: 0,
  userTotalBalanceUsd: 0
})

const alerts = reactive<{ failedTransactions: any[] }>({
  failedTransactions: []
})

const resourceData = ref<{ resources: ResourceStatItem[] } | null>(null)

const trendChartRef = ref<HTMLElement | null>(null)
const resourceChartRef = ref<HTMLElement | null>(null)
let trendChart: EChartsType | null = null
let resourceChart: EChartsType | null = null

const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value))
const rangeWindow = computed(() => buildWorkbenchRangeWindow(selectedRange.value))
const periodLabel = computed(() => selectedRange.value.label)

const fmtUSD = (value?: number) => `$${Number(value || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`
const fmtMinorUSD = (value?: number) => fmtUSD(Number(value || 0) / 100)
const fmtNum = (n?: number) => (n || 0).toLocaleString()

// echarts 绘制在 canvas 上无法直接消费 CSS 变量，挂载时把 --ds-* token 解析成具体色值
const resolveTokenColor = (token: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(token).trim()

// 给 #rrggbb 色值追加透明度（8 位 hex，canvas 支持）；其他格式原样返回
const withAlpha = (color: string, alpha: number) =>
  /^#[0-9a-fA-F]{6}$/.test(color)
    ? color + Math.round(alpha * 255).toString(16).padStart(2, '0')
    : color

// 饼图配色（依次取 token），图例圆点与 echarts color 共用同一份解析结果
const chartPalette = shallowRef<string[]>([])
const chartTheme = shallowRef({ accent: '', inkSoft: '', faint: '', line: '', panel: '' })

const resolveChartTheme = () => {
  chartPalette.value = [
    '--ds-accent',
    '--ds-info',
    '--ds-positive',
    '--ds-warning',
    '--ds-danger',
    '--ds-accent-hover',
    '--ds-faint'
  ].map(resolveTokenColor)
  chartTheme.value = {
    accent: resolveTokenColor('--ds-accent'),
    inkSoft: resolveTokenColor('--ds-ink-soft'),
    faint: resolveTokenColor('--ds-faint'),
    line: resolveTokenColor('--ds-line'),
    panel: resolveTokenColor('--ds-panel')
  }
}

const buildRangeParams = () => ({
  timeFrom: rangeWindow.value.startTime,
  timeTo: rangeWindow.value.endTime
})

const fetchStats = async () => {
  const data = await platformAdminApi.getGlobalStats(buildRangeParams())
  Object.assign(stats, data)
}

const fetchAlerts = async () => {
  try {
    const data = await platformAdminApi.getDashboardAlerts()
    alerts.failedTransactions = Array.isArray(data?.failedTransactions) ? data.failedTransactions : []
  } catch {
    alerts.failedTransactions = []
  }
}

const fetchTrend = async () => {
  trendLoading.value = true
  try {
    const params: { timeFrom?: number; timeTo?: number; accountType?: string } = buildRangeParams()
    if (trendAccountType.value) params.accountType = trendAccountType.value
    const res = await platformAdminApi.getConsumptionTrend(params)
    updateTrendChart(res)
  } finally {
    trendLoading.value = false
  }
}

const fetchResource = async () => {
  resourceLoading.value = true
  try {
    const res = await platformAdminApi.getResourceStatistics(buildRangeParams())
    resourceData.value = res
    updateResourceChart(res)
  } finally {
    resourceLoading.value = false
  }
}

const fetchAll = async () => {
  loading.value = true
  try {
    await Promise.all([fetchStats(), fetchAlerts(), fetchTrend(), fetchResource()])
  } finally {
    loading.value = false
  }
}

const handleRangeChange = (rangeId: WorkbenchRangeId) => {
  selectedRangeId.value = rangeId
  fetchAll()
}

const updateTrendChart = (data: any) => {
  if (!trendChart) return
  const theme = chartTheme.value
  trendChart.setOption({
    grid: { left: '2%', right: '2%', bottom: '3%', top: '8%', containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: withAlpha(theme.panel, 0.95),
      borderRadius: 12,
      padding: 12,
      textStyle: { color: theme.inkSoft, fontWeight: 600, fontSize: 12 }
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: (data?.dataPoints || []).map((p: any) => p.timeLabel),
      axisLine: { lineStyle: { color: theme.line } },
      axisLabel: { color: theme.faint, fontSize: 11, fontWeight: 600 }
    },
    yAxis: {
      type: 'value',
      splitLine: { lineStyle: { color: theme.line, type: 'dashed' } },
      axisLabel: { color: theme.faint, fontSize: 11, fontWeight: 600 }
    },
    series: [{
      type: 'line',
      smooth: 0.4,
      showSymbol: false,
      data: (data?.dataPoints || []).map((p: any) => p.credits || 0),
      lineStyle: { width: 4, color: theme.accent },
      areaStyle: {
        color: new graphic.LinearGradient(0, 0, 0, 1, [
          { offset: 0, color: withAlpha(theme.accent, 0.15) },
          { offset: 1, color: withAlpha(theme.accent, 0) }
        ])
      }
    }]
  })
}

const updateResourceChart = (data: any) => {
  if (!resourceChart) return
  resourceChart.setOption({
    color: chartPalette.value,
    tooltip: { trigger: 'item' },
    series: [{
      type: 'pie',
      radius: ['50%', '80%'],
      center: ['50%', '50%'],
      avoidLabelOverlap: false,
      itemStyle: { borderRadius: 10, borderColor: chartTheme.value.panel, borderWidth: 4 },
      label: { show: false },
      data: (data?.resources || []).map((r: any) => ({ name: r.clientName || r.clientId || '未知系统', value: r.credits || 0 }))
    }]
  })
}

const handleResize = () => {
  trendChart?.resize()
  resourceChart?.resize()
}

const countdown = ref(30)
let countdownTimer: any = null
let refreshTimer: any = null

const startAutoRefresh = () => {
  countdown.value = 30
  countdownTimer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) countdown.value = 30
  }, 1000)
  refreshTimer = setInterval(fetchAll, 30000)
}

onMounted(() => {
  resolveChartTheme()
  if (trendChartRef.value) trendChart = init(trendChartRef.value)
  if (resourceChartRef.value) resourceChart = init(resourceChartRef.value)
  fetchAll()
  startAutoRefresh()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  clearInterval(countdownTimer)
  clearInterval(refreshTimer)
  window.removeEventListener('resize', handleResize)
  trendChart?.dispose()
  resourceChart?.dispose()
})
</script>

<style scoped>
.dashboard-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 看板主体:PortalPagePanel body 无内边距,用 24px 容器排布原栅格与卡片 */
.dashboard-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

/* 页头操作区：时间段切换 + 倒计时 */
.dash-segmented {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
  border: 1px solid var(--ds-line);
}

.dash-segmented__btn {
  padding: 5px 12px;
  border: 0;
  border-radius: var(--ds-radius-pill);
  background: transparent;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.dash-segmented__btn:hover {
  color: var(--ds-ink);
}

.dash-segmented__btn.is-active {
  background: var(--ds-panel);
  color: var(--ds-accent-hover);
  box-shadow: var(--ds-shadow-sm);
}

.dash-countdown {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 12px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
  border: 1px solid var(--ds-line);
}

.dash-countdown__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--ds-positive);
  animation: dash-pulse 1.6s ease-in-out infinite;
}

.dash-countdown__text {
  font-size: 12px;
  font-weight: 600;
  color: var(--ds-muted);
}

@keyframes dash-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}

/* 趋势 + 告警 两栏 */
.dash-grid {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: 20px;
  align-items: stretch;
}

.dash-alerts {
  height: 100%;
}

@media (max-width: 1024px) {
  .dash-grid {
    grid-template-columns: 1fr;
  }
}

/* 消耗分布 */
.dash-resource {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: 28px;
  align-items: center;
}

.dash-resource__chart {
  height: 360px;
}

.dash-resource__legend {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.dash-resource__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

.dash-resource__name {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-ink-soft);
}

.dash-resource__dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dash-resource__val {
  text-align: right;
}

.dash-resource__credits {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--ds-ink);
}

.dash-resource__unit {
  font-size: 12px;
  font-weight: 500;
  color: var(--ds-faint);
}

.dash-resource__pct {
  margin: 2px 0 0;
  font-size: 11px;
  font-weight: 600;
  color: var(--ds-faint);
}

.dash-resource__empty {
  padding: 32px 0;
  text-align: center;
  font-size: 12px;
  color: var(--ds-faint);
}

@media (max-width: 900px) {
  .dash-resource {
    grid-template-columns: 1fr;
  }
}
</style>
