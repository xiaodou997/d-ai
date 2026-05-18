<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Edit, Plus, Refresh } from '@element-plus/icons-vue'
import {
  capabilityOptions,
  createRuntimeLimitPolicy,
  formatTimestamp,
  listRuntimeLimitPolicies,
  statusOptions,
  updateRuntimeLimitPolicy,
  updateRuntimeLimitPolicyStatus
} from '@/api/aiGateway'

const scopeOptions = [
  { label: '租户', value: 'tenant', desc: '限制整个租户，适合给团队或客户设置总额度。' },
  { label: '用户', value: 'user', desc: '限制单个用户，适合控制个人用量。' },
  { label: 'API Key', value: 'api_key', desc: '限制单个 Key，适合给不同应用分配独立配额。' },
  { label: 'Provider', value: 'provider', desc: '限制某个上游厂商，适合保护特定供应商额度。' },
  { label: 'Endpoint', value: 'endpoint', desc: '限制某个上游端点，适合保护单个部署或地址。' }
]

const limitDimensions = [
  {
    key: 'rpm_limit',
    label: '每分钟请求数',
    code: 'RPM',
    color: '#3b82f6',
    desc: '一分钟内最多允许多少次请求。适合防止短时间内请求突然打满。'
  },
  {
    key: 'tpm_limit',
    label: '每分钟 Token',
    code: 'TPM',
    color: '#10b981',
    desc: '一分钟内最多消耗多少 Token。适合控制长文本、批量生成等高消耗请求。'
  },
  {
    key: 'concurrency_limit',
    label: '同时请求数',
    code: '并发',
    color: '#f59e0b',
    desc: '同一时间最多允许多少个请求在处理中。适合保护上游服务不被排队压垮。'
  }
]

const limitSteps = [
  {
    title: '先匹配对象',
    desc: '系统会根据租户、用户、API Key、Provider、Endpoint 找出和当前请求有关的限流策略。'
  },
  {
    title: '再匹配能力和模型',
    desc: '能力用于区分聊天、图片、视频等类型；模型留空表示该能力下所有模型都适用。'
  },
  {
    title: '任一超限就拒绝',
    desc: '同一个请求可能命中多条策略，只要其中一条达到上限，请求就会被限流。'
  },
  {
    title: '状态控制生效',
    desc: '只有启用状态的策略参与判断。想临时停用策略，可以直接改状态，不需要删除。'
  }
]

const limitPresets = [
  {
    key: 'starter',
    name: '轻量试用',
    values: { rpm_limit: 60, tpm_limit: 120000, concurrency_limit: 5 },
    desc: '适合测试环境、小客户或刚接入的应用。'
  },
  {
    key: 'standard',
    name: '标准业务',
    values: { rpm_limit: 300, tpm_limit: 600000, concurrency_limit: 20 },
    desc: '适合常规线上业务，兼顾体验和风险控制。'
  },
  {
    key: 'busy',
    name: '高并发业务',
    values: { rpm_limit: 1000, tpm_limit: 2000000, concurrency_limit: 80 },
    desc: '适合有明确容量评估的付费客户或内部核心应用。'
  },
  {
    key: 'request-only',
    name: '只控频率',
    values: { rpm_limit: 120, tpm_limit: null, concurrency_limit: null },
    desc: '只限制请求次数，不限制 Token 和同时请求数。'
  }
]

const loading = shallowRef(false)
const saving = shallowRef(false)
const dialogVisible = shallowRef(false)
const editingId = shallowRef('')
const guidePanels = shallowRef([])
const policies = shallowRef([])
const filters = reactive({
  scope_type: '',
  scope_id: '',
  capability_type: '',
  model_code: '',
  status: ''
})
const form = reactive({
  scope_type: 'tenant',
  scope_id: '',
  capability_type: 'chat',
  model_code: '',
  rpm_limit: null,
  tpm_limit: null,
  concurrency_limit: null,
  status: 'active'
})

const activePolicies = computed(() => policies.value.filter(item => item.status === 'active').length)
const disabledPolicies = computed(() => policies.value.length - activePolicies.value)
const modelSpecificPolicies = computed(() => policies.value.filter(item => item.model_code).length)
const objectTypeCount = computed(() => new Set(policies.value.map(item => item.scope_type)).size)

const summaryCards = computed(() => [
  { label: '启用策略', value: activePolicies.value, desc: '正在参与限流判断的策略' },
  { label: '停用策略', value: disabledPolicies.value, desc: '保留配置但暂不生效' },
  { label: '指定模型', value: modelSpecificPolicies.value, desc: '只约束某个模型的策略' },
  { label: '对象类型', value: objectTypeCount.value, desc: '当前覆盖的限流对象种类' }
])

const currentScopeDesc = computed(() => scopeOptions.find(item => item.value === form.scope_type)?.desc || '')

const fetchPolicies = async () => {
  loading.value = true
  try {
    policies.value = await listRuntimeLimitPolicies({
      limit: 200,
      scope_type: filters.scope_type || undefined,
      scope_id: filters.scope_id || undefined,
      capability_type: filters.capability_type || undefined,
      model_code: filters.model_code || undefined,
      status: filters.status || undefined
    })
  } finally {
    loading.value = false
  }
}

const resetForm = () => {
  editingId.value = ''
  Object.assign(form, {
    scope_type: 'tenant',
    scope_id: '',
    capability_type: 'chat',
    model_code: '',
    rpm_limit: null,
    tpm_limit: null,
    concurrency_limit: null,
    status: 'active'
  })
}

const openCreate = () => {
  resetForm()
  dialogVisible.value = true
}

const openEdit = (row) => {
  editingId.value = row.id
  Object.assign(form, {
    scope_type: row.scope_type,
    scope_id: row.scope_id,
    capability_type: row.capability_type,
    model_code: row.model_code || '',
    rpm_limit: row.rpm_limit ?? null,
    tpm_limit: row.tpm_limit ?? null,
    concurrency_limit: row.concurrency_limit ?? null,
    status: row.status
  })
  dialogVisible.value = true
}

const savePolicy = async () => {
  saving.value = true
  try {
    const payload = {
      ...form,
      model_code: form.model_code || undefined,
      rpm_limit: form.rpm_limit || undefined,
      tpm_limit: form.tpm_limit || undefined,
      concurrency_limit: form.concurrency_limit || undefined
    }
    if (editingId.value) {
      await updateRuntimeLimitPolicy(editingId.value, payload)
    } else {
      await createRuntimeLimitPolicy(payload)
    }
    dialogVisible.value = false
    await fetchPolicies()
    ElMessage.success(editingId.value ? '限流策略已更新' : '限流策略已创建')
  } finally {
    saving.value = false
  }
}

const changeStatus = async (row, status) => {
  await updateRuntimeLimitPolicyStatus(row.id, status)
  await fetchPolicies()
  ElMessage.success('策略状态已更新')
}

const scopeLabel = (value) => scopeOptions.find(item => item.value === value)?.label || value || '-'
const capabilityLabel = (value) => capabilityOptions.find(item => item.value === value)?.label || value || '-'
const statusLabel = (value) => statusOptions.find(item => item.value === value)?.label || value || '-'
const statusTagType = (value) => value === 'active' ? 'success' : value === 'inactive' ? 'warning' : 'info'

const formatLimit = (value, unit) => {
  if (!value) return '不限制'
  return `${Number(value).toLocaleString()} ${unit}`
}

const applyLimitPreset = (preset) => {
  form.rpm_limit = preset.values.rpm_limit
  form.tpm_limit = preset.values.tpm_limit
  form.concurrency_limit = preset.values.concurrency_limit
  ElMessage.success(`已套用“${preset.name}”`)
}

function matchesLimitPreset(preset) {
  return ['rpm_limit', 'tpm_limit', 'concurrency_limit'].every((key) => {
    return (form[key] ?? null) === (preset.values[key] ?? null)
  })
}

const activeLimitPreset = computed(() => limitPresets.find(matchesLimitPreset)?.key || '')

onMounted(fetchPolicies)
</script>

<template>
  <section class="limits-page">
    <el-card shadow="never" class="limits-card">
      <template #header>
        <div class="card-header">
          <div>
            <h3 class="card-title">限流策略</h3>
            <p class="card-desc">
              设置谁、调用什么能力、每分钟最多能用多少。多条策略会一起判断，任意一条达到上限就会拒绝本次请求。
            </p>
          </div>
          <div class="header-actions">
            <el-button :icon="Refresh" :loading="loading" @click="fetchPolicies">刷新</el-button>
            <el-button :icon="Plus" type="primary" @click="openCreate">新增策略</el-button>
          </div>
        </div>
      </template>

      <el-collapse v-model="guidePanels" class="guide-collapse">
        <el-collapse-item name="limits-guide">
          <template #title>
            <div class="guide-collapse-title">
              <span>限流如何生效</span>
              <small>查看匹配顺序、拒绝规则，以及 RPM / TPM / 并发的含义</small>
            </div>
          </template>
          <div class="guide-content">
            <div class="step-grid">
              <div v-for="(step, index) in limitSteps" :key="step.title" class="step-card">
                <span class="step-index">{{ index + 1 }}</span>
                <div>
                  <strong>{{ step.title }}</strong>
                  <p>{{ step.desc }}</p>
                </div>
              </div>
            </div>

            <div class="dimension-grid">
              <div v-for="item in limitDimensions" :key="item.key" class="dimension-card">
                <span class="dimension-badge" :style="{ background: item.color }">{{ item.code }}</span>
                <div>
                  <strong>{{ item.label }}</strong>
                  <p>{{ item.desc }}</p>
                </div>
              </div>
            </div>
          </div>
        </el-collapse-item>
      </el-collapse>

      <div class="summary-grid">
        <div v-for="item in summaryCards" :key="item.label" class="summary-card">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <p>{{ item.desc }}</p>
        </div>
      </div>

      <div class="filters">
        <el-select v-model="filters.scope_type" clearable placeholder="作用对象">
          <el-option v-for="item in scopeOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-input v-model="filters.scope_id" clearable placeholder="对象 ID" />
        <el-select v-model="filters.capability_type" clearable placeholder="能力类型">
          <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-input v-model="filters.model_code" clearable placeholder="模型，留空为全部" />
        <el-select v-model="filters.status" clearable placeholder="状态">
          <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-button :loading="loading" type="primary" @click="fetchPolicies">查询</el-button>
      </div>

      <el-table v-loading="loading" :data="policies" border stripe class="limits-table">
        <el-table-column prop="scope_type" label="作用对象" width="110">
          <template #default="{ row }">{{ scopeLabel(row.scope_type) }}</template>
        </el-table-column>
        <el-table-column prop="scope_id" label="对象 ID" min-width="220" show-overflow-tooltip />
        <el-table-column prop="capability_type" label="能力" width="100">
          <template #default="{ row }">{{ capabilityLabel(row.capability_type) }}</template>
        </el-table-column>
        <el-table-column prop="model_code" label="模型" min-width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.model_code || '全部模型' }}</template>
        </el-table-column>
        <el-table-column prop="rpm_limit" label="每分钟请求" width="120" align="right">
          <template #default="{ row }">{{ formatLimit(row.rpm_limit, '次') }}</template>
        </el-table-column>
        <el-table-column prop="tpm_limit" label="每分钟 Token" width="140" align="right">
          <template #default="{ row }">{{ formatLimit(row.tpm_limit, 'Token') }}</template>
        </el-table-column>
        <el-table-column prop="concurrency_limit" label="同时请求" width="120" align="right">
          <template #default="{ row }">{{ formatLimit(row.concurrency_limit, '个') }}</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="130">
          <template #default="{ row }">
            <div class="status-cell">
              <el-tag :type="statusTagType(row.status)" effect="light">{{ statusLabel(row.status) }}</el-tag>
              <el-select :model-value="row.status" size="small" @change="changeStatus(row, $event)">
                <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="170">
          <template #default="{ row }">{{ formatTimestamp(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button :icon="Edit" size="small" link type="primary" @click="openEdit(row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑限流策略' : '新增限流策略'"
      width="760px"
      append-to-body
      class="limit-dialog"
    >
      <el-form :model="form" label-position="top" class="limit-form">
        <section class="dialog-section">
          <div class="dialog-section-head">
            <h4>作用范围</h4>
            <p>先确定这条策略管谁，再决定它约束哪类能力和模型。</p>
          </div>
          <div class="dialog-grid two-cols">
            <el-form-item label="作用对象">
              <el-select v-model="form.scope_type" class="full-field">
                <el-option v-for="item in scopeOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
              <div class="form-hint">{{ currentScopeDesc }}</div>
            </el-form-item>
            <el-form-item label="对象 ID">
              <el-input v-model="form.scope_id" placeholder="租户、用户、API Key、Provider 或 Endpoint 的 ID" />
            </el-form-item>
            <el-form-item label="能力">
              <el-select v-model="form.capability_type" class="full-field">
                <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="模型">
              <el-input v-model="form.model_code" clearable placeholder="留空表示所有模型" />
            </el-form-item>
          </div>
        </section>

        <section class="dialog-section">
          <div class="dialog-section-head">
            <h4>推荐预设</h4>
            <p>点击后自动填入限流数值，再按实际客户、模型成本和上游容量微调。</p>
          </div>
          <div class="preset-grid">
            <button
              v-for="preset in limitPresets"
              :key="preset.key"
              type="button"
              class="preset-card-button"
              :class="{ active: activeLimitPreset === preset.key }"
              @click="applyLimitPreset(preset)"
            >
              <span class="preset-button-name">{{ preset.name }}</span>
              <span class="preset-button-values">
                {{ formatLimit(preset.values.rpm_limit, '次/分钟') }} / {{ formatLimit(preset.values.tpm_limit, 'Token/分钟') }} / {{ formatLimit(preset.values.concurrency_limit, '并发') }}
              </span>
              <span class="preset-button-desc">{{ preset.desc }}</span>
            </button>
          </div>
        </section>

        <section class="dialog-section">
          <div class="dialog-section-head">
            <h4>限流数值</h4>
            <p>至少填写一项。留空表示这一项不限制。</p>
          </div>
          <div class="dialog-grid three-cols">
            <el-form-item label="每分钟请求">
              <el-input-number v-model="form.rpm_limit" :min="1" :step="10" controls-position="right" class="full-field" />
              <div class="form-hint">留空表示不按请求次数限制。</div>
            </el-form-item>
            <el-form-item label="每分钟 Token">
              <el-input-number v-model="form.tpm_limit" :min="1" :step="1000" controls-position="right" class="full-field" />
              <div class="form-hint">留空表示不按 Token 消耗限制。</div>
            </el-form-item>
            <el-form-item label="同时请求">
              <el-input-number v-model="form.concurrency_limit" :min="1" :step="1" controls-position="right" class="full-field" />
              <div class="form-hint">留空表示不按同时请求数限制。</div>
            </el-form-item>
          </div>
        </section>

        <section class="dialog-section compact-section">
          <div class="dialog-section-head">
            <h4>策略状态</h4>
            <p>启用后立即参与后续请求判断；停用后保留配置但不生效。</p>
          </div>
          <el-form-item label="状态" class="status-form-item">
            <el-select v-model="form.status" class="status-select">
              <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </section>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="savePolicy">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.limits-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.limits-card {
  min-width: 0;
  border: 1px solid #f1f5f9;
  border-radius: 12px;
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.card-title {
  margin: 0;
  color: #0f172a;
  font-size: 15px;
  font-weight: 700;
}

.card-desc {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.summary-grid,
.dimension-grid,
.step-grid,
.preset-grid {
  display: grid;
  gap: 12px;
}

.summary-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-bottom: 14px;
}

.summary-card {
  padding: 14px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #f8fafc;
}

.summary-card span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.summary-card strong {
  display: block;
  margin-top: 8px;
  color: #0f172a;
  font-size: 24px;
  font-weight: 900;
}

.summary-card p,
.dimension-card p,
.step-card p {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.dimension-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.dimension-card,
.step-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #ffffff;
}

.dimension-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  color: #ffffff;
  font-size: 11px;
  font-weight: 800;
  flex-shrink: 0;
  margin-top: 1px;
}

.dimension-card strong,
.step-card strong {
  color: #0f172a;
  font-size: 13px;
  font-weight: 800;
}

.filters {
  display: grid;
  grid-template-columns: 130px minmax(180px, 1fr) 130px minmax(160px, 1fr) 130px auto;
  gap: 8px;
  align-items: center;
  margin-bottom: 14px;
}

.limits-table {
  width: 100%;
}

.status-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.step-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.guide-collapse {
  margin-bottom: 16px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  overflow: hidden;
}

.guide-collapse :deep(.el-collapse-item__header) {
  height: auto;
  padding: 14px 16px;
  border-bottom: 0;
  background: #f8fafc;
}

.guide-collapse :deep(.el-collapse-item__content) {
  padding: 0;
}

.guide-collapse :deep(.el-collapse-item__wrap) {
  border-bottom: 0;
}

.guide-collapse-title {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.guide-collapse-title span {
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
}

.guide-collapse-title small {
  color: #64748b;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
}

.guide-content {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
  background: #ffffff;
}

.step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: #eff6ff;
  color: #2563eb;
  font-size: 12px;
  font-weight: 800;
  flex-shrink: 0;
}

.form-hint {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.preset-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.limit-dialog :deep(.el-dialog__body) {
  padding-top: 8px;
}

.limit-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dialog-section {
  padding: 16px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #ffffff;
}

.dialog-section-head {
  margin-bottom: 14px;
}

.dialog-section-head h4 {
  margin: 0;
  color: #0f172a;
  font-size: 14px;
  font-weight: 800;
}

.dialog-section-head p {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.dialog-grid {
  display: grid;
  gap: 14px;
}

.two-cols {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.three-cols {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.dialog-grid :deep(.el-form-item),
.compact-section :deep(.el-form-item) {
  margin-bottom: 0;
}

.full-field {
  width: 100%;
}

.full-field :deep(.el-input-number) {
  width: 100%;
}

.preset-card-button {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
  width: 100%;
  min-height: 136px;
  padding: 16px;
  border: 1px solid #dbe3ef;
  border-radius: 8px;
  background: #ffffff;
  color: #334155;
  cursor: pointer;
  text-align: left;
  line-height: 1.4;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background 0.2s ease;
}

.preset-card-button:hover {
  border-color: #93c5fd;
  box-shadow: 0 8px 20px rgba(37, 99, 235, 0.08);
}

.preset-card-button.active {
  border-color: #2563eb;
  background: #eff6ff;
  color: #1d4ed8;
}

.preset-button-name {
  display: block;
  color: inherit;
  font-size: 13px;
  font-weight: 800;
}

.preset-button-values {
  display: block;
  margin-top: 8px;
  color: inherit;
  font-size: 12px;
  line-height: 1.55;
}

.preset-button-desc {
  display: block;
  margin-top: 8px;
  color: inherit;
  font-size: 12px;
  line-height: 1.6;
  opacity: 0.86;
}

.compact-section {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 16px;
  align-items: start;
}

.compact-section .dialog-section-head {
  margin-bottom: 0;
}

.status-select {
  width: 100%;
}

@media (max-width: 1180px) {
  .filters {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .summary-grid,
  .dimension-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .three-cols {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 760px) {
  .card-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .summary-grid,
  .dimension-grid,
  .step-grid,
  .filters,
  .preset-grid,
  .two-cols,
  .three-cols,
  .compact-section {
    grid-template-columns: 1fr;
  }
}
</style>
