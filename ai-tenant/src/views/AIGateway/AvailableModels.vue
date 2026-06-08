<script setup>
import { onMounted, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { listTenantModelGrants, capabilityOptions } from '@/api/aiGateway'

const loading = shallowRef(false)
const models = shallowRef([])
const selectedModel = shallowRef(null)

const fetchModels = async () => {
  loading.value = true
  try {
    const res = await listTenantModelGrants()
    models.value = res || []
  } finally {
    loading.value = false
  }
}

const selectModel = (model) => {
  selectedModel.value = model
}

const statusTagType = (status) => {
  const map = { active: 'success', inactive: 'warning', disabled: 'danger' }
  return map[status] || 'info'
}

const capabilityLabel = (value) =>
  capabilityOptions.find((o) => o.value === value)?.label || value || '-'

onMounted(fetchModels)
</script>

<template>
  <div class="page-container">
    <!-- Header -->
    <header class="page-header">
      <div class="page-title">
        <p class="eyebrow">Available Models</p>
        <h1>已授权模型</h1>
        <p>平台已授权给租户的模型列表，点击查看模型详情和平台公价</p>
      </div>
      <el-button :icon="Refresh" @click="fetchModels" :loading="loading">刷新</el-button>
    </header>

    <!-- Content -->
    <main class="page-main flex gap-6 min-h-0">
      <!-- Left: Model List -->
      <section class="model-list-panel flex-1 min-w-0">
        <el-table :data="models" v-loading="loading" stripe>
          <el-table-column prop="model_code" label="模型编码" min-width="200" />
          <el-table-column label="能力类型" min-width="100">
            <template #default="{ row }">
              <el-tag size="small">{{ capabilityLabel(row.capability_type) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" min-width="80">
            <template #default="{ row }">
              <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="80" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="selectModel(row)">查看</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <!-- Right: Model Detail -->
      <section class="model-detail-panel w-80" v-if="selectedModel">
        <el-card shadow="never" class="detail-card">
          <template #header>
            <div class="card-header">
              <span class="font-bold">{{ selectedModel.model_code }}</span>
              <el-tag size="small" :type="statusTagType(selectedModel.status)">
                {{ selectedModel.status }}
              </el-tag>
            </div>
          </template>

          <div class="detail-info">
            <p class="text-sm text-slate-500 mb-2">模型编码: {{ selectedModel.model_code }}</p>
            <p class="text-sm text-slate-500 mb-4">能力类型: {{ capabilityLabel(selectedModel.capability_type) }}</p>
          </div>

          <el-divider />

          <p class="text-xs text-slate-400">
            该模型的积分单价与售价请在「租户定价」页面查看与设置。
          </p>
        </el-card>
      </section>

      <!-- Empty State -->
      <section class="model-detail-panel w-80 flex items-center justify-center" v-else>
        <p class="text-slate-400 text-sm">选择左侧模型查看详情</p>
      </section>
    </main>
  </div>
</template>

<style scoped>
.page-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.page-header {
  flex-shrink: 0;
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
}

.page-title { display: flex; flex-direction: column; }
.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.page-title h1 { margin: 0; color: #0f172a; font-size: 22px; font-weight: 900; }
.page-title p  { margin: 4px 0 0; color: #64748b; font-size: 13px; }

.page-main {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: 24px;
}

.model-list-panel {
  background: #fff;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
  padding: 0;
  overflow: hidden;
}

.model-detail-panel {
  background: #fff;
  border-radius: 16px;
  border: 1px solid #f1f5f9;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
  padding: 20px;
  overflow-y: auto;
}

.detail-card { border: none !important; box-shadow: none !important; }
:deep(.detail-card .el-card__header) { padding: 0 0 14px; border-bottom: 1px solid #f1f5f9; }
:deep(.detail-card .el-card__body)   { padding: 0; }

.card-header { display: flex; justify-content: space-between; align-items: center; }
.detail-info { padding: 12px 0; }
.price-section { padding: 8px 0; }

.price-row {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
  font-size: 13.5px;
  border-bottom: 1px solid #f8fafc;
}
.price-row .label { color: #64748b; }
.price-row .value { color: #0f172a; font-weight: 600; }

.size-prices { margin-top: 8px; padding-left: 12px; }
.size-item {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  font-size: 13px;
  color: #64748b;
}

:deep(.el-table__header th) {
  background: #f8fafc !important;
  color: #64748b;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
}
</style>
