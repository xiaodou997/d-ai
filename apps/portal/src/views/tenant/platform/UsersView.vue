<!--
  租户端终端用户列表:搜索 + 表格 + 分页 + 创建/充值弹窗。
  重构:迁移至 DsUI 一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡),el-table → DsTable,el-tag → DsTag,空态 DsEmpty,
       分页始终渲染;创建/充值弹窗仍为 element-plus,业务逻辑与请求参数不变。
-->
<template>
  <div class="page-container users-page">
    <PortalPagePanel
      :icon="Users"
      :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '终端用户' }]"
      description="管理属于本租户的所有终端用户"
    >
      <template #actions>
        <el-button type="primary" :icon="Plus" @click="openCreateDialog">创建用户</el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input
              v-model="keyword"
              placeholder="搜索用户名 / 邮箱 / 手机"
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
          <DsTag :tone="row.status === 1 ? 'positive' : 'danger'">
            {{ row.status === 1 ? '正常' : '禁用' }}
          </DsTag>
        </template>
        <template #cell-createdTime="{ row }">
          <span class="users-time">{{ formatTime(row.createdTime) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button type="primary" link @click="openOverview(row)">详情</el-button>
          <el-button v-if="row.status === 1" type="danger" link @click="handleDisable(row)">停用</el-button>
          <el-button v-else type="success" link @click="handleEnable(row)">启用</el-button>
          <el-button type="warning" link @click="handleResetPassword(row)">重置密码</el-button>
          <el-button type="primary" link @click="openRecharge(row)">充值</el-button>
          <el-button type="danger" link @click="handleDelete(row)">删除</el-button>
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
        <el-alert
          title="默认密码为 123456，创建后请通知终端用户登录后自行修改。"
          type="warning"
          :closable="false"
          show-icon
        />
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
      :target-credits="rechargeTarget?.credits ?? 0"
      :submitting="rechargeLoading"
      @submit="submitRecharge"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { Plus, Search } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { FormInstance, FormRules } from "element-plus";
import { Users } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import RechargeDialog from "@/components/RechargeDialog.vue";
import type { RechargeFormPayload } from "@/components/recharge";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";

import { platformTenantApi } from "@/api/platformTenant";
import type { EndUserItem } from "@/api/types/platformTenant";

const columns: DsTableColumn[] = [
  { key: "userId", title: "用户 ID", width: 110, mono: true },
  { key: "username", title: "用户名", width: 130 },
  { key: "email", title: "邮箱" },
  { key: "status", title: "状态", width: 80 },
  { key: "createdTime", title: "注册时间", width: 170 },
  { key: "actions", title: "操作", width: 285 }
];

const router = useRouter();
const keyword = ref("");
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const loading = ref(false);
const userList = ref<EndUserItem[]>([]);

const showCreateDialog = ref(false);
const createLoading = ref(false);
const createFormRef = ref<FormInstance>();
const createForm = reactive({ username: "", email: "", phone: "" });
const createRules: FormRules = {
  username: [{ required: true, message: "请输入用户名", trigger: "blur" }]
};

function formatTime(ts?: number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString("zh-CN");
}

async function fetchUsers() {
  loading.value = true;
  try {
    const res = await platformTenantApi.getUsers({
      keyword: keyword.value || undefined,
      page: page.value,
      size: pageSize.value
    });
    userList.value = res?.items ?? [];
    total.value = res?.total ?? userList.value.length;
  } catch (e) {
    console.error("获取用户列表失败:", e);
    userList.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

function handleSearch() {
  page.value = 1;
  fetchUsers();
}

function handlePageChange(value: number) {
  page.value = value;
  fetchUsers();
}

function handleSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
  fetchUsers();
}

function openCreateDialog() {
  createForm.username = "";
  createForm.email = "";
  createForm.phone = "";
  showCreateDialog.value = true;
}

function resetCreateForm() {
  createForm.username = "";
  createForm.email = "";
  createForm.phone = "";
  createFormRef.value?.clearValidate();
}

async function submitCreateUser() {
  const valid = await createFormRef.value?.validate().catch(() => false);
  if (!valid) return;
  createLoading.value = true;
  try {
    const data = await platformTenantApi.createEndUser({
      username: createForm.username.trim(),
      email: createForm.email?.trim() || undefined,
      phone: createForm.phone?.trim() || undefined
    });
    ElMessage.success(`用户创建成功，默认密码 ${data?.defaultPassword || "123456"}`);
    showCreateDialog.value = false;
    fetchUsers();
  } catch (e: any) {
    ElMessage.error(e?.message || "创建失败");
  } finally {
    createLoading.value = false;
  }
}

async function handleToggleStatus(row: EndUserItem, status: "active" | "disabled") {
  const action = status === "active" ? "启用" : "停用";
  const isDisable = status === "disabled";
  try {
    await ElMessageBox.confirm(
      `确定要${action}用户「${row.username}」吗？${isDisable ? "停用后该用户将立即无法登录。" : ""}`,
      `确认${action}`,
      {
        confirmButtonText: `确认${action}`,
        cancelButtonText: "取消",
        type: isDisable ? "warning" : "info",
        confirmButtonClass: isDisable ? "el-button--danger" : ""
      }
    );
    await platformTenantApi.updateUserStatus(row.userId, status);
    ElMessage.success(`用户已${action}`);
    fetchUsers();
  } catch (e: any) {
    if (e !== "cancel") ElMessage.error(e?.message || "操作失败");
  }
}

const handleDisable = (row: EndUserItem) => handleToggleStatus(row, "disabled");
const handleEnable = (row: EndUserItem) => handleToggleStatus(row, "active");

async function handleDelete(row: EndUserItem) {
  try {
    await ElMessageBox.confirm(
      `确定要删除用户「${row.username}」吗？账号将立即无法登录，且会从用户列表移除；历史账务与用量记录仍会保留。`,
      "确认删除用户",
      {
        confirmButtonText: "确认删除",
        cancelButtonText: "取消",
        type: "warning",
        confirmButtonClass: "el-button--danger"
      }
    );
    await platformTenantApi.deleteEndUser(row.userId);
    ElMessage.success("用户已删除");
    if (userList.value.length === 1 && page.value > 1) {
      page.value -= 1;
    }
    fetchUsers();
  } catch (e: any) {
    if (e !== "cancel") ElMessage.error(e?.message || "删除失败");
  }
}

function openOverview(row: EndUserItem) {
  void router.push(`/tenant/users/directory/${encodeURIComponent(row.userId)}`);
}

async function handleResetPassword(row: EndUserItem) {
  try {
    await ElMessageBox.confirm(`确定要将用户「${row.username}」的密码重置为 123456 吗？`, "确认重置密码", {
      confirmButtonText: "确认重置",
      cancelButtonText: "取消",
      type: "warning"
    });
    await platformTenantApi.resetUserPassword(row.userId);
    ElMessage.success("密码已重置为 123456");
  } catch (e: any) {
    if (e !== "cancel") ElMessage.error(e?.message || "操作失败");
  }
}

// --- 充值 ---
const showRechargeDialog = ref(false);
const rechargeLoading = ref(false);
const rechargeTarget = ref<EndUserItem | null>(null);
const rechargeTargetIdentity = computed(() => {
  const target = rechargeTarget.value;
  if (!target) return "";
  if (target.phone?.trim()) return `手机号 ${target.phone.trim()}`;
  if (target.email?.trim()) return `邮箱 ${target.email.trim()}`;
  return `UID ${target.userId}`;
});

function openRecharge(row: EndUserItem) {
  rechargeTarget.value = row;
  showRechargeDialog.value = true;
}

async function submitRecharge(payload: RechargeFormPayload) {
  if (!rechargeTarget.value) return;
  rechargeLoading.value = true;
  try {
    await platformTenantApi.rechargeUser({
      userId: rechargeTarget.value.userId,
      ...payload
    });
    ElMessage.success(`已成功为「${rechargeTarget.value.username}」充值 ${payload.creditAmount.toLocaleString()} 积分`);
    showRechargeDialog.value = false;
    fetchUsers();
  } catch (e: any) {
    ElMessage.error(e?.message || "充值失败");
  } finally {
    rechargeLoading.value = false;
  }
}

onMounted(() => {
  fetchUsers();
});
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

</style>
