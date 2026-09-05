<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { DsButton, DsEmpty, DsMetricCard, DsTag } from '@/shared/ui'
import { riskControlApi, type RiskControlConfigDTO, type RiskControlTestResultDTO } from '../api'
import RiskControlConfigPanel from './RiskControlConfigPanel.vue'

const config = ref<RiskControlConfigDTO | null>(null)
const testText = ref('')
const testResult = ref<RiskControlTestResultDTO | null>(null)
const loading = ref(false)
const testing = ref(false)
async function load() { loading.value = true; try { config.value = await riskControlApi.getRiskControlConfig() } finally { loading.value = false } }
async function test() { if (!testText.value.trim()) return; testing.value = true; try { testResult.value = await riskControlApi.testRiskControlModeration(testText.value) } finally { testing.value = false } }
onMounted(load)
</script>

<template>
  <div class="provider-panel">
    <div v-if="loading" class="loading-state">加载审核 API 配置…</div>
    <template v-else-if="config">
      <div class="metric-grid"><DsMetricCard label="Provider" :value="config.provider.model || '-'" hint="OpenAI Moderations 协议"/><DsMetricCard label="API Key" :value="config.provider.has_api_key ? '已配置' : '未配置'"/><DsMetricCard label="采样率" :value="`${Math.round(config.sample_rate * 100)}%`" :hint="`${config.provider.timeout_ms} ms 超时`"/></div>
      <div class="provider-card"><div class="provider-heading"><div><h3>审核 API</h3><p>仅对选定的上游账号调用 Moderations 接口。</p></div><div class="test-actions"><DsTag :tone="config.enabled ? 'positive' : 'info'">{{ config.enabled ? config.mode : '已关闭' }}</DsTag><RiskControlConfigPanel section="provider" /></div></div><dl><dt>Base URL</dt><dd>{{ config.provider.base_url || '-' }}</dd><dt>模型</dt><dd>{{ config.provider.model || '-' }}</dd><dt>API Key</dt><dd>{{ config.provider.has_api_key ? '已配置（不回显）' : '未配置' }}</dd></dl></div>
      <div class="provider-card"><h3>快速测试</h3><p>测试不会写入审核日志，也不会影响真实请求。</p><el-input v-model="testText" type="textarea" :rows="3" placeholder="输入一段文本"/><div class="test-actions"><DsButton variant="primary" :loading="testing" @click="test">测试内容审核</DsButton></div><pre v-if="testResult" class="test-result">{{ JSON.stringify(testResult, null, 2) }}</pre></div>
    </template>
    <DsEmpty v-else title="审核 API 配置不可用" description="请检查服务状态或点击右上角配置重试。"/>
  </div>
</template>

<style scoped>
.provider-panel{display:flex;flex-direction:column;gap:18px}.metric-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.provider-card{display:flex;flex-direction:column;gap:10px;border:1px solid var(--ds-line);border-radius:var(--ds-radius-panel);background:var(--ds-panel);padding:18px}.provider-heading,.test-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.provider-card h3{margin:0;color:var(--ds-ink);font-size:16px}.provider-card p{margin:0;color:var(--ds-muted);font-size:13px}.provider-card dl{display:grid;grid-template-columns:120px 1fr;gap:8px 16px;margin:8px 0 0;font-size:13px}.provider-card dt{color:var(--ds-muted)}.provider-card dd{margin:0;color:var(--ds-ink-soft);word-break:break-all}.test-result{margin:0;padding:12px;overflow:auto;border-radius:var(--ds-radius-sm);background:var(--ds-panel-muted);color:var(--ds-ink-soft);font-size:12px}.loading-state{padding:32px;text-align:center;color:var(--ds-muted)}@media(max-width:700px){.metric-grid{grid-template-columns:1fr}.provider-card dl{grid-template-columns:1fr}.provider-heading{align-items:flex-start;flex-direction:column}}
</style>
