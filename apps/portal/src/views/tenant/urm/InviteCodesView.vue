<!--
  租户端 — 邀请码管理:创建和管理用于用户注册的邀请码。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       表格/分页同卡);el-table → DsTable(邀请码列 mono),el-tag → DsTag(有效=positive、
       禁用=danger),text-slate-*/text-primary-* 颜色类全部换 --ds-* token;
       数据接入 useListPage,分页始终渲染;创建/禁用/删除逻辑与请求参数保持不变,
       创建弹窗仍为 element-plus;GuideHelpLink 保留在 #actions。
-->
<template>
  <div class="page-container invite-codes-page">
    <PortalPagePanel
      fill
      :icon="Ticket"
      :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '邀请码管理' }]"
      description="创建和管理用于用户注册的邀请码"
    >
      <template #actions>
        <GuideHelpLink to="/help/urm/invite-codes" />
        <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建邀请码</el-button>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="id"
        :loading="loading"
        empty-title="暂无邀请码"
      >
        <template #empty>
          <DsEmpty title="暂无邀请码" description="还没有邀请码,先创建一个吧">
            <template #action>
              <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建邀请码</el-button>
            </template>
          </DsEmpty>
        </template>
        <template #cell-code="{ row }">
          <div class="flex items-center gap-2">
            <span class="invite-codes-code">{{ row.code }}</span>
            <el-tooltip content="复制原始邀请码" placement="top">
              <el-button link type="info" size="small" @click="copyCode(row.code)">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </template>
        <template #cell-registrationUrl="{ row }">
          <div v-if="row.registrationUrl" class="flex flex-col gap-2">
            <span class="invite-codes-url">{{ row.registrationUrl }}</span>
            <div class="invite-codes-url-actions">
              <button type="button" class="invite-codes-link" @click="copyRegistrationUrl(row)">
                复制注册链接
              </button>
              <a
                :href="row.registrationUrl"
                target="_blank"
                rel="noreferrer"
                class="invite-codes-preview-link"
              >
                预览链接
              </a>
            </div>
          </div>
          <span v-else class="invite-codes-dash">—</span>
        </template>
        <template #cell-usage="{ row }">
          <span class="invite-codes-usage">
            {{ row.usedCount ?? 0 }} /
            <span :class="row.maxUses === 0 ? 'invite-codes-usage--unlimited' : 'invite-codes-usage--max'">
              {{ row.maxUses === 0 ? '不限' : row.maxUses }}
            </span>
          </span>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="row.status === 1 ? 'positive' : 'danger'">
            {{ row.status === 1 ? '有效' : '禁用' }}
          </DsTag>
        </template>
        <template #cell-expireTime="{ row }">
          <span v-if="row.expireTime" :class="isExpired(row.expireTime) ? 'invite-codes-expired' : 'invite-codes-expire-time'">
            {{ formatTime(row.expireTime) }}
          </span>
          <span v-else class="invite-codes-never-expire">永不过期</span>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="invite-codes-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button :type="row.status === 1 ? 'warning' : 'success'" link @click="toggleStatus(row)">
            {{ row.status === 1 ? '禁用' : '启用' }}
          </el-button>
          <el-button type="danger" link @click="handleDelete(row)">删除</el-button>
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

    <!-- 创建邀请码 Dialog -->
    <el-dialog
      v-model="createDialogVisible"
      title="创建邀请码"
      width="480px"
      :close-on-click-modal="false"
      :append-to-body="true"
      class="modern-dialog"
    >
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-position="top" class="space-y-1">
        <el-form-item label="备注说明" prop="description">
          <el-input v-model="createForm.description" placeholder="请输入邀请码用途说明（可选）" class="modern-input" />
        </el-form-item>
        <el-form-item label="最大使用次数" prop="max_uses">
          <el-input-number v-model="createForm.max_uses" :min="0" :controls="false" placeholder="0 表示不限" class="w-full!" />
          <p class="invite-codes-form-hint">填写 0 表示不限使用次数</p>
        </el-form-item>
        <el-form-item label="过期时间" prop="expire_time">
          <el-date-picker
            v-model="createForm.expire_time"
            type="datetime"
            placeholder="不填则永不过期"
            format="YYYY-MM-DD HH:mm:ss"
            value-format="x"
            class="w-full!"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <el-button round @click="createDialogVisible = false">取消</el-button>
          <el-button type="primary" round :loading="submitting" @click="handleCreate">创建</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { CopyDocument, Plus } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { FormInstance, FormRules } from "element-plus";
import { Ticket } from "lucide-vue-next";
import { PortalPagePanel, useListPage } from "@dai/app-core";
import { GuideHelpLink } from "@dai/app-core/guide";
import { DsEmpty, DsPagination, DsTable, DsTag, type DsTableColumn } from "@dai/ui";

import { urmTenantApi } from "../../api/urmTenant";
import type { InviteCodeItem } from "../../types/urmTenant";

const columns: DsTableColumn[] = [
  { key: "code", title: "邀请码", width: 200, mono: true },
  { key: "registrationUrl", title: "注册链接", width: 280 },
  { key: "description", title: "备注" },
  { key: "usage", title: "使用情况", width: 120 },
  { key: "status", title: "状态", width: 90 },
  { key: "expireTime", title: "过期时间", width: 180 },
  { key: "createdTime", title: "创建时间", width: 180 },
  { key: "actions", title: "操作", width: 110 }
];

const {
  rows,
  total,
  loading,
  page,
  pageSize,
  refresh,
  handlePageChange,
  handlePageSizeChange
} = useListPage<Record<string, unknown>, InviteCodeItem>({
  initialQuery: {},
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const res = await urmTenantApi.getInviteCodes({ page: params.page, size: params.pageSize });
      const items = res?.items ?? [];
      return { items, total: res?.total ?? items.length };
    } catch (e) {
      console.error("获取邀请码列表失败:", e);
      return { items: [], total: 0 };
    }
  }
});

const createDialogVisible = ref(false);
const submitting = ref(false);
const createFormRef = ref<FormInstance>();

const createForm = reactive<{ description: string; max_uses: number; expire_time: string | null }>({
  description: "",
  max_uses: 100,
  expire_time: null
});

const createRules: FormRules = {
  max_uses: [{ required: true, message: "请填写最大使用次数", trigger: "blur" }]
};

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

function isExpired(ts?: number | null) {
  if (!ts) return false;
  return ts < Date.now();
}

function openCreateDialog() {
  createForm.description = "";
  createForm.max_uses = 100;
  createForm.expire_time = null;
  createDialogVisible.value = true;
}

async function handleCreate() {
  if (!createFormRef.value) return;
  const valid = await createFormRef.value.validate().catch(() => false);
  if (!valid) return;
  submitting.value = true;
  try {
    const created = await urmTenantApi.createInviteCode({
      description: createForm.description,
      max_uses: createForm.max_uses,
      expire_time: createForm.expire_time ? Number(createForm.expire_time) : null
    });
    if (created.registrationUrl) {
      await copyText(created.registrationUrl, "邀请码创建成功，注册链接已复制");
    } else {
      ElMessage.success("邀请码创建成功");
    }
    createDialogVisible.value = false;
    refresh();
  } catch (e) {
    console.error("创建邀请码失败:", e);
  } finally {
    submitting.value = false;
  }
}

async function toggleStatus(row: InviteCodeItem) {
  const newStatus = row.status === 1 ? 2 : 1;
  const actionText = newStatus === 1 ? "启用" : "禁用";
  try {
    await ElMessageBox.confirm(`确定要${actionText}邀请码 "${row.code}" 吗？`, "确认操作", {
      confirmButtonText: "确定",
      cancelButtonText: "取消",
      type: "warning",
      roundButton: true
    });
    await urmTenantApi.updateInviteCode(row.id, { status: newStatus });
    ElMessage.success(`已${actionText}`);
    refresh();
  } catch (e) {
    if (e !== "cancel") console.error("更新邀请码状态失败:", e);
  }
}

async function handleDelete(row: InviteCodeItem) {
  try {
    await ElMessageBox.confirm(`确定要删除邀请码 "${row.code}" 吗？此操作不可撤销。`, "确认删除", {
      confirmButtonText: "确认删除",
      cancelButtonText: "取消",
      type: "warning",
      roundButton: true
    });
    await urmTenantApi.deleteInviteCode(row.id);
    ElMessage.success("删除成功");
    refresh();
  } catch (e) {
    if (e !== "cancel") console.error("删除邀请码失败:", e);
  }
}

async function copyRegistrationUrl(row: InviteCodeItem) {
  if (!row.registrationUrl) return;
  await copyText(row.registrationUrl, "注册链接已复制");
}

async function copyCode(code: string) {
  await copyText(code, "邀请码已复制");
}

async function copyText(text: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(text);
    ElMessage.success(successMessage);
  } catch {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);
    ElMessage.success(successMessage);
  }
}
</script>

<style scoped>
.invite-codes-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.invite-codes-code {
  font-weight: 700;
  font-size: 13px;
  color: var(--ds-accent-hover);
}

.invite-codes-dash {
  color: var(--ds-faint);
}

.invite-codes-url {
  font-size: 12px;
  color: var(--ds-muted);
  word-break: break-all;
}

.invite-codes-url-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  font-weight: 600;
}

.invite-codes-link {
  color: var(--ds-accent-hover);
  cursor: pointer;
}

.invite-codes-link:hover {
  color: var(--ds-accent);
}

.invite-codes-preview-link {
  color: var(--ds-muted);
}

.invite-codes-preview-link:hover {
  color: var(--ds-ink-soft);
}

.invite-codes-usage {
  font-size: 13px;
  font-weight: 500;
  color: var(--ds-ink-soft);
}

.invite-codes-usage--unlimited {
  color: var(--ds-accent-hover);
  font-weight: 700;
}

.invite-codes-usage--max {
  color: var(--ds-muted);
}

.invite-codes-expired {
  color: var(--ds-danger);
}

.invite-codes-expire-time {
  color: var(--ds-ink-soft);
}

.invite-codes-never-expire {
  color: var(--ds-faint);
  font-size: 12px;
}

.invite-codes-time {
  font-size: 12px;
  color: var(--ds-faint);
}

.invite-codes-form-hint {
  font-size: 12px;
  color: var(--ds-faint);
  margin-top: 4px;
}

:deep(.el-input-number) {
  width: 100%;
}
</style>
