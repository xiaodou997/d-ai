<!--
  风控审核日志列表 — 风控中心「审核日志」Tab 内容。
  重构：el-table 迁移至 DsTable(:frame="false"),筛选改为 DsFilterBar/DsFilterField,
       结果 el-tag 换成 DsTag(success→positive 等 tone 映射);接口仅支持 limit 不支持分页,
       故不接 DsPagination,保留「共 N 条」计数;业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { DsFilterBar, DsFilterField, DsTable, DsTag, type DsTableColumn } from '@dai/ui'

import { useRiskControlLogs } from '../composables/useRiskControlLogs'

const { fetchLogs, logFilters, logs, logsLoading, logsTotal } = useRiskControlLogs()

const columns: DsTableColumn[] = [
  { key: 'created_at', title: '时间', width: 170 },
  { key: 'tenant_id', title: '租户', mono: true },
  { key: 'user_id', title: '用户', mono: true },
  { key: 'model_code', title: '模型', mono: true },
  { key: 'mode', title: '模式', width: 100 },
  { key: 'action', title: '结果', width: 120 },
  { key: 'hit', title: '命中类别' },
  { key: 'input_excerpt', title: '输入摘要' },
  { key: 'upstream_latency_ms', title: '审核延迟(ms)', width: 120, align: 'right' }
]

function formatTimestamp(value: unknown) {
  if (!value) return ''
  return new Date(value as string | number).toLocaleString('zh-CN', { hour12: false })
}

function actionTone(action: string): 'positive' | 'danger' | 'warning' | 'info' {
  const map: Record<string, 'positive' | 'danger' | 'warning'> = {
    allow: 'positive',
    block: 'danger',
    keyword_block: 'danger',
    error: 'warning'
  }
  return map[action] || 'info'
}

onMounted(fetchLogs)
</script>

<template>
  <!-- 单根节点:父组件对本面板使用 v-show,多根会导致指令失效 -->
  <div class="risk-logs-panel">
    <DsFilterBar class="risk-logs-filters">
      <DsFilterField label="租户 ID">
        <el-input v-model="logFilters.tenant_id" clearable placeholder="租户 ID" class="filter-input" />
      </DsFilterField>
      <DsFilterField label="用户 ID">
        <el-input v-model="logFilters.user_id" clearable placeholder="用户 ID" class="filter-input" />
      </DsFilterField>
      <DsFilterField label="模式">
        <el-select v-model="logFilters.mode" clearable placeholder="模式" class="filter-input">
          <el-option label="旁路观察" value="observe" />
          <el-option label="同步拦截" value="pre_block" />
        </el-select>
      </DsFilterField>
      <DsFilterField label="结果">
        <el-select v-model="logFilters.action" clearable placeholder="结果" class="filter-input">
          <el-option label="放行" value="allow" />
          <el-option label="拦截" value="block" />
          <el-option label="关键词拦截" value="keyword_block" />
          <el-option label="检测出错" value="error" />
        </el-select>
      </DsFilterField>
      <DsFilterField label="是否命中">
        <el-select v-model="logFilters.flagged" clearable placeholder="是否命中" class="filter-input">
          <el-option label="命中" value="true" />
          <el-option label="未命中" value="false" />
        </el-select>
      </DsFilterField>
      <template #actions>
        <span class="result-count">共 {{ logsTotal }} 条</span>
        <el-button type="primary" :icon="Refresh" :loading="logsLoading" @click="fetchLogs">刷新</el-button>
      </template>
    </DsFilterBar>

    <DsTable
      :frame="false"
      :columns="columns"
      :rows="logs"
      row-key="id"
      :loading="logsLoading"
      empty-title="暂无审核日志"
    >
      <template #cell-created_at="{ row }">{{ formatTimestamp(row.created_at) }}</template>
      <template #cell-action="{ row }">
        <DsTag :tone="actionTone(row.action)">{{ row.action }}</DsTag>
      </template>
      <template #cell-hit="{ row }">
        {{ row.matched_keyword ? `关键词: ${row.matched_keyword}` : row.highest_category || '-' }}
        <span v-if="row.highest_score != null">（{{ row.highest_score.toFixed(2) }}）</span>
      </template>
    </DsTable>
  </div>
</template>

<style scoped>
.risk-logs-filters {
  margin-bottom: 16px;
}

.filter-input {
  width: min(160px, 100%);
}

/* el-select 默认 width:100%(--el-select-width),而 DsFilterField 是 shrink-to-fit 的
   纵向 flex 项,空占位时内容固有宽度极小,select 会被压成几十像素;
   这里固定与输入框一致的宽度(scoped 样式可作用于子组件根节点) */
.filter-input.el-select {
  width: 160px;
}

.result-count {
  font-size: 12.5px;
  color: var(--ds-muted);
  white-space: nowrap;
}
</style>
