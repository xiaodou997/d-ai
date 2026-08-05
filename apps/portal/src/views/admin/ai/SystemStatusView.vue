<!--
  系统状态 — 1:1 搬运自 v1/uni-ai-api/ai-admin/src/views/SystemStatus/index.vue。
  适配：getSystemStatus → aiAdminApi.getSystemStatus()；v4 返回强类型 SystemStatusDTO。
  V1 表格用 row.kind===1（数值）与 row.state_str；v4 返回 kind 为字符串("deployment"/"credential")、字段名为 state。已按 v4 字段适配，UI 1:1。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       状态卡片收进同卡 body 的 24px 容器）;熔断器列表 el-table → DsTable(:frame="false"),
       el-tag → DsTag,空态统一 DsEmpty;数据获取逻辑与请求参数保持不变。
       注:getSystemStatus 一次性返回全量状态,无分页参数,故不挂 DsPagination。
-->
<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { Refresh, CircleCheck, CircleClose, Warning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { HeartPulse } from 'lucide-vue-next'
import { PortalContentCard, PortalMetricGrid, PortalPagePanel } from '@dai/app-core'
import { DsEmpty, DsTable, DsTag, type DsTableColumn } from '@dai/ui'
import { aiAdminApi } from '../../api/aiAdmin'

const loading = shallowRef(false)
const sysStatus = shallowRef<any>(null)
const healthSummary = computed(() => sysStatus.value?.health || {
  total_tracked: 0,
  open_count: 0,
  half_open_count: 0,
  records: []
})

const healthColumns: DsTableColumn[] = [
  { key: 'target_id', title: '目标 ID', mono: true },
  { key: 'kind', title: '类型', width: 100 },
  { key: 'state', title: '状态', width: 120 },
  { key: 'consecutive_failures', title: '连续失败次数', width: 140, align: 'right' },
  { key: 'next_probe_at', title: '下次探测时间' }
]

const componentTag = (s: any) => (s === 'ok' ? 'success' : s === 'disabled' ? 'info' : 'danger')
const componentIcon = (s: any) => (s === 'ok' ? CircleCheck : s === 'disabled' ? Warning : CircleClose)
const formatUntil = (ts: any) => (ts ? new Date(ts).toLocaleString('zh-CN') : '—')
const healthStateTone = (state: any) => (state === 'open' ? 'danger' : state === 'half_open' ? 'warning' : 'positive') as 'danger' | 'warning' | 'positive'
const healthStateLabel = (state: any) => (state === 'open' ? '熔断' : state === 'half_open' ? '半开' : '正常')
const kindLabel = (kind: any) => (kind === 'credential' ? '凭证' : '部署')

const fetchSystem = async () => {
  loading.value = true
  try {
    sysStatus.value = await aiAdminApi.getSystemStatus()
  } catch (err: any) {
    sysStatus.value = null
    ElMessage.error('加载系统状态失败：' + (err?.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

onMounted(fetchSystem)
</script>

<template>
  <div class="page-container system-status-page">
    <PortalPagePanel
      :icon="HeartPulse"
      :breadcrumbs="[{ label: '智能服务' }, { label: '数据监控' }, { label: '系统状态' }]"
      description="基础设施健康与熔断器状态 — 故障排查时使用。"
    >
      <template #actions>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchSystem">刷新</el-button>
      </template>

      <!-- 状态卡片:body 无内边距,用 24px 容器承载 -->
      <div class="status-body">
        <template v-if="sysStatus">
          <!-- 基础设施 -->
          <PortalContentCard title="基础设施健康">
            <PortalMetricGrid class="infra-cards">
              <div class="infra-card" :class="`infra-card--${componentTag(sysStatus.db?.status)}`">
                <el-icon :size="28"><component :is="componentIcon(sysStatus.db?.status)" /></el-icon>
                <div class="infra-info">
                  <p class="infra-name">PostgreSQL</p>
                  <p class="infra-status">{{ sysStatus.db?.status }}</p>
                  <p v-if="sysStatus.db?.error" class="infra-error">{{ sysStatus.db.error }}</p>
                </div>
              </div>
              <div class="infra-card" :class="`infra-card--${componentTag(sysStatus.redis?.status)}`">
                <el-icon :size="28"><component :is="componentIcon(sysStatus.redis?.status)" /></el-icon>
                <div class="infra-info">
                  <p class="infra-name">Redis</p>
                  <p class="infra-status">{{ sysStatus.redis?.status }}</p>
                  <p v-if="sysStatus.redis?.error" class="infra-error">{{ sysStatus.redis.error }}</p>
                </div>
              </div>
            </PortalMetricGrid>
          </PortalContentCard>

          <!-- 熔断器 -->
          <PortalContentCard title="熔断器状态">
            <template #meta>
              <span class="cb-summary">
                跟踪中 {{ healthSummary.total_tracked }} 个目标
                <DsTag v-if="healthSummary.open_count > 0" tone="danger">
                  {{ healthSummary.open_count }} 个熔断
                </DsTag>
                <DsTag v-else tone="positive">全部健康</DsTag>
              </span>
            </template>

            <DsEmpty
              v-if="healthSummary.total_tracked === 0"
              title="暂无跟踪中的目标"
              description="所有线路处于默认健康状态"
            />
            <DsTable
              v-else
              :columns="healthColumns"
              :rows="healthSummary.records"
              row-key="target_id"
              :frame="false"
            >
              <template #cell-kind="{ row }">{{ kindLabel(row.kind) }}</template>
              <template #cell-state="{ row }">
                <DsTag :tone="healthStateTone(row.state)">{{ healthStateLabel(row.state) }}</DsTag>
              </template>
              <template #cell-next_probe_at="{ row }">{{ formatUntil(row.next_probe_at) }}</template>
            </DsTable>

            <p class="page-footer">最后刷新：{{ new Date(sysStatus.timestamp).toLocaleString('zh-CN') }}</p>
          </PortalContentCard>
        </template>

        <el-skeleton v-else-if="loading" :rows="6" animated />
        <DsEmpty v-else title="暂无数据" description="请点击右上角刷新加载系统状态" />
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.system-status-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 状态卡片主体:PortalPagePanel body 无内边距,用 24px 容器排布 */
.status-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

.cb-summary { display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 13px; color: var(--ds-ink-soft); }
.infra-card {
  display: flex; align-items: center; gap: 16px;
  padding: 18px 20px; border-radius: var(--ds-radius-panel);
  border: 1px solid var(--ds-line); background: var(--ds-panel);
  min-width: 220px; box-shadow: var(--ds-shadow-sm);
}
.infra-card--success :deep(.el-icon) { color: var(--ds-positive); }
.infra-card--danger { border-color: color-mix(in srgb, var(--ds-danger) 30%, var(--ds-line)); background: var(--ds-danger-soft); }
.infra-card--danger :deep(.el-icon) { color: var(--ds-danger); }
.infra-card--info :deep(.el-icon) { color: var(--ds-faint); }
.infra-info { display: flex; flex-direction: column; gap: 2px; }
.infra-name { margin: 0; font-weight: 700; font-size: 14px; color: var(--ds-ink); }
.infra-status { margin: 0; font-size: 12px; color: var(--ds-muted); text-transform: uppercase; letter-spacing: 0.05em; }
.infra-error { margin: 0; font-size: 11px; color: var(--ds-danger); }
.page-footer { margin-top: 16px; font-size: 12px; color: var(--ds-faint); text-align: right; }
</style>
