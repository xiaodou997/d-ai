<!-- Tenant user-management workspace.
  租户端终端用户列表:搜索 + 表格 + 分页 + 详情/分组策略/创建/充值弹窗。
  重构:迁移至 DsUI 一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡),el-table → DsTable,状态改为确认式开关,空态 DsEmpty,
       分页始终渲染;用户资料与并发统一编辑,分组策略单独配置。
-->
<template>
  <div class="page-container users-page">
    <PortalPagePanel
      :icon="Users"
      :breadcrumbs="[{ label: '用户与权限' }, { label: '用户管理' }]"
      description="管理属于本租户的所有终端用户"
    >
      <template #actions>
        <el-button data-testid="create-user-button" type="primary" :icon="Plus" @click="openCreateDialog">创建用户</el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input
              v-model="keyword"
              placeholder="搜索用户名 / 邮箱 / 手机 / 备注"
              clearable
              class="users-search"
              @keyup.enter="handleSearch"
              @clear="handleSearch"
            >
              <template #prefix><Search class="users-search__icon" /></template>
            </el-input>
          </DsFilterField>
          <template #actions>
            <el-button type="primary" @click="handleSearch">搜索</el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="userList"
        row-key="userId"
        :loading="loading"
      >
        <template #empty>
          <DsEmpty title="暂无终端用户" description="还没有终端用户,先创建一个吧">
            <template #action>
              <el-button type="primary" @click="openCreateDialog">创建用户</el-button>
            </template>
          </DsEmpty>
        </template>
        <template #cell-status="{ row }">
          <DsTag v-if="row.credentialState === 'pending_activation'" tone="neutral">待激活</DsTag>
          <el-tooltip v-else :content="row.status === 1 ? '点击停用用户' : '点击启用用户'" placement="top">
            <el-switch
              :model-value="row.status === 1"
              :loading="isStatusUpdating(row.userId)"
              :disabled="isStatusUpdating(row.userId)"
              :aria-label="`${row.username}状态`"
              inline-prompt
              active-text="启"
              inactive-text="停"
              @change="handleStatusChange(row, Boolean($event))"
            />
          </el-tooltip>
        </template>
        <template #cell-balance="{ row }">
          <span class="users-balance" :class="{ 'users-balance--negative': Number(row.balanceUsd || 0) < 0 }">
            {{ formatDisplayUSD(row.balanceUsd) }}
          </span>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="users-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-internalNote="{ row }">
          <span class="users-note" :title="row.internalNote || ''">{{ row.internalNote || '—' }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button type="primary" link @click="openOverview(row)">详情</el-button>
          <el-button type="primary" link @click="openGroupPolicy(row)">分组策略</el-button>
          <el-button type="primary" link @click="openRecharge(row)">充值</el-button>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handleSizeChange"
        />
      </template>
    </PortalPagePanel>

    <UserGroupPolicyDialog
      :open="groupPolicyDialogOpen"
      :user="groupPolicyTarget"
      @close="groupPolicyDialogOpen = false"
    />

    <!-- 创建用户弹窗 -->
    <el-dialog
      v-model="showCreateDialog"
      title="创建终端用户"
      width="520px"
      :close-on-click-modal="false"
      :append-to-body="true"
      @close="resetCreateForm"
    >
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="90px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="createForm.username" placeholder="请输入用户名" maxlength="50" show-word-limit />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="createForm.email" placeholder="请输入邮箱（可选）" />
        </el-form-item>
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="createForm.phone" placeholder="请输入手机号（可选）" />
        </el-form-item>
        <el-form-item label="内部备注" prop="internalNote">
          <el-input
            v-model="createForm.internalNote"
            type="textarea"
            :rows="3"
            maxlength="500"
            show-word-limit
            placeholder="仅租户内部可见（可选）"
          />
        </el-form-item>
        <el-alert title="创建后将生成一次性激活链接。" type="info" :closable="false" show-icon />
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" :loading="createLoading" @click="submitCreateUser">创建</el-button>
      </template>
    </el-dialog>

    <RechargeDialog
      v-model="showRechargeDialog"
      title="给用户充值"
      target-type-label="用户"
      :target-name="rechargeTarget?.username || ''"
      :target-identity="rechargeTargetIdentity"
      :target-balance-usd="rechargeTarget?.balanceUsd ?? 0"
      :submitting="rechargeLoading"
      @submit="submitRecharge"
    />
  </div>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router";
import { Plus, Search } from "@element-plus/icons-vue";
import { Users } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import RechargeDialog from "@/components/RechargeDialog.vue";
import UserGroupPolicyDialog from "./UserGroupPolicyDialog.vue";
import { formatDisplayUSD } from "@/shared/currency";
import { useTenantUsers } from "./useTenantUsers";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";

import type { EndUserItem } from "@/api/types/platformTenant";

const columns: DsTableColumn[] = [
  { key: "userId", title: "用户 ID", width: 110, mono: true },
  { key: "username", title: "用户名", width: 130 },
  { key: "email", title: "邮箱" },
  { key: "balance", title: "余额", width: 110, align: "right" },
  { key: "internalNote", title: "内部备注" },
  { key: "status", title: "状态", width: 90 },
  { key: "createdTime", title: "注册时间", width: 170 },
  { key: "actions", title: "操作", width: 240 }
];

const router = useRouter();
const {
  keyword,
  page,
  pageSize,
  total,
  loading,
  userList,
  groupPolicyDialogOpen,
  groupPolicyTarget,
  showCreateDialog,
  createLoading,
  createFormRef,
  createForm,
  createRules,
  showRechargeDialog,
  rechargeLoading,
  rechargeTarget,
  rechargeTargetIdentity,
  fetchUsers,
  handleSearch,
  handlePageChange,
  handleSizeChange,
  openCreateDialog,
  resetCreateForm,
  submitCreateUser,
  isStatusUpdating,
  handleStatusChange,
  openGroupPolicy,
  openRecharge,
  submitRecharge
} = useTenantUsers();

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

function openOverview(row: EndUserItem) {
  void router.push(`/tenant/users/directory/${encodeURIComponent(row.userId)}`);
}
</script>

<style scoped>
.users-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.users-search {
  width: 260px;
}

.users-search__icon {
  width: 16px;
  height: 16px;
  color: var(--ds-faint);
}

.users-time {
  font-size: 12px;
  color: var(--ds-faint);
}

.users-balance {
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.users-balance--negative {
  color: var(--ds-danger);
}

.users-note {
  display: block;
  max-width: 220px;
  overflow: hidden;
  color: var(--ds-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

</style>
