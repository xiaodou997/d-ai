<!--
  数据大盘。
  保留 V1 的手绘 SVG polyline 趋势图（V1 此页本就不用 echarts）、指标卡、Top 模型/租户、最近错误、OAuth 池、每日明细。
  适配：aiGateway axios → aiAdminApi；list* 在 v4 返回 {items,total}，已取 .items；summary 直接是 DTO；
       formatCredits/formatTimestamp 内联实现；基础设施徽标跳转改为真实路由 /ai-gateway/status；
       去掉 animate-in/fade-in（未装该插件）。错误读 err.message。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       看板内容置于同卡 body 内 24px 容器排布）,el-table → DsTable,el-tag → DsTag;
       SVG 趋势图/图例颜色在挂载时由 --ds-* token 解析,无硬编码色值;
       数据获取逻辑与请求参数保持不变。
       ⚠ 看板页各接口均无分页参数,故不加 DsPagination。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, Warning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { BarChart3 } from 'lucide-vue-next'
import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from '@/platform'
import { DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import {
  EMPTY_IDENTITY_INCLUDED,
  normalizeIdentityIncluded,
  PortalIdentityCell,
  resolveIdentityTenantLabel,
  resolveIdentityTenantMeta
} from '@/platform/ai/identity'
import { aiAdminApi } from '@/api/aiAdmin'
import { adminUsageApi, listAdminUsageDailyTrend } from '@/features/ai/usage'
import type { IdentityIncludedDTO } from '@/api/types/ai'
import {
  buildWorkbenchRangeWindow,
  getWorkbenchRangeOption,
  WORKBENCH_RANGE_OPTIONS,
  type WorkbenchRangeId
} from '@/components/workbench/workbenchRanges'

const router = useRouter()

// formatCredits 用于实际消耗/统计结果，保留最多 4 位小数。
const formatCredits = (value: any) => {
  const n = Number(value) || 0
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 4 })
}
const formatTimestamp = (value: any) => {
  if (!value) return ''
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}
const formatMs = (value: any) => `${Math.round(Number(value) || 0)} ms`

const selectedRangeId = shallowRef<WorkbenchRangeId>('24h')
const loading = shallowRef(false)

const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value))
const periodLabel = computed(() => selectedRange.value.label)
const showTrend = computed(() => selectedRange.value.hours >= 168)

// 概览数据
const emptySummary = () => ({
  total_requests: 0,
  successful_requests: 0,
  failed_requests: 0,
  total_tokens: 0,
  total_prompt_tokens: 0,
  total_completion_tokens: 0,
  total_catalog_base_credits: 0,
  total_tenant_payable_credits: 0,
  total_retail_base_credits: 0,
  total_user_payable_credits: 0,
  total_user_charged_credits: 0,
  avg_latency_ms: 0,
  avg_request_total_ms: 0,
  avg_first_response_byte_ms: 0
})
const summary = reactive<any>(emptySummary())
const topModels = shallowRef<any[]>([])
const topTenants = shallowRef<any[]>([])
const topTenantIncluded = shallowRef<IdentityIncludedDTO>(EMPTY_IDENTITY_INCLUDED)
const recentErrors = shallowRef<any[]>([])
const oauthPoolHealth = shallowRef<any[]>([])
const upstreamCosts = shallowRef<any[]>([])

const successRate = computed(() => {
  const total = Number(summary.total_requests) || 0
  if (!total) return '0%'
  return `${((Number(summary.successful_requests) || 0) * 100 / total).toFixed(1)}%`
})

const statusTone = (s: any): 'positive' | 'danger' | 'warning' | 'info' =>
  (({ success: 'positive', failed: 'danger', rejected: 'warning', partial: 'warning' } as any)[s] || 'info')
const topTenantLabel = (tenantId: string) => resolveIdentityTenantLabel(tenantId, topTenantIncluded.value)
const topTenantMeta = (tenantId: string) => resolveIdentityTenantMeta(tenantId, topTenantIncluded.value)

// 趋势数据
const rows = shallowRef<any[]>([])

const CHART_W = 600
const CHART_H = 120
const PAD = { t: 8, r: 8, b: 24, l: 8 }

// SVG 画不进 CSS 变量,挂载时把 --ds-* token 解析成具体色值(与 Platform 控制概览同一套做法)
const resolveTokenColor = (token: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(token).trim()

const chartColors = shallowRef({
  accent: '',
  danger: '',
  info: '',
  positive: '',
  warning: '',
  faint: ''
})

const resolveChartColors = () => {
  chartColors.value = {
    accent: resolveTokenColor('--ds-accent'),
    danger: resolveTokenColor('--ds-danger'),
    info: resolveTokenColor('--ds-info'),
    positive: resolveTokenColor('--ds-positive'),
    warning: resolveTokenColor('--ds-warning'),
    faint: resolveTokenColor('--ds-faint')
  }
}

function polyline(data: any[], getY: (d: any) => number, color: string, maxY: number) {
  if (!data.length) return ''
  const n = data.length
  const xStep = (CHART_W - PAD.l - PAD.r) / Math.max(n - 1, 1)
  const yRange = CHART_H - PAD.t - PAD.b
  const pts = data.map((d, i) => {
    const x = PAD.l + i * xStep
    const y = maxY ? PAD.t + yRange * (1 - getY(d) / maxY) : PAD.t + yRange
    return `${x},${y}`
  })
  return `<polyline fill="none" stroke="${color}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" points="${pts.join(' ')}" />`
}

function xLabels(data: any[], color: string) {
  if (!data.length) return ''
  const n = data.length
  const xStep = (CHART_W - PAD.l - PAD.r) / Math.max(n - 1, 1)
  const step = Math.ceil(n / 6)
  return data
    .flatMap((d, i) => {
      if (i % step !== 0 && i !== n - 1) return []
      const x = PAD.l + i * xStep
	  const label = escapeSvgText(d.date ? String(d.date).slice(5) : '')
	  return [`<text x="${x}" y="${CHART_H - 4}" text-anchor="middle" font-size="10" fill="${color}">${label}</text>`]
    })
    .join('')
}

function escapeSvgText(value: string) {
  return value.replace(/[&<>"']/g, (char) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  })[char] || char)
}

function maxVal(data: any[], ...fns: ((d: any) => number)[]) {
  return Math.max(1, ...data.flatMap((d) => fns.map((fn) => fn(d))))
}

const costSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const c = chartColors.value
  const mx = maxVal(d, (r) => r.tenant_payable_credits, (r) => r.catalog_base_credits)
  return [polyline(d, (r) => r.tenant_payable_credits, c.accent, mx), polyline(d, (r) => r.catalog_base_credits, c.danger, mx), xLabels(d, c.faint)].join('')
})

const volumeSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const c = chartColors.value
  const mx = maxVal(d, (r) => r.request_count)
  return [polyline(d, (r) => r.request_count, c.info, mx), polyline(d, (r) => r.success_count, c.positive, mx), polyline(d, (r) => r.failed_count, c.danger, mx), xLabels(d, c.faint)].join('')
})

const tokenSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const c = chartColors.value
  const mx = maxVal(d, (r) => r.prompt_tokens, (r) => r.completion_tokens)
  return [polyline(d, (r) => r.prompt_tokens, c.accent, mx), polyline(d, (r) => r.completion_tokens, c.warning, mx), xLabels(d, c.faint)].join('')
})

const timingSvg = computed(() => {
  const d = rows.value
  if (!d.length) return ''
  const c = chartColors.value
  const mx = maxVal(d, (r) => r.avg_request_total_ms, (r) => r.avg_first_response_byte_ms)
  return [
    polyline(d, (r) => r.avg_request_total_ms, c.info, mx),
    polyline(d, (r) => r.avg_first_response_byte_ms, c.positive, mx),
    xLabels(d, c.faint)
  ].join('')
})

// 基础设施异常徽标
const infraAlert = shallowRef<any>(null) // { reason: string }

const fetchInfraStatus = async () => {
  try {
    const sys: any = await aiAdminApi.getSystemStatus()
    const health = sys?.health
    const reasons: string[] = []
    if (sys?.db?.status && sys.db.status !== 'ok') reasons.push('数据库异常')
    if (sys?.redis?.status && sys.redis.status !== 'ok' && sys.redis.status !== 'disabled') reasons.push('Redis 异常')
    if (health?.open_count > 0) reasons.push(`${health.open_count} 个熔断`)
	return reasons.length ? { reason: reasons.join(' · ') } : null
  } catch {
	return null
  }
}

// 拉取
let refreshSequence = 0
const refreshAll = async () => {
	const sequence = ++refreshSequence
	const window = buildWorkbenchRangeWindow(selectedRange.value)
	const includeTrend = selectedRange.value.hours >= 168
  loading.value = true
  try {
	const params = {
	  date_from: window.date_from,
	  date_to: window.date_to
	}
	const results = await Promise.allSettled([
	  aiAdminApi.getDashboardSummary(params),
	  aiAdminApi.listDashboardTopModels({ ...params, limit: 8 }),
	  aiAdminApi.listDashboardTopTenants({ ...params, limit: 8 }),
	  aiAdminApi.listDashboardRecentErrors({ ...params, limit: 8 }),
	  aiAdminApi.getOAuthPoolHealth(),
	  fetchInfraStatus(),
	  includeTrend ? listAdminUsageDailyTrend(params) : Promise.resolve({ items: [] }),
	  adminUsageApi.listUpstreamSummary(params)
	])
	if (sequence !== refreshSequence) return

	const value = <T,>(index: number): T | undefined => {
	  const result = results[index]
	  return result?.status === 'fulfilled' ? result.value as T : undefined
	}
	const summaryResult = value<any>(0)
	Object.assign(summary, summaryResult || emptySummary())
	const modelResult = value<any>(1)
	topModels.value = modelResult?.items || []
	const tenantResult = value<any>(2)
	if (tenantResult) {
	  topTenants.value = tenantResult.items || []
	  topTenantIncluded.value = normalizeIdentityIncluded(tenantResult.included)
	} else {
	  topTenants.value = []
	  topTenantIncluded.value = EMPTY_IDENTITY_INCLUDED
	}
	const errorsResult = value<any>(3)
	recentErrors.value = errorsResult?.items || []
	const oauthResult = value<any>(4)
	oauthPoolHealth.value = oauthResult?.items || []
	infraAlert.value = value<any>(5) ?? null
	rows.value = value<any>(6)?.items || []
	upstreamCosts.value = value<any>(7)?.items || []

	const failed = results.filter((result) => result.status === 'rejected').length
	if (failed > 0) ElMessage.error(`数据大盘有 ${failed} 项数据加载失败`)
  } finally {
	if (sequence === refreshSequence) loading.value = false
  }
}

const handleRangeChange = (rangeId: WorkbenchRangeId) => {
  selectedRangeId.value = rangeId
  refreshAll()
}

// 表格列定义
const topModelColumns: DsTableColumn[] = [
  { key: 'model_code', title: '模型', mono: true },
  { key: 'request_count', title: '请求数', width: 100, align: 'right' },
  { key: 'total_tenant_payable_credits', title: '业务计费', width: 110, align: 'right' },
  { key: 'total_tokens', title: 'Token', width: 110, align: 'right' }
]

const topTenantColumns: DsTableColumn[] = [
  { key: 'tenant', title: '租户' },
  { key: 'request_count', title: '请求数', width: 100, align: 'right' },
  { key: 'total_tokens', title: 'Token', width: 110, align: 'right' },
  { key: 'total_tenant_payable_credits', title: '业务计费', width: 120, align: 'right' }
]

const upstreamCostColumns: DsTableColumn[] = [
  { key: 'provider_code', title: '上游资源' },
  { key: 'target_kind', title: '类型', width: 120 },
  { key: 'target_id', title: '资源 ID', mono: true },
  { key: 'request_count', title: '请求数', width: 100, align: 'right' },
  { key: 'total_billable_units', title: '计费单位', width: 110, align: 'right' },
  { key: 'catalog_base_credits', title: '上游参考成本', width: 140, align: 'right' },
  { key: 'tenant_payable_credits', title: '租户结算应收', width: 140, align: 'right' }
]

const recentErrorColumns: DsTableColumn[] = [
  { key: 'created_at', title: '时间', width: 170 },
  { key: 'model_code', title: '模型', mono: true },
  { key: 'request_status', title: '状态', width: 110 },
  { key: 'http_status', title: 'HTTP', width: 90, align: 'right' },
  { key: 'request_id', title: '请求 ID', mono: true },
  { key: 'error_code', title: '错误码' },
  { key: 'error_message', title: '错误信息' }
]

const oauthPoolColumns: DsTableColumn[] = [
  { key: 'pool_name', title: '账号池' },
  { key: 'fixed_provider_type', title: '类型', width: 140 },
  { key: 'oauth_strategy', title: '调度策略', width: 110, align: 'center' },
  { key: 'total', title: '总数', width: 70, align: 'right' },
  { key: 'active', title: '正常', width: 80, align: 'right' },
  { key: 'invalid', title: '失效', width: 80, align: 'right' },
  { key: 'disabled', title: '停用', width: 80, align: 'right' },
  { key: 'expiring_soon', title: '即将过期', width: 100, align: 'right' },
  { key: 'health', title: '健康度', width: 120, align: 'center' }
]

const dailyColumns: DsTableColumn[] = [
  { key: 'date', title: '日期', width: 110, mono: true },
  { key: 'request_count', title: '请求数', width: 90, align: 'right' },
  { key: 'success_count', title: '成功', width: 80, align: 'right' },
  { key: 'failed_count', title: '失败', width: 80, align: 'right' },
  { key: 'total_tokens', title: 'Token', width: 110, align: 'right' },
  { key: 'prompt_tokens', title: '输入', width: 100, align: 'right' },
  { key: 'completion_tokens', title: '输出', width: 110, align: 'right' },
  { key: 'tenant_payable_credits', title: '租户结算', width: 110, align: 'right' },
  { key: 'catalog_base_credits', title: '上游参考成本', width: 120, align: 'right' },
  { key: 'avg_request_total_ms', title: '均总耗时(ms)', width: 122, align: 'right' },
  { key: 'avg_first_response_byte_ms', title: '均首响(ms)', width: 116, align: 'right' }
]

onMounted(() => {
  resolveChartColors()
  refreshAll()
})
</script>

<template>
  <div class="ai-dashboard-page">
    <PortalPagePanel
      :icon="BarChart3"
      :breadcrumbs="[{ label: '智能服务' }, { label: '数据监控' }, { label: '数据大盘' }]"
      description="请求统计、历史趋势与异常监控的一站式视图。"
    >
      <template #actions>
        <button
          v-if="infraAlert"
          class="infra-alert-badge"
          @click="router.push('/admin/ai/monitoring/status')"
        >
          <el-icon :size="14"><Warning /></el-icon>
          基础设施：{{ infraAlert.reason }}
        </button>
        <div class="range-segmented">
          <button
            v-for="opt in WORKBENCH_RANGE_OPTIONS"
            :key="opt.id"
            type="button"
            class="range-segmented__btn"
            :class="{ 'is-active': selectedRangeId === opt.id }"
            @click="handleRangeChange(opt.id)"
          >{{ opt.label }}</button>
        </div>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="refreshAll">刷新</el-button>
      </template>

      <!-- 看板主体:body 无内边距,用 24px 容器承载原栅格与卡片 -->
      <div class="dashboard-body">
        <!-- 指标卡 -->
        <PortalMetricGrid v-loading="loading">
          <div class="metric">
            <span>请求数</span>
            <strong>{{ formatCredits(summary.total_requests) }}</strong>
            <p>{{ periodLabel }}总调用</p>
          </div>
          <div class="metric">
            <span>成功率</span>
            <strong>{{ successRate }}</strong>
            <p>{{ formatCredits(summary.successful_requests) }} 次成功</p>
          </div>
          <div class="metric">
            <span>用户零售应收</span>
            <strong>{{ formatCredits(summary.total_user_payable_credits) }}</strong>
            <p>租户零售参考口径</p>
          </div>
          <div class="metric">
            <span>Token</span>
            <strong>{{ formatCredits(summary.total_tokens) }}</strong>
            <p>Chat / Responses / Embedding</p>
          </div>
          <div class="metric">
            <span>租户结算扣费</span>
            <strong>{{ formatCredits(summary.total_tenant_payable_credits) }}</strong>
            <p>平台营收口径</p>
          </div>
          <div class="metric">
            <span>平均总耗时</span>
            <strong>{{ formatMs(summary.avg_request_total_ms) }}</strong>
            <p>平均首响 {{ formatMs(summary.avg_first_response_byte_ms) }} · 失败 {{ formatCredits(summary.failed_requests) }}</p>
          </div>
        </PortalMetricGrid>

        <div class="dashboard-workbench">
          <!-- 趋势图区（仅 ≥7 天显示） -->
          <template v-if="showTrend">
            <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
              <PortalContentCard title="结算趋势" v-loading="loading">
                <template #actions>
                  <div class="legend">
                    <span class="dot" :style="{ background: chartColors.accent }"></span>租户结算扣费
                    <span class="dot legend__dot--gap" :style="{ background: chartColors.danger }"></span>上游参考成本
                  </div>
                </template>
                <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="costSvg" />
                <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
              </PortalContentCard>
              <PortalContentCard title="请求量趋势" v-loading="loading">
                <template #actions>
                  <div class="legend">
                    <span class="dot" :style="{ background: chartColors.info }"></span>总请求
                    <span class="dot legend__dot--gap" :style="{ background: chartColors.positive }"></span>成功
                    <span class="dot legend__dot--gap" :style="{ background: chartColors.danger }"></span>失败
                  </div>
                </template>
                <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="volumeSvg" />
                <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
              </PortalContentCard>
            </section>

            <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
              <PortalContentCard title="Token 趋势" v-loading="loading">
                <template #actions>
                  <div class="legend">
                    <span class="dot" :style="{ background: chartColors.accent }"></span>输入 Token
                    <span class="dot legend__dot--gap" :style="{ background: chartColors.warning }"></span>输出 Token
                  </div>
                </template>
                <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="tokenSvg" />
                <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
              </PortalContentCard>
              <PortalContentCard title="时延剖面趋势" v-loading="loading">
                <template #actions>
                  <div class="legend">
                    <span class="dot" :style="{ background: chartColors.info }"></span>平均总耗时
                    <span class="dot legend__dot--gap" :style="{ background: chartColors.positive }"></span>平均首响
                  </div>
                </template>
                <svg :viewBox="`0 0 ${CHART_W} ${CHART_H}`" class="chart-svg" v-html="timingSvg" />
                <p v-if="!rows.length && !loading" class="no-data">暂无数据</p>
              </PortalContentCard>
            </section>
          </template>

          <!-- Top 模型 / 租户 -->
          <section class="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <PortalContentCard title="Top 模型" description="按积分消耗和请求量排序">
              <DsTable
                :frame="false"
                :columns="topModelColumns"
                :rows="topModels"
                row-key="model_code"
                :loading="loading"
              >
                <template #cell-request_count="{ row }">{{ formatCredits(row.request_count) }}</template>
                <template #cell-total_tenant_payable_credits="{ row }">{{ formatCredits(row.total_tenant_payable_credits) }}</template>
                <template #cell-total_tokens="{ row }">{{ formatCredits(row.total_tokens) }}</template>
              </DsTable>
            </PortalContentCard>

            <PortalContentCard title="Top 租户" description="按积分消耗和请求量排序">
              <DsTable
                :frame="false"
                :columns="topTenantColumns"
                :rows="topTenants"
                row-key="tenant_id"
                :loading="loading"
              >
                <template #cell-tenant="{ row }">
                  <PortalIdentityCell :label="topTenantLabel(row.tenant_id)" :meta="topTenantMeta(row.tenant_id)" />
                </template>
                <template #cell-request_count="{ row }">{{ formatCredits(row.request_count) }}</template>
                <template #cell-total_tokens="{ row }">{{ formatCredits(row.total_tokens) }}</template>
                <template #cell-total_tenant_payable_credits="{ row }">{{ formatCredits(row.total_tenant_payable_credits) }}</template>
              </DsTable>
            </PortalContentCard>
          </section>

          <PortalContentCard
            title="上游资源参考费用"
            description="按请求实际命中的上游账号或账号池汇总，与用量日志使用同一结算结果。"
          >
            <DsTable
              :frame="false"
              :columns="upstreamCostColumns"
              :rows="upstreamCosts"
              row-key="target_id"
              :loading="loading"
            >
              <template #cell-target_kind="{ row }">{{ row.target_kind === 'oauth_pool' ? '账号池' : '上游账号' }}</template>
              <template #cell-request_count="{ row }">{{ formatCredits(row.request_count) }}</template>
              <template #cell-total_billable_units="{ row }">{{ formatCredits(row.total_billable_units) }}</template>
              <template #cell-catalog_base_credits="{ row }">{{ formatCredits(row.catalog_base_credits) }}</template>
              <template #cell-tenant_payable_credits="{ row }">{{ formatCredits(row.tenant_payable_credits) }}</template>
            </DsTable>
          </PortalContentCard>

          <!-- 最近错误 -->
          <PortalContentCard title="最近错误" description="用于快速定位上游失败、流式中断和已路由请求异常。">
            <DsTable
              :frame="false"
              :columns="recentErrorColumns"
              :rows="recentErrors"
              row-key="request_id"
              :loading="loading"
            >
              <template #cell-created_at="{ row }">{{ formatTimestamp(row.created_at) }}</template>
              <template #cell-request_status="{ row }">
                <DsTag :tone="statusTone(row.request_status)">{{ row.request_status }}</DsTag>
              </template>
            </DsTable>
          </PortalContentCard>

          <!-- OAuth 凭据池 -->
          <PortalContentCard
            v-if="oauthPoolHealth.length > 0"
            title="OAuth 凭据池"
            description="各账号池 OAuth 凭据状态汇总，快速识别失效或即将过期的凭据。"
          >
            <DsTable
              :frame="false"
              :columns="oauthPoolColumns"
              :rows="oauthPoolHealth"
              row-key="pool_name"
              :loading="loading"
            >
              <template #cell-fixed_provider_type="{ row }">
                <DsTag tone="accent">{{ row.fixed_provider_type || '自定义' }}</DsTag>
              </template>
              <template #cell-oauth_strategy="{ row }">
                <span class="strategy-badge" :class="row.oauth_strategy">{{ row.oauth_strategy }}</span>
              </template>
              <template #cell-active="{ row }"><span class="count-active">{{ row.active }}</span></template>
              <template #cell-invalid="{ row }">
                <span :class="row.invalid > 0 ? 'count-invalid' : 'count-zero'">{{ row.invalid }}</span>
              </template>
              <template #cell-disabled="{ row }">
                <span :class="row.disabled > 0 ? 'count-warn' : 'count-zero'">{{ row.disabled }}</span>
              </template>
              <template #cell-expiring_soon="{ row }">
                <span :class="row.expiring_soon > 0 ? 'count-warn' : 'count-zero'">{{ row.expiring_soon }}</span>
              </template>
              <template #cell-health="{ row }">
                <el-progress
                  :percentage="row.total > 0 ? Math.round(row.active * 100 / row.total) : 0"
                  :status="row.active === 0 ? 'exception' : row.invalid > 0 ? 'warning' : 'success'"
                  :stroke-width="8"
                  style="width:90px;display:inline-block"
                />
              </template>
            </DsTable>
          </PortalContentCard>

          <!-- 每日明细（仅 ≥7 天显示） -->
          <PortalContentCard v-if="showTrend" title="每日明细" description="按自然日汇总，最新在上。">
            <DsTable
              :frame="false"
              :columns="dailyColumns"
              :rows="[...rows].reverse()"
              row-key="date"
              :loading="loading"
            >
              <template #cell-request_count="{ row }">{{ formatCredits(row.request_count) }}</template>
              <template #cell-success_count="{ row }">{{ formatCredits(row.success_count) }}</template>
              <template #cell-failed_count="{ row }">
                <span :class="row.failed_count > 0 ? 'count-invalid' : ''">{{ formatCredits(row.failed_count) }}</span>
              </template>
              <template #cell-total_tokens="{ row }">{{ formatCredits(row.total_tokens) }}</template>
              <template #cell-prompt_tokens="{ row }">{{ formatCredits(row.prompt_tokens) }}</template>
              <template #cell-completion_tokens="{ row }">{{ formatCredits(row.completion_tokens) }}</template>
              <template #cell-tenant_payable_credits="{ row }">{{ formatCredits(row.tenant_payable_credits) }}</template>
              <template #cell-catalog_base_credits="{ row }">{{ formatCredits(row.catalog_base_credits) }}</template>
              <template #cell-avg_request_total_ms="{ row }">{{ formatCredits(row.avg_request_total_ms) }}</template>
              <template #cell-avg_first_response_byte_ms="{ row }">{{ formatCredits(row.avg_first_response_byte_ms) }}</template>
            </DsTable>
          </PortalContentCard>
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.ai-dashboard-page {
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

.dashboard-workbench {
  display: grid;
  gap: 20px;
}

.infra-alert-badge {
  display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid color-mix(in srgb, var(--ds-danger) 30%, var(--ds-line)); background: var(--ds-danger-soft); color: var(--ds-danger);
  border-radius: var(--ds-radius-pill); padding: 6px 14px; cursor: pointer;
  font-size: 12px; font-weight: 700;
  transition: background 0.2s;
}
.infra-alert-badge:hover { background: color-mix(in srgb, var(--ds-danger) 18%, var(--ds-panel)); }

/* 时间段选择 */
.range-segmented {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
  border: 1px solid var(--ds-line);
}
.range-segmented__btn {
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
.range-segmented__btn:hover { color: var(--ds-ink); }
.range-segmented__btn.is-active {
  background: var(--ds-panel);
  color: var(--ds-accent-hover);
  box-shadow: var(--ds-shadow-sm);
}

/* 指标卡（放进 PortalMetricGrid 默认插槽） */
.metric {
  min-width: 0; padding: 18px;
  border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}
.metric span { display: block; color: var(--ds-muted); font-size: 12px; font-weight: 600; }
.metric strong { display: block; margin-top: 8px; color: var(--ds-ink); font-size: 22px; font-weight: 700; line-height: 1.1; }
.metric p { margin: 8px 0 0; color: var(--ds-faint); font-size: 12px; font-weight: 500; }

/* 图表 */
.legend { display: flex; align-items: center; font-size: 12px; color: var(--ds-muted); }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 4px; }
.legend__dot--gap { margin-left: 10px; }
.chart-svg { width: 100%; height: auto; display: block; }
.no-data { text-align: center; color: var(--ds-faint); font-size: 13px; margin: 20px 0; }

/* OAuth 凭据池 */
.count-active { color: var(--ds-positive); font-weight: 700; }
.count-invalid { color: var(--ds-danger); font-weight: 700; }
.count-warn { color: var(--ds-warning); font-weight: 700; }
.count-zero { color: var(--ds-faint); }
.strategy-badge { display: inline-block; padding: 2px 8px; border-radius: 6px; font-size: 11px; font-weight: 700; background: var(--ds-panel-muted); color: var(--ds-ink-soft); }
.strategy-badge.weighted { background: color-mix(in srgb, var(--ds-accent) 14%, var(--ds-panel)); color: var(--ds-accent-hover); }
</style>
