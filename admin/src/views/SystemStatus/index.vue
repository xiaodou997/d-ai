<script setup>
import { onMounted, shallowRef } from 'vue'
import { Refresh, CircleCheck, CircleClose, Warning } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getSystemStatus } from '@/api/aiGateway'

const loading = shallowRef(false)
const sysStatus = shallowRef(null)

const componentTag = s => s === 'ok' ? 'success' : s === 'disabled' ? 'info' : 'danger'
const componentIcon = s => s === 'ok' ? CircleCheck : s === 'disabled' ? Warning : CircleClose
const formatUntil = ts => ts ? new Date(ts).toLocaleString('zh-CN') : '—'

const fetchSystem = async () => {
  loading.value = true
  try {
    sysStatus.value = await getSystemStatus()
  } catch (err) {
    sysStatus.value = null
    ElMessage.error('加载系统状态失败：' + (err?.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

onMounted(fetchSystem)
</script>

<template>
  <div class="page-container space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="page-head">
      <div>
        <p class="eyebrow">Uni AI API</p>
        <h1>系统状态</h1>
        <p class="subtitle">基础设施健康与熔断器状态 — 故障排查时使用。</p>
      </div>
      <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchSystem">刷新</el-button>
    </section>

    <div v-if="sysStatus">
      <!-- 基础设施 -->
      <p class="section-label">基础设施健康</p>
      <div class="infra-cards">
        <div class="infra-card" :class="`infra-card--${componentTag(sysStatus.db?.status)}`">
          <el-icon :size="28"><component :is="componentIcon(sysStatus.db?.status)" /></el-icon>
          <div class="infra-info">
            <p class="infra-name">PostgreSQL</p>
            <p class="infra-status">{{ sysStatus.db?.status }}</p>
            <p v-if="sysStatus.db?.error" class="infra-error">{{ sysStatus.db.error }}</p>
          </div>
        </div>
        <div class="infra-card" :class="`infra-card--${componentTag(sysStatus.redis?.status)}`">
          <el-icon :size="28"><component :is="componentIcon(sysStatus.redis?.status)" /></el-icon>
          <div class="infra-info">
            <p class="infra-name">Redis</p>
            <p class="infra-status">{{ sysStatus.redis?.status }}</p>
            <p v-if="sysStatus.redis?.error" class="infra-error">{{ sysStatus.redis.error }}</p>
          </div>
        </div>
      </div>

      <!-- 熔断器 -->
      <p class="section-label" style="margin-top:2rem">
        熔断器状态
        <span class="cb-summary">
          跟踪中 {{ sysStatus.circuit_breaker.total_tracked }} 个部署
          <el-tag v-if="sysStatus.circuit_breaker.open_count > 0" type="danger" size="small" class="ml-2">
            {{ sysStatus.circuit_breaker.open_count }} 个熔断
          </el-tag>
          <el-tag v-else type="success" size="small" class="ml-2">全部健康</el-tag>
        </span>
      </p>

      <div v-if="sysStatus.circuit_breaker.total_tracked === 0" class="empty-hint">
        暂无跟踪中的部署（所有线路处于默认健康状态）
      </div>
      <el-table v-else :data="sysStatus.circuit_breaker.states" stripe class="cb-table">
        <el-table-column label="部署 ID" prop="deployment_id" min-width="280">
          <template #default="{ row }"><span class="mono">{{ row.deployment_id }}</span></template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="row.open ? 'danger' : 'success'" size="small">{{ row.open ? '熔断' : '正常' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="连续失败次数" prop="consecutive_failures" width="140" align="right" />
        <el-table-column label="冷却到期时间" min-width="180">
          <template #default="{ row }">{{ formatUntil(row.unhealthy_until) }}</template>
        </el-table-column>
      </el-table>

      <p class="page-footer">最后刷新：{{ new Date(sysStatus.timestamp).toLocaleString('zh-CN') }}</p>
    </div>

    <el-skeleton v-else-if="loading" :rows="6" animated />
    <div v-else class="empty-hint">暂无数据，请点击刷新</div>
  </div>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  background: #fff;
  padding: 24px;
  box-shadow: 0 10px 30px rgba(15,23,42,.04);
}
.eyebrow { margin: 0 0 6px; color: #64748b; font-size: 12px; font-weight: 900; text-transform: uppercase; }
.page-head h1 { margin: 0; color: #0f172a; font-size: 24px; font-weight: 900; }
.subtitle { margin: 8px 0 0; color: #64748b; font-size: 14px; }

.section-label {
  display: flex; align-items: center; gap: 8px;
  font-size: 12px; font-weight: 900; letter-spacing: .05em;
  text-transform: uppercase; color: #94a3b8; margin-bottom: 12px;
}
.cb-summary { display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 13px; text-transform: none; letter-spacing: 0; color: #475569; }
.infra-cards { display: flex; gap: 16px; flex-wrap: wrap; }
.infra-card {
  display: flex; align-items: center; gap: 16px;
  padding: 20px 24px; border-radius: 16px;
  border: 1px solid #f1f5f9; background: #fff;
  min-width: 220px; box-shadow: 0 2px 8px rgba(15,23,42,.05);
}
.infra-card--success :deep(.el-icon) { color: #22c55e; }
.infra-card--danger { border-color: #fecaca; background: #fff5f5; }
.infra-card--danger :deep(.el-icon) { color: #ef4444; }
.infra-card--info :deep(.el-icon) { color: #94a3b8; }
.infra-info { display: flex; flex-direction: column; gap: 2px; }
.infra-name { margin: 0; font-weight: 800; font-size: 14px; color: #1e293b; }
.infra-status { margin: 0; font-size: 12px; color: #64748b; text-transform: uppercase; letter-spacing: .05em; }
.infra-error { margin: 0; font-size: 11px; color: #ef4444; }
.cb-table { border-radius: 12px; overflow: hidden; }
.mono { font-family: 'JetBrains Mono','Fira Code',monospace; font-size: 13px; color: #475569; }
.empty-hint { color: #94a3b8; font-size: 14px; padding: 24px; text-align: center; background: #f8fafc; border-radius: 12px; }
.page-footer { margin-top: 24px; font-size: 12px; color: #cbd5e1; text-align: right; }
.ml-2 { margin-left: 8px; }
</style>
