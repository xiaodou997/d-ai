<!--
  账号池。
  适配 v4：aiGateway → aiAdminApi（list* 返回 {items} 已取 .items；listUpstreamDeployments 参数走 camelCase credentialPoolId）；
       capabilityOptions/statusOptions/protocolOptions 走共享 constants；错误读 err.message。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行，
       工作台左右两栏置于同卡 body 内 24px 容器，栏间只用 1px 分隔线/边框分区）；
       凭据列表 el-table → DsTable(:frame="false")，状态/到期标签 el-tag → DsTag，空态统一 DsEmpty；
       Provider 类型色由 --ds-* token 提供，无硬编码色值；弹窗仍为 element-plus。
       注：listCredentialPools/listPoolCredentials 接口本身不支持分页，本页不渲染 DsPagination。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh, Setting } from '@element-plus/icons-vue'
import { Boxes } from 'lucide-vue-next'
import { PortalContentCard, PortalPagePanel } from '@/platform'
import { DsEmpty, DsNumberInput, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { formatMultiplier } from '@/platform/ai/utils'
import { aiAdminApi } from '@/api/aiAdmin'
import UpstreamModelBindingsPanel from './components/UpstreamModelBindingsPanel.vue'

// ============================================================================
// Provider type meta
// ============================================================================

// color 为 --ds-* token（经内联 --tab-color 变量注入），禁止硬编码色值
const PROVIDER_TABS = [
  { key: 'codex',        label: 'Codex',       color: 'var(--ds-positive)', desc: 'ChatGPT / OpenAI Responses API' },
  { key: 'claude_oauth', label: 'Claude OAuth', color: 'var(--ds-accent)',   desc: 'Anthropic Messages API（OAuth）' },
  { key: 'gemini_cli',   label: 'Gemini CLI',   color: 'var(--ds-info)',     desc: 'Google Cloud Code Assist（Gemini）' },
  { key: 'antigravity',  label: 'Antigravity',  color: 'var(--ds-warning)',  desc: 'Google Cloud Code Assist（Antigravity）' }
]

const statusTone = (s: string) => (({ active: 'positive', disabled: 'info', invalid: 'danger' } as any)[s] || 'info')
const strategyLabel = (s: string) => (s === 'weighted' ? '加权' : '轮询')

const credColumns: DsTableColumn[] = [
  { key: 'name', title: '名称' },
  { key: 'email', title: '邮箱' },
  { key: 'status', title: '状态', width: 100 },
  { key: 'expires', title: 'Token 到期', width: 120 },
  { key: 'weight', title: '权重', width: 75, align: 'right' },
  { key: 'stats', title: '成功/失败', width: 110, align: 'right' },
  { key: 'invalid_reason', title: '无效原因' },
  { key: 'actions', title: '操作', width: 140 }
]

// ============================================================================
// State
// ============================================================================

const activeTab     = shallowRef('codex')
const poolsLoading  = shallowRef(false)
const allPools      = shallowRef<any[]>([])
const selectedPool  = shallowRef<any>(null)
const priceBooks    = shallowRef<any[]>([])
const updatingPoolStatusId = shallowRef('')

const credLoading   = shallowRef(false)
const credentials   = shallowRef<any[]>([])

// Pool dialog
const poolDialogVisible = shallowRef(false)
const poolDialogLoading = shallowRef(false)
const editingPoolId     = shallowRef('')
const POOL_FORM_DEFAULTS = () => ({
  name: '', tenant_display_name: '', tenant_access_mode: 'public' as 'public' | 'restricted',
  oauth_strategy: 'round_robin', notes: '',
  price_book_id: '', tenant_multiplier: 1
})
const poolForm = reactive(POOL_FORM_DEFAULTS())

// Credential import dialog
const credDialogVisible = shallowRef(false)
const credDialogLoading = shallowRef(false)
const importMode        = ref('file')   // 'file' | 'manual'
const fileInputRef      = ref<HTMLInputElement | null>(null)
const parsedFiles       = ref<any[]>([])       // [{ name, data, error, importStatus, importError }]
const credForm = reactive<any>({
  name: '', email: '', access_token: '', refresh_token: '',
  expires_at: null, weight: 100
})

// ============================================================================
// Computed
// ============================================================================

const filteredPools = computed(() =>
  allPools.value.filter((p: any) => p.fixed_provider_type === activeTab.value)
)

const currentTab = computed(() =>
  PROVIDER_TABS.find((t) => t.key === activeTab.value)
)

const activeCredCount  = computed(() => credentials.value.filter((c: any) => c.status === 'active').length)
const invalidCredCount = computed(() => credentials.value.filter((c: any) => c.status === 'invalid').length)

// ============================================================================
// Data loading
// ============================================================================

const fetchPools = async () => {
  poolsLoading.value = true
  try {
    const data = await aiAdminApi.listCredentialPools()
    allPools.value = data.items || []
    if (selectedPool.value) {
      const still = allPools.value.find((p: any) => p.id === selectedPool.value.id)
      if (!still) selectedPool.value = null
      else selectedPool.value = still
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '加载账号池失败')
  } finally {
    poolsLoading.value = false
  }
}

const fetchCredentials = async () => {
  if (!selectedPool.value) { credentials.value = []; return }
  credLoading.value = true
  try {
    const res = await aiAdminApi.listPoolCredentials(selectedPool.value.id)
    credentials.value = res.items || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载凭据失败')
  } finally {
    credLoading.value = false
  }
}

watch(selectedPool, () => { fetchCredentials() })

watch(activeTab, () => { selectedPool.value = null })

onMounted(async () => {
  const [books] = await Promise.allSettled([aiAdminApi.listPriceBooks(), fetchPools()])
  if (books.status === 'fulfilled') priceBooks.value = books.value.items || []
})

// ============================================================================
// Pool CRUD
// ============================================================================

const openCreatePool = () => {
  editingPoolId.value = ''
  Object.assign(poolForm, POOL_FORM_DEFAULTS())
  poolDialogVisible.value = true
}

const openEditPool = (pool: any) => {
  editingPoolId.value = pool.id
  Object.assign(poolForm, {
    name: pool.name,
    tenant_display_name: pool.tenant_display_name || pool.name,
    tenant_access_mode: pool.tenant_access_mode || 'public',
    oauth_strategy: pool.oauth_strategy,
    notes: pool.notes || '',
    price_book_id: pool.price_book_id || '',
    tenant_multiplier: pool.tenant_multiplier ?? 1
  })
  poolDialogVisible.value = true
}

// 创建/更新池只负责池元数据；显式模型绑定通过独立入口维护。
const buildPoolPayload = () => ({
  name: poolForm.name,
  tenant_display_name: poolForm.tenant_display_name.trim() || poolForm.name.trim(),
  tenant_access_mode: poolForm.tenant_access_mode,
  oauth_strategy: poolForm.oauth_strategy,
  notes: poolForm.notes,
  price_book_id: poolForm.price_book_id,
  tenant_multiplier: poolForm.tenant_multiplier
})

const submitPool = async () => {
  if (!poolForm.name.trim()) { ElMessage.error('池名称不能为空'); return }
  poolDialogLoading.value = true
  try {
    if (editingPoolId.value) {
      await aiAdminApi.patchCredentialPool(editingPoolId.value, buildPoolPayload() as any)
      ElMessage.success('账号池已保存')
    } else {
      await aiAdminApi.createCredentialPool({ ...buildPoolPayload(), fixed_provider_type: activeTab.value } as any)
      ElMessage.success('账号池已创建')
    }
    poolDialogVisible.value = false
    await fetchPools()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    poolDialogLoading.value = false
  }
}

const defaultBindingProtocolForPool = (pool: any) => {
  switch (pool?.fixed_provider_type) {
    case 'claude_oauth':
      return 'anthropic_messages'
    case 'gemini_cli':
    case 'antigravity':
      return 'gemini_generate'
    case 'codex':
      return 'openai_responses'
    default:
      return 'openai_chat'
  }
}

const handleDeletePool = async (pool: any) => {
  await ElMessageBox.confirm(`确认删除账号池「${pool.name}」？所有凭据将一并删除。`, '危险操作', {
    type: 'error', confirmButtonText: '确认删除', cancelButtonText: '取消',
    confirmButtonClass: 'el-button--danger'
  })
  try {
    await aiAdminApi.deleteCredentialPool(pool.id)
    ElMessage.success('账号池已删除')
    if (selectedPool.value?.id === pool.id) selectedPool.value = null
    await fetchPools()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

const changePoolStatus = async (pool: any, enabled: boolean) => {
  const status = enabled ? 'active' : 'disabled'
  if (pool.status === status) return
  updatingPoolStatusId.value = pool.id
  try {
    await aiAdminApi.updateCredentialPoolStatus(pool.id, status)
    ElMessage.success(status === 'active' ? '账号池已启用' : '账号池已停用')
    await fetchPools()
  } catch (e: any) {
    ElMessage.error(e?.message || '状态更新失败')
  } finally {
    updatingPoolStatusId.value = ''
  }
}

// ============================================================================
// Credential CRUD
// ============================================================================

const openCredDialog = () => {
  importMode.value = 'file'
  parsedFiles.value = []
  Object.assign(credForm, { name: '', email: '', access_token: '', refresh_token: '', expires_at: null, weight: 100 })
  credDialogVisible.value = true
}

const onFilesSelected = (event: any) => {
  const files = Array.from(event.target.files || []) as File[]
  if (!files.length) return
  parsedFiles.value = []
  files.forEach((file) => {
    const entry: any = { name: file.name, data: null, error: null, importStatus: null, importError: '' }
    const reader = new FileReader()
    reader.onload = (e: any) => {
      try {
        entry.data = JSON.parse(e.target.result)
      } catch {
        entry.error = 'JSON 格式错误'
      }
      parsedFiles.value = [...parsedFiles.value]
    }
    reader.onerror = () => {
      entry.error = '文件读取失败'
      parsedFiles.value = [...parsedFiles.value]
    }
    parsedFiles.value.push(entry)
    reader.readAsText(file)
  })
  event.target.value = ''
}

// 仅提取后端接受的字段，丢弃 provider 特定的未知键（如 organizations / exported_at），避免 DisallowUnknownFields 拒绝。
const extractCredentialPayload = (raw: any) => {
  const KNOWN = ['name', 'provider_type', 'email', 'access_token', 'refresh_token',
    'token_type', 'scope', 'expires_at', 'weight', 'auth_metadata',
    'account_id', 'plan_type', 'user_id', 'account_user_id']
  const payload: any = {}
  for (const key of KNOWN) {
    if (raw[key] !== undefined) payload[key] = raw[key]
  }
  if (!payload.access_token) return { error: 'access_token 字段缺失' }
  return { payload }
}

const submitCredential = async () => {
  if (!selectedPool.value) return
  if (importMode.value === 'file') {
    const toImport = parsedFiles.value.filter((f: any) => f.data && !f.error)
    if (!toImport.length) { ElMessage.error('没有可导入的有效凭据'); return }
    credDialogLoading.value = true
    let successCount = 0
    for (const entry of toImport) {
      entry.importStatus = 'pending'
      const { payload, error } = extractCredentialPayload(entry.data)
      if (error) {
        entry.importStatus = 'failed'
        entry.importError = error
        parsedFiles.value = [...parsedFiles.value]
        continue
      }
      try {
        await aiAdminApi.createPoolCredential(selectedPool.value.id, payload)
        entry.importStatus = 'success'
        successCount++
      } catch (err: any) {
        entry.importStatus = 'failed'
        entry.importError = err?.message || '导入失败'
      }
      parsedFiles.value = [...parsedFiles.value]
    }
    credDialogLoading.value = false
    if (successCount > 0) {
      ElMessage.success(`成功导入 ${successCount} 条凭据`)
      await fetchCredentials()
    }
    const failCount = toImport.length - successCount
    if (failCount === 0) credDialogVisible.value = false
  } else {
    if (!credForm.access_token.trim()) { ElMessage.error('access_token 不能为空'); return }
    credDialogLoading.value = true
    try {
      await aiAdminApi.createPoolCredential(selectedPool.value.id, { ...credForm })
      ElMessage.success('凭据已导入')
      credDialogVisible.value = false
      await fetchCredentials()
    } catch (e: any) {
      ElMessage.error(e?.message || '导入失败')
    } finally {
      credDialogLoading.value = false
    }
  }
}

const toggleCredStatus = async (cred: any) => {
  const next = cred.status === 'active' ? 'disabled' : 'active'
  try {
    await aiAdminApi.patchPoolCredential(selectedPool.value.id, cred.id, { status: next })
    ElMessage.success('状态已更新')
    await fetchCredentials()
  } catch (e: any) {
    ElMessage.error(e?.message || '更新失败')
  }
}

const handleRefreshCred = async (cred: any) => {
  try {
    await aiAdminApi.refreshPoolCredential(selectedPool.value.id, cred.id)
    ElMessage.success(`凭据「${cred.name}」刷新成功`)
    await fetchCredentials()
  } catch (err: any) {
    ElMessage.error('刷新失败: ' + (err?.message || '未知错误'))
  }
}

const handleDeleteCred = async (cred: any) => {
  await ElMessageBox.confirm(`确认删除凭据「${cred.name}」？`, '删除凭据', {
    type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消'
  })
  try {
    await aiAdminApi.deletePoolCredential(selectedPool.value.id, cred.id)
    ElMessage.success('凭据已删除')
    await fetchCredentials()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

// ============================================================================
// Helpers
// ============================================================================

const formatExpiry = (ms: any) => {
  if (!ms) return '—'
  const now = Date.now()
  const diffH = (ms - now) / 3600000
  if (diffH < 0) return `已过期`
  if (diffH < 1) return `${Math.round(diffH * 60)} 分钟后`
  if (diffH < 24) return `${Math.round(diffH)} 小时后`
  return `${Math.round(diffH / 24)} 天后`
}

const expiryTone = (ms: any) => {
  if (!ms) return 'info'
  const diffH = (ms - Date.now()) / 3600000
  if (diffH < 0) return 'danger'
  if (diffH < 1) return 'danger'
  if (diffH < 24) return 'warning'
  return 'positive'
}
</script>

<template>
  <div class="pools-page">
    <PortalPagePanel
      :icon="Boxes"
      :breadcrumbs="[{ label: '智能服务' }, { label: '网关配置' }, { label: '账号池' }]"
      description="按 Provider 类型管理 OAuth Token、公共模型、结算价格表和租户扣费倍率。"
      fill
    >
      <!-- 工作台主体:body 无内边距,用 24px 容器承载左右两栏 -->
      <div class="pools-body">
        <!-- Left: Provider type tabs + pool list -->
        <aside class="pools-rail">
          <!-- Provider type tabs -->
          <div class="provider-tabs">
            <button
              v-for="tab in PROVIDER_TABS"
              :key="tab.key"
              class="provider-tab"
              :class="{ active: activeTab === tab.key }"
              :style="activeTab === tab.key ? `--tab-color: ${tab.color}` : ''"
              @click="activeTab = tab.key"
            >
              <span class="tab-dot" :style="`background: ${tab.color}`" />
              {{ tab.label }}
            </button>
          </div>

          <!-- Pool list for active tab -->
          <div v-loading="poolsLoading" class="pool-list">
            <div
              v-for="pool in filteredPools"
              :key="pool.id"
              class="pool-item"
              :class="{ active: selectedPool?.id === pool.id }"
              @click="selectedPool = pool"
            >
              <div class="pool-item-body">
                <div class="pool-item-title">
                  <strong>{{ pool.name }}</strong>
                  <DsTag :tone="pool.tenant_access_mode === 'restricted' ? 'warning' : 'positive'">
                    {{ pool.tenant_access_mode === 'restricted' ? '专属' : '公开' }}
                  </DsTag>
                </div>
                <p class="pool-meta">{{ strategyLabel(pool.oauth_strategy) }} 策略</p>
                <p v-if="pool.notes" class="pool-notes">{{ pool.notes }}</p>
              </div>
              <div class="pool-item-actions" @click.stop>
                <el-switch
                  :model-value="pool.status === 'active'"
                  inline-prompt
                  active-text="启用"
                  inactive-text="停用"
                  :width="52"
                  :loading="updatingPoolStatusId === pool.id"
                  @change="changePoolStatus(pool, Boolean($event))"
                />
                <el-button link type="primary" :icon="Setting" size="small" aria-label="编辑账号池" @click="openEditPool(pool)" />
                <el-button link type="danger" :icon="Delete" size="small" aria-label="删除账号池" @click="handleDeletePool(pool)" />
              </div>
            </div>

            <DsEmpty
              v-if="filteredPools.length === 0"
              :title="`暂无 ${currentTab?.label} 账号池`"
            />
          </div>

          <div class="rail-footer">
            <el-button type="primary" :icon="Plus" class="w-full" @click="openCreatePool">
              新建 {{ currentTab?.label }} 账号池
            </el-button>
          </div>
        </aside>

        <!-- Right: Credentials workspace -->
        <main v-if="selectedPool" class="pools-workspace">
          <!-- Pool header -->
          <section class="workspace-hero">
            <div class="hero-left">
              <p class="eyebrow">Credential Pool</p>
              <div class="hero-title-row">
                <h2>{{ selectedPool.name }}</h2>
                <DsTag :tone="statusTone(selectedPool.status)">{{ selectedPool.status }}</DsTag>
                <DsTag tone="info">{{ strategyLabel(selectedPool.oauth_strategy) }}</DsTag>
              </div>
              <p v-if="selectedPool.notes" class="pool-hero-notes">{{ selectedPool.notes }}</p>
              <p class="pool-hero-type">
                <span class="type-dot" :style="`background: ${currentTab?.color}`" />
                {{ currentTab?.label }} · {{ currentTab?.desc }}
              </p>
            </div>
            <div class="hero-metrics">
              <div class="metric-cell">
                <strong>{{ credentials.length }}</strong>
                <span>凭据总数</span>
              </div>
              <div class="metric-cell">
                <strong class="text-green">{{ activeCredCount }}</strong>
                <span>Active</span>
              </div>
              <div class="metric-cell">
                <strong :class="invalidCredCount > 0 ? 'text-red' : ''">{{ invalidCredCount }}</strong>
                <span>Invalid</span>
              </div>
              <div class="metric-cell"><strong>{{ formatMultiplier(selectedPool.tenant_multiplier ?? 1) }}</strong><span>租户扣费倍率</span></div>
            </div>
          </section>

          <UpstreamModelBindingsPanel
            target-kind="pool"
            :target-id="selectedPool.id"
            :default-binding-protocol="defaultBindingProtocolForPool(selectedPool)"
            :locked-api-format="defaultBindingProtocolForPool(selectedPool)"
            title="凭证池显式模型绑定"
            description="显式绑定直接落到 ai_upstream_models。池元数据编辑与模型绑定编辑已彻底分离。"
            empty-text="当前凭证池暂无显式模型绑定。可从预设模型导入后再手工细调。"
            import-button-label="导入预设模型"
            import-dialog-title="导入预设模型"
            import-alert-title="按固定供应商预设创建凭证池的显式上游模型绑定。"
          />

          <!-- Credentials table -->
          <PortalContentCard title="凭据列表" description="管理该账号池的 OAuth Token，支持导入、刷新、启停和删除" body-padding="none">
            <template #actions>
              <el-button :icon="Refresh" :loading="credLoading" @click="fetchCredentials">刷新</el-button>
              <el-button type="primary" :icon="Plus" @click="openCredDialog">导入凭据</el-button>
            </template>

            <DsTable
              :frame="false"
              :columns="credColumns"
              :rows="credentials"
              row-key="id"
              :loading="credLoading"
            >
              <template #empty>
                <DsEmpty title="暂无凭据" description="导入 OAuth 凭据后，池内模型将按策略调度。">
                  <template #action>
                    <el-button type="primary" :icon="Plus" @click="openCredDialog">导入凭据</el-button>
                  </template>
                </DsEmpty>
              </template>
              <template #cell-status="{ row }">
                <el-tooltip
                  v-if="row.status === 'invalid'"
                  :content="row.invalid_reason || '凭据已失效，请点击刷新尝试恢复'"
                  placement="top"
                >
                  <el-switch
                    :model-value="row.status === 'active'"
                    disabled
                    inline-prompt
                    active-text="启用"
                    inactive-text="停用"
                    size="small"
                  />
                </el-tooltip>
                <el-switch
                  v-else
                  :model-value="row.status === 'active'"
                  inline-prompt
                  active-text="启用"
                  inactive-text="停用"
                  size="small"
                  @change="toggleCredStatus(row)"
                />
              </template>
              <template #cell-expires="{ row }">
                <DsTag v-if="row.expires_at" :tone="expiryTone(row.expires_at)">
                  {{ formatExpiry(row.expires_at) }}
                </DsTag>
                <span v-else class="text-muted">—</span>
              </template>
              <template #cell-stats="{ row }">
                <span class="text-green">{{ row.success_count }}</span>
                <span class="text-muted"> / </span>
                <span :class="row.fail_count > 0 ? 'text-red' : 'text-muted'">{{ row.fail_count }}</span>
              </template>
              <template #cell-invalid_reason="{ row }">
                <span v-if="row.invalid_reason" class="text-danger">{{ row.invalid_reason }}</span>
                <span v-else class="text-muted">—</span>
              </template>
              <template #cell-actions="{ row }">
                <el-button link type="primary" size="small" @click="handleRefreshCred(row)">刷新</el-button>
                <el-button link type="danger" size="small" @click="handleDeleteCred(row)">删除</el-button>
              </template>
            </DsTable>
          </PortalContentCard>
        </main>

        <main v-else class="empty-workspace-wrap">
          <section class="empty-workspace">
            <DsEmpty
              title="选择或新建账号池"
              description="账号池按 Provider 类型分组管理 OAuth Token，每个池可绑定多条模型路由。"
            >
              <template #action>
                <el-button type="primary" :icon="Plus" @click="openCreatePool">
                  新建 {{ currentTab?.label }} 账号池
                </el-button>
              </template>
            </DsEmpty>
          </section>
        </main>
      </div>
    </PortalPagePanel>

    <!-- Pool create / edit dialog -->
    <el-dialog
    v-model="poolDialogVisible"
    :title="editingPoolId ? '编辑账号池' : `新建 ${currentTab?.label} 账号池`"
    width="560px"
  >
    <el-form :model="poolForm" label-width="90px">
      <el-form-item label="池名称" required>
        <el-input v-model="poolForm.name" placeholder="例如：GPT-4o 主力池" />
      </el-form-item>
      <el-form-item label="租户展示名称" required>
        <el-input v-model="poolForm.tenant_display_name" placeholder="租户目录中显示的名称" />
      </el-form-item>
      <el-form-item label="租户专属">
        <el-switch
          v-model="poolForm.tenant_access_mode"
          active-value="restricted"
          inactive-value="public"
          active-text="开启"
          inactive-text="关闭"
        />
      </el-form-item>
      <el-form-item label="轮询策略">
        <el-select v-model="poolForm.oauth_strategy" class="w-full">
          <el-option value="round_robin" label="轮询（round_robin）" />
          <el-option value="weighted" label="加权随机（weighted）" />
        </el-select>
      </el-form-item>
      <el-form-item label="结算价格表" required>
        <el-select v-model="poolForm.price_book_id" class="w-full" filterable>
          <el-option v-for="book in priceBooks" :key="book.id" :label="book.name" :value="book.id" :disabled="book.status !== 'active'" />
        </el-select>
      </el-form-item>
      <el-form-item label="租户扣费倍率" required>
        <DsNumberInput v-model="poolForm.tenant_multiplier" :min="0" :step="0.1" :precision="4" class="w-full" />
      </el-form-item>
      <p class="form-hint form-hint-block">显式模型绑定请在工作区通过「导入预设模型 / 新增绑定」维护，本表单只编辑池元数据。</p>
      <el-form-item label="备注">
        <el-input v-model="poolForm.notes" type="textarea" :rows="2" placeholder="可选" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="poolDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="poolDialogLoading" @click="submitPool">
        {{ editingPoolId ? '保存' : '创建' }}
      </el-button>
    </template>
  </el-dialog>

    <!-- Credential import dialog -->
    <el-dialog v-model="credDialogVisible" title="导入 OAuth 凭据" width="640px">
    <el-tabs v-model="importMode">
      <el-tab-pane label="选择 JSON 文件" name="file">
        <div class="file-import-area">
          <p class="import-hint">选择 Provider 工具导出的 JSON 文件，支持多选，系统将自动解析并批量导入</p>
          <input
            ref="fileInputRef"
            type="file"
            accept=".json,application/json"
            multiple
            style="display:none"
            @change="onFilesSelected"
          />
          <el-button :icon="Plus" @click="fileInputRef?.click()">选择 JSON 文件</el-button>

          <div v-if="parsedFiles.length" class="parsed-file-list">
            <div
              v-for="(f, i) in parsedFiles"
              :key="i"
              class="parsed-file-item"
              :class="{
                'parse-ok': !f.error,
                'parse-err': !!f.error,
                'import-success': f.importStatus === 'success',
                'import-failed': f.importStatus === 'failed'
              }"
            >
              <div class="pf-name">{{ f.name }}</div>
              <div class="pf-meta">
                <template v-if="f.error">
                  <el-tag type="danger" size="small">解析失败</el-tag>
                  <span class="pf-err-msg">{{ f.error }}</span>
                </template>
                <template v-else-if="f.importStatus === 'success'">
                  <el-tag type="success" size="small">导入成功</el-tag>
                </template>
                <template v-else-if="f.importStatus === 'failed'">
                  <el-tag type="danger" size="small">导入失败</el-tag>
                  <span class="pf-err-msg">{{ f.importError }}</span>
                </template>
                <template v-else>
                  <el-tag type="success" size="small" effect="plain">解析成功</el-tag>
                  <span class="pf-detail">{{ f.data?.email || f.data?.name || '无邮箱' }}</span>
                </template>
              </div>
            </div>
          </div>

          <div v-else class="file-drop-hint">
            <p>尚未选择文件</p>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="手动填写" name="manual">
        <el-form :model="credForm" label-width="100px">
          <el-form-item label="显示名称">
            <el-input v-model="credForm.name" placeholder="留空则用邮箱" />
          </el-form-item>
          <el-form-item label="邮箱">
            <el-input v-model="credForm.email" />
          </el-form-item>
          <el-form-item label="Access Token" required>
            <el-input v-model="credForm.access_token" type="textarea" :rows="3" />
          </el-form-item>
          <el-form-item label="Refresh Token">
            <el-input v-model="credForm.refresh_token" type="textarea" :rows="2" />
          </el-form-item>
          <el-form-item label="到期时间">
            <el-date-picker
              v-model="credForm.expires_at"
              type="datetime"
              value-format="x"
              placeholder="可选"
            />
          </el-form-item>
          <el-form-item label="权重">
            <el-input-number v-model="credForm.weight" :min="0" :max="1000" :controls="false" />
          </el-form-item>
        </el-form>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="credDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="credDialogLoading" @click="submitCredential">
        {{ importMode === 'file'
          ? `导入 ${parsedFiles.filter((f: any) => !f.error).length} 条凭据`
          : '导入' }}
      </el-button>
    </template>
    </el-dialog>
  </div>

</template>

<style scoped>
.pools-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 工作台主体:PortalPagePanel body 无内边距,用 24px 容器排布左右两栏 */
.pools-body {
  flex: 1;
  min-height: 600px;
  display: flex;
  gap: 20px;
  padding: 24px;
}

.pools-rail {
  width: 300px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
}

.pools-workspace {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.eyebrow {
  margin: 0 0 4px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--ds-faint);
}

.provider-tabs {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--ds-line);
}

.provider-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: none;
  background: transparent;
  border-radius: var(--ds-radius-control);
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-muted);
  cursor: pointer;
  text-align: left;
  transition: all 0.15s;
}

.provider-tab:hover {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.provider-tab.active {
  background: color-mix(in srgb, var(--tab-color, var(--ds-accent)) 10%, transparent);
  color: var(--tab-color, var(--ds-accent));
}

.tab-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.pool-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.pool-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  cursor: pointer;
  transition: all 0.15s;
}

.pool-item:hover {
  border-color: var(--ds-line-strong);
  background: var(--ds-panel-muted);
}

.pool-item.active {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.pool-item-body {
  flex: 1;
  min-width: 0;
}

.pool-item-title {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.pool-item-title strong {
  font-size: 13px;
  color: var(--ds-ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pool-meta {
  margin: 4px 0 0;
  font-size: 11px;
  color: var(--ds-faint);
}

.pool-notes {
  margin: 4px 0 0;
  font-size: 11px;
  color: var(--ds-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pool-item-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.rail-footer {
  padding: 12px;
  border-top: 1px solid var(--ds-line);
}

.workspace-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
  padding: 18px 20px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
}

.hero-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}

.hero-title-row h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--ds-ink);
}

.pool-hero-notes {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--ds-muted);
}

.pool-hero-type {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--ds-faint);
}

.type-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.hero-metrics {
  display: flex;
  gap: 16px;
  flex-shrink: 0;
}

.metric-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 56px;
}

.metric-cell strong {
  font-size: 22px;
  font-weight: 700;
  color: var(--ds-ink);
  line-height: 1;
}

.metric-cell span {
  font-size: 11px;
  color: var(--ds-faint);
  margin-top: 4px;
}

.empty-workspace-wrap {
  flex: 1;
  display: flex;
}

.empty-workspace {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100%;
  padding: 60px 32px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
}

.import-hint {
  font-size: 13px;
  color: var(--ds-muted);
  margin-bottom: 12px;
}

.file-import-area {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.file-drop-hint {
  text-align: center;
  padding: 32px 0;
  color: var(--ds-faint);
  font-size: 13px;
}

.parsed-file-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 280px;
  overflow-y: auto;
}

.parsed-file-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 12px;
  border-radius: var(--ds-radius-control);
  border: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
}

.parsed-file-item.parse-err {
  border-color: color-mix(in srgb, var(--ds-danger) 30%, var(--ds-line));
  background: var(--ds-danger-soft);
}

.parsed-file-item.import-success {
  border-color: color-mix(in srgb, var(--ds-positive) 30%, var(--ds-line));
  background: var(--ds-positive-soft);
}

.parsed-file-item.import-failed {
  border-color: color-mix(in srgb, var(--ds-danger) 30%, var(--ds-line));
  background: var(--ds-danger-soft);
}

.pf-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-ink);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pf-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.pf-detail {
  font-size: 12px;
  color: var(--ds-muted);
}

.pf-err-msg {
  font-size: 12px;
  color: var(--ds-danger);
}

.text-green  { color: var(--ds-positive); }
.text-red    { color: var(--ds-danger); }
.text-muted  { color: var(--ds-faint); }
.text-danger { color: var(--ds-danger); font-size: 12px; }
.w-full      { width: 100%; }
.form-hint   { display: block; margin-top: 4px; font-size: 12px; color: var(--ds-faint); line-height: 1.4; }
.form-hint-block { margin: 0 0 12px; }
@media (max-width: 1200px) {
  .pools-body { min-height: 0; }
}
</style>
