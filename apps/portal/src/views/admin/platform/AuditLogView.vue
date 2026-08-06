<template>
  <div class="audit-log-page">
    <PortalPagePanel
      :icon="FileSearch"
      :breadcrumbs="[{ label: '用户中心' }, { label: '系统审计' }, { label: '认证审计' }]"
      description="查看用户登录与 Token 刷新的认证审计流水。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="事件类型">
            <el-select v-model="query.eventType" placeholder="全部事件类型" clearable class="audit-log-select">
              <el-option label="用户登录" value="user_login" />
              <el-option label="Token 刷新" value="token_refresh" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="主体">
            <el-select v-model="query.principalType" placeholder="全部主体" clearable class="audit-log-select">
              <el-option label="用户" value="user" />
              <el-option label="管理员" value="admin" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="决策">
            <el-select v-model="query.decision" placeholder="全部决策" clearable class="audit-log-select">
              <el-option label="成功" value="success" />
              <el-option label="拒绝" value="deny" />
              <el-option label="错误" value="error" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="User ID">
            <el-input v-model="query.userId" placeholder="User ID" clearable class="audit-log-input" @keyup.enter="search" />
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search">
              <Search class="audit-log-button-icon" />
              筛选
            </el-button>
            <el-button @click="resetQuery">
              <RefreshRight class="audit-log-button-icon" />
              重置
            </el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="id"
        :loading="loading"
        empty-title="暂无审计记录"
      >
        <template #cell-eventType="{ row }">
          <span class="audit-log-mono">{{ row.eventType }}</span>
        </template>
        <template #cell-principalType="{ row }">
          <DsTag :tone="principalTone(row.principalType)">
            {{ principalLabel(row.principalType) }}
          </DsTag>
        </template>
        <template #cell-decision="{ row }">
          <DsTag :tone="decisionTone(row.decision)">
            {{ decisionLabel(row.decision) }}
          </DsTag>
        </template>
        <template #cell-userId="{ row }">
          <span v-if="row.userId" class="audit-log-mono">{{ row.userId }}</span>
          <span v-else class="audit-log-empty">—</span>
        </template>
        <template #cell-reasonCode="{ row }">
          <span v-if="row.reasonCode" class="audit-log-reason">{{ row.reasonCode }}</span>
          <span v-else class="audit-log-empty">—</span>
        </template>
        <template #cell-createdAt="{ row }">
          <span class="audit-log-time">{{ formatTime(row.createdAt) }}</span>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { RefreshRight, Search } from '@element-plus/icons-vue'
import { FileSearch } from 'lucide-vue-next'
import { PortalPagePanel, useListPage } from '@/platform'
import {
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'
import type { AuditLogItem } from '@/api/types/admin'

const columns: DsTableColumn[] = [
  { key: 'eventType', title: '事件', width: 160 },
  { key: 'principalType', title: '主体', width: 90, align: 'center' },
  { key: 'decision', title: '决策', width: 90, align: 'center' },
  { key: 'userId', title: 'User ID' },
  { key: 'reasonCode', title: '拒绝原因' },
  { key: 'createdAt', title: '时间', width: 170 }
]

const {
  rows,
  total,
  loading,
  page,
  pageSize,
  query,
  search,
  resetQuery,
  handlePageChange,
  handlePageSizeChange
} = useListPage<
  { eventType: string; principalType: string; decision: string; userId: string },
  AuditLogItem
>({
  initialQuery: { eventType: '', principalType: '', decision: '', userId: '' },
  pageSize: 20,
  // 显式标注返回类型：catch 分支引用 total.value，避免与 total 的推断形成循环
  fetcher: async (params): Promise<{ items: AuditLogItem[]; total: number }> => {
    try {
      const data = await platformAdminApi.getAuthAuditLogs({
        page: params.page,
        size: params.pageSize,
        eventType: params.eventType || undefined,
        principalType: params.principalType || undefined,
        decision: params.decision || undefined,
        userId: params.userId.trim() || undefined
      })
      return { items: data.items || [], total: data.total || 0 }
    } catch {
      // 保持原页面行为：请求失败时静默清空列表，total 维持不变
      return { items: [], total: total.value }
    }
  }
})

const formatTime = (ts?: number) => (ts ? new Date(ts).toLocaleString('zh-CN', { hour12: false }) : '—')

const principalLabel = (t: string) => (({ user: '用户', admin: '管理员' } as any)[t] || t)
const principalTone = (t: string): 'accent' | 'warning' | 'neutral' | 'info' =>
  (({ user: 'accent', admin: 'neutral' } as any)[t] || 'info')
const decisionLabel = (d: string) => (({ success: '通过', deny: '拒绝', error: '错误' } as any)[d] || d)
const decisionTone = (d: string): 'positive' | 'danger' | 'warning' | 'info' =>
  (({ success: 'positive', deny: 'danger', error: 'warning' } as any)[d] || 'info')
</script>

<style scoped>
.audit-log-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.audit-log-select {
  width: 160px;
}

.audit-log-input {
  width: min(180px, 100%);
}

.audit-log-button-icon {
  width: 16px;
  height: 16px;
  margin-right: 4px;
}

.audit-log-mono {
  font-family: var(--ds-font-mono);
  font-size: 12px;
  color: var(--ds-ink-soft);
}

.audit-log-empty {
  color: var(--ds-faint);
}

.audit-log-reason {
  font-size: 12px;
  color: var(--ds-danger);
}

.audit-log-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
