<script setup>
import { computed, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh, Setting } from '@element-plus/icons-vue'
import {
  createCredentialPool,
  createPoolCredential,
  deleteCredentialPool,
  deletePoolCredential,
  listCredentialPools,
  listPoolCredentials,
  patchCredentialPool,
  patchPoolCredential,
  refreshPoolCredential
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

// Credential import dialog
const credDialogVisible = shallowRef(false)
const credDialogLoading = shallowRef(false)
const importMode        = ref('json')   // 'json' | 'manual'
const importJsonText    = ref('')
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

watch(selectedPool, fetchCredentials)

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
// Credential CRUD
// ============================================================================

const openCredDialog = () => {
  importMode.value = 'json'
  importJsonText.value = ''
  Object.assign(credForm, { name: '', email: '', access_token: '', refresh_token: '', expires_at: null, weight: 100 })
  credDialogVisible.value = true
}

const parseJsonImport = () => {
  try {
    return JSON.parse(importJsonText.value.trim())
  } catch {
    ElMessage.error('JSON 格式错误，请检查后重试')
    return null
  }
}

const submitCredential = async () => {
  if (!selectedPool.value) return
  credDialogLoading.value = true
  try {
    let payload
    if (importMode.value === 'json') {
      payload = parseJsonImport()
      if (!payload) return
    } else {
      if (!credForm.access_token.trim()) { ElMessage.error('access_token 不能为空'); return }
      payload = { ...credForm }
    }
    await createPoolCredential(selectedPool.value.id, payload)
    ElMessage.success('凭据已导入')
    credDialogVisible.value = false
    await fetchCredentials()
  } finally {
    credDialogLoading.value = false
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
       Credential import dialog
       ================================================================ -->
  <el-dialog v-model="credDialogVisible" title="导入 OAuth 凭据" width="580px">
    <el-tabs v-model="importMode">
      <el-tab-pane label="粘贴导出 JSON" name="json">
        <p class="import-hint">将 Provider 工具导出的 JSON 完整粘贴到下方，系统会自动解析所有字段</p>
        <el-input
          v-model="importJsonText"
          type="textarea"
          :rows="10"
          placeholder='{"provider_type":"codex","access_token":"eyJ...","refresh_token":"rt_...","email":"...",...}'
          class="json-input"
        />
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
      <el-button type="primary" :loading="credDialogLoading" @click="submitCredential">导入</el-button>
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
  margin-bottom: 10px;
}

.json-input :deep(textarea) {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 12px;
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
