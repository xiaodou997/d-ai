import { computed, shallowRef } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";

import {
  upstreamModelBindingBatchApi,
  type UpstreamModelBindingBatchApi
} from "../api";

interface SelectableBinding {
  id: string;
}

interface UseModelBindingBatchDeleteOptions {
  api?: UpstreamModelBindingBatchApi;
  targetKind: () => "account" | "pool";
  targetId: () => string;
  reload: () => Promise<void>;
}

function messageFromError(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "批量删除失败";
}

export function useModelBindingBatchDelete(options: UseModelBindingBatchDeleteOptions) {
  const api = options.api ?? upstreamModelBindingBatchApi;
  const selectedBindingIds = shallowRef<string[]>([]);
  const deleting = shallowRef(false);
  const selectedCount = computed(() => selectedBindingIds.value.length);

  function setSelection(rows: SelectableBinding[]) {
    selectedBindingIds.value = rows.map((row) => row.id);
  }

  function clearSelection() {
    selectedBindingIds.value = [];
  }

  async function deleteSelection() {
    const bindingIds = [...selectedBindingIds.value];
    if (!bindingIds.length) return;
    try {
      await ElMessageBox.confirm(
        `将删除选中的 ${bindingIds.length} 条显式模型绑定。删除后，相关模型将无法通过当前上游路由。`,
        "批量删除模型绑定",
        {
          type: "warning",
          confirmButtonText: "删除",
          cancelButtonText: "取消",
          confirmButtonClass: "el-button--danger"
        }
      );
    } catch {
      return;
    }

    deleting.value = true;
    try {
      const result = options.targetKind() === "account"
        ? await api.deleteAccountBindings(options.targetId(), bindingIds)
        : await api.deletePoolBindings(options.targetId(), bindingIds);
      ElMessage.success(`已删除 ${result.deleted} 条模型绑定`);
      await options.reload();
    } catch (error: unknown) {
      ElMessage.error(messageFromError(error));
    } finally {
      deleting.value = false;
    }
  }

  return {
    clearSelection,
    deleteSelection,
    deleting,
    selectedCount,
    setSelection
  };
}
