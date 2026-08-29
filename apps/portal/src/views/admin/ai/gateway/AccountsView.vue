<!--
  上游账号 — 智能服务 / AI 网关：维护上游连接、公共模型、价格表和租户倍率。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       账号操作收进页头 #actions,主从布局置于同卡 body 的 24px 容器）;
       列表空态用 DsEmpty,状态徽章 el-tag → DsTag,导入预检 el-table → DsTable;
       listUpstreamAccounts 一次返回全量账号、无分页参数,故本页不渲染分页。
       弹窗/抽屉/表单仍为 element-plus(过渡期)。
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Download, Edit, Plus, Refresh, Upload, VideoPlay } from '@element-plus/icons-vue'
import { Database } from 'lucide-vue-next'
import { PortalContentCard, PortalPagePanel } from '@/platform'
import { DsEmpty, DsNumberInput, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { formatMultiplier } from '@/platform/ai/utils'
import { aiAdminApi } from '@/api/aiAdmin'
import type {
  UpstreamAccountImportPreviewOutputBody,
  UpstreamAccountImportRequest,
  UpstreamAccountTransferAccountDTO,
  UpstreamAccountTestImage,
  UpstreamAccountTestResult
} from '@/api/types/ai'
import { defaultBindingProtocolForProviderFamily, endpointProtocolOptions } from './constants'
import { firstActivePriceBookId } from './priceBookSelection'
import type { PriceBookRecord } from './pricingTypes'
import KeyValueEditor from './components/KeyValueEditor.vue'
import UpstreamImageTestUpload from './components/UpstreamImageTestUpload.vue'
import UpstreamModelBindingsPanel from '@/features/ai/upstream-model-bindings/UpstreamModelBindingsPanel.vue'
import UpstreamAccountStatusControl from './components/upstream-account/UpstreamAccountStatusControl.vue'
import {
  upstreamAccountStatusLabel,
  upstreamAccountStatusTagType
} from './components/upstream-account/status'

const loading = shallowRef(false)
const accounts = shallowRef<any[]>([])
const priceBooks = shallowRef<PriceBookRecord[]>([])
const selectedAccountId = shallowRef('')
const selectedExportAccountIds = shallowRef<string[]>([])
const updatingAccountStatusId = shallowRef('')

const activePriceBookId = computed(() => firstActivePriceBookId(priceBooks.value))
const selectedAccount = computed(() => accounts.value.find((a: any) => a.id === selectedAccountId.value))
const selectedExportAccounts = computed(() => accounts.value.filter((a: any) => selectedExportAccountIds.value.includes(a.id)))
const priceBookName = (id?: string) => priceBooks.value.find((priceBook) => priceBook.id === id)?.name || '-'
// 列表只显示 base_url 的 host:名称虽然唯一,但「某中转 A / B」这类近似命名很常见,
// 去掉地址后无法分辨是哪个上游;host 不含路径与查询串,不暴露完整上游地址。
function accountHost(baseUrl?: string) {
  if (!baseUrl) return ''
  try {
    return new URL(baseUrl).host
  } catch {
    return baseUrl.replace(/^https?:\/\//, '').split('/')[0]
  }
}
// 展示名与名称一致时不重复渲染,只留倍率。
// formatMultiplier 返回纯数字,列表里没有 label 承载语义,故补 × 前缀;未设置时留空不占位。
function accountSubtitle(account: any) {
  const raw = formatMultiplier(account.tenant_multiplier)
  const multiplier = raw === '-' ? '' : `×${raw}`
  const displayName = account.tenant_display_name || ''
  if (!displayName || displayName === account.name) return multiplier
  return multiplier ? `对外:${displayName} · ${multiplier}` : `对外:${displayName}`
}
const providerFamilyLabel = (v: string) => endpointProtocolOptions.find((o) => o.value === v)?.label || v || '-'
// ── 账号 CRUD ────────────────────────────────────────────────────────────────
const accountDialog = shallowRef(false)
const editingAccountId = shallowRef('')
const submittingAccount = shallowRef(false)
const accountAdvancedSections = shallowRef<string[]>([])
const accountForm = reactive<any>({
  name: '',
  tenant_display_name: '',
  tenant_access_mode: 'public',
  base_url: '',
  api_key: '',
  default_provider_family: 'openai_compatible',
  concurrency_limit: null,
  price_book_id: '',
  tenant_multiplier: 1,
  extra_headers: {}
})
const isEditingAccount = computed(() => Boolean(editingAccountId.value))

function blankAccount() {
  return {
    name: '', tenant_display_name: '', tenant_access_mode: 'public', base_url: '', api_key: '', default_provider_family: 'openai_compatible',
    concurrency_limit: null, price_book_id: '', tenant_multiplier: 1,
    extra_headers: {}
  }
}

function resetAccountForm() {
  editingAccountId.value = ''
  accountAdvancedSections.value = []
  Object.assign(accountForm, blankAccount(), {
    price_book_id: activePriceBookId.value
  })
}

function openAccountCreate() { resetAccountForm(); accountDialog.value = true }
function openAccountEdit(row: any) {
  editingAccountId.value = row.id
  accountAdvancedSections.value = row.extra_headers && Object.keys(row.extra_headers || {}).length ? ['headers'] : []
  Object.assign(accountForm, {
    ...blankAccount(),
    name: row.name,
    tenant_display_name: row.tenant_display_name || row.name,
    tenant_access_mode: row.tenant_access_mode || 'public',
    base_url: row.base_url,
    api_key: '',
    default_provider_family: row.default_provider_family || 'openai_compatible',
    concurrency_limit: row.concurrency_limit ?? null,
    price_book_id: row.price_book_id || '',
    tenant_multiplier: row.tenant_multiplier ?? null,
    status: row.status || 'active',
    extra_headers: row.extra_headers && typeof row.extra_headers === 'object' ? { ...row.extra_headers } : {}
  })
  accountDialog.value = true
}

function buildAccountPayload() {
  const p: any = {
    name: accountForm.name.trim(),
    tenant_display_name: accountForm.tenant_display_name.trim() || accountForm.name.trim(),
    tenant_access_mode: accountForm.tenant_access_mode,
    base_url: accountForm.base_url.trim(),
    default_provider_family: accountForm.default_provider_family,
    concurrency_limit: accountForm.concurrency_limit ?? null,
    price_book_id: accountForm.price_book_id || undefined,
    tenant_multiplier: accountForm.tenant_multiplier ?? undefined,
    extra_headers: accountForm.extra_headers && Object.keys(accountForm.extra_headers).length ? accountForm.extra_headers : undefined
  }
  if (accountForm.api_key.trim()) p.api_key = accountForm.api_key.trim()
  return p
}

async function submitAccount() {
  if (!accountForm.name.trim()) { ElMessage.warning('请填写账号名称'); return }
  if (!accountForm.base_url.trim()) { ElMessage.warning('请填写 base_url'); return }
  if (!isEditingAccount.value && !accountForm.api_key.trim()) { ElMessage.warning('请填写上游 API key'); return }
  submittingAccount.value = true
  try {
    if (isEditingAccount.value) {
      await aiAdminApi.updateUpstreamAccount(editingAccountId.value, buildAccountPayload())
      ElMessage.success('账号已更新')
    } else {
      const created = await aiAdminApi.createUpstreamAccount(buildAccountPayload())
      selectedAccountId.value = created.id
      ElMessage.success('账号已创建')
    }
    accountDialog.value = false
    await fetchAccounts()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    submittingAccount.value = false
  }
}

async function changeAccountStatus(row: any, status: "active" | "disabled") {
  updatingAccountStatusId.value = row.id
  try {
    await aiAdminApi.updateUpstreamAccountStatus(row.id, status)
    ElMessage.success(status === 'active' ? '账号已启用' : '账号已停用')
    await fetchAccounts()
  } catch (e: any) {
    ElMessage.error(e?.message || '操作失败')
  } finally {
    updatingAccountStatusId.value = ''
  }
}

async function removeAccount(row: any) {
  try {
    await ElMessageBox.confirm(`删除上游账号「${row.name}」？分组对它的关联会一并解除。`, '确认删除', { type: 'warning' })
  } catch { return }
  try {
    await aiAdminApi.deleteUpstreamAccount(row.id)
    ElMessage.success('已删除')
    if (selectedAccountId.value === row.id) selectedAccountId.value = ''
    await fetchAccounts()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

// ── 导入 / 导出 ──────────────────────────────────────────────────────────────
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
const importSettings = reactive({
  default_price_book_id: '',
  default_tenant_multiplier: 1
})

watch(activePriceBookId, (nextId, previousId) => {
  if (!nextId || previousId) return
  if (accountDialog.value && !isEditingAccount.value && !accountForm.price_book_id) {
    accountForm.price_book_id = nextId
  }
  if (importDialog.value && !importSettings.default_price_book_id) {
    importSettings.default_price_book_id = nextId
    if (importAccounts.value.length) void refreshImportPreview()
  }
})

function isExportSelected(id: string) {
  return selectedExportAccountIds.value.includes(id)
}

function toggleExportSelection(id: string, checked: boolean) {
  const next = new Set(selectedExportAccountIds.value)
  if (checked) next.add(id)
  else next.delete(id)
  selectedExportAccountIds.value = Array.from(next)
}

function handleExportSelectionChange(id: string, checked: string | number | boolean) {
  toggleExportSelection(id, Boolean(checked))
}

function openExportDialog() {
  if (!selectedExportAccountIds.value.length) {
    ElMessage.warning('请先选择要导出的上游账号')
    return
  }
  exportDialog.value = true
}

async function confirmExportAccounts() {
  exportingAccounts.value = true
  try {
    const data = await aiAdminApi.exportUpstreamAccounts({
      account_ids: selectedExportAccountIds.value,
      include_model_bindings: exportIncludeModelBindings.value
    })
    downloadJSON(data, `upstream-accounts-${new Date().toISOString().slice(0, 10)}.json`)
    exportDialog.value = false
    ElMessage.success('导出文件已生成')
  } catch (e: any) {
    ElMessage.error(e?.message || '导出失败')
  } finally {
    exportingAccounts.value = false
  }
}

function openImportDialog() {
  importPreviewGeneration += 1
  importFileName.value = ''
  importAccounts.value = []
  importPreview.value = null
  previewingImport.value = false
  importSettings.default_price_book_id = activePriceBookId.value
  importSettings.default_tenant_multiplier = 1
  importDialog.value = true
}

function chooseImportFile() {
  importFileInput.value?.click()
}

async function handleImportFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  importPreviewGeneration += 1
  previewingImport.value = false
  try {
    const text = await file.text()
    const parsed = JSON.parse(text)
    const accountsToImport = parseImportAccounts(parsed)
    importFileName.value = file.name
    importAccounts.value = accountsToImport
    await refreshImportPreview()
  } catch (e: any) {
    importFileName.value = ''
    importAccounts.value = []
    importPreview.value = null
    ElMessage.error(e?.message || '导入文件解析失败')
  } finally {
    input.value = ''
  }
}

function parseImportAccounts(parsed: any): UpstreamAccountTransferAccountDTO[] {
  const accountsToImport = Array.isArray(parsed) ? parsed : parsed?.accounts
  if (!Array.isArray(accountsToImport) || accountsToImport.length === 0) {
    throw new Error('导入文件缺少 accounts 数组')
  }
  return accountsToImport
}

function buildImportRequest(): UpstreamAccountImportRequest {
  return {
    accounts: importAccounts.value,
    default_price_book_id: importSettings.default_price_book_id || undefined,
    default_tenant_multiplier: importSettings.default_tenant_multiplier || 1,
    duplicate_account_strategy: 'skip',
    duplicate_binding_strategy: 'skip'
  }
}

async function refreshImportPreview() {
  if (!importAccounts.value.length) return
  const generation = ++importPreviewGeneration
  previewingImport.value = true
  try {
    const preview = await aiAdminApi.previewImportUpstreamAccounts(buildImportRequest())
    if (generation !== importPreviewGeneration) return
    importPreview.value = preview
  } catch (e: any) {
    if (generation !== importPreviewGeneration) return
    importPreview.value = null
    ElMessage.error(e?.message || '导入预检失败')
  } finally {
    if (generation === importPreviewGeneration) previewingImport.value = false
  }
}

async function confirmImportAccounts() {
  if (!importAccounts.value.length || !importPreview.value?.summary.create_accounts) return
  importingAccounts.value = true
  try {
    const result = await aiAdminApi.importUpstreamAccounts(buildImportRequest())
    ElMessage.success(`已导入 ${result.summary.create_accounts} 个账号，${result.summary.create_model_bindings} 条模型绑定`)
    importDialog.value = false
    await fetchAccounts()
  } catch (e: any) {
    ElMessage.error(e?.message || '导入失败')
  } finally {
    importingAccounts.value = false
  }
}

function downloadJSON(data: unknown, filename: string) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

function importActionTone(action: string): 'positive' | 'danger' | 'info' {
  if (action === 'create') return 'positive'
  if (action === 'error') return 'danger'
  return 'info'
}

function importActionLabel(action: string) {
  if (action === 'create') return '创建'
  if (action === 'error') return '错误'
  return '跳过'
}

// DsTag tone 与 element-plus tag type 的映射(组件 status.ts 仍返回 el 语义,供未迁移弹窗使用)
function statusTone(status: string): 'positive' | 'danger' | 'info' {
  const type = upstreamAccountStatusTagType(status)
  return type === 'success' ? 'positive' : type
}

const importPreviewColumns: DsTableColumn[] = [
  { key: 'name', title: '账号' },
  { key: 'base_url', title: 'Base URL' },
  { key: 'action', title: '动作', width: 86 },
  { key: 'model_binding_count', title: '模型绑定', width: 96, align: 'right' },
  { key: 'reason', title: '说明' }
]

// ── 数据加载 ─────────────────────────────────────────────────────────────────
async function fetchAccounts() {
  loading.value = true
  try {
    const res = await aiAdminApi.listUpstreamAccounts()
    accounts.value = res.items || []
    if (!accounts.value.some((a: any) => a.id === selectedAccountId.value)) {
      selectedAccountId.value = accounts.value[0]?.id || ''
    }
    const validIds = new Set(accounts.value.map((a: any) => a.id))
    selectedExportAccountIds.value = selectedExportAccountIds.value.filter((id) => validIds.has(id))
  } catch (e: any) {
    ElMessage.error(e?.message || '加载账号失败')
  } finally {
    loading.value = false
  }
}

async function fetchPriceBooks() {
  try {
    const res = await aiAdminApi.listPriceBooks()
    priceBooks.value = res.items || []
  } catch { priceBooks.value = [] }
}

function selectAccount(id: string) {
  selectedAccountId.value = id
}

// ── 列表增量渲染(接口全量返回,前端分批挂载) ──────────────────────────────────
const LIST_PAGE_SIZE = 20
const visibleCount = shallowRef(LIST_PAGE_SIZE)
const accountListEl = ref<HTMLElement | null>(null)
const listSentinelEl = ref<HTMLElement | null>(null)
const visibleAccounts = computed(() => accounts.value.slice(0, visibleCount.value))
const hasMoreAccounts = computed(() => visibleCount.value < accounts.value.length)

let listObserver: IntersectionObserver | null = null

function revealMoreAccounts() {
  if (!hasMoreAccounts.value) return
  visibleCount.value = Math.min(visibleCount.value + LIST_PAGE_SIZE, accounts.value.length)
}

// 账号数据变化(刷新/增删/导入)后回到第一批,哨兵进入视口再继续追加
watch(accounts, () => {
  visibleCount.value = listObserver ? LIST_PAGE_SIZE : accounts.value.length
})

function setupListObserver() {
  // 测试环境(happy-dom)没有 IntersectionObserver,直接全量渲染兜底
  if (typeof IntersectionObserver === 'undefined') {
    visibleCount.value = accounts.value.length
    return
  }
  listObserver = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) revealMoreAccounts()
    },
    { root: accountListEl.value, rootMargin: '120px' }
  )
  if (listSentinelEl.value) listObserver.observe(listSentinelEl.value)
}

// ── 连通性测试 ───────────────────────────────────────────────────────────────
const testDialog = shallowRef(false)
const testAccount = shallowRef<any>(null)
const testModels = ref<{ model_code: string; capability_type: string; api_format: string }[]>([])
const testModelsLoading = shallowRef(false)
const testForm = reactive({ modelCode: '', prompt: '', imageEdit: false })
const testImage = shallowRef<UpstreamAccountTestImage | null>(null)
const testing = shallowRef(false)
const testResult = shallowRef<UpstreamAccountTestResult | null>(null)
const testError = shallowRef('')

const testSelectedBinding = computed(() => testModels.value.find((m) => m.model_code === testForm.modelCode))
const testSelectedCapability = computed(() => testSelectedBinding.value?.capability_type ?? '')
const testIsImage = computed(() => testSelectedCapability.value === 'image')
const testSupportsImageEdit = computed(() => testIsImage.value && testSelectedBinding.value?.api_format === 'openai_images')
const testCanRun = computed(() => Boolean(testForm.modelCode) && (!testForm.imageEdit || Boolean(testImage.value)))

watch(testSupportsImageEdit, (supported) => {
  if (!supported) testForm.imageEdit = false
})

watch(() => testForm.imageEdit, (imageEdit) => {
  if (!imageEdit) testImage.value = null
})

async function openTestDialog(account: any) {
  if (!account) return
  testAccount.value = account
  testForm.modelCode = ''
  testForm.prompt = ''
  testForm.imageEdit = false
  testImage.value = null
  testResult.value = null
  testError.value = ''
  testDialog.value = true
  testModelsLoading.value = true
  try {
    const data = await aiAdminApi.listAccountModelBindings(account.id)
    testModels.value = (data.items ?? []).map((b: any) => ({
      model_code: b.model_code,
      capability_type: b.capability_type,
      api_format: b.api_format
    }))
    if (testModels.value.length) testForm.modelCode = testModels.value[0].model_code
  } catch (e: any) {
    ElMessage.error(e?.message || '加载模型绑定失败')
  } finally {
    testModelsLoading.value = false
  }
}

async function runAccountTest() {
  if (!testAccount.value || !testForm.modelCode) return
  if (testForm.imageEdit && !testImage.value) {
    ElMessage.warning('请选择图片编辑测试使用的参考图片')
    return
  }
  testing.value = true
  testResult.value = null
  testError.value = ''
  try {
    const previousStatus = testAccount.value.status
    testResult.value = await aiAdminApi.testUpstreamAccount(testAccount.value.id, {
      model_code: testForm.modelCode,
      prompt: testForm.prompt.trim() || undefined,
      image_edit: testIsImage.value && testForm.imageEdit,
      image: testIsImage.value && testForm.imageEdit ? testImage.value || undefined : undefined
    })
    if (!testResult.value.ok) {
      testError.value = testResult.value.error || '上游未返回可用结果'
    }
    if (testResult.value.ok || [401, 403].includes(testResult.value.http_status)) {
      await fetchAccounts()
      testAccount.value = accounts.value.find((account: any) => account.id === testAccount.value?.id) || testAccount.value
      if (testResult.value.ok && previousStatus === 'invalid') {
        ElMessage.success('验证成功，账号已恢复启用')
      }
    }
  } catch (e: any) {
    testError.value = e?.message || '测试请求失败'
  } finally {
    testing.value = false
  }
}

function testModelLabel(m: { model_code: string; capability_type: string }) {
  const cap = m.capability_type === 'image' ? '生图' : m.capability_type === 'chat' ? '对话' : m.capability_type
  return `${m.model_code} · ${cap}`
}

function imageTestStreamLabel(value?: string) {
  return value === 'force_stream' ? '流式' : '非流式'
}

function imageTestFormatLabel(value?: string) {
  return value === 'url' ? 'URL' : value === 'b64_json' ? 'Base64' : '-'
}

function imageTestTransportLabel(value?: string) {
  return value === 'application/json' ? 'JSON 图片 URL' : value === 'multipart/form-data' ? 'Multipart 文件上传' : '-'
}

onMounted(async () => {
  await Promise.all([fetchAccounts(), fetchPriceBooks()])
  setupListObserver()
})

onBeforeUnmount(() => {
  listObserver?.disconnect()
  listObserver = null
})
</script>

<template>
  <div class="page-container accounts-view">
    <PortalPagePanel
      :icon="Database"
      :breadcrumbs="[{ label: '智能服务' }, { label: '网关配置' }, { label: '上游账号' }]"
      description="维护上游连接、公共模型、价格表和租户倍率。租户只会看到安全的账号目录与公开价格。"
      fill
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="fetchAccounts">刷新</el-button>
        <el-button :icon="Upload" @click="openImportDialog">导入</el-button>
        <el-button :icon="Download" :disabled="!selectedExportAccountIds.length" @click="openExportDialog">
          导出<span v-if="selectedExportAccountIds.length">({{ selectedExportAccountIds.length }})</span>
        </el-button>
        <el-button type="primary" :icon="Plus" @click="openAccountCreate">新增账号</el-button>
      </template>

      <!-- 主从布局:body 无内边距,用 24px 容器承载原栅格 -->
      <div class="accounts-body">
        <el-row :gutter="16" class="accounts-main-row">
      <!-- 账号列表 -->
      <el-col :span="7" class="accounts-list-col">
        <PortalContentCard title="上游账号" body-padding="none" class="accounts-list-card">
          <div v-loading="loading" ref="accountListEl" class="account-list">
            <div
              v-for="a in visibleAccounts"
              :key="a.id"
              class="account-item"
              :class="{ active: a.id === selectedAccountId }"
              @click="selectAccount(a.id)"
            >
              <div class="account-item-header">
                <div class="account-item-title">
                  <el-checkbox
                    :model-value="isExportSelected(a.id)"
                    @click.stop
                    @change="handleExportSelectionChange(a.id, $event)"
                  />
                  <span class="font-bold text-slate-800 truncate">{{ a.name }}</span>
                  <DsTag :tone="a.tenant_access_mode === 'restricted' ? 'warning' : 'positive'">
                    {{ a.tenant_access_mode === 'restricted' ? '专属' : '公开' }}
                  </DsTag>
                </div>
                <UpstreamAccountStatusControl
                  :status="a.status"
                  :invalid-reason="a.invalid_reason"
                  :loading="updatingAccountStatusId === a.id"
                  @change="changeAccountStatus(a, $event)"
                  @verify="openTestDialog(a)"
                />
              </div>
              <div class="account-item-subtitle truncate">{{ accountSubtitle(a) }}</div>
              <div class="account-item-host truncate">{{ accountHost(a.base_url) }}</div>
            </div>
            <div ref="listSentinelEl" class="account-list-sentinel" aria-hidden="true">
              <span v-if="hasMoreAccounts">加载中…</span>
              <span v-else-if="accounts.length > LIST_PAGE_SIZE">共 {{ accounts.length }} 条</span>
            </div>
            <DsEmpty
              v-if="!accounts.length && !loading"
              title="暂无上游账号"
              description="先新增一个上游账号,或从 JSON 文件导入"
            >
              <template #action>
                <el-button type="primary" :icon="Plus" @click="openAccountCreate">新增账号</el-button>
              </template>
            </DsEmpty>
          </div>
        </PortalContentCard>
      </el-col>

      <!-- 账号详情 + 路由配置 -->
      <el-col :span="17" class="account-content-column">
        <div v-if="selectedAccount" class="account-content-stack">
          <PortalContentCard title="账号概览" :description="`当前账号:${selectedAccount.name}`">
            <template #actions>
              <el-button
                size="small"
                type="success"
                plain
                :icon="VideoPlay"
                @click="openTestDialog(selectedAccount)"
              >测试连通</el-button>
              <el-button size="small" :icon="Edit" @click="openAccountEdit(selectedAccount)">编辑账号</el-button>
              <el-button size="small" type="danger" plain :icon="Delete" @click="removeAccount(selectedAccount)">删除</el-button>
            </template>
            <el-descriptions :key="selectedAccountId" :column="2" size="small" border>
              <el-descriptions-item label="Base URL">{{ selectedAccount.base_url }}</el-descriptions-item>
              <el-descriptions-item label="协议">{{ providerFamilyLabel(selectedAccount.default_provider_family) }}</el-descriptions-item>
              <el-descriptions-item label="最大并发">{{ selectedAccount.concurrency_limit ? `${selectedAccount.concurrency_limit} 并发` : '不限制' }}</el-descriptions-item>
              <el-descriptions-item label="运行状态">
                <DsTag :tone="statusTone(selectedAccount.status)">
                  {{ upstreamAccountStatusLabel(selectedAccount.status) }}
                </DsTag>
              </el-descriptions-item>
              <el-descriptions-item label="租户可见性">{{ selectedAccount.tenant_access_mode === 'restricted' ? '专属' : '公开' }}</el-descriptions-item>
              <el-descriptions-item label="价格表">{{ priceBookName(selectedAccount.price_book_id) }}</el-descriptions-item>
              <el-descriptions-item label="租户倍率">{{ formatMultiplier(selectedAccount.tenant_multiplier) }}</el-descriptions-item>
              <el-descriptions-item v-if="selectedAccount.status === 'invalid'" label="失效原因" :span="2">
                {{ selectedAccount.invalid_reason || '上游拒绝了账号凭据' }}
              </el-descriptions-item>
            </el-descriptions>
          </PortalContentCard>

          <UpstreamModelBindingsPanel
            target-kind="account"
            :target-id="selectedAccount.id"
            :default-binding-protocol="defaultBindingProtocolForProviderFamily(selectedAccount?.default_provider_family)"
            title="上游账号显式模型绑定"
            description="用显式绑定声明这个上游账号实际可用的模型、API 格式和模型配置。"
            empty-text="当前账号暂无显式模型绑定。可先从上游发现模型，再补充精细化编辑。"
            import-button-label="发现上游模型"
            import-dialog-title="发现上游模型"
            import-alert-title="从上游 /v1/models 拉取，勾选后创建该账号的显式上游模型绑定。"
          />
        </div>
      </el-col>
        </el-row>
      </div>
    </PortalPagePanel>

    <!-- 连通性测试弹窗 -->
    <el-dialog v-model="testDialog" title="测试账号连通" width="620px">
      <div v-if="testAccount" class="test-dialog">
        <div class="test-account-head">
          <el-icon class="test-account-icon"><VideoPlay /></el-icon>
          <div class="test-account-meta">
            <span class="test-account-name">{{ testAccount.name }}</span>
            <span class="test-account-sub">直连上游账号测试</span>
          </div>
          <el-tag :type="upstreamAccountStatusTagType(testAccount.status)" size="small" effect="light">
            {{ upstreamAccountStatusLabel(testAccount.status) }}
          </el-tag>
        </div>

        <el-form label-position="top" class="test-form">
          <el-form-item label="选择测试模型">
            <el-select
              v-model="testForm.modelCode"
              :loading="testModelsLoading"
              placeholder="选择该账号下的显式绑定模型"
              style="width: 100%"
            >
              <el-option
                v-for="m in testModels"
                :key="m.model_code"
                :label="testModelLabel(m)"
                :value="m.model_code"
              />
            </el-select>
            <p v-if="!testModelsLoading && !testModels.length" class="test-empty-hint">
              该账号暂无显式模型绑定，请先在下方「显式模型绑定」里发现/添加模型。
            </p>
          </el-form-item>

          <el-form-item :label="testIsImage ? '生图提示词' : '对话提示词'">
            <el-input
              v-model="testForm.prompt"
              type="textarea"
              :rows="3"
              :placeholder="testIsImage
                ? 'Generate a cute orange cat astronaut sticker on a clean pastel background.'
                : 'Reply with a short friendly greeting.'"
            />
            <p class="test-hint">
              {{ testIsImage
                ? '选择图片模型后，这里会直接发起生图测试，并在下方展示返回图片。'
                : '选择对话模型后，这里会发送一条对话测试，并在下方展示返回文本。' }}
            </p>
          </el-form-item>
          <el-form-item v-if="testIsImage" label="测试类型">
            <el-radio-group v-model="testForm.imageEdit">
              <el-radio :value="false">图片生成</el-radio>
              <el-radio :value="true" :disabled="!testSupportsImageEdit">图片编辑</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="testIsImage && testForm.imageEdit" label="参考图片" required>
            <UpstreamImageTestUpload v-model="testImage" />
          </el-form-item>
        </el-form>

        <!-- 结果区 -->
        <div class="test-console">
          <template v-if="testing">
            <span class="test-line test-warn">⟳ 连接上游中…</span>
            <span class="test-line test-muted">模型：{{ testForm.modelCode }}（{{ testIsImage ? '生图' : '对话' }}）</span>
          </template>
          <template v-else-if="testResult">
            <span class="test-line" :class="testResult.ok ? 'test-ok' : 'test-err'">
              {{ testResult.ok ? '✓ 测试成功' : '✗ 测试失败' }}
            </span>
            <span class="test-line test-muted">HTTP {{ testResult.http_status }} · {{ testResult.latency_ms }}ms · {{ testResult.api_format }}</span>
            <span class="test-line test-muted">上游模型 ID：{{ testResult.upstream_model }}</span>
            <span v-if="testIsImage" class="test-line test-muted">
              上游请求：{{ imageTestStreamLabel(testResult.image_stream_mode) }}<template v-if="testForm.imageEdit"> · {{ imageTestTransportLabel(testResult.image_edit_transport) }}</template> · 返回格式配置：{{ imageTestFormatLabel(testResult.image_upstream_response_format) }}
            </span>
            <span v-if="testIsImage && testResult.actual_image_format" class="test-line test-muted">
              上游响应：{{ imageTestFormatLabel(testResult.actual_image_format) }}
            </span>
            <span
              v-if="testResult.total_tokens"
              class="test-line test-muted"
            >tokens：in {{ testResult.prompt_tokens ?? 0 }} / out {{ testResult.output_tokens ?? 0 }} / total {{ testResult.total_tokens }}</span>
            <span v-if="testError" class="test-line test-err">{{ testError }}</span>
            <template v-if="testResult.ok && !testIsImage && testResult.reply_text">
              <span class="test-line test-muted">回复：</span>
              <span class="test-line test-reply">{{ testResult.reply_text }}</span>
            </template>
            <div v-if="testResult.ok && testIsImage" class="test-image-wrap">
              <img
                v-if="testResult.image_b64"
                :src="`data:${testResult.image_mime || 'image/png'};base64,${testResult.image_b64}`"
                alt="生图测试结果"
                class="test-image"
              />
              <img v-else-if="testResult.image_url" :src="testResult.image_url" alt="生图测试结果" class="test-image" />
            </div>
          </template>
          <span v-else class="test-line test-muted">尚未测试。选择模型后点击「开始测试」。</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="testDialog = false">关闭</el-button>
        <el-button
          type="primary"
          :loading="testing"
          :disabled="!testCanRun"
          :icon="VideoPlay"
          @click="runAccountTest"
        >
          {{ testing ? '测试中…' : '开始测试' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 账号弹窗 -->
    <el-dialog v-model="accountDialog" :title="isEditingAccount ? '编辑上游账号' : '新增上游账号'" width="640px">
      <el-form label-width="130px">
        <el-form-item label="名称" required>
          <el-input v-model="accountForm.name" placeholder="如 OpenAI 官方 / 某中转" />
        </el-form-item>
        <el-form-item label="展示名称" required>
          <el-input v-model="accountForm.tenant_display_name" placeholder="租户目录中显示的名称" />
        </el-form-item>
        <el-form-item label="租户专属">
          <el-switch
            v-model="accountForm.tenant_access_mode"
            active-value="restricted"
            inactive-value="public"
            active-text="开启"
            inactive-text="关闭"
          />
        </el-form-item>
        <el-form-item label="Base URL" required>
          <el-input v-model="accountForm.base_url" placeholder="https://api.openai.com" />
        </el-form-item>
        <el-form-item label="API Key" :required="!isEditingAccount">
          <el-input
            v-model="accountForm.api_key"
            type="password"
            show-password
            :placeholder="isEditingAccount ? '留空不改；密文存储' : '输入上游 API Key（密文存储）'"
          />
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="accountForm.default_provider_family" class="w-full">
            <el-option v-for="o in endpointProtocolOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="最大并发数">
          <DsNumberInput v-model="accountForm.concurrency_limit" :min="1" :step="1" />
          <span class="hint">
            留空表示不限制；指该账号同时在飞的上游请求数上限。
            上游若按每分钟请求数给配额，可按「RPM ÷ 60 × 平均请求秒数」估算
          </span>
        </el-form-item>
        <el-form-item label="价格表">
          <el-select
            v-model="accountForm.price_book_id"
            clearable
            class="w-full"
            :placeholder="activePriceBookId ? '该账号成本基准' : '暂无启用价格表'"
          >
            <el-option v-for="b in priceBooks" :key="b.id" :label="b.name" :value="b.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="租户倍率">
          <DsNumberInput v-model="accountForm.tenant_multiplier" :min="0" :step="0.1" :precision="4" />
          <span class="hint">租户扣费=价格表 USD × 默认倍率；默认 1</span>
        </el-form-item>
        <el-collapse v-model="accountAdvancedSections" class="advanced-sections">
          <el-collapse-item name="headers" title="高级设置：附加请求头">
            <p class="advanced-hint">默认无需填写，仅当某些上游要求固定额外 Header 时再配置。</p>
            <el-form-item label="附加请求头">
              <KeyValueEditor v-model="accountForm.extra_headers" />
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="accountDialog = false">取消</el-button>
        <el-button type="primary" :loading="submittingAccount" @click="submitAccount">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="exportDialog" title="导出上游账号" width="560px">
      <el-alert
        title="导出文件会包含上游 API key 明文，请只在可信环境保存和传输。"
        type="warning"
        show-icon
        :closable="false"
      />
      <div class="transfer-panel">
        <div class="transfer-summary">
          <span>已选择</span>
          <strong>{{ selectedExportAccounts.length }}</strong>
          <span>个上游账号</span>
        </div>
        <div class="transfer-account-tags">
          <el-tag v-for="account in selectedExportAccounts" :key="account.id" size="small">{{ account.name }}</el-tag>
        </div>
        <el-checkbox v-model="exportIncludeModelBindings">同时导出显式模型绑定</el-checkbox>
      </div>
      <template #footer>
        <el-button @click="exportDialog = false">取消</el-button>
        <el-button type="primary" :loading="exportingAccounts" @click="confirmExportAccounts">导出 JSON</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="importDialog" title="导入上游账号" width="780px">
      <el-alert
        title="导入文件中的 API key 会按当前系统密钥重新加密；价格表不从文件继承，需要在本窗口选择。"
        type="warning"
        show-icon
        :closable="false"
      />
      <div class="import-file-row">
        <el-button :icon="Upload" @click="chooseImportFile">选择 JSON 文件</el-button>
        <span class="import-file-name">{{ importFileName || '未选择文件' }}</span>
        <input ref="importFileInput" type="file" accept="application/json,.json" class="hidden-file-input" @change="handleImportFileSelected" />
      </div>
      <el-form v-if="importAccounts.length" label-width="110px" class="import-form">
        <el-form-item label="价格表">
          <el-select
            v-model="importSettings.default_price_book_id"
            clearable
            class="w-full"
            :placeholder="activePriceBookId ? '不绑定价格表' : '暂无启用价格表'"
            @change="refreshImportPreview"
          >
            <el-option v-for="b in priceBooks" :key="b.id" :label="b.name" :value="b.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="租户倍率">
          <DsNumberInput
            v-model="importSettings.default_tenant_multiplier"
            :min="0"
            :step="0.1"
            :precision="4"
            class="plain-number-input"
            @change="refreshImportPreview"
          />
        </el-form-item>
      </el-form>

      <div v-if="importPreview" v-loading="previewingImport" class="import-preview">
        <div class="import-stats">
          <span>创建账号 <strong>{{ importPreview.summary.create_accounts }}</strong></span>
          <span>跳过账号 <strong>{{ importPreview.summary.skip_accounts }}</strong></span>
          <span>错误账号 <strong>{{ importPreview.summary.error_accounts }}</strong></span>
          <span>创建模型绑定 <strong>{{ importPreview.summary.create_model_bindings }}</strong></span>
          <span>跳过模型绑定 <strong>{{ importPreview.summary.skip_model_bindings }}</strong></span>
        </div>
        <div class="import-preview-table">
          <DsTable
            :columns="importPreviewColumns"
            :rows="importPreview.items"
            row-key="name"
            empty-title="暂无预检结果"
          >
            <template #cell-action="{ row }">
              <DsTag :tone="importActionTone(row.action)">{{ importActionLabel(row.action) }}</DsTag>
            </template>
            <template #cell-reason="{ row }">
              {{ row.reason || row.warnings?.join('；') || '—' }}
            </template>
          </DsTable>
        </div>
      </div>

      <template #footer>
        <el-button @click="importDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="importingAccounts"
          :disabled="!importPreview?.summary.create_accounts"
          @click="confirmImportAccounts"
        >
          导入可创建项
        </el-button>
      </template>
    </el-dialog>

  </div>
</template>

<style scoped>
.accounts-view {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.account-list { flex: 1; min-height: 0; overflow-y: auto; padding: 8px 12px; }
.account-list-sentinel { text-align: center; font-size: 12px; color: var(--ds-faint); }
.account-list-sentinel span { display: block; padding: 4px 0 10px; }
.account-item { padding: 10px 12px; border-radius: var(--ds-radius-control); cursor: pointer; border: 1px solid transparent; margin-bottom: 6px; }
.account-item:hover { background: var(--ds-panel-muted); }
.account-item.active { background: var(--ds-accent-soft); border-color: color-mix(in srgb, var(--ds-accent) 40%, var(--ds-line)); }
.account-item-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; min-width: 0; }
.account-item-title { display: flex; align-items: center; gap: 8px; min-width: 0; }
.account-item-subtitle { font-size: 12px; color: var(--ds-muted); margin-top: 2px; }
.account-item-host { font-size: 12px; color: var(--ds-faint); }
.account-content-column { min-width: 0; min-height: 0; }
.account-content-stack { display: flex; flex-direction: column; gap: 16px; height: 100%; }
.hint { color: var(--ds-faint); font-size: 12px; margin-left: 8px; }
.advanced-sections { margin-bottom: 18px; }
.advanced-hint { margin: 0 0 12px; color: var(--ds-faint); font-size: 12px; line-height: 1.4; }
.transfer-panel { display: flex; flex-direction: column; gap: 14px; margin-top: 16px; }
.transfer-summary { display: inline-flex; align-items: baseline; gap: 6px; color: var(--ds-muted); }
.transfer-summary strong { color: var(--ds-ink); font-size: 20px; line-height: 1; }
.transfer-account-tags { display: flex; flex-wrap: wrap; gap: 8px; }
.import-file-row { display: flex; align-items: center; gap: 12px; margin: 16px 0; }
.import-file-name { min-width: 0; color: var(--ds-muted); font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.hidden-file-input { display: none; }
.import-form { padding: 12px 0 2px; border-top: 1px solid var(--ds-line); }
.plain-number-input { width: 100%; }
.import-preview { margin-top: 10px; }
.import-preview-table { max-height: 260px; overflow-y: auto; }
.import-stats { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 12px; color: var(--ds-muted); font-size: 13px; }
.import-stats span { padding: 6px 10px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.import-stats strong { color: var(--ds-ink); }

/* 主从布局:PortalPagePanel body 无内边距,用 24px 容器承载原栅格;fill 模式下容器自身伸展、两栏等高撑满 */
.accounts-body { flex: 1; min-height: 0; display: flex; flex-direction: column; padding: 24px; }
.accounts-main-row { flex: 1; min-height: 640px; align-items: stretch; }
.accounts-list-col { min-height: 0; }
.accounts-list-card { height: 100%; }
:deep(.accounts-list-card.portal-content-card) { display: flex; flex-direction: column; height: 100%; }
:deep(.accounts-list-card .portal-content-card__body) { display: flex; flex-direction: column; flex: 1; min-height: 0; }

@media (max-width: 1200px) {
  .accounts-main-row { min-height: 0; }
}

/* ── 连通性测试弹窗 ── */
.test-dialog { display: flex; flex-direction: column; gap: 16px; }
.test-account-head {
  display: flex; align-items: center; gap: 12px;
  padding: 12px 14px; border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px; background: var(--el-fill-color-lighter);
}
.test-account-icon {
  width: 40px; height: 40px; border-radius: 10px;
  display: flex; align-items: center; justify-content: center;
  background: var(--el-color-success); color: var(--ds-accent-contrast); font-size: 18px; flex: none;
}
.test-account-meta { display: flex; flex-direction: column; flex: 1; min-width: 0; }
.test-account-name { font-weight: 600; }
.test-account-sub { font-size: 12px; color: var(--el-text-color-secondary); }
.test-form { margin: 0; }
.test-hint, .test-empty-hint { margin: 6px 0 0; font-size: 12px; color: var(--el-text-color-secondary); }
.test-empty-hint { color: var(--el-color-warning); }
.test-console {
  /* 深色控制台局部色板:全部由 ds token + color-mix 推导,语义 = 成功绿/失败红/警告黄/次要灰 */
  --console-bg: color-mix(in srgb, var(--ds-ink) 94%, var(--ds-panel));
  --console-fg: color-mix(in srgb, var(--ds-panel) 80%, var(--ds-ink));
  --console-ok: color-mix(in srgb, var(--ds-positive) 65%, var(--ds-panel));
  --console-err: color-mix(in srgb, var(--ds-danger) 60%, var(--ds-panel));
  --console-warn: color-mix(in srgb, var(--ds-warning) 70%, var(--ds-panel));
  --console-muted: var(--ds-faint);
  --console-reply: var(--ds-line);
  --console-border: color-mix(in srgb, var(--ds-faint) 30%, transparent);
  border-radius: 10px; padding: 14px 16px;
  background: var(--console-bg); color: var(--console-fg);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px; line-height: 1.7; max-height: 320px; overflow: auto;
  display: flex; flex-direction: column;
}
.test-line { white-space: pre-wrap; word-break: break-word; }
.test-ok { color: var(--console-ok); }
.test-err { color: var(--console-err); }
.test-warn { color: var(--console-warn); }
.test-muted { color: var(--console-muted); }
.test-reply { color: var(--console-reply); }
.test-image-wrap { margin-top: 10px; }
.test-image {
  max-width: 100%; max-height: 240px; border-radius: 8px;
  border: 1px solid var(--console-border);
}
</style>
