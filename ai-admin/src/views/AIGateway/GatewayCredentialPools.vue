<script setup>
import { computed, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh, Setting } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createCredentialPool,
  createPoolCredential,
  createUpstreamDeployment,
  deleteCredentialPool,
  deletePoolCredential,
  deleteUpstreamDeployment,
  listCredentialPools,
  listPoolCredentials,
  listUpstreamDeployments,
  patchCredentialPool,
  patchPoolCredential,
  protocolOptions,
  refreshPoolCredential,
  statusOptions,
  updateUpstreamDeploymentStatus
} from '@/api/aiGateway'

// ============================================================================
// Provider type meta
// ============================================================================

const PROVIDER_TABS = [
  { key: 'codex',        label: 'Codex',       color: '#10b981', desc: 'ChatGPT / OpenAI Responses API' },
  { key: 'claude_oauth', label: 'Claude OAuth', color: '#8b5cf6', desc: 'Anthropic Messages API（OAuth）' },
  { key: 'gemini_cli',   label: 'Gemini CLI',   color: '#3b82f6', desc: 'Google Cloud Code Assist（Gemini）' },
  { key: 'antigravity',  label: 'Antigravity',  color: '#f59e0b', desc: 'Google Cloud Code Assist（Antigravity）' }
]

const statusTagType = (s) => ({ active: 'success', disabled: 'info', invalid: 'danger' }[s] || 'info')
const strategyLabel = (s) => s === 'weighted' ? '加权' : '轮询'

// ============================================================================
// State
// ============================================================================

const activeTab     = shallowRef('codex')
const poolsLoading  = shallowRef(false)
const allPools      = shallowRef([])
const selectedPool  = shallowRef(null)

const credLoading   = shallowRef(false)
const credentials   = shallowRef([])

// Pool dialog
const poolDialogVisible = shallowRef(false)
const poolDialogLoading = shallowRef(false)
const editingPoolId     = shallowRef('')
const poolForm = reactive({ name: '', oauth_strategy: 'round_robin', notes: '', status: 'active' })

// Pool deployment section
const deployments    = shallowRef([])
const deployLoading  = shallowRef(false)
const deployDialogVisible  = shallowRef(false)
const deployDialogLoading  = shallowRef(false)
const deployForm = reactive({
  upstream_model: '',
  capability_type: 'chat',
  upstream_protocol: 'openai_chat',
  status: 'active'
})

// Credential import dialog
const credDialogVisible = shallowRef(false)
const credDialogLoading = shallowRef(false)
const importMode        = ref('file')   // 'file' | 'manual'
const fileInputRef      = ref(null)
const parsedFiles       = ref([])       // [{ name, data, error, importStatus, importError }]
const credForm = reactive({
  name: '', email: '', access_token: '', refresh_token: '',
  expires_at: null, weight: 100
})

// ============================================================================
// Computed
// ============================================================================

const filteredPools = computed(() =>
  allPools.value.filter(p => p.fixed_provider_type === activeTab.value)
)

const currentTab = computed(() =>
  PROVIDER_TABS.find(t => t.key === activeTab.value)
)

const activeCredCount  = computed(() => credentials.value.filter(c => c.status === 'active').length)
const invalidCredCount = computed(() => credentials.value.filter(c => c.status === 'invalid').length)

// ============================================================================
// Data loading
// ============================================================================

const fetchPools = async () => {
  poolsLoading.value = true
  try {
    const data = await listCredentialPools()
    allPools.value = data || []
    // Auto-select first in active tab when current selection disappears
    if (selectedPool.value) {
      const still = allPools.value.find(p => p.id === selectedPool.value.id)
      if (!still) selectedPool.value = null
    }
  } finally {
    poolsLoading.value = false
  }
}

const fetchCredentials = async () => {
  if (!selectedPool.value) { credentials.value = []; return }
  credLoading.value = true
  try {
    credentials.value = (await listPoolCredentials(selectedPool.value.id)) || []
  } finally {
    credLoading.value = false
  }
}

const fetchPoolDeployments = async () => {
  if (!selectedPool.value) { deployments.value = []; return }
  deployLoading.value = true
  try {
    deployments.value = (await listUpstreamDeployments({ credential_pool_id: selectedPool.value.id })) || []
  } finally {
    deployLoading.value = false
  }
}

watch(selectedPool, () => { fetchCredentials(); fetchPoolDeployments() })

watch(activeTab, () => { selectedPool.value = null })

onMounted(fetchPools)

// ============================================================================
// Pool CRUD
// ============================================================================

const openCreatePool = () => {
  editingPoolId.value = ''
  Object.assign(poolForm, { name: '', oauth_strategy: 'round_robin', notes: '', status: 'active' })
  poolDialogVisible.value = true
}

const openEditPool = (pool) => {
  editingPoolId.value = pool.id
  Object.assign(poolForm, {
    name: pool.name,
    oauth_strategy: pool.oauth_strategy,
    notes: pool.notes || '',
    status: pool.status
  })
  poolDialogVisible.value = true
}

const submitPool = async () => {
  if (!poolForm.name.trim()) { ElMessage.error('池名称不能为空'); return }
  poolDialogLoading.value = true
  try {
    if (editingPoolId.value) {
      await patchCredentialPool(editingPoolId.value, poolForm)
      ElMessage.success('账号池已保存')
    } else {
      await createCredentialPool({ ...poolForm, fixed_provider_type: activeTab.value })
      ElMessage.success('账号池已创建')
    }
    poolDialogVisible.value = false
    await fetchPools()
  } finally {
    poolDialogLoading.value = false
  }
}

const handleDeletePool = async (pool) => {
  await ElMessageBox.confirm(`确认删除账号池「${pool.name}」？所有凭据将一并删除。`, '危险操作', {
    type: 'error', confirmButtonText: '确认删除', cancelButtonText: '取消',
    confirmButtonClass: 'el-button--danger'
  })
  await deleteCredentialPool(pool.id)
  ElMessage.success('账号池已删除')
  if (selectedPool.value?.id === pool.id) selectedPool.value = null
  await fetchPools()
}

// ============================================================================
// Pool Deployment CRUD
// ============================================================================

const openDeploymentDialog = () => {
  Object.assign(deployForm, { upstream_model: '', capability_type: 'chat', upstream_protocol: 'openai_chat', status: 'active' })
  deployDialogVisible.value = true
}

const submitPoolDeployment = async () => {
  if (!deployForm.upstream_model.trim()) { ElMessage.error('上游模型名不能为空'); return }
  deployDialogLoading.value = true
  try {
    await createUpstreamDeployment({
      credential_pool_id: selectedPool.value.id,
      upstream_model: deployForm.upstream_model.trim(),
      capability_type: deployForm.capability_type,
      upstream_protocol: deployForm.upstream_protocol,
      status: deployForm.status
    })
    ElMessage.success('部署配置已创建')
    deployDialogVisible.value = false
    await fetchPoolDeployments()
  } finally {
    deployDialogLoading.value = false
  }
}

const togglePoolDeployment = async (row) => {
  const next = row.status === 'active' ? 'disabled' : 'active'
  await updateUpstreamDeploymentStatus(row.id, next)
  ElMessage.success('状态已更新')
  await fetchPoolDeployments()
}

const handleDeletePoolDeployment = async (row) => {
  await ElMessageBox.confirm(`确认删除部署「${row.upstream_model}」？关联的模型路由也将一并删除。`, '删除部署', {
    type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消'
  })
  await deleteUpstreamDeployment(row.id)
  ElMessage.success('部署已删除')
  await fetchPoolDeployments()
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

const onFilesSelected = (event) => {
  const files = Array.from(event.target.files || [])
  if (!files.length) return
  parsedFiles.value = []
  files.forEach(file => {
    const entry = { name: file.name, data: null, error: null, importStatus: null, importError: '' }
    const reader = new FileReader()
    reader.onload = (e) => {
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

// Extract only the fields the backend accepts, discarding unknown provider-specific keys
// (e.g. organizations, exported_at) that would cause DisallowUnknownFields to reject the request.
const extractCredentialPayload = (raw) => {
  const KNOWN = ['name', 'provider_type', 'email', 'access_token', 'refresh_token',
    'token_type', 'scope', 'expires_at', 'weight', 'auth_metadata',
    'account_id', 'plan_type', 'user_id', 'account_user_id']
  const payload = {}
  for (const key of KNOWN) {
    if (raw[key] !== undefined) payload[key] = raw[key]
  }
  if (!payload.access_token) return { error: 'access_token 字段缺失' }
  return { payload }
}

const submitCredential = async () => {
  if (!selectedPool.value) return
  if (importMode.value === 'file') {
    const toImport = parsedFiles.value.filter(f => f.data && !f.error)
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
        await createPoolCredential(selectedPool.value.id, payload)
        entry.importStatus = 'success'
        successCount++
      } catch (err) {
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
      await createPoolCredential(selectedPool.value.id, { ...credForm })
      ElMessage.success('凭据已导入')
      credDialogVisible.value = false
      await fetchCredentials()
    } finally {
      credDialogLoading.value = false
    }
  }
}

const toggleCredStatus = async (cred) => {
  const next = cred.status === 'active' ? 'disabled' : 'active'
  await patchPoolCredential(selectedPool.value.id, cred.id, { status: next })
  ElMessage.success('状态已更新')
  await fetchCredentials()
}

const handleRefreshCred = async (cred) => {
  try {
    await refreshPoolCredential(selectedPool.value.id, cred.id)
    ElMessage.success(`凭据「${cred.name}」刷新成功`)
    await fetchCredentials()
  } catch (err) {
    ElMessage.error('刷新失败: ' + (err?.message || '未知错误'))
  }
}

const handleDeleteCred = async (cred) => {
  await ElMessageBox.confirm(`确认删除凭据「${cred.name}」？`, '删除凭据', {
    type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消'
  })
  await deletePoolCredential(selectedPool.value.id, cred.id)
  ElMessage.success('凭据已删除')
  await fetchCredentials()
}

// ============================================================================
// Helpers
// ============================================================================

const formatExpiry = (ms) => {
  if (!ms) return '—'
  const d = new Date(ms)
  const now = Date.now()
  const diffH = (ms - now) / 3600000
  if (diffH < 0) return `已过期`
  if (diffH < 1) return `${Math.round(diffH * 60)} 分钟后`
  if (diffH < 24) return `${Math.round(diffH)} 小时后`
  return `${Math.round(diffH / 24)} 天后`
}

const expiryTagType = (ms) => {
  if (!ms) return 'info'
  const diffH = (ms - Date.now()) / 3600000
  if (diffH < 0) return 'danger'
  if (diffH < 1) return 'danger'
  if (diffH < 24) return 'warning'
  return 'success'
}
</script>

<template>
  <div class="pools-workbench">
    <!-- ================================================================
         Left: Provider type tabs + pool list
         ================================================================ -->
    <aside class="pools-rail">
      <div class="rail-head">
        <div>
          <p class="eyebrow">OAuth Fixed Providers</p>
          <h2>账号池管理</h2>
        </div>
      </div>

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
              <el-tag :type="statusTagType(pool.status)" size="small" effect="plain">{{ pool.status }}</el-tag>
            </div>
            <p class="pool-meta">{{ strategyLabel(pool.oauth_strategy) }} 策略</p>
            <p v-if="pool.notes" class="pool-notes">{{ pool.notes }}</p>
          </div>
          <div class="pool-item-actions" @click.stop>
            <el-button link type="primary" :icon="Setting" size="small" @click="openEditPool(pool)" />
            <el-button link type="danger" :icon="Delete" size="small" @click="handleDeletePool(pool)" />
          </div>
        </div>

        <div v-if="filteredPools.length === 0" class="empty-pools">
          <p>暂无 {{ currentTab?.label }} 账号池</p>
        </div>
      </div>

      <div class="rail-footer">
        <el-button type="primary" :icon="Plus" class="w-full" @click="openCreatePool">
          新建 {{ currentTab?.label }} 账号池
        </el-button>
      </div>
    </aside>

    <!-- ================================================================
         Right: Credentials workspace
         ================================================================ -->
    <main v-if="selectedPool" class="pools-workspace">
      <!-- Pool header -->
      <section class="workspace-hero">
        <div class="hero-left">
          <p class="eyebrow">Credential Pool</p>
          <div class="hero-title-row">
            <h2>{{ selectedPool.name }}</h2>
            <el-tag :type="statusTagType(selectedPool.status)" effect="dark">{{ selectedPool.status }}</el-tag>
            <el-tag type="info" effect="plain">{{ strategyLabel(selectedPool.oauth_strategy) }}</el-tag>
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
        </div>
      </section>

      <!-- Credentials table -->
      <section class="panel">
        <div class="section-head">
          <div>
            <h3>凭据列表</h3>
            <p>管理该账号池的 OAuth Token，支持导入、刷新、启停和删除</p>
          </div>
          <div class="head-actions">
            <el-button :icon="Refresh" circle @click="fetchCredentials" />
            <el-button type="primary" :icon="Plus" @click="openCredDialog">导入凭据</el-button>
          </div>
        </div>

        <el-table v-loading="credLoading" :data="credentials" border stripe>
          <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
          <el-table-column prop="email" label="邮箱" min-width="180" show-overflow-tooltip />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Token 到期" width="120">
            <template #default="{ row }">
              <el-tag v-if="row.expires_at" :type="expiryTagType(row.expires_at)" size="small">
                {{ formatExpiry(row.expires_at) }}
              </el-tag>
              <span v-else class="text-muted">—</span>
            </template>
          </el-table-column>
          <el-table-column label="权重" prop="weight" width="75" align="right" />
          <el-table-column label="成功/失败" width="110" align="right">
            <template #default="{ row }">
              <span class="text-green">{{ row.success_count }}</span>
              <span class="text-muted"> / </span>
              <span :class="row.fail_count > 0 ? 'text-red' : 'text-muted'">{{ row.fail_count }}</span>
            </template>
          </el-table-column>
          <el-table-column label="无效原因" min-width="140" show-overflow-tooltip>
            <template #default="{ row }">
              <span v-if="row.invalid_reason" class="text-danger">{{ row.invalid_reason }}</span>
              <span v-else class="text-muted">—</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="200" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="handleRefreshCred(row)">刷新</el-button>
              <el-button
                link
                :type="row.status === 'active' ? 'warning' : 'success'"
                size="small"
                :disabled="row.status === 'invalid'"
                @click="toggleCredStatus(row)"
              >
                {{ row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button link type="danger" size="small" @click="handleDeleteCred(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
      <!-- Pool deployments table -->
      <section class="panel">
        <div class="section-head">
          <div>
            <h3>上游模型部署</h3>
            <p>为该账号池配置可调用的上游模型（路由器将从此列表选路）</p>
          </div>
          <div class="head-actions">
            <el-button :icon="Refresh" circle @click="fetchPoolDeployments" />
            <el-button type="primary" :icon="Plus" @click="openDeploymentDialog">新增部署</el-button>
          </div>
        </div>
        <el-table v-loading="deployLoading" :data="deployments" border stripe>
          <el-table-column prop="upstream_model" label="上游模型" min-width="180" show-overflow-tooltip />
          <el-table-column label="能力类型" width="110">
            <template #default="{ row }">
              <el-tag size="small">{{ row.capability_type }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="upstream_protocol" label="协议" min-width="140" show-overflow-tooltip />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'success' : 'warning'" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button
                link
                :type="row.status === 'active' ? 'warning' : 'success'"
                size="small"
                @click="togglePoolDeployment(row)"
              >{{ row.status === 'active' ? '禁用' : '启用' }}</el-button>
              <el-button link type="danger" size="small" @click="handleDeletePoolDeployment(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <main v-else class="empty-workspace">
      <p class="eyebrow">No Pool Selected</p>
      <h2>选择或新建账号池</h2>
      <p>账号池按 Provider 类型分组管理 OAuth Token，每个池可绑定多条模型路由。</p>
      <el-button type="primary" :icon="Plus" @click="openCreatePool">
        新建 {{ currentTab?.label }} 账号池
      </el-button>
    </main>
  </div>

  <!-- ================================================================
       Pool create / edit dialog
       ================================================================ -->
  <el-dialog
    v-model="poolDialogVisible"
    :title="editingPoolId ? '编辑账号池' : `新建 ${currentTab?.label} 账号池`"
    width="480px"
  >
    <el-form :model="poolForm" label-width="90px">
      <el-form-item label="池名称" required>
        <el-input v-model="poolForm.name" placeholder="例如：GPT-4o 主力池" />
      </el-form-item>
      <el-form-item label="轮询策略">
        <el-select v-model="poolForm.oauth_strategy" class="w-full">
          <el-option value="round_robin" label="轮询（round_robin）" />
          <el-option value="weighted" label="加权随机（weighted）" />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="poolForm.status" class="w-full">
          <el-option value="active" label="active" />
          <el-option value="disabled" label="disabled" />
        </el-select>
      </el-form-item>
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

  <!-- ================================================================
       Pool deployment create dialog
       ================================================================ -->
  <el-dialog v-model="deployDialogVisible" title="新增上游模型部署" width="480px">
    <el-form :model="deployForm" label-width="100px">
      <el-form-item label="上游模型名" required>
        <el-input v-model="deployForm.upstream_model" placeholder="例如 gpt-4o、claude-opus-4-5" />
      </el-form-item>
      <el-form-item label="能力类型">
        <el-select v-model="deployForm.capability_type" class="w-full">
          <el-option v-for="c in capabilityOptions" :key="c.value" :label="c.label" :value="c.value" />
        </el-select>
      </el-form-item>
      <el-form-item label="上游协议">
        <el-select v-model="deployForm.upstream_protocol" class="w-full">
          <el-option v-for="p in protocolOptions" :key="p.value" :label="p.label" :value="p.value" />
        </el-select>
      </el-form-item>
      <el-form-item label="状态">
        <el-select v-model="deployForm.status" class="w-full">
          <el-option v-for="s in statusOptions" :key="s.value" :label="s.label" :value="s.value" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="deployDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="deployDialogLoading" @click="submitPoolDeployment">创建</el-button>
    </template>
  </el-dialog>

  <!-- ================================================================
       Credential import dialog
       ================================================================ -->
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
          <el-button :icon="Plus" @click="fileInputRef.click()">选择 JSON 文件</el-button>

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
            <el-input-number v-model="credForm.weight" :min="0" :max="1000" />
          </el-form-item>
        </el-form>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="credDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="credDialogLoading" @click="submitCredential">
        {{ importMode === 'file'
          ? `导入 ${parsedFiles.filter(f => !f.error).length} 条凭据`
          : '导入' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
/* ============================================================================
   Layout
   ============================================================================ */

.pools-workbench {
  display: flex;
  gap: 20px;
  min-height: 600px;
}

.pools-rail {
  width: 300px;
  flex-shrink: 0;
  background: #ffffff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 4px 20px rgba(15, 23, 42, 0.04);
}

.pools-workspace {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* ============================================================================
   Rail
   ============================================================================ */

.rail-head {
  padding: 20px 20px 16px;
  border-bottom: 1px solid #f1f5f9;
}

.eyebrow {
  margin: 0 0 4px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #94a3b8;
}

.rail-head h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 800;
  color: #0f172a;
}

.provider-tabs {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 12px;
  border-bottom: 1px solid #f1f5f9;
}

.provider-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: none;
  background: transparent;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
  text-align: left;
  transition: all 0.15s;
}

.provider-tab:hover {
  background: #f8fafc;
  color: #0f172a;
}

.provider-tab.active {
  background: color-mix(in srgb, var(--tab-color, #3b82f6) 10%, transparent);
  color: var(--tab-color, #3b82f6);
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
  border: 1px solid #f1f5f9;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.15s;
}

.pool-item:hover {
  border-color: #e2e8f0;
  background: #fafafa;
}

.pool-item.active {
  border-color: #3b82f6;
  background: #eff6ff;
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
  color: #0f172a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pool-meta {
  margin: 4px 0 0;
  font-size: 11px;
  color: #94a3b8;
}

.pool-notes {
  margin: 4px 0 0;
  font-size: 11px;
  color: #64748b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pool-item-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.empty-pools {
  padding: 32px 16px;
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
}

.rail-footer {
  padding: 12px;
  border-top: 1px solid #f1f5f9;
}

/* ============================================================================
   Workspace
   ============================================================================ */

.workspace-hero {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  background: #ffffff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 24px;
  box-shadow: 0 4px 20px rgba(15, 23, 42, 0.04);
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
  font-weight: 800;
  color: #0f172a;
}

.pool-hero-notes {
  margin: 4px 0 0;
  font-size: 13px;
  color: #64748b;
}

.pool-hero-type {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 8px 0 0;
  font-size: 12px;
  color: #94a3b8;
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
  font-weight: 800;
  color: #0f172a;
  line-height: 1;
}

.metric-cell span {
  font-size: 11px;
  color: #94a3b8;
  margin-top: 4px;
}

.panel {
  background: #ffffff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 0 4px 20px rgba(15, 23, 42, 0.04);
}

.section-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.section-head h3 {
  margin: 0 0 4px;
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
}

.section-head p {
  margin: 0;
  font-size: 13px;
  color: #64748b;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.empty-workspace {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: #ffffff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 60px 32px;
  text-align: center;
  box-shadow: 0 4px 20px rgba(15, 23, 42, 0.04);
}

.empty-workspace h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 800;
  color: #0f172a;
}

.empty-workspace p {
  margin: 0;
  font-size: 14px;
  color: #64748b;
  max-width: 360px;
  line-height: 1.6;
}

/* ============================================================================
   Import dialog
   ============================================================================ */

.import-hint {
  font-size: 13px;
  color: #64748b;
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
  color: #94a3b8;
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
  border-radius: 8px;
  border: 1px solid #f1f5f9;
  background: #fafafa;
}

.parsed-file-item.parse-err {
  border-color: #fecaca;
  background: #fff5f5;
}

.parsed-file-item.import-success {
  border-color: #bbf7d0;
  background: #f0fdf4;
}

.parsed-file-item.import-failed {
  border-color: #fecaca;
  background: #fff5f5;
}

.pf-name {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
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
  color: #64748b;
}

.pf-err-msg {
  font-size: 12px;
  color: #ef4444;
}

/* ============================================================================
   Helpers
   ============================================================================ */

.text-green  { color: #10b981; }
.text-red    { color: #ef4444; }
.text-muted  { color: #94a3b8; }
.text-danger { color: #ef4444; font-size: 12px; }
.w-full      { width: 100%; }
</style>
