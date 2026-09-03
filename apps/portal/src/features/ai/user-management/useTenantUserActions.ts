import { ElMessage, ElMessageBox } from "element-plus";

import { platformTenantApi } from "@/api/platformTenant";
import type { ActivationCredentialOutput, EndUserItem } from "@/api/types/platformTenant";
import { showActivationCredential } from "@/platform/auth/activation";

type TenantUserActionTarget = Pick<EndUserItem, "userId" | "username">;

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export function useTenantUserActions() {
  async function deleteUser(user: TenantUserActionTarget): Promise<boolean> {
    try {
      await ElMessageBox.confirm(
        `确定要删除用户「${user.username}」吗？账号将立即无法登录，且会从用户列表移除；历史账务与用量记录仍会保留。`,
        "确认删除用户",
        {
          confirmButtonText: "确认删除",
          cancelButtonText: "取消",
          type: "warning",
          confirmButtonClass: "el-button--danger"
        }
      );
      await platformTenantApi.deleteEndUser(user.userId);
      ElMessage.success("用户已删除");
      return true;
    } catch (error) {
      if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error, "删除失败"));
      return false;
    }
  }

  async function resetPassword(user: TenantUserActionTarget): Promise<boolean> {
    try {
      await ElMessageBox.confirm(
        `重置后「${user.username}」的现有会话将全部失效，账号需通过新链接重新激活。`,
        "确认重置密码",
        {
          confirmButtonText: "确认重置",
          cancelButtonText: "取消",
          type: "warning"
        }
      );
      const credential: ActivationCredentialOutput = await platformTenantApi.resetUserPassword(user.userId);
      await showActivationCredential(credential, `用户「${user.username}」`);
      return true;
    } catch (error) {
      if (error !== "cancel" && error !== "close") ElMessage.error(errorMessage(error, "操作失败"));
      return false;
    }
  }

  return { deleteUser, resetPassword };
}
