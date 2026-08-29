import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { FormInstance, FormRules } from "element-plus";

import { platformTenantApi } from "@/api/platformTenant";
import type { EndUserItem } from "@/api/types/platformTenant";
import { showActivationCredential } from "@/platform/auth/activation";
import type { RechargeFormPayload } from "@/components/recharge";

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export function useTenantUsers() {
  const keyword = ref("");
  const page = ref(1);
  const pageSize = ref(20);
  const total = ref(0);
  const loading = ref(false);
  const userList = ref<EndUserItem[]>([]);
  const statusUpdatingIds = ref<Set<string>>(new Set());
  const editDialogOpen = ref(false);
  const editTarget = ref<EndUserItem | null>(null);
  const groupPolicyDialogOpen = ref(false);
  const groupPolicyTarget = ref<EndUserItem | null>(null);

  const showCreateDialog = ref(false);
  const createLoading = ref(false);
  const createFormRef = ref<FormInstance>();
  const createForm = reactive({ username: "", email: "", phone: "", internalNote: "" });
  const createRules: FormRules = {
    username: [{ required: true, message: "请输入用户名", trigger: "blur" }]
  };

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
    } catch (error) {
      console.error("获取用户列表失败:", error);
      userList.value = [];
      total.value = 0;
    } finally {
      loading.value = false;
    }
  }

  function handleSearch() {
    page.value = 1;
    void fetchUsers();
  }

  function handlePageChange(value: number) {
    page.value = value;
    void fetchUsers();
  }

  function handleSizeChange(value: number) {
    pageSize.value = value;
    page.value = 1;
    void fetchUsers();
  }

  function resetCreateForm() {
    createForm.username = "";
    createForm.email = "";
    createForm.phone = "";
    createForm.internalNote = "";
    createFormRef.value?.clearValidate();
  }

  function openCreateDialog() {
    resetCreateForm();
    showCreateDialog.value = true;
  }

  async function submitCreateUser() {
    const valid = await createFormRef.value?.validate().catch(() => false);
    if (!valid) return;
    createLoading.value = true;
    try {
      const data = await platformTenantApi.createEndUser({
        username: createForm.username.trim(),
        email: createForm.email.trim() || undefined,
        phone: createForm.phone.trim() || undefined,
        internalNote: createForm.internalNote.trim() || undefined
      });
      showCreateDialog.value = false;
      void fetchUsers();
      await showActivationCredential(data, `用户「${createForm.username.trim()}」`);
    } catch (error) {
      ElMessage.error(errorMessage(error, "创建失败"));
    } finally {
      createLoading.value = false;
    }
  }

  function setStatusUpdating(userId: string, updating: boolean) {
    const next = new Set(statusUpdatingIds.value);
    if (updating) next.add(userId);
    else next.delete(userId);
    statusUpdatingIds.value = next;
  }

  function isStatusUpdating(userId: string) {
    return statusUpdatingIds.value.has(userId);
  }

  async function handleStatusChange(row: EndUserItem, enabled: boolean) {
    if (isStatusUpdating(row.userId) || enabled === (row.status === 1)) return;
    const status = enabled ? "active" : "disabled";
    const action = enabled ? "启用" : "停用";
    setStatusUpdating(row.userId, true);
    try {
      await ElMessageBox.confirm(
        `确定要${action}用户「${row.username}」吗？${enabled ? "" : "停用后该用户将立即无法登录。"}`,
        `确认${action}`,
        {
          confirmButtonText: `确认${action}`,
          cancelButtonText: "取消",
          type: enabled ? "info" : "warning",
          confirmButtonClass: enabled ? "" : "el-button--danger"
        }
      );
      await platformTenantApi.updateUserStatus(row.userId, status);
      row.status = enabled ? 1 : 2;
      ElMessage.success(`用户已${action}`);
    } catch (error) {
      if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error, "操作失败"));
    } finally {
      setStatusUpdating(row.userId, false);
    }
  }

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
      if (userList.value.length === 1 && page.value > 1) page.value -= 1;
      void fetchUsers();
    } catch (error) {
      if (error !== "cancel") ElMessage.error(errorMessage(error, "删除失败"));
    }
  }

  function openEditUser(row: EndUserItem) {
    editTarget.value = row;
    editDialogOpen.value = true;
  }

  function openGroupPolicy(row: EndUserItem) {
    groupPolicyTarget.value = row;
    groupPolicyDialogOpen.value = true;
  }

  async function handleResetPassword(row: EndUserItem) {
    try {
      await ElMessageBox.confirm(
        `重置后「${row.username}」的现有会话将全部失效，账号需通过新链接重新激活。`,
        "确认重置密码",
        {
          confirmButtonText: "确认重置",
          cancelButtonText: "取消",
          type: "warning"
        }
      );
      const credential = await platformTenantApi.resetUserPassword(row.userId);
      await showActivationCredential(credential, `用户「${row.username}」`);
    } catch (error) {
      if (error !== "cancel") ElMessage.error(errorMessage(error, "操作失败"));
    }
  }

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
      ElMessage.success(`已成功为「${rechargeTarget.value.username}」充值 $${(payload.amountMicroUsd / 1_000_000).toLocaleString()}`);
      showRechargeDialog.value = false;
      void fetchUsers();
    } catch (error) {
      ElMessage.error(errorMessage(error, "充值失败"));
    } finally {
      rechargeLoading.value = false;
    }
  }

  onMounted(() => {
    void fetchUsers();
  });

  return {
    keyword,
    page,
    pageSize,
    total,
    loading,
    userList,
    editDialogOpen,
    editTarget,
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
    handleDelete,
    openEditUser,
    openGroupPolicy,
    handleResetPassword,
    openRecharge,
    submitRecharge
  };
}
