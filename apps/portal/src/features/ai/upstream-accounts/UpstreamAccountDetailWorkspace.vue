<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Delete, Edit, MoreFilled, Plus, VideoPlay } from '@element-plus/icons-vue'
import { Database } from 'lucide-vue-next'

import { aiAdminApi } from '@/api/aiAdmin'
import type { AccountDTO, UpstreamAccountEndpointDTO, UpstreamAccountEndpointWriteRequest, UpstreamAccountTestResult } from '@/api/types/ai'
import { PortalContentCard, PortalPagePanel } from '@/platform'
import { DsEmpty, DsTabs, DsTag } from '@/shared/ui'
import { formatMultiplier } from '@/platform/ai/utils'
import type { PriceBookRecord } from '@/features/ai/price-books/pricingTypes'
import UpstreamModelBindingsPanel from '@/features/ai/upstream-model-bindings/UpstreamModelBindingsPanel.vue'
import UpstreamStabilityPanel from '@/features/ai/upstream-stability/UpstreamStabilityPanel.vue'
import type { Stability, StabilityWindow } from '@/features/ai/upstream-stability/api'
import UpstreamAccountEditorDialog from './components/UpstreamAccountEditorDialog.vue'
import UpstreamAccountTestDialog from './components/UpstreamAccountTestDialog.vue'
import UpstreamAccountStatusControl from './components/UpstreamAccountStatusControl.vue'
import KeyValueEditor from './components/KeyValueEditor.vue'
import { upstreamAccountStatusLabel, upstreamAccountStatusTagType } from './components/status'
import { endpointAuthSchemeOptions, upstreamAPIFormatLabel, upstreamAPIFormatOptions } from './constants'
import { availabilityLabels, availabilityTone, endpointSuccessRates, successRateLabel, successRateTitle } from './presentation'

const route = useRoute()
const router = useRouter()
const accountId = computed(() => String(route.params.accountId || ''))
const accounts = shallowRef<AccountDTO[]>([])
const priceBooks = shallowRef<PriceBookRecord[]>([])
const loading = shallowRef(true)
const loadError = shallowRef('')
const runtimeSnapshot = shallowRef<Stability | null>(null)
const runtimeError = shallowRef('')
const account = computed(() => accounts.value.find(item => item.id === accountId.value) || null)
const priceBookName = computed(() => priceBooks.value.find(book => book.id === account.value?.price_book_id)?.name || '—')
const endpointRates = computed(() => runtimeError.value ? new Map<string, number>() : endpointSuccessRates(runtimeSnapshot.value))
const activeTab = shallowRef('overview')
const stabilityWindow = shallowRef<StabilityWindow>('24h')
const tabs = [
  { key: 'overview', label: '账号概览' },
  { key: 'endpoints', label: '请求端点' },
  { key: 'models', label: '模型绑定' },
  { key: 'runtime', label: '运行诊断' }
]
const validTabs = new Set(tabs.map(tab => tab.key))
const validWindows: StabilityWindow[] = ['1h', '24h', '7d']

function stringQuery(value: unknown) { return typeof value === 'string' ? value : '' }
function applyRouteQuery() {
  const tab = stringQuery(route.query.tab)
  activeTab.value = validTabs.has(tab) ? tab : 'overview'
  const window = stringQuery(route.query.window) as StabilityWindow
  stabilityWindow.value = validWindows.includes(window) ? window : '24h'
}
function updateRouteQuery() {
  const query = {
    ...(stringQuery(route.query.from) ? { from: stringQuery(route.query.from) } : {}),
    ...(activeTab.value !== 'overview' ? { tab: activeTab.value } : {}),
    ...(stabilityWindow.value !== '24h' ? { window: stabilityWindow.value } : {})
  }
  if (JSON.stringify(query) !== JSON.stringify(route.query)) void router.replace({ query })
}
applyRouteQuery()
watch(() => route.query, applyRouteQuery)
watch([activeTab, stabilityWindow], updateRouteQuery)
watch(accountId, () => { runtimeSnapshot.value = null; runtimeError.value = ''; void fetchData() })

async function fetchData() {
  loading.value = true
  loadError.value = ''
  try {
    const [accountResponse, priceBookResponse] = await Promise.all([aiAdminApi.listUpstreamAccounts(), aiAdminApi.listPriceBooks()])
    accounts.value = accountResponse.items || []
    priceBooks.value = priceBookResponse.items || []
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '加载上游账号失败'
  } finally {
    loading.value = false
  }
}

function listReturnPath() {
  const from = stringQuery(route.query.from)
  return from.startsWith('/admin/ai/upstreams/accounts') && !from.includes(`/${accountId.value}`) ? from : '/admin/ai/upstreams/accounts'
}
function backToList() { void router.push(listReturnPath()) }
function statusTone(status?: string): 'positive' | 'danger' | 'info' {
  const type = upstreamAccountStatusTagType(status || '')
  return type === 'success' ? 'positive' : type
}
function runtimeLabel() {
  if (runtimeError.value) return '读取失败'
  if (!runtimeSnapshot.value) return '状态未知'
  return availabilityLabels[runtimeSnapshot.value.availability] || '状态未知'
}
function accountAPIFormats(value: AccountDTO) { return (value.endpoints || []).map(endpoint => upstreamAPIFormatLabel(endpoint.api_format)).join('、') || '—' }

const updatingStatus = shallowRef(false)
async function changeStatus(status: 'active' | 'disabled') {
  if (!account.value) return
  updatingStatus.value = true
  try { await aiAdminApi.updateUpstreamAccountStatus(account.value.id, status); ElMessage.success(status === 'active' ? '账号已启用' : '账号已停用'); await fetchData() }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '操作失败') }
  finally { updatingStatus.value = false }
}

const editorDialog = shallowRef(false)
const testDialog = shallowRef(false)
async function editorSaved() { await fetchData() }
async function accountTested(result: UpstreamAccountTestResult) { if (result.ok || [401, 403].includes(result.http_status)) await fetchData() }
async function removeAccount() {
  if (!account.value) return
  try { await ElMessageBox.confirm(`删除上游账号「${account.value.name}」？分组对它的关联会一并解除。`, '确认删除', { type: 'warning' }) } catch { return }
  try { await aiAdminApi.deleteUpstreamAccount(account.value.id); ElMessage.success('已删除'); await router.replace(listReturnPath()) }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '删除失败') }
}
function handleMoreCommand(command: string | number | object) { if (command === 'delete') void removeAccount() }

type EndpointDraft = Omit<UpstreamAccountEndpointWriteRequest, 'extra_headers'> & { id?: string; extra_headers?: Record<string, unknown> }
function blankEndpoint(): EndpointDraft {
  return { api_format: 'openai_responses', base_url: '', path_override: '', auth_scheme: 'format_default', auth_header: '', extra_headers: {}, status: 'active' }
}
const endpointDialog = shallowRef(false)
const editingEndpointId = shallowRef('')
const submittingEndpoint = shallowRef(false)
const endpointForm = reactive<EndpointDraft>(blankEndpoint())
const isEditingEndpoint = computed(() => Boolean(editingEndpointId.value))
function openEndpointCreate() {
  if (!account.value) return
  const used = new Set((account.value.endpoints || []).map(endpoint => endpoint.api_format))
  const next = upstreamAPIFormatOptions.find(option => !used.has(option.value))
  if (!next) { ElMessage.info('该账号已配置全部 API 格式'); return }
  editingEndpointId.value = ''; Object.assign(endpointForm, blankEndpoint(), { api_format: next.value }); endpointDialog.value = true
}
function openEndpointEdit(endpoint: UpstreamAccountEndpointDTO) {
  editingEndpointId.value = endpoint.id
  Object.assign(endpointForm, blankEndpoint(), { ...endpoint, extra_headers: endpoint.extra_headers && typeof endpoint.extra_headers === 'object' ? { ...endpoint.extra_headers as object } : {} })
  endpointDialog.value = true
}
function endpointFormatDisabled(format: string) { return (account.value?.endpoints || []).some(endpoint => endpoint.api_format === format && endpoint.id !== editingEndpointId.value) }
function endpointPayload(): UpstreamAccountEndpointWriteRequest {
  return { api_format: endpointForm.api_format, base_url: endpointForm.base_url.trim(), path_override: endpointForm.path_override?.trim() || undefined, auth_scheme: endpointForm.auth_scheme || 'format_default', auth_header: endpointForm.auth_scheme === 'custom_header' ? endpointForm.auth_header?.trim() : undefined, extra_headers: endpointForm.extra_headers && Object.keys(endpointForm.extra_headers).length ? endpointForm.extra_headers : undefined, status: endpointForm.status || 'active' }
}
function validateEndpoint() {
  if (!endpointForm.base_url.trim()) { ElMessage.warning('请填写 Base URL'); return false }
  if (endpointForm.auth_scheme === 'custom_header' && !endpointForm.auth_header?.trim()) { ElMessage.warning('自定义认证方式必须填写请求头名称'); return false }
  return true
}
async function submitEndpoint() {
  if (!account.value || !validateEndpoint()) return
  submittingEndpoint.value = true
  try {
    if (isEditingEndpoint.value) await aiAdminApi.updateUpstreamAccountEndpoint(account.value.id, editingEndpointId.value, endpointPayload())
    else await aiAdminApi.createUpstreamAccountEndpoint(account.value.id, endpointPayload())
    endpointDialog.value = false; ElMessage.success(isEditingEndpoint.value ? '请求端点已更新' : '请求端点已添加'); await fetchData()
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '保存请求端点失败') }
  finally { submittingEndpoint.value = false }
}
async function removeEndpoint(endpoint: UpstreamAccountEndpointDTO) {
  if (!account.value) return
  const endpoints = account.value.endpoints || []
  if (endpoints.length <= 1) { ElMessage.warning('账号至少需要一个请求端点'); return }
  if (account.value.status === 'active' && endpoint.status === 'active' && endpoints.filter(item => item.status === 'active').length <= 1) { ElMessage.warning('启用账号至少需要一个启用中的请求端点'); return }
  try { await ElMessageBox.confirm(`删除请求端点「${upstreamAPIFormatLabel(endpoint.api_format)}」？`, '确认删除', { type: 'warning' }) } catch { return }
  try { await aiAdminApi.deleteUpstreamAccountEndpoint(account.value.id, endpoint.id); ElMessage.success('请求端点已删除'); await fetchData() }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '删除请求端点失败') }
}

onMounted(fetchData)
</script>

<template>
  <div class="page-container account-detail-view">
    <PortalPagePanel
      :icon="Database"
      :breadcrumbs="[{ label: '上游账号', to: '/admin/ai/upstreams/accounts' }, { label: account?.name || '账号详情' }]"
      :description="account ? '配置账号连接并查看真实上游尝试的聚合运行信息。' : '加载上游账号信息。'"
      fill
    >
      <template #actions>
        <el-button :icon="ArrowLeft" @click="backToList">返回列表</el-button>
        <template v-if="account">
          <el-button type="success" plain :icon="VideoPlay" @click="testDialog = true">测试连通</el-button>
          <el-button :icon="Edit" @click="editorDialog = true">编辑账号</el-button>
          <el-dropdown trigger="click" @command="handleMoreCommand">
            <el-button :icon="MoreFilled">更多</el-button>
            <template #dropdown><el-dropdown-menu><el-dropdown-item command="delete" :icon="Delete">删除账号</el-dropdown-item></el-dropdown-menu></template>
          </el-dropdown>
        </template>
      </template>

      <div v-if="loading" class="detail-loading">正在加载账号详情…</div>
      <el-alert v-else-if="loadError" title="账号详情加载失败" :description="loadError" type="warning" :closable="false" />
      <DsEmpty v-else-if="!account" title="上游账号不存在" description="账号可能已被删除，或当前地址中的账号 ID 无效。"><template #action><el-button type="primary" @click="backToList">返回账号列表</el-button></template></DsEmpty>
      <div v-else class="detail-content">
        <section class="detail-summary">
          <div class="detail-summary__identity">
            <div class="detail-summary__name"><strong>{{ account.name }}</strong><DsTag v-if="!runtimeError && runtimeSnapshot?.success_rate != null" tone="neutral" :title="successRateTitle(runtimeSnapshot, stabilityWindow)">成功率 {{ successRateLabel(runtimeSnapshot.success_rate) }}</DsTag><span v-if="!runtimeError && runtimeSnapshot && runtimeSnapshot.samples > 0 && runtimeSnapshot.samples < 20" class="sample-hint">样本较少</span></div>
            <span>{{ account.tenant_display_name || account.name }}</span>
          </div>
          <div class="detail-summary__states">
            <DsTag :tone="statusTone(account.status)">{{ upstreamAccountStatusLabel(account.status) }}</DsTag>
            <DsTag :tone="runtimeError ? 'warning' : availabilityTone(runtimeSnapshot?.availability)">{{ runtimeLabel() }}</DsTag>
            <UpstreamAccountStatusControl :status="account.status" :invalid-reason="account.invalid_reason" :loading="updatingStatus" @change="changeStatus" @verify="testDialog = true" />
          </div>
        </section>
        <DsTabs v-model="activeTab" :tabs="tabs" />

        <div v-show="activeTab === 'overview'" class="detail-pane">
          <PortalContentCard title="账号概览" description="账号对内连接、对外展示及结算配置。">
            <el-descriptions :column="2" size="small" border>
              <el-descriptions-item label="账号描述" :span="2"><span :class="{ muted: !account.description }">{{ account.description || '暂无描述' }}</span></el-descriptions-item>
              <el-descriptions-item label="展示名称">{{ account.tenant_display_name || account.name }}</el-descriptions-item>
              <el-descriptions-item label="租户可见性">{{ account.tenant_access_mode === 'restricted' ? '专属' : '公开' }}</el-descriptions-item>
              <el-descriptions-item label="价格表">{{ priceBookName }}</el-descriptions-item>
              <el-descriptions-item label="租户倍率">{{ formatMultiplier(account.tenant_multiplier) }}</el-descriptions-item>
              <el-descriptions-item label="最大并发">{{ account.concurrency_limit ? `${account.concurrency_limit} 并发` : '不限制' }}</el-descriptions-item>
              <el-descriptions-item label="请求端点">{{ account.endpoints?.length || 0 }} 个</el-descriptions-item>
              <el-descriptions-item label="API 格式" :span="2">{{ accountAPIFormats(account) }}</el-descriptions-item>
              <el-descriptions-item v-if="account.status === 'invalid'" label="失效原因" :span="2">{{ account.invalid_reason || '上游拒绝了账号凭据' }}</el-descriptions-item>
            </el-descriptions>
          </PortalContentCard>
        </div>

        <div v-show="activeTab === 'endpoints'" class="detail-pane">
          <PortalContentCard title="请求端点" description="配置完整上游地址、协议格式与认证方式；每种 API 格式只能配置一个端点。">
            <template #actions><el-button type="primary" :icon="Plus" @click="openEndpointCreate">添加端点</el-button></template>
            <el-table :data="account.endpoints || []" border stripe>
              <el-table-column label="API 格式" min-width="180"><template #default="{ row }">{{ upstreamAPIFormatLabel(row.api_format) }}</template></el-table-column>
              <el-table-column prop="base_url" label="Base URL" min-width="300" show-overflow-tooltip />
              <el-table-column prop="path_override" label="路径覆盖" min-width="160"><template #default="{ row }">{{ row.path_override || '使用格式默认路径' }}</template></el-table-column>
              <el-table-column label="成功率" width="110" align="right"><template #default="{ row }"><span class="numeric">{{ successRateLabel(endpointRates.get(row.id)) }}</span></template></el-table-column>
              <el-table-column label="状态" width="105"><template #default="{ row }"><DsTag :tone="row.status === 'active' ? (row.health_status === 'unhealthy' ? 'danger' : 'positive') : 'info'">{{ row.status === 'active' ? (row.health_status === 'unhealthy' ? '异常' : '启用') : '停用' }}</DsTag></template></el-table-column>
              <el-table-column label="操作" width="130" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openEndpointEdit(row)">编辑</el-button><el-button link type="danger" @click="removeEndpoint(row)">删除</el-button></template></el-table-column>
            </el-table>
          </PortalContentCard>
        </div>

        <div v-show="activeTab === 'models'" class="detail-pane"><UpstreamModelBindingsPanel target-kind="account" :target-id="account.id" title="上游模型绑定" description="显式声明该账号可用的上游模型及能力。" import-dialog-title="发现上游模型" import-alert-title="从上游 /v1/models 拉取，勾选后创建该账号的显式上游模型绑定。" /></div>
        <div v-show="activeTab === 'runtime'" class="detail-pane"><UpstreamStabilityPanel kind="direct_upstream" :resource-id="account.id" :window="stabilityWindow" @update:window="stabilityWindow = $event" @update:error="runtimeError = $event" @snapshot="runtimeSnapshot = $event" /></div>
      </div>
    </PortalPagePanel>

    <UpstreamAccountEditorDialog v-model="editorDialog" :account="account" :price-books="priceBooks" @saved="editorSaved" />
    <UpstreamAccountTestDialog v-model="testDialog" :account="account" @tested="accountTested" />
    <el-dialog v-model="endpointDialog" :title="isEditingEndpoint ? '编辑请求端点' : '添加请求端点'" width="640px">
      <el-form label-width="120px">
        <el-form-item label="API 格式" required><el-select v-model="endpointForm.api_format" class="w-full"><el-option v-for="option in upstreamAPIFormatOptions" :key="option.value" :label="option.label" :value="option.value" :disabled="endpointFormatDisabled(option.value)" /></el-select></el-form-item>
        <el-form-item label="Base URL" required><el-input v-model="endpointForm.base_url" placeholder="https://api.example.com" /></el-form-item>
        <el-form-item label="路径覆盖"><el-input v-model="endpointForm.path_override" placeholder="留空使用该 API 格式的默认路径" /></el-form-item>
        <el-form-item label="认证方式"><el-select v-model="endpointForm.auth_scheme" class="w-full"><el-option v-for="option in endpointAuthSchemeOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select></el-form-item>
        <el-form-item v-if="endpointForm.auth_scheme === 'custom_header'" label="认证请求头" required><el-input v-model="endpointForm.auth_header" placeholder="如 X-API-Key" /></el-form-item>
        <el-form-item label="状态"><el-radio-group v-model="endpointForm.status"><el-radio value="active">启用</el-radio><el-radio value="disabled">停用</el-radio></el-radio-group></el-form-item>
        <el-form-item label="附加请求头"><KeyValueEditor v-model="endpointForm.extra_headers" /><span class="hint">敏感值显示为 ***REDACTED***；保持该值会保留原配置。</span></el-form-item>
      </el-form>
      <template #footer><el-button @click="endpointDialog = false">取消</el-button><el-button type="primary" :loading="submittingEndpoint" @click="submitEndpoint">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.account-detail-view { display: flex; min-height: 0; flex: 1; flex-direction: column; }
.detail-loading { padding: 48px 20px; color: var(--ds-muted); text-align: center; }
.detail-content { display: flex; overflow-y: auto; min-height: 0; flex: 1; flex-direction: column; gap: 16px; padding: 20px 24px 24px; }
.detail-summary { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 16px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel-muted); }
.detail-summary__identity { display: flex; min-width: 0; flex-direction: column; gap: 5px; }
.detail-summary__identity > span { color: var(--ds-muted); font-size: 12px; }
.detail-summary__name { display: flex; min-width: 0; flex-wrap: wrap; align-items: center; gap: 8px; }
.detail-summary__name strong { max-width: 100%; color: var(--ds-ink); font-size: 16px; overflow-wrap: anywhere; }
.detail-summary__states { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 8px; }
.detail-pane { min-width: 0; }
.sample-hint { color: var(--ds-warning); font-size: 11px; }
.muted, .hint { color: var(--ds-faint); }
.hint { margin-left: 8px; font-size: 12px; }
.numeric { font-variant-numeric: tabular-nums; }
@media (max-width: 768px) { .detail-content { padding: 16px; } .detail-summary { align-items: flex-start; flex-direction: column; } .detail-summary__states { justify-content: flex-start; } }
</style>
