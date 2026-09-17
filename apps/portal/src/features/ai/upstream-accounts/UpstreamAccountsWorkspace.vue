<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Download, Plus, Refresh, Upload, VideoPlay } from '@element-plus/icons-vue'
import { Database, Search } from 'lucide-vue-next'

import { aiAdminApi } from '@/api/aiAdmin'
import type { AccountDTO, UpstreamAccountImportPreviewOutputBody, UpstreamAccountImportRequest, UpstreamAccountTransferAccountDTO, UpstreamAccountTestResult } from '@/api/types/ai'
import { PortalPagePanel } from '@/platform'
import { DsFilterBar, DsPagination, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { formatMultiplier } from '@/platform/ai/utils'
import type { PriceBookRecord } from '@/features/ai/price-books/pricingTypes'
import { firstActivePriceBookId } from '@/features/ai/price-books/priceBookSelection'
import { useUpstreamStability } from '@/features/ai/upstream-stability/useUpstreamStability'
import type { StabilityWindow } from '@/features/ai/upstream-stability/api'
import UpstreamAccountStatusControl from './components/UpstreamAccountStatusControl.vue'
import UpstreamAccountEditorDialog from './components/UpstreamAccountEditorDialog.vue'
import UpstreamAccountTestDialog from './components/UpstreamAccountTestDialog.vue'
import { accountEndpointHosts, availabilityLabels, availabilityTone, stabilityWindowLabels, successRateLabel, successRateTitle } from './presentation'

const route = useRoute()
const router = useRouter()
const loading = shallowRef(false)
const accounts = shallowRef<AccountDTO[]>([])
const priceBooks = shallowRef<PriceBookRecord[]>([])
const selectedAccounts = shallowRef<AccountDTO[]>([])
const updatingAccountStatusId = shallowRef('')
const search = shallowRef('')
const configStatus = shallowRef('all')
const runtimeStatus = shallowRef('all')
const page = shallowRef(1)
const pageSize = shallowRef(20)
const stabilityWindow = shallowRef<StabilityWindow>('24h')
const validWindows: StabilityWindow[] = ['1h', '24h', '7d']
const stability = useUpstreamStability('direct_upstream', undefined, stabilityWindow)

const stabilityById = computed(() => new Map(stability.items.value.filter(item => item.window === stabilityWindow.value).map(item => [item.resource_id, item])))
const filteredAccounts = computed(() => {
  const keyword = search.value.trim().toLocaleLowerCase()
  return accounts.value.filter(account => {
    if (keyword && !`${account.name} ${account.tenant_display_name || ''} ${accountEndpointHosts(account)}`.toLocaleLowerCase().includes(keyword)) return false
    if (configStatus.value !== 'all' && account.status !== configStatus.value) return false
    if (runtimeStatus.value !== 'all' && (stabilityById.value.get(account.id)?.availability || 'unknown') !== runtimeStatus.value) return false
    return true
  })
})
const totalPages = computed(() => Math.max(1, Math.ceil(filteredAccounts.value.length / pageSize.value)))
const pageRows = computed(() => filteredAccounts.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const activePriceBookId = computed(() => firstActivePriceBookId(priceBooks.value))
const selectedExportAccounts = computed(() => selectedAccounts.value.filter(selected => accounts.value.some(account => account.id === selected.id)))
const columns: DsTableColumn[] = [
  { key: 'account', title: '账号', width: 300, wrap: true },
  { key: 'config', title: '配置状态', width: 132 },
  { key: 'runtime', title: '运行状态', width: 128 },
  { key: 'endpoints', title: '端点', width: 80, align: 'right' },
  { key: 'visibility', title: '租户可见性', width: 110 },
  { key: 'multiplier', title: '倍率', width: 90, align: 'right' },
  { key: 'actions', title: '操作', width: 190, align: 'right' }
]

function stringQuery(value: unknown) { return typeof value === 'string' ? value : '' }
function positiveInt(value: unknown, fallback: number) { const parsed = Number(stringQuery(value)); return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback }
function applyRouteQuery() {
  if (route.path !== '/admin/ai/upstreams/accounts') return
  search.value = stringQuery(route.query.q)
  configStatus.value = ['active', 'disabled', 'invalid'].includes(stringQuery(route.query.status)) ? stringQuery(route.query.status) : 'all'
  runtimeStatus.value = ['available', 'partial', 'unavailable', 'unknown'].includes(stringQuery(route.query.runtime)) ? stringQuery(route.query.runtime) : 'all'
  page.value = positiveInt(route.query.page, 1)
  const size = positiveInt(route.query.size, 20)
  pageSize.value = [20, 50, 100].includes(size) ? size : 20
  const window = stringQuery(route.query.window) as StabilityWindow
  stabilityWindow.value = validWindows.includes(window) ? window : '24h'
}
function normalizedQuery() {
  return {
    ...(search.value ? { q: search.value } : {}),
    ...(configStatus.value !== 'all' ? { status: configStatus.value } : {}),
    ...(runtimeStatus.value !== 'all' ? { runtime: runtimeStatus.value } : {}),
    ...(page.value !== 1 ? { page: String(page.value) } : {}),
    ...(pageSize.value !== 20 ? { size: String(pageSize.value) } : {}),
    ...(stabilityWindow.value !== '24h' ? { window: stabilityWindow.value } : {})
  }
}
applyRouteQuery()
watch(() => route.query, applyRouteQuery)
watch([search, configStatus, runtimeStatus, pageSize], () => { page.value = 1 })
watch([search, configStatus, runtimeStatus, page, pageSize, stabilityWindow], () => {
  if (route.path !== '/admin/ai/upstreams/accounts') return
  const query = normalizedQuery()
  if (JSON.stringify(query) !== JSON.stringify(route.query)) void router.replace({ query })
})
watch(totalPages, lastPage => { if (page.value > lastPage) page.value = lastPage })

async function fetchAccounts() {
  loading.value = true
  try {
    const response = await aiAdminApi.listUpstreamAccounts()
    accounts.value = response.items || []
    const selectedIds = new Set(selectedAccounts.value.map(account => account.id))
    selectedAccounts.value = accounts.value.filter(account => selectedIds.has(account.id))
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '加载账号失败') }
  finally { loading.value = false }
}
async function fetchPriceBooks() {
  try { priceBooks.value = (await aiAdminApi.listPriceBooks()).items || [] } catch { priceBooks.value = [] }
}
async function refreshWorkspace() { await Promise.all([fetchAccounts(), stability.refresh()]) }
function updateSelection(value: unknown[]) { selectedAccounts.value = value as AccountDTO[] }
async function changeAccountStatus(account: AccountDTO, status: 'active' | 'disabled') {
  updatingAccountStatusId.value = account.id
  try { await aiAdminApi.updateUpstreamAccountStatus(account.id, status); ElMessage.success(status === 'active' ? '账号已启用' : '账号已停用'); await fetchAccounts() }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '操作失败') }
  finally { updatingAccountStatusId.value = '' }
}
function runtimeLabel(account: AccountDTO) { return stability.error.value ? '读取失败' : availabilityLabels[stabilityById.value.get(account.id)?.availability || 'unknown'] || '状态未知' }
function runtimeTone(account: AccountDTO) { return stability.error.value ? 'warning' as const : availabilityTone(stabilityById.value.get(account.id)?.availability) }
const scrollKeyPrefix = 'dai:upstream-accounts:scroll:'
function openDetail(account: AccountDTO) {
  sessionStorage.setItem(`${scrollKeyPrefix}${route.fullPath}`, String(window.scrollY || document.documentElement.scrollTop || 0))
  void router.push({ path: `/admin/ai/upstreams/accounts/${encodeURIComponent(account.id)}`, query: { from: route.fullPath } })
}

const accountDialog = shallowRef(false)
const testDialog = shallowRef(false)
const testAccount = shallowRef<AccountDTO | null>(null)
function openTest(account: AccountDTO) { testAccount.value = account; testDialog.value = true }
async function accountTested(result: UpstreamAccountTestResult) { if (result.ok || [401, 403].includes(result.http_status)) await fetchAccounts() }

const exportDialog = shallowRef(false)
const exportIncludeModelBindings = shallowRef(true)
const exportingAccounts = shallowRef(false)
const importDialog = shallowRef(false)
const importFileInput = ref<HTMLInputElement | null>(null)
const importFileName = shallowRef('')
const importAccounts = shallowRef<UpstreamAccountTransferAccountDTO[]>([])
const importPreview = shallowRef<UpstreamAccountImportPreviewOutputBody | null>(null)
const previewingImport = shallowRef(false)
const importingAccounts = shallowRef(false)
let importPreviewGeneration = 0
const importSettings = reactive({ default_price_book_id: '', default_tenant_multiplier: 1 })
watch(activePriceBookId, nextId => {
  if (!nextId || !importDialog.value || importSettings.default_price_book_id) return
  importSettings.default_price_book_id = nextId
  if (importAccounts.value.length) void refreshImportPreview()
})
function openExportDialog() { if (!selectedExportAccounts.value.length) { ElMessage.warning('请先选择要导出的上游账号'); return }; exportDialog.value = true }
async function confirmExportAccounts() {
  exportingAccounts.value = true
  try {
    const data = await aiAdminApi.exportUpstreamAccounts({ account_ids: selectedExportAccounts.value.map(account => account.id), include_model_bindings: exportIncludeModelBindings.value })
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json;charset=utf-8' })
    const url = URL.createObjectURL(blob); const link = document.createElement('a')
    link.href = url; link.download = `upstream-accounts-${new Date().toISOString().slice(0, 10)}.json`; document.body.appendChild(link); link.click(); document.body.removeChild(link); URL.revokeObjectURL(url)
    exportDialog.value = false; ElMessage.success('导出文件已生成')
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '导出失败') }
  finally { exportingAccounts.value = false }
}
function openImportDialog() {
  importPreviewGeneration += 1; importFileName.value = ''; importAccounts.value = []; importPreview.value = null; previewingImport.value = false
  importSettings.default_price_book_id = activePriceBookId.value; importSettings.default_tenant_multiplier = 1; importDialog.value = true
}
function parseImportAccounts(parsed: any): UpstreamAccountTransferAccountDTO[] {
  if (!Array.isArray(parsed) && parsed?.schema_version !== undefined && ![5, 6].includes(parsed.schema_version)) throw new Error(`不支持的上游账号导入版本：${parsed.schema_version}；当前支持 schema_version 5/6`)
  const values = Array.isArray(parsed) ? parsed : parsed?.accounts
  if (!Array.isArray(values) || !values.length) throw new Error('导入文件缺少 accounts 数组')
  return values
}
function buildImportRequest(): UpstreamAccountImportRequest {
  return { accounts: importAccounts.value, default_price_book_id: importSettings.default_price_book_id || undefined, default_tenant_multiplier: importSettings.default_tenant_multiplier || 1, duplicate_account_strategy: 'skip', duplicate_binding_strategy: 'skip' }
}
async function refreshImportPreview() {
  if (!importAccounts.value.length) return
  const generation = ++importPreviewGeneration; previewingImport.value = true
  try { const preview = await aiAdminApi.previewImportUpstreamAccounts(buildImportRequest()); if (generation === importPreviewGeneration) importPreview.value = preview }
  catch (error) { if (generation === importPreviewGeneration) { importPreview.value = null; ElMessage.error(error instanceof Error ? error.message : '导入预检失败') } }
  finally { if (generation === importPreviewGeneration) previewingImport.value = false }
}
async function handleImportFileSelected(event: Event) {
  const input = event.target as HTMLInputElement; const file = input.files?.[0]; if (!file) return
  importPreviewGeneration += 1
  try { importAccounts.value = parseImportAccounts(JSON.parse(await file.text())); importFileName.value = file.name; await refreshImportPreview() }
  catch (error) { importFileName.value = ''; importAccounts.value = []; importPreview.value = null; ElMessage.error(error instanceof Error ? error.message : '导入文件解析失败') }
  finally { input.value = '' }
}
async function confirmImportAccounts() {
  if (!importAccounts.value.length || !importPreview.value?.summary.create_accounts) return
  importingAccounts.value = true
  try { const result = await aiAdminApi.importUpstreamAccounts(buildImportRequest()); ElMessage.success(`已导入 ${result.summary.create_accounts} 个账号，${result.summary.create_model_bindings} 条模型绑定`); importDialog.value = false; await fetchAccounts() }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '导入失败') }
  finally { importingAccounts.value = false }
}
function importActionTone(action: string): 'positive' | 'danger' | 'info' { return action === 'create' ? 'positive' : action === 'error' ? 'danger' : 'info' }
function importActionLabel(action: string) { return action === 'create' ? '创建' : action === 'error' ? '错误' : '跳过' }
const importPreviewColumns: DsTableColumn[] = [
  { key: 'name', title: '账号' }, { key: 'endpoint_count', title: '请求端点', width: 96, align: 'right' }, { key: 'action', title: '动作', width: 86 },
  { key: 'model_binding_count', title: '模型绑定', width: 96, align: 'right' }, { key: 'reason', title: '说明' }
]
onMounted(async () => {
  await Promise.all([fetchAccounts(), fetchPriceBooks()]); await nextTick()
  const saved = sessionStorage.getItem(`${scrollKeyPrefix}${route.fullPath}`)
  if (saved) requestAnimationFrame(() => window.scrollTo({ top: Number(saved), behavior: 'auto' }))
})
</script>

<template>
  <div class="page-container accounts-view">
    <PortalPagePanel :icon="Database" :breadcrumbs="[{ label: '智能服务' }, { label: '网关配置' }, { label: '上游账号' }]" description="查找、比较和管理上游账号；进入详情可配置端点、模型及查看运行诊断。" fill>
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="refreshWorkspace">刷新</el-button>
        <el-button :icon="Upload" @click="openImportDialog">导入</el-button>
        <el-button :icon="Download" :disabled="!selectedExportAccounts.length" @click="openExportDialog">导出<span v-if="selectedExportAccounts.length">（{{ selectedExportAccounts.length }}）</span></el-button>
        <el-button type="primary" :icon="Plus" @click="accountDialog = true">新增账号</el-button>
      </template>
      <template #filters>
        <DsFilterBar>
          <label class="filter-field filter-field--search"><span>账号或域名</span><el-input v-model="search" clearable placeholder="搜索名称、展示名称或域名"><template #prefix><Search :size="14" /></template></el-input></label>
          <label class="filter-field"><span>配置状态</span><el-select v-model="configStatus"><el-option label="全部" value="all" /><el-option label="已启用" value="active" /><el-option label="已停用" value="disabled" /><el-option label="凭据失效" value="invalid" /></el-select></label>
          <label class="filter-field"><span>运行状态</span><el-select v-model="runtimeStatus"><el-option label="全部" value="all" /><el-option label="全部可用" value="available" /><el-option label="部分受限" value="partial" /><el-option label="全部受限" value="unavailable" /><el-option label="状态未知" value="unknown" /></el-select></label>
          <label class="filter-field"><span>成功率统计</span><el-select v-model="stabilityWindow"><el-option v-for="(label, value) in stabilityWindowLabels" :key="value" :label="label" :value="value" /></el-select></label>
        </DsFilterBar>
        <el-alert v-if="stability.error.value" class="stability-alert" title="成功率与运行状态加载失败" :description="stability.error.value" type="warning" :closable="false" />
      </template>
      <div class="accounts-table-shell">
        <DsTable :columns="columns" :rows="pageRows" row-key="id" :loading="loading" selectable :selection="selectedAccounts" :frame="false" aria-label="上游账号列表" empty-title="没有符合条件的上游账号" empty-description="调整筛选条件，或新增一个上游账号。" @update:selection="updateSelection">
          <template #cell-account="{ row }"><div class="account-cell"><div class="account-cell__title"><button type="button" @click="openDetail(row)">{{ row.name }}</button><DsTag v-if="!stability.error.value && stabilityById.get(row.id)?.success_rate != null" tone="neutral" :title="successRateTitle(stabilityById.get(row.id)!, stabilityWindow)">成功率 {{ successRateLabel(stabilityById.get(row.id)?.success_rate) }}</DsTag><span v-if="(stabilityById.get(row.id)?.samples || 0) > 0 && (stabilityById.get(row.id)?.samples || 0) < 20" class="sample-hint">样本较少</span></div><span class="account-cell__host" :title="accountEndpointHosts(row)">{{ accountEndpointHosts(row) || '尚未配置域名' }}</span></div></template>
          <template #cell-config="{ row }"><UpstreamAccountStatusControl :status="row.status" :invalid-reason="row.invalid_reason" :loading="updatingAccountStatusId === row.id" @change="changeAccountStatus(row, $event)" @verify="openTest(row)" /></template>
          <template #cell-runtime="{ row }"><DsTag :tone="runtimeTone(row)">{{ runtimeLabel(row) }}</DsTag></template>
          <template #cell-endpoints="{ row }"><span class="numeric-cell">{{ row.endpoints?.length || 0 }}</span></template>
          <template #cell-visibility="{ row }"><DsTag :tone="row.tenant_access_mode === 'restricted' ? 'warning' : 'positive'">{{ row.tenant_access_mode === 'restricted' ? '专属' : '公开' }}</DsTag></template>
          <template #cell-multiplier="{ row }"><span class="numeric-cell">{{ formatMultiplier(row.tenant_multiplier) }}</span></template>
          <template #cell-actions="{ row }"><div class="row-actions"><el-button link type="primary" @click="openDetail(row)">查看详情</el-button><el-button link type="success" :icon="VideoPlay" @click="openTest(row)">测试连通</el-button></div></template>
        </DsTable>
      </div>
      <template #pagination><DsPagination v-model:page="page" v-model:page-size="pageSize" :page-sizes="[20, 50, 100]" :total="filteredAccounts.length" /></template>
    </PortalPagePanel>

    <UpstreamAccountEditorDialog v-model="accountDialog" :price-books="priceBooks" @saved="fetchAccounts" />
    <UpstreamAccountTestDialog v-model="testDialog" :account="testAccount" @tested="accountTested" />
    <el-dialog v-model="exportDialog" title="导出上游账号" width="560px">
      <el-alert title="导出文件会包含上游 API key 明文，请只在可信环境保存和传输。" type="warning" show-icon :closable="false" />
      <div class="transfer-panel"><div class="transfer-summary"><span>已选择</span><strong>{{ selectedExportAccounts.length }}</strong><span>个上游账号</span></div><div class="transfer-tags"><el-tag v-for="account in selectedExportAccounts" :key="account.id" size="small">{{ account.name }}</el-tag></div><el-checkbox v-model="exportIncludeModelBindings">同时导出显式模型绑定</el-checkbox></div>
      <template #footer><el-button @click="exportDialog = false">取消</el-button><el-button type="primary" :loading="exportingAccounts" @click="confirmExportAccounts">导出 JSON</el-button></template>
    </el-dialog>
    <el-dialog v-model="importDialog" title="导入上游账号" width="780px">
      <el-alert title="导入文件中的 API key 会按当前系统密钥重新加密；价格表不从文件继承，需要在本窗口选择。" type="warning" show-icon :closable="false" />
      <div class="import-file-row"><el-button :icon="Upload" @click="importFileInput?.click()">选择 JSON 文件</el-button><span>{{ importFileName || '未选择文件' }}</span><input ref="importFileInput" type="file" accept="application/json,.json" class="hidden-file-input" @change="handleImportFileSelected" /></div>
      <el-form v-if="importAccounts.length" label-width="110px" class="import-form"><el-form-item label="价格表"><el-select v-model="importSettings.default_price_book_id" clearable class="w-full" :placeholder="activePriceBookId ? '不绑定价格表' : '暂无启用价格表'" @change="refreshImportPreview"><el-option v-for="book in priceBooks" :key="book.id" :label="book.name" :value="book.id" /></el-select></el-form-item><el-form-item label="租户倍率"><el-input-number v-model="importSettings.default_tenant_multiplier" :min="0" :step="0.1" :precision="4" @change="refreshImportPreview" /></el-form-item></el-form>
      <div v-if="importPreview" v-loading="previewingImport" class="import-preview"><div class="import-stats"><span>创建账号 <strong>{{ importPreview.summary.create_accounts }}</strong></span><span>跳过账号 <strong>{{ importPreview.summary.skip_accounts }}</strong></span><span>错误账号 <strong>{{ importPreview.summary.error_accounts }}</strong></span><span>创建模型绑定 <strong>{{ importPreview.summary.create_model_bindings }}</strong></span></div><DsTable :columns="importPreviewColumns" :rows="importPreview.items" row-key="name" empty-title="暂无预检结果"><template #cell-action="{ row }"><DsTag :tone="importActionTone(row.action)">{{ importActionLabel(row.action) }}</DsTag></template><template #cell-reason="{ row }">{{ row.reason || row.warnings?.join('；') || '—' }}</template></DsTable></div>
      <template #footer><el-button @click="importDialog = false">取消</el-button><el-button type="primary" :loading="importingAccounts" :disabled="!importPreview?.summary.create_accounts" @click="confirmImportAccounts">导入可创建项</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.accounts-view { display: flex; min-height: 0; flex: 1; flex-direction: column; }
.filter-field { display: flex; width: 170px; min-width: 0; flex-direction: column; gap: 6px; color: var(--ds-muted); font-size: 12px; font-weight: 600; }
.filter-field--search { width: min(320px, 100%); }
.filter-field :deep(.el-select), .filter-field :deep(.el-input) { width: 100%; }
.stability-alert { margin-top: 12px; }
.accounts-table-shell { overflow: auto; height: 100%; min-height: 0; }
.account-cell { display: flex; min-width: 0; flex-direction: column; gap: 5px; }
.account-cell__title { display: flex; min-width: 0; flex-wrap: wrap; align-items: center; gap: 6px; }
.account-cell__title button { overflow: hidden; max-width: 190px; padding: 0; border: 0; background: transparent; color: var(--ds-accent); font: inherit; font-weight: 650; text-align: left; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.account-cell__host { overflow: hidden; color: var(--ds-faint); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.sample-hint { color: var(--ds-warning); font-size: 11px; white-space: nowrap; }
.numeric-cell { color: var(--ds-ink-soft); font-variant-numeric: tabular-nums; }
.row-actions { display: flex; justify-content: flex-end; white-space: nowrap; }
.transfer-panel { display: flex; flex-direction: column; gap: 14px; margin-top: 16px; }
.transfer-summary { display: inline-flex; align-items: baseline; gap: 6px; color: var(--ds-muted); }
.transfer-summary strong { color: var(--ds-ink); font-size: 20px; }
.transfer-tags { display: flex; flex-wrap: wrap; gap: 8px; }
.import-file-row { display: flex; align-items: center; gap: 12px; margin: 16px 0; color: var(--ds-muted); font-size: 13px; }
.hidden-file-input { display: none; }
.import-form { padding: 12px 0 2px; border-top: 1px solid var(--ds-line); }
.import-preview { overflow: auto; max-height: 360px; margin-top: 10px; }
.import-stats { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 12px; color: var(--ds-muted); font-size: 13px; }
.import-stats span { padding: 6px 10px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.import-stats strong { color: var(--ds-ink); }
@media (max-width: 768px) { .filter-field, .filter-field--search { width: 100%; } .accounts-table-shell { min-height: 480px; } }
</style>
