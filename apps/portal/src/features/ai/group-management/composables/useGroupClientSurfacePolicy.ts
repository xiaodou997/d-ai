import { computed, readonly, shallowRef, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";

import { aiTenantApi } from "../../../../api/aiTenant";
import type {
  TenantAiClientSurface,
  TenantAiClientSurfacePolicy,
  TenantAiClientSurfacePolicyWrite
} from "../../../../types/aiTenant";
import { allClientSurfaces, clientSurfacePresets } from "../catalog";
import { errorMessage } from "../errorMessage";

export interface GroupClientSurfacePolicyApi {
  get(groupId: string): Promise<TenantAiClientSurfacePolicy>;
  replace(groupId: string, body: TenantAiClientSurfacePolicyWrite): Promise<TenantAiClientSurfacePolicy>;
}

interface UseGroupClientSurfacePolicyOptions {
  groupId: () => string;
  api?: GroupClientSurfacePolicyApi;
}

const defaultApi: GroupClientSurfacePolicyApi = {
  get: (groupId) => aiTenantApi.getGroupClientSurfacePolicy(groupId),
  replace: (groupId, body) => aiTenantApi.replaceGroupClientSurfacePolicy(groupId, body)
};

function normalize(items: readonly TenantAiClientSurface[]) {
  const selected = new Set(items);
  return allClientSurfaces.filter((surface) => selected.has(surface));
}

function effective(mode: "all" | "restricted", selected: readonly TenantAiClientSurface[]) {
  return mode === "all" ? allClientSurfaces : selected;
}

export function useGroupClientSurfacePolicy(options: UseGroupClientSurfacePolicyOptions) {
  const api = options.api ?? defaultApi;
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const mode = shallowRef<"all" | "restricted">("all");
  const selectedSurfaces = shallowRef<TenantAiClientSurface[]>([]);
  const persisted = shallowRef<TenantAiClientSurfacePolicy | null>(null);
  let loadGeneration = 0;
  let saveGeneration = 0;

  const selectedCount = computed(() => effective(mode.value, selectedSurfaces.value).length);
  const isDirty = computed(() => {
    if (!persisted.value) return false;
    const before = effective(persisted.value.mode, persisted.value.allowed_surfaces || []);
    const after = effective(mode.value, selectedSurfaces.value);
    return before.length !== after.length || before.some((item, index) => item !== after[index]);
  });
  const canSave = computed(() => isDirty.value && !loading.value && !saving.value && (
    mode.value === "all" || selectedSurfaces.value.length > 0
  ));

  function applyPolicy(policy: TenantAiClientSurfacePolicy) {
    const allowedSurfaces = normalize(policy.allowed_surfaces || []);
    persisted.value = { ...policy, allowed_surfaces: allowedSurfaces };
    mode.value = policy.mode;
    selectedSurfaces.value = allowedSurfaces;
  }

  async function load() {
    const generation = ++loadGeneration;
    saveGeneration++;
    const groupId = options.groupId();
    persisted.value = null;
    mode.value = "all";
    selectedSurfaces.value = [];
    saving.value = false;
    if (!groupId) return;
    loading.value = true;
    try {
      const policy = await api.get(groupId);
      if (generation !== loadGeneration || groupId !== options.groupId()) return;
      applyPolicy(policy);
    } catch (error: unknown) {
      if (generation === loadGeneration) ElMessage.error(errorMessage(error, "加载 API 入口策略失败"));
    } finally {
      if (generation === loadGeneration) loading.value = false;
    }
  }

  function setMode(value: "all" | "restricted") {
    mode.value = value;
  }

  function setSelectedSurfaces(items: TenantAiClientSurface[]) {
    selectedSurfaces.value = normalize(items);
  }

  function applyPreset(name: keyof typeof clientSurfacePresets) {
    mode.value = "restricted";
    selectedSurfaces.value = [...clientSurfacePresets[name]];
  }

  function discard() {
    if (persisted.value) applyPolicy(persisted.value);
  }

  async function save() {
    const groupId = options.groupId();
    if (!groupId || !canSave.value) {
      if (mode.value === "restricted" && selectedSurfaces.value.length === 0) {
        ElMessage.warning("自定义限制至少保留一个 API 入口");
      }
      return;
    }
    const previous = effective(persisted.value?.mode || "all", persisted.value?.allowed_surfaces || []);
    const next = effective(mode.value, selectedSurfaces.value);
    const removed = previous.filter((item) => !next.includes(item));
    if (removed.length) {
      try {
        await ElMessageBox.confirm(`将关闭 ${removed.length} 个 API 入口，是否继续？`, "确认入口变更", {
          type: "warning",
          confirmButtonText: "保存",
          cancelButtonText: "取消"
        });
      } catch {
        return;
      }
    }
    const loadToken = loadGeneration;
    const saveToken = ++saveGeneration;
    const body: TenantAiClientSurfacePolicyWrite = {
      mode: mode.value,
      allowed_surfaces: mode.value === "restricted" ? [...selectedSurfaces.value] : []
    };
    saving.value = true;
    try {
      const policy = await api.replace(groupId, body);
      if (saveToken !== saveGeneration || loadToken !== loadGeneration || groupId !== options.groupId()) return;
      applyPolicy(policy);
      ElMessage.success("API 入口策略已保存");
    } catch (error: unknown) {
      if (saveToken === saveGeneration) ElMessage.error(errorMessage(error, "保存 API 入口策略失败"));
    } finally {
      if (saveToken === saveGeneration) saving.value = false;
    }
  }

  async function confirmDiscardChanges(message: string) {
    if (saving.value) return false;
    if (!isDirty.value) return true;
    try {
      await ElMessageBox.confirm(message, "存在未保存变更", {
        type: "warning",
        confirmButtonText: "放弃变更",
        cancelButtonText: "继续编辑"
      });
      discard();
      return true;
    } catch {
      return false;
    }
  }

  watch(options.groupId, load, { immediate: true });

  return {
    loading: readonly(loading),
    saving: readonly(saving),
    mode: readonly(mode),
    selectedSurfaces: readonly(selectedSurfaces),
    selectedCount,
    isDirty,
    canSave,
    setMode,
    setSelectedSurfaces,
    applyPreset,
    discard,
    save,
    load,
    confirmDiscardChanges
  };
}
