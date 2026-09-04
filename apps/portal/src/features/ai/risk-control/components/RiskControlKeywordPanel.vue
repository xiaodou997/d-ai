<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { DsEmpty, DsMetricCard, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { riskControlApi, type RiskControlConfigDTO } from '../api'

const config = ref<RiskControlConfigDTO | null>(null)
const loading = ref(false)
const columns: DsTableColumn[] = [
  { key: 'word', title: '词条' },
  { key: 'level', title: '级别', width: 110 },
  { key: 'require_with', title: '共现词' },
  { key: 'note', title: '备注' }
]
const entries = computed(() => config.value?.keyword.entries ?? [])
const pinyinEntries = computed(() => config.value?.keyword.pinyin.entries ?? [])

async function load() {
  loading.value = true
  try { config.value = await riskControlApi.getRiskControlConfig() } finally { loading.value = false }
}
function levelTone(level: string): 'danger' | 'warning' | 'info' { return level === 'block' ? 'danger' : level === 'suspect' ? 'warning' : 'info' }
onMounted(load)
</script>

<template>
  <div class="keyword-panel">
    <div class="metric-grid">
      <DsMetricCard label="关键词引擎" :value="config?.keyword.enabled ? '已启用' : '已关闭'" :hint="config?.keyword.enabled ? 'AC 自动机 + 归一化' : '不会执行词条匹配'" />
      <DsMetricCard label="主词库" :value="String(entries.length)" hint="过滤空词条后" />
      <DsMetricCard label="拼音词库" :value="String(pinyinEntries.length)" :hint="config?.keyword.pinyin.enabled ? '已启用' : '未启用'" />
    </div>
    <div class="panel-heading"><div><h3>关键词引擎</h3><p>配置入口统一位于页面右上角；此处提供当前词库的可审阅视图。</p></div><DsTag :tone="config?.keyword.enabled ? 'positive' : 'info'">{{ config?.keyword.enabled ? '运行中' : '已关闭' }}</DsTag></div>
    <DsTable :columns="columns" :rows="entries" row-key="word" :loading="loading" empty-title="暂无关键词">
      <template #cell-level="{ row }"><DsTag :tone="levelTone(row.level)">{{ row.level }}</DsTag></template>
      <template #cell-require_with="{ row }">{{ row.require_with?.join('、') || '-' }}</template>
      <template #empty><DsEmpty title="暂无关键词" description="点击右上角“配置”添加关键词或拼音词条。" /></template>
    </DsTable>
  </div>
</template>

<style scoped>
.keyword-panel{display:flex;flex-direction:column;gap:18px}.metric-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.panel-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.panel-heading h3{margin:0;color:var(--ds-ink);font-size:16px}.panel-heading p{margin:5px 0 0;color:var(--ds-muted);font-size:13px}@media(max-width:700px){.metric-grid{grid-template-columns:1fr}}
</style>

