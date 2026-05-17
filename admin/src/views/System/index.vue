<script setup>
import { onMounted, shallowRef } from 'vue'
import { Refresh, CircleCheck, CircleClose, Warning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getSystemStatus } from '@/api/aiGateway'

const loading = shallowRef(false)
const status = shallowRef(null)

const load = async () => {
  loading.value = true
  try {
    status.value = await getSystemStatus()
  } catch (err) {
    status.value = null
    ElMessage.error('加载系统状态失败：' + (err?.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

const componentTag = (s) => {
  if (s === 'ok') return 'success'
  if (s === 'disabled') return 'info'
  return 'danger'
}

const componentIcon = (s) => {
  if (s === 'ok') return CircleCheck
  if (s === 'disabled') return Warning
  return CircleClose
}

const formatUntil = (ts) => {
  if (!ts) return '—'
  return new Date(ts).toLocaleString('zh-CN')
}

onMounted(load)
</script>

<template>
  <div class="system-page">
    <div class="page-header">
      <h1 class="page-title">系统状态</h1>
      <el-button :loading="loading" :icon="Refresh" @click="load">刷新</el-button>
    </div>

    <div v-if="status" class="content">
      <!-- DB / Redis health -->
      <div class="section-title">基础设施健康</div>
      <div class="infra-cards">
        <div class="infra-card" :class="`infra-card--${componentTag(status.db?.status)}`">
          <el-icon :size="28"><component :is="componentIcon(status.db?.status)" /></el-icon>
          <div class="infra-info">
            <p class="infra-name">PostgreSQL</p>
            <p class="infra-status">{{ status.db?.status }}</p>
            <p v-if="status.db?.error" class="infra-error">{{ status.db?.error }}</p>
          </div>
        </div>
        <div class="infra-card" :class="`infra-card--${componentTag(status.redis?.status)}`">
          <el-icon :size="28"><component :is="componentIcon(status.redis?.status)" /></el-icon>
          <div class="infra-info">
            <p class="infra-name">Redis</p>
            <p class="infra-status">{{ status.redis?.status }}</p>
            <p v-if="status.redis?.error" class="infra-error">{{ status.redis?.error }}</p>
          </div>
        </div>
      </div>

      <!-- Circuit Breaker -->
      <div class="section-title" style="margin-top: 2rem;">
        熔断器状态
        <span class="cb-summary">
          跟踪中 {{ status.circuit_breaker.total_tracked }} 个部署
          <el-tag v-if="status.circuit_breaker.open_count > 0" type="danger" size="small" class="ml-2">
            {{ status.circuit_breaker.open_count }} 个熔断
          </el-tag>
          <el-tag v-else type="success" size="small" class="ml-2">全部健康</el-tag>
        </span>
      </div>

      <div v-if="status.circuit_breaker.total_tracked === 0" class="empty-hint">
        暂无跟踪中的部署（所有线路处于默认健康状态）
      </div>

      <el-table
        v-else
        :data="status.circuit_breaker.states"
        stripe
        class="cb-table"
      >
        <el-table-column label="部署 ID" prop="deployment_id" min-width="280">
          <template #default="{ row }">
            <span class="mono">{{ row.deployment_id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="row.open ? 'danger' : 'success'" size="small">
              {{ row.open ? '熔断' : '正常' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="连续失败次数" prop="consecutive_failures" width="140" align="right" />
        <el-table-column label="冷却到期时间" min-width="180">
          <template #default="{ row }">
            {{ formatUntil(row.unhealthy_until) }}
          </template>
        </el-table-column>
      </el-table>

      <p class="page-footer">
        最后刷新：{{ new Date(status.timestamp).toLocaleString('zh-CN') }}
      </p>
    </div>

    <el-skeleton v-else :rows="6" animated />
  </div>
</template>

<style scoped lang="scss">
.system-page {
  padding: 2rem;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.5rem;
}

.page-title {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 800;
  color: #1e293b;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  font-weight: 900;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: #94a3b8;
  margin-bottom: 0.75rem;
}

.cb-summary {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 600;
  font-size: 0.85rem;
  text-transform: none;
  letter-spacing: 0;
  color: #475569;
}

.infra-cards {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.infra-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.25rem 1.5rem;
  border-radius: 1rem;
  border: 1px solid #f1f5f9;
  background: #fff;
  min-width: 220px;
  box-shadow: 0 2px 8px rgba(15, 23, 42, 0.05);
}

.infra-card--success {
  .el-icon { color: #22c55e; }
}
.infra-card--danger {
  border-color: #fecaca;
  background: #fff5f5;
  .el-icon { color: #ef4444; }
}
.infra-card--info {
  .el-icon { color: #94a3b8; }
}

.infra-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.infra-name {
  margin: 0;
  font-weight: 800;
  font-size: 0.9rem;
  color: #1e293b;
}

.infra-status {
  margin: 0;
  font-size: 0.75rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.infra-error {
  margin: 0;
  font-size: 0.72rem;
  color: #ef4444;
}

.cb-table {
  border-radius: 0.75rem;
  overflow: hidden;
}

.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 0.8rem;
  color: #475569;
}

.empty-hint {
  color: #94a3b8;
  font-size: 0.875rem;
  padding: 1.5rem;
  text-align: center;
  background: #f8fafc;
  border-radius: 0.75rem;
}

.page-footer {
  margin-top: 1.5rem;
  font-size: 0.75rem;
  color: #cbd5e1;
  text-align: right;
}

.ml-2 {
  margin-left: 0.5rem;
}
</style>
