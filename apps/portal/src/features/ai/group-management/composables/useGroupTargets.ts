import { computed, readonly, shallowRef, watch } from "vue";

import { aiTenantApi } from "@/api/aiTenant";
import { HttpProblem } from "@/platform";
import type {
  TenantAiGroupTarget,
  TenantAiGroupTargetWriteRequest,
  TenantAiUpstreamResource
} from "@/api/types/aiTenant";
import {
  bindingKey,
  bindingKind,
  bindingTargetId,
  bindingUnavailableReason,
  resourceAvailableModelCount,
  resourceKey,
  type GroupTargetChange,
  type GroupTargetDraft,
  type GroupTargetRow,
  type GroupTargetSaveFailure,
  type GroupTargetSaveResult,
  type GroupTargetStatus
} from "../groupTargets";
import { errorMessage } from "../errorMessage";
import { protocolLabel } from "../../upstream-catalog/presentation";

export interface GroupTargetsApi {
  listTargets(groupId: string): Promise<{ items: TenantAiGroupTarget[]; total: number; route_policy_version?: number }>;
  listResources(): Promise<{ items: TenantAiUpstreamResource[]; total: number }>;
  add(groupId: string, body: TenantAiGroupTargetWriteRequest): Promise<TenantAiGroupTarget>;
  update(groupId: string, bindingId: string, body: TenantAiGroupTargetWriteRequest): Promise<TenantAiGroupTarget>;
  remove(groupId: string, bindingId: string): Promise<unknown>;
  replace?(groupId: string, body: {
    expected_version: number;
    targets: TenantAiGroupTargetWriteRequest[];
  }): Promise<{ items: TenantAiGroupTarget[]; total: number; route_policy_version: number }>;
}

interface UseGroupTargetsOptions {
  groupId: () => string;
  api?: GroupTargetsApi;
}

const defaultApi: GroupTargetsApi = {
  listTargets: (groupId) => aiTenantApi.listGroupTargets(groupId),
  listResources: () => aiTenantApi.listUpstreamResources(),
  add: (groupId, body) => aiTenantApi.addGroupTarget(groupId, body),
  update: (groupId, bindingId, body) => aiTenantApi.updateGroupTarget(groupId, bindingId, body),
  remove: (groupId, bindingId) => aiTenantApi.deleteGroupTarget(groupId, bindingId),
  replace: (groupId, body) => aiTenantApi.replaceGroupTargets(groupId, body)
};

export function useGroupTargets(options: UseGroupTargetsOptions) {
  const api = options.api ?? defaultApi;
  const bindings = shallowRef<TenantAiGroupTarget[]>([]);
  const resources = shallowRef<TenantAiUpstreamResource[]>([]);
  const selectedKeys = shallowRef<string[]>([]);
  const drafts = shallowRef<Record<string, GroupTargetDraft>>({});
  const routePolicyVersion = shallowRef(0);
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const loadError = shallowRef("");
  let loadGeneration = 0;

  const bindingByKey = computed(() => new Map(bindings.value.map((binding) => [bindingKey(binding), binding])));
  const selectedSet = computed(() => new Set(selectedKeys.value));
  const rows = computed<GroupTargetRow[]>(() => {
    const options = new Map<string, Omit<GroupTargetRow, keyof GroupTargetDraft | "selected" | "change">>();
    for (const resource of resources.value) {
      const key = resourceKey(resource.resource_kind, resource.id);
      const availableModels = resourceAvailableModelCount(resource);
      const binding = bindingByKey.value.get(key);
      const linked = binding !== undefined;
      options.set(key, {
        key,
        targetId: resource.id,
        kind: resource.resource_kind,
        name: resource.name,
        protocols: [...new Set(resource.models.map((model) => protocolLabel(model.api_format)))],
        tenantMultiplier: resource.tenant_multiplier,
        availableModels,
        linked,
        selectable: availableModels > 0 || linked,
        resourceState: availableModels > 0 ? "available" : "unpriced",
        bindingUnavailableReason: bindingUnavailableReason(binding)
      });
    }
    for (const binding of bindings.value) {
      const key = bindingKey(binding);
      if (options.has(key)) continue;
      options.set(key, {
        key,
        targetId: bindingTargetId(binding),
        kind: bindingKind(binding),
        name: binding.account_name || binding.pool_name || bindingTargetId(binding) || "未知资源",
        protocols: binding.default_provider_family ? [protocolLabel(binding.default_provider_family)] : [],
        tenantMultiplier: null,
        availableModels: 0,
        linked: true,
        selectable: true,
        resourceState: "missing",
        bindingUnavailableReason: bindingUnavailableReason(binding) ?? "missing"
      });
    }
    return [...options.values()].map((option) => {
      const binding = bindingByKey.value.get(option.key);
      const selected = selectedSet.value.has(option.key);
      const draft = drafts.value[option.key] || { status: "active" as GroupTargetStatus };
      let change: GroupTargetChange | null = null;
      if (option.linked && !selected) change = "remove";
      else if (!option.linked && selected) change = "add";
      else if (binding && selected && binding.status !== draft.status) change = "update";
      return { ...option, ...draft, selected, change };
    }).sort((left, right) => {
      if (left.linked !== right.linked) return left.linked ? -1 : 1;
      if (left.kind !== right.kind) return left.kind.localeCompare(right.kind);
      return left.name.localeCompare(right.name, "zh-CN");
    });
  });
  const additions = computed(() => rows.value.filter((row) => row.change === "add"));
  const updates = computed(() => rows.value.filter((row) => row.change === "update"));
  const removals = computed(() => rows.value.filter((row) => row.change === "remove"));
  const hasChanges = computed(() => additions.value.length + updates.value.length + removals.value.length > 0);

  function resetDrafts(nextBindings: TenantAiGroupTarget[]) {
    selectedKeys.value = nextBindings.map(bindingKey);
    drafts.value = Object.fromEntries(nextBindings.map((binding) => [bindingKey(binding), {
      status: binding.status
    }]));
  }

  async function load() {
    const groupId = options.groupId();
    const generation = ++loadGeneration;
    if (!groupId) {
      bindings.value = [];
      resources.value = [];
      routePolicyVersion.value = 0;
      resetDrafts([]);
      return;
    }
    loading.value = true;
    loadError.value = "";
    try {
      const [targetResponse, resourceResponse] = await Promise.all([
        api.listTargets(groupId),
        api.listResources()
      ]);
      if (generation !== loadGeneration || groupId !== options.groupId()) return;
      bindings.value = targetResponse.items || [];
      resources.value = resourceResponse.items || [];
      routePolicyVersion.value = targetResponse.route_policy_version ?? 0;
      resetDrafts(bindings.value);
    } catch (error: unknown) {
      if (generation === loadGeneration) loadError.value = errorMessage(error, "加载上游关联失败");
    } finally {
      if (generation === loadGeneration) loading.value = false;
    }
  }

  function setSelected(key: string, selected: boolean) {
    const row = rows.value.find((item) => item.key === key);
    if (!row || (!row.selectable && selected)) return;
    const next = new Set(selectedKeys.value);
    if (selected) {
      next.add(key);
      if (!drafts.value[key]) {
        drafts.value = { ...drafts.value, [key]: { status: "active" } };
      }
    } else next.delete(key);
    selectedKeys.value = [...next];
  }

  function updateDraft(key: string, patch: Partial<GroupTargetDraft>) {
    const current = drafts.value[key] || { status: "active" as GroupTargetStatus };
    drafts.value = { ...drafts.value, [key]: { ...current, ...patch } };
  }

  function discard() {
    resetDrafts(bindings.value);
  }

  function applySavedBinding(saved: TenantAiGroupTarget) {
    const index = bindings.value.findIndex((item) => item.id === saved.id || bindingKey(item) === bindingKey(saved));
    bindings.value = index < 0
      ? [...bindings.value, saved]
      : bindings.value.map((item, itemIndex) => itemIndex === index ? saved : item);
  }

  async function save(): Promise<GroupTargetSaveResult> {
    const groupId = options.groupId();
    const generation = loadGeneration;
    const commands = [
      ...additions.value.map((row) => ({ action: "add" as const, row })),
      ...updates.value.map((row) => ({ action: "update" as const, row })),
      ...removals.value.map((row) => ({ action: "remove" as const, row }))
    ];
    const result: GroupTargetSaveResult = { added: 0, updated: 0, removed: 0, failures: [] };
    if (!groupId || !commands.length) return result;
    saving.value = true;
    try {
      if (api.replace && routePolicyVersion.value > 0) {
        try {
          // Capture the diff before resetDrafts() replaces the selected/draft
          // state with the server snapshot.
          const addedCount = additions.value.length;
          const updatedCount = updates.value.length;
          const removedCount = removals.value.length;
          const desiredTargets = rows.value.filter((row) => row.selected).map((row) => ({
            ...(row.kind === "direct_upstream" ? { account_id: row.targetId } : { credential_pool_id: row.targetId }),
            status: row.status
          }));
          const response = await api.replace(groupId, {
            expected_version: routePolicyVersion.value,
            targets: desiredTargets
          });
          if (generation !== loadGeneration || groupId !== options.groupId()) return result;
          bindings.value = response.items || [];
          routePolicyVersion.value = response.route_policy_version;
          resetDrafts(bindings.value);
          result.added = addedCount;
          result.updated = updatedCount;
          result.removed = removedCount;
          return result;
        } catch (error: unknown) {
          result.failures.push({
            action: "update",
            targetKey: "__group__",
            targetName: "分组目标配置",
            message: errorMessage(error, "保存失败，配置可能已被其他人修改"),
            code: error instanceof HttpProblem ? error.code : undefined
          });
          return result;
        }
      }
      for (const command of commands) {
        if (generation !== loadGeneration || groupId !== options.groupId()) break;
        try {
          const binding = bindingByKey.value.get(command.row.key);
          if (command.action === "add") {
            const saved = await api.add(groupId, {
              ...(command.row.kind === "direct_upstream" ? { account_id: command.row.targetId } : { credential_pool_id: command.row.targetId }),
              status: command.row.status
            });
            applySavedBinding(saved);
            result.added++;
          } else if (command.action === "update" && binding) {
            const saved = await api.update(groupId, binding.id, {
              status: command.row.status
            });
            applySavedBinding(saved);
            result.updated++;
          } else if (command.action === "remove" && binding) {
            await api.remove(groupId, binding.id);
            bindings.value = bindings.value.filter((item) => item.id !== binding.id);
            result.removed++;
          }
        } catch (error: unknown) {
          result.failures.push({
            action: command.action,
            targetKey: command.row.key,
            targetName: command.row.name,
            message: errorMessage(error, "操作失败")
          });
        }
      }
      return result;
    } finally {
      saving.value = false;
    }
  }

  watch(options.groupId, load, { immediate: true });

  return {
    rows,
    loading: readonly(loading),
    saving: readonly(saving),
    loadError: readonly(loadError),
    routePolicyVersion: readonly(routePolicyVersion),
    additions,
    updates,
    removals,
    hasChanges,
    load,
    setSelected,
    updateDraft,
    discard,
    save
  };
}
