<script setup>
import { onMounted, reactive, shallowRef } from 'vue'
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
  { label: '租户', value: 'tenant' },
  { label: '用户', value: 'user' },
  { label: 'API Key', value: 'api_key' },
  { label: 'Provider', value: 'provider' },
  { label: 'Endpoint', value: 'endpoint' }
]

const loading = shallowRef(false)
const saving = shallowRef(false)
const dialogVisible = shallowRef(false)
const editingId = shallowRef('')
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
  } finally {
    saving.value = false
  }
}

const changeStatus = async (row, status) => {
  await updateRuntimeLimitPolicyStatus(row.id, status)
  await fetchPolicies()
}

onMounted(fetchPolicies)
</script>

<template>
  <section class="panel">
    <div class="section-head">
      <div>
        <h3>限流策略</h3>
        <p>策略按 scope、能力和模型叠加生效，任一超限即拒绝</p>
      </div>
      <div class="actions">
        <el-button :icon="Plus" type="primary" @click="openCreate">新增策略</el-button>
        <el-button :icon="Refresh" :loading="loading" @click="fetchPolicies">刷新</el-button>
      </div>
    </div>

    <div class="filters">
      <el-select v-model="filters.scope_type" clearable placeholder="Scope">
        <el-option v-for="item in scopeOptions" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <el-input v-model="filters.scope_id" clearable placeholder="Scope ID" />
      <el-select v-model="filters.capability_type" clearable placeholder="能力">
        <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <el-input v-model="filters.model_code" clearable placeholder="模型" />
      <el-select v-model="filters.status" clearable placeholder="状态">
        <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
      </el-select>
      <el-button :loading="loading" @click="fetchPolicies">查询</el-button>
    </div>

    <el-table v-loading="loading" :data="policies" border stripe class="w-full">
      <el-table-column prop="scope_type" label="Scope" width="110" />
      <el-table-column prop="scope_id" label="Scope ID" min-width="220" show-overflow-tooltip />
      <el-table-column prop="capability_type" label="能力" width="100" />
      <el-table-column prop="model_code" label="模型" min-width="140" show-overflow-tooltip />
      <el-table-column prop="rpm_limit" label="RPM" width="90" align="right" />
      <el-table-column prop="tpm_limit" label="TPM" width="100" align="right" />
      <el-table-column prop="concurrency_limit" label="并发" width="90" align="right" />
      <el-table-column prop="status" label="状态" width="110">
        <template #default="{ row }">
          <el-select :model-value="row.status" size="small" @change="changeStatus(row, $event)">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
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

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑限流策略' : '新增限流策略'" width="560px">
      <el-form :model="form" label-width="110px">
        <el-form-item label="Scope">
          <el-select v-model="form.scope_type">
            <el-option v-for="item in scopeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="Scope ID">
          <el-input v-model="form.scope_id" placeholder="租户/用户 ID 或 UUID" />
        </el-form-item>
        <el-form-item label="能力">
          <el-select v-model="form.capability_type">
            <el-option v-for="item in capabilityOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.model_code" clearable placeholder="留空表示所有模型" />
        </el-form-item>
        <el-form-item label="RPM">
          <el-input-number v-model="form.rpm_limit" :min="1" :step="10" controls-position="right" />
        </el-form-item>
        <el-form-item label="TPM">
          <el-input-number v-model="form.tpm_limit" :min="1" :step="1000" controls-position="right" />
        </el-form-item>
        <el-form-item label="并发">
          <el-input-number v-model="form.concurrency_limit" :min="1" :step="1" controls-position="right" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="savePolicy">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.panel {
  border: 1px solid #f1f5f9;
  border-radius: 14px;
  padding: 16px;
  min-width: 0;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.section-head h3 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
  font-weight: 800;
}

.section-head p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 12px;
}

.actions {
  display: flex;
  gap: 8px;
}

.filters {
  display: grid;
  grid-template-columns: 130px minmax(180px, 1fr) 130px minmax(160px, 1fr) 130px auto;
  gap: 8px;
  margin-bottom: 14px;
}

@media (max-width: 1180px) {
  .section-head {
    align-items: stretch;
    flex-direction: column;
  }

  .filters {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
