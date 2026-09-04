<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { DsButton, DsDrawer, DsEmpty, DsFilterBar, DsFilterField, DsInput, DsMetricCard, DsSwitch, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { toast } from 'vue-sonner'
import {
  getPromptAuditConfig,
  getPromptAuditRuntime,
  deletePromptAuditEvent,
  listPromptAuditEvents,
  probePromptAuditEndpoint,
  updatePromptAuditConfig,
  type PromptAuditConfig,
  type PromptAuditEndpointWrite,
  type PromptAuditEvent,
  type PromptAuditRuntime
} from './api'

const scannerOptions = [
  ['violent', '暴力'], ['non_violent_illegal_acts', '非暴力违法'], ['sexual_content_or_sexual_acts', '性内容'],
  ['pii', '个人敏感信息'], ['suicide_and_self_harm', '自杀与自残'], ['unethical_acts', '不道德行为'],
  ['politically_sensitive_topics', '政治敏感'], ['copyright_violation', '版权侵权'], ['jailbreak', '越狱/提示词注入']
] as const

const columns: DsTableColumn[] = [
  { key: 'created_at', title: '时间', width: 170 }, { key: 'tenant', title: '租户', mono: true },
  { key: 'model', title: '模型', mono: true }, { key: 'decision', title: '判定', width: 110 },
  { key: 'categories', title: '分类' }, { key: 'preview', title: '脱敏预览' },
  { key: 'latency', title: '延迟(ms)', width: 100, align: 'right' }, { key: 'actions', title: '操作', width: 80 }
]

const config = ref<PromptAuditConfig | null>(null)
const events = ref<PromptAuditEvent[]>([])
const total = ref(0)
const runtime = ref<PromptAuditRuntime | null>(null)
const loading = ref(false)
const saving = ref(false)
const drawerOpen = ref(false)
const filters = reactive({ tenant_id: '', user_id: '', decision: '' })
const draft = reactive({ enabled: false, mode: 'off', latest_turn_only: false, store_pass_events: false, worker_count: 4, queue_capacity: 4096, scanners: [] as string[], tenant_ids_text: '', endpoints: [] as Array<PromptAuditEndpointWrite & { has_api_key?: boolean }> })

const statusText = computed(() => !config.value?.enabled ? '已关闭' : config.value.mode === 'blocking' ? '同步阻断' : '旁路观察')
function decisionTone(value: string): 'positive' | 'warning' | 'danger' | 'info' { if (value === 'critical') return 'danger'; if (value === 'flag') return 'warning'; if (value === 'pass') return 'positive'; return 'info' }
function eventOf(row: unknown) { return row as PromptAuditEvent }
function formatTime(value: string) { return new Date(value).toLocaleString('zh-CN', { hour12: false }) }

async function loadConfig() { try { config.value = await getPromptAuditConfig() } catch { toast.error('提示词审计配置加载失败') } }
async function loadRuntime() { try { runtime.value = await getPromptAuditRuntime() } catch { toast.error('提示词审计运行状态加载失败') } }
async function loadEvents() { loading.value = true; try { const page = await listPromptAuditEvents({ tenant_id: filters.tenant_id || undefined, user_id: filters.user_id || undefined, decision: (filters.decision || undefined) as '' | 'pass' | 'flag' | 'critical' | 'error' | undefined, limit: 100 }); events.value = page.items ?? []; total.value = page.total } catch { toast.error('提示词审计事件加载失败') } finally { loading.value = false } }
function openConfig() { if (!config.value) return; const value = config.value; Object.assign(draft, { enabled: value.enabled, mode: value.mode, latest_turn_only: value.latest_turn_only, store_pass_events: value.store_pass_events, worker_count: value.worker_count, queue_capacity: value.queue_capacity, scanners: [...(value.scanners ?? [])], tenant_ids_text: (value.tenant_ids ?? []).join('\n'), endpoints: (value.endpoints ?? []).map((ep) => ({ id: ep.id, name: ep.name, base_url: ep.base_url, model: ep.model, timeout_ms: ep.timeout_ms, input_limit: ep.input_limit, enabled: ep.enabled, has_api_key: ep.has_api_key })) }); drawerOpen.value = true }
function addEndpoint() { draft.endpoints.push({ id: crypto.randomUUID(), name: 'Qwen3Guard', base_url: '', model: 'sileader/qwen3guard:0.6b', timeout_ms: 3000, input_limit: 4000, enabled: true }) }
function removeEndpoint(index: number) { draft.endpoints.splice(index, 1) }
function toggleScanner(id: string, enabled: boolean) { const set = new Set(draft.scanners); enabled ? set.add(id) : set.delete(id); draft.scanners = [...set] }
async function probe(endpoint: PromptAuditEndpointWrite) { try { const result = await probePromptAuditEndpoint(endpoint); result.ok ? toast.success('节点调用正常') : toast.error(`节点探测失败：${result.error_code || 'unknown'}`) } catch { toast.error('节点探测失败') } }
async function save() { if (!config.value) return; saving.value = true; try { const endpoints = draft.endpoints.map(({ has_api_key: _ignored, ...ep }) => ({ ...ep, api_key: ep.api_key === '' ? undefined : ep.api_key })); const saved = await updatePromptAuditConfig({ expected_config_revision: config.value.config_revision, enabled: draft.mode !== 'off' && draft.enabled, mode: draft.enabled ? draft.mode as 'off' | 'observe' | 'blocking' : 'off', latest_turn_only: draft.latest_turn_only, store_pass_events: draft.store_pass_events, worker_count: draft.worker_count, queue_capacity: draft.queue_capacity, scanners: draft.scanners, tenant_ids: draft.tenant_ids_text.split('\n').map((v) => v.trim()).filter(Boolean), endpoints }); config.value = saved; drawerOpen.value = false; toast.success('提示词审计配置已保存') } catch (error) { toast.error(error instanceof Error ? error.message : '保存失败') } finally { saving.value = false } }
async function removeEvent(id: string) { try { const result = await deletePromptAuditEvent(id); if (result.deleted) { toast.success('审计事件已删除'); await loadEvents() } } catch { toast.error('删除审计事件失败') } }

onMounted(async () => { await Promise.all([loadConfig(), loadRuntime(), loadEvents()]) })
</script>

<template>
  <div class="prompt-audit-panel">
    <div class="prompt-audit-toolbar">
      <div><h3>Qwen3Guard 提示词审计</h3><p>扫描完整客户端可控会话；仅保存 Hash 和不可恢复的脱敏预览。</p></div>
      <div class="prompt-audit-actions"><DsTag :tone="config?.enabled ? 'positive' : 'info'">{{ statusText }}</DsTag><DsButton variant="primary" @click="openConfig">配置</DsButton></div>
    </div>
    <div v-if="runtime" class="runtime-grid"><DsMetricCard label="队列" :value="`${runtime.queue_depth} / ${runtime.queue_capacity}`"/><DsMetricCard label="已处理" :value="String(runtime.processed)"/><DsMetricCard label="已阻断" :value="String(runtime.blocked)"/><DsMetricCard label="失败/丢弃" :value="`${runtime.failed} / ${runtime.dropped}`"/></div>
    <DsFilterBar>
      <DsFilterField label="租户 ID"><DsInput v-model="filters.tenant_id" placeholder="租户 ID" /></DsFilterField>
      <DsFilterField label="用户 ID"><DsInput v-model="filters.user_id" placeholder="用户 ID" /></DsFilterField>
      <DsFilterField label="判定"><el-select v-model="filters.decision" clearable placeholder="全部"><el-option label="通过" value="pass"/><el-option label="警告" value="flag"/><el-option label="阻断" value="critical"/><el-option label="错误" value="error"/></el-select></DsFilterField>
      <template #actions><span class="result-count">共 {{ total }} 条</span><DsButton @click="loadEvents">刷新</DsButton></template>
    </DsFilterBar>
    <DsTable :frame="false" :columns="columns" :rows="events" row-key="id" :loading="loading" empty-title="暂无提示词审计事件">
      <template #cell-created_at="{ row }">{{ formatTime(eventOf(row).created_at) }}</template>
      <template #cell-tenant="{ row }">{{ eventOf(row).snapshot.tenant_id || '-' }}</template>
      <template #cell-model="{ row }">{{ eventOf(row).snapshot.model_code || '-' }}</template>
      <template #cell-decision="{ row }"><DsTag :tone="decisionTone(eventOf(row).decision)">{{ eventOf(row).decision }}</DsTag></template>
      <template #cell-categories="{ row }">{{ eventOf(row).matched_scanners?.join(', ') || eventOf(row).error_code || '-' }}</template>
      <template #cell-preview="{ row }">{{ eventOf(row).snapshot.redacted_preview || '-' }}</template>
      <template #cell-latency="{ row }">{{ eventOf(row).latency_ms }}</template>
      <template #cell-actions="{ row }"><DsButton variant="link-danger" size="sm" @click="removeEvent(eventOf(row).id)">删除</DsButton></template>
      <template #empty><DsEmpty title="暂无提示词审计事件" description="开启旁路观察或同步阻断后，审计结果会显示在这里。" /></template>
    </DsTable>

    <DsDrawer :open="drawerOpen" title="提示词审计配置" subtitle="Guard 出站默认只允许 HTTPS 公网目标；API Key 不会回显。" width="720px" @close="drawerOpen = false">
      <div class="config-grid"><label>启用<DsSwitch v-model="draft.enabled" /></label><label>模式<el-select v-model="draft.mode" :disabled="!draft.enabled"><el-option label="关闭" value="off"/><el-option label="旁路观察" value="observe"/><el-option label="同步阻断" value="blocking"/></el-select></label><label>只扫描最新轮次<DsSwitch v-model="draft.latest_turn_only" /></label><label>保存通过事件<DsSwitch v-model="draft.store_pass_events" /></label><label>Worker 数量<el-input-number v-model="draft.worker_count" :min="1" :max="32" /></label><label>内存队列容量<el-input-number v-model="draft.queue_capacity" :min="1" :max="100000" /></label></div>
      <section><h4>风险分类</h4><div class="scanner-grid"><label v-for="item in scannerOptions" :key="item[0]"><DsSwitch :model-value="draft.scanners.includes(item[0])" @update:model-value="toggleScanner(item[0], $event)"/>{{ item[1] }}</label></div></section>
      <section><h4>租户范围</h4><p>留空表示所有租户；每行一个 Tenant ID。</p><el-input v-model="draft.tenant_ids_text" type="textarea" :rows="3" /></section>
      <section><div class="section-title"><h4>Guard 节点</h4><DsButton @click="addEndpoint">添加节点</DsButton></div><div v-for="(endpoint,index) in draft.endpoints" :key="endpoint.id" class="endpoint-card"><div class="config-grid"><label>名称<DsInput v-model="endpoint.name"/></label><label>Base URL<DsInput v-model="endpoint.base_url" placeholder="https://guard.example"/></label><label>模型<DsInput v-model="endpoint.model"/></label><label>API Key<DsInput v-model="endpoint.api_key" type="password" :placeholder="endpoint.has_api_key ? '已配置，留空则保留' : '可选'"/></label><label>超时(ms)<el-input-number v-model="endpoint.timeout_ms" :min="100" :max="30000"/></label><label>单片字符数<el-input-number v-model="endpoint.input_limit" :min="128" :max="100000"/></label></div><div class="endpoint-actions"><DsSwitch v-model="endpoint.enabled"/><DsButton @click="probe(endpoint)">探测</DsButton><DsButton variant="danger" @click="removeEndpoint(index)">删除</DsButton></div></div></section>
      <template #footer><DsButton @click="drawerOpen=false">取消</DsButton><DsButton variant="primary" :loading="saving" @click="save">保存</DsButton></template>
    </DsDrawer>
  </div>
</template>

<style scoped>
.prompt-audit-panel{display:flex;flex-direction:column;gap:16px}.prompt-audit-toolbar,.prompt-audit-actions,.section-title,.endpoint-actions{display:flex;align-items:center;justify-content:space-between;gap:12px}.prompt-audit-toolbar h3,section h4{margin:0;color:var(--ds-ink)}.prompt-audit-toolbar p,section p{margin:4px 0 0;color:var(--ds-muted);font-size:13px}.runtime-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.result-count{font-size:12px;color:var(--ds-muted)}.config-grid,.scanner-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.config-grid label,.scanner-grid label{display:flex;flex-direction:column;gap:6px;color:var(--ds-ink-soft);font-size:13px}.scanner-grid label{flex-direction:row;align-items:center}section{margin-top:24px}.endpoint-card{margin-top:12px;padding:16px;border:1px solid var(--ds-line);border-radius:var(--ds-radius-panel);background:var(--ds-panel-muted)}.endpoint-actions{justify-content:flex-end;margin-top:12px}@media(max-width:700px){.config-grid,.scanner-grid,.runtime-grid{grid-template-columns:1fr}.prompt-audit-toolbar{align-items:flex-start;flex-direction:column}}
</style>
