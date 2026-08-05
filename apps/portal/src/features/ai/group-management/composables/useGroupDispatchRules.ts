import { computed, readonly, shallowRef, watch } from "vue";

import { aiTenantApi } from "@/api/aiTenant";
import type {
  TenantAiDispatchModel,
  TenantAiDispatchRule,
  TenantAiDispatchRuleWriteRequest
} from "@/api/types/aiTenant";
import { errorMessage } from "../errorMessage";

export interface GroupDispatchRulesApi {
  list(groupId: string): Promise<{ items: TenantAiDispatchRule[] | null; total: number }>;
  listModels(groupId: string, clientSurface: string): Promise<{ items: TenantAiDispatchModel[] | null; total: number }>;
  create(groupId: string, body: TenantAiDispatchRuleWriteRequest): Promise<TenantAiDispatchRule>;
  update(groupId: string, ruleId: string, body: TenantAiDispatchRuleWriteRequest): Promise<TenantAiDispatchRule>;
  updateStatus(groupId: string, ruleId: string, status: "active" | "disabled"): Promise<TenantAiDispatchRule>;
  remove(groupId: string, ruleId: string): Promise<unknown>;
}

interface UseGroupDispatchRulesOptions {
  groupId: () => string;
  api?: GroupDispatchRulesApi;
}

const defaultApi: GroupDispatchRulesApi = {
  list: (groupId) => aiTenantApi.listGroupDispatchRules(groupId),
  listModels: (groupId, clientSurface) => aiTenantApi.listGroupDispatchModels(groupId, clientSurface),
  create: (groupId, body) => aiTenantApi.createGroupDispatchRule(groupId, body),
  update: (groupId, ruleId, body) => aiTenantApi.updateGroupDispatchRule(groupId, ruleId, body),
  updateStatus: (groupId, ruleId, status) => aiTenantApi.updateGroupDispatchRuleStatus(groupId, ruleId, status),
  remove: (groupId, ruleId) => aiTenantApi.deleteGroupDispatchRule(groupId, ruleId)
};

export function useGroupDispatchRules(options: UseGroupDispatchRulesOptions) {
  const api = options.api ?? defaultApi;
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const loadError = shallowRef("");
  const rules = shallowRef<TenantAiDispatchRule[]>([]);
  const models = shallowRef<TenantAiDispatchModel[]>([]);
  const modelsLoading = shallowRef(false);
  const modelsError = shallowRef("");
  let loadGeneration = 0;
  let modelGeneration = 0;
  let mutationGeneration = 0;

  const unpricedRuleIds = computed(() => new Set(
    rules.value.filter((rule) => !rule.can_enable).map((rule) => rule.id)
  ));

  async function load() {
    const groupId = options.groupId();
    const generation = ++loadGeneration;
    modelGeneration++;
    modelsLoading.value = false;
    modelsError.value = "";
    mutationGeneration++;
    if (!groupId) {
      rules.value = [];
      models.value = [];
      return;
    }
    rules.value = [];
    models.value = [];
    loading.value = true;
    loadError.value = "";
    try {
      const ruleResponse = await api.list(groupId);
      if (generation !== loadGeneration || groupId !== options.groupId()) return;
      rules.value = ruleResponse.items || [];
    } catch (error: unknown) {
      if (generation === loadGeneration) loadError.value = errorMessage(error, "加载调度规则失败");
    } finally {
      if (generation === loadGeneration) loading.value = false;
    }
  }

  async function loadModels(clientSurface: string) {
    const groupId = options.groupId();
    const generation = ++modelGeneration;
    models.value = [];
    modelsError.value = "";
    if (!groupId || !clientSurface) {
      modelsLoading.value = false;
      return;
    }
    modelsLoading.value = true;
    try {
      const response = await api.listModels(groupId, clientSurface);
      if (generation !== modelGeneration || groupId !== options.groupId()) return;
      models.value = response.items || [];
    } catch (error: unknown) {
      if (generation === modelGeneration) modelsError.value = errorMessage(error, "加载逻辑模型失败");
    } finally {
      if (generation === modelGeneration) modelsLoading.value = false;
    }
  }

  async function mutate(run: () => Promise<TenantAiDispatchRule>, existingId?: string) {
    const groupId = options.groupId();
    const loadToken = loadGeneration;
    const mutationToken = ++mutationGeneration;
    saving.value = true;
    try {
      const saved = await run();
      if (mutationToken !== mutationGeneration || loadToken !== loadGeneration || groupId !== options.groupId()) return saved;
      rules.value = existingId
        ? rules.value.map((rule) => rule.id === existingId ? saved : rule)
        : [...rules.value, saved].sort((left, right) => left.priority - right.priority);
      return saved;
    } finally {
      if (mutationToken === mutationGeneration) saving.value = false;
    }
  }

  function create(body: TenantAiDispatchRuleWriteRequest) {
    const groupId = options.groupId();
    return mutate(() => api.create(groupId, body));
  }

  function update(ruleId: string, body: TenantAiDispatchRuleWriteRequest) {
    const groupId = options.groupId();
    return mutate(() => api.update(groupId, ruleId, body), ruleId);
  }

  function updateStatus(ruleId: string, status: "active" | "disabled") {
    const groupId = options.groupId();
    return mutate(() => api.updateStatus(groupId, ruleId, status), ruleId);
  }

  async function remove(ruleId: string) {
    const groupId = options.groupId();
    const loadToken = loadGeneration;
    const mutationToken = ++mutationGeneration;
    saving.value = true;
    try {
      await api.remove(groupId, ruleId);
      if (mutationToken === mutationGeneration && loadToken === loadGeneration && groupId === options.groupId()) {
        rules.value = rules.value.filter((rule) => rule.id !== ruleId);
      }
    } finally {
      if (mutationToken === mutationGeneration) saving.value = false;
    }
  }

  watch(options.groupId, load, { immediate: true });

  return {
    loading: readonly(loading),
    saving: readonly(saving),
    loadError: readonly(loadError),
    rules: readonly(rules),
    models: readonly(models),
    modelsLoading: readonly(modelsLoading),
    modelsError: readonly(modelsError),
    unpricedRuleIds,
    load,
    loadModels,
    create,
    update,
    updateStatus,
    remove
  };
}
