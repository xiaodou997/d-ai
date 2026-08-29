<!-- JWT key-management workspace — 1:1 搬运自 v1/platform/platform-admin/src/views/System/JwtKeys.vue（api → platformAdminApi，dayjs → 本地格式化）
     重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
          提示条与表格同卡、仅以 1px 分隔线分区）；轮换确认框、接口参数与格式化函数保持不变。 -->
<template>
  <div class="jwt-keys-page">
    <PortalPagePanel
      :icon="KeyRound"
      :breadcrumbs="[{ label: '用户中心' }, { label: '系统审计' }, { label: 'JWT 密钥' }]"
      description="管理用于签发和验证 JWT 的 RSA 密钥对。"
    >
      <template #actions>
        <el-button type="primary" class="rounded-2xl! font-bold px-6 h-11" :loading="rotating" @click="confirmRotate">
          轮换密钥
        </el-button>
      </template>

      <!-- 提示条:同卡置于页头与表格之间,仅用 1px 分隔线与表格分区 -->
      <div class="jwt-keys-tips-section">
        <div class="jwt-keys-tips">
          <el-icon class="jwt-keys-tips__icon"><InfoFilled /></el-icon>
          <div class="jwt-keys-tips__body">
            <p>业务系统通过 <code>/public/jwks.json</code> 获取公钥。</p>
            <p>轮换后旧密钥进入 <span class="jwt-keys-grace">24 小时宽限期</span>，期间新旧 token 均有效。</p>
          </div>
        </div>
      </div>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="keys"
        row-key="kid"
        :loading="loading"
        empty-title="暂无密钥数据"
      >
        <template #cell-status="{ row }">
          <DsTag :tone="statusTone(row.status)">
            {{ statusLabel(row.status) }}
          </DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="jwt-keys-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-graceUntil="{ row }">
          <span v-if="row.graceUntil" class="jwt-keys-time jwt-keys-time--warning">{{ formatTime(row.graceUntil) }}</span>
          <span v-else class="jwt-keys-time jwt-keys-time--empty">—</span>
        </template>
        <template #cell-retiredTime="{ row }">
          <span v-if="row.retiredTime" class="jwt-keys-time">{{ formatTime(row.retiredTime) }}</span>
          <span v-else class="jwt-keys-time jwt-keys-time--empty">—</span>
        </template>
      </DsTable>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { InfoFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { KeyRound } from 'lucide-vue-next'
import { PortalPagePanel } from '@/platform'
import { DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import type { JwtKeyItem } from '@/api/types/admin'

const loading = ref(false)
const rotating = ref(false)
const keys = ref<JwtKeyItem[]>([])

const columns: DsTableColumn[] = [
  { key: 'kid', title: '密钥 ID (kid)', mono: true },
  { key: 'status', title: '状态', width: 100 },
  { key: 'createdTime', title: '创建时间', width: 180 },
  { key: 'graceUntil', title: '宽限期截止', width: 180 },
  { key: 'retiredTime', title: '退役时间', width: 180 }
]

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : '—')

const statusLabel = (status: string) => {
  if (status === 'active') return '签发中'
  if (status === 'grace') return '宽限期'
  return '已退役'
}

const statusTone = (status: string): 'positive' | 'warning' | 'neutral' => {
  if (status === 'active') return 'positive'
  if (status === 'grace') return 'warning'
  return 'neutral'
}

const fetchKeys = async () => {
  loading.value = true
  try {
    const res = await platformAdminApi.listJwtKeys()
    keys.value = res?.keys || []
  } catch {
    ElMessage.error('获取密钥列表失败')
  } finally {
    loading.value = false
  }
}

const confirmRotate = async () => {
  try {
    await ElMessageBox.confirm(
      '确定要轮换 JWT 密钥吗？\n\n轮换后：\n• 立即生成新密钥用于签发 token\n• 旧密钥进入 24 小时宽限期，期间签发的 token 仍可正常验签\n• 24 小时后旧密钥自动退役',
      '确认密钥轮换',
      { confirmButtonText: '确认轮换', cancelButtonText: '取消', type: 'warning' }
    )
    rotating.value = true
    await platformAdminApi.rotateJwtKey()
    ElMessage.success('密钥轮换成功，旧密钥将在 24 小时后退役')
    await fetchKeys()
  } catch (e: unknown) {
    if (e !== 'cancel') ElMessage.error(errorMessage(e, '密钥轮换失败'))
  } finally {
    rotating.value = false
  }
}

onMounted(fetchKeys)

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'object' && error !== null && 'detail' in error) {
    const detail = (error as { detail?: unknown }).detail
    if (typeof detail === 'string' && detail) return detail
  }
  return fallback
}
</script>

<style scoped>
.jwt-keys-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 提示条分区:同卡内嵌,仅保留下方 1px 分隔线 */
.jwt-keys-tips-section {
  padding: 14px 24px;
  border-bottom: 1px solid var(--ds-line);
}

.jwt-keys-tips {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-accent-soft);
  box-shadow: var(--ds-shadow-sm);
}

.jwt-keys-tips__icon {
  flex: 0 0 auto;
  margin-top: 1px;
  color: var(--ds-accent);
  font-size: 17px;
}

.jwt-keys-tips__body {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.jwt-keys-tips__body p {
  margin: 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.jwt-keys-tips code {
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--ds-panel-muted);
  border: 1px solid var(--ds-line);
  font-family: var(--ds-font-mono);
  font-size: 11px;
  color: var(--ds-ink-soft);
}

.jwt-keys-grace {
  color: var(--ds-warning);
  font-weight: 600;
}

.jwt-keys-time {
  font-size: 12px;
  color: var(--ds-muted);
}

.jwt-keys-time--warning {
  color: var(--ds-warning);
}

.jwt-keys-time--empty {
  color: var(--ds-faint);
}

@media (max-width: 768px) {
  .jwt-keys-tips-section {
    padding-inline: 16px;
  }
}
</style>
