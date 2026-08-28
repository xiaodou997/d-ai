<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";

import { aiAdminApi } from "@/api/aiAdmin";
import type { RuntimeLimitPolicyDTO, TenantUpstreamAccessDTO, TenantUpstreamPolicyRef } from "@/api/types/ai";
import { DsTabs } from "@/shared/ui";

import AdminTenantLimitPanel from "./AdminTenantLimitPanel.vue";
import AdminTenantUpstreamAccessPanel from "./AdminTenantUpstreamAccessPanel.vue";
import type {
  AdminTenantLimitForm,
  AdminTenantPolicySubject,
  AdminTenantUpstreamPolicyDraft
} from "../types";

const props = defineProps<{
  tenant: AdminTenantPolicySubject | null;
}>();

const activePolicyTab = ref("capacity");
const policyTabs = [
  { key: "capacity", label: "容量限制" },
  { key: "upstream", label: "上游权限" }
];

const policiesLoading = ref(false);
const limitPolicies = ref<RuntimeLimitPolicyDTO[]>([]);
const savingPolicy = ref(false);
const upstreamAccessLoading = ref(false);
const savingUpstreamAccess = ref(false);
const upstreamResources = ref<TenantUpstreamAccessDTO[]>([]);
const upstreamPolicyDrafts = ref<Record<string, AdminTenantUpstreamPolicyDraft>>({});
const savedUpstreamPolicyDrafts = ref<Record<string, AdminTenantUpstreamPolicyDraft>>({});
const limitForm = reactive<AdminTenantLimitForm>({
  concurrency_limit: null,
  status: "active"
});

const currentPolicy = computed(() => {
  const tenantId = props.tenant?.tenantId;
  return tenantId
    ? limitPolicies.value.find((item) => item.scope_type === "tenant" && item.scope_id === tenantId) || null
    : null;
});

const limitSummaryText = computed(() => {
  if (!currentPolicy.value) return "当前未配置平台硬限额，租户仍可自行维护用户和 API 密钥的细粒度策略。";
  if (currentPolicy.value.status === "disabled") return "平台硬限额已保存，但当前处于停用状态。";
  return "平台硬限额是保护上游和平台容量的最终边界；租户只能在此边界内配置自己的用户策略。";
});

const upstreamAccessDirty = computed(() => {
  const keys = new Set([
    ...Object.keys(upstreamPolicyDrafts.value),
    ...Object.keys(savedUpstreamPolicyDrafts.value)
  ]);
  return Array.from(keys).some((key) => {
    const current = upstreamPolicyDrafts.value[key];
    const saved = savedUpstreamPolicyDrafts.value[key];
    return current?.access_granted !== saved?.access_granted
      || current?.tenant_multiplier_override !== saved?.tenant_multiplier_override;
  });
});

const loading = computed(() => policiesLoading.value || upstreamAccessLoading.value);

function upstreamResourceKey(resource: TenantUpstreamAccessDTO) {
  return `${resource.resource_kind}:${resource.resource_id}`;
}

function resetPolicyState() {
  limitPolicies.value = [];
  upstreamResources.value = [];
  upstreamPolicyDrafts.value = {};
  savedUpstreamPolicyDrafts.value = {};
}

async function loadPolicies(tenantId: string) {
  policiesLoading.value = true;
  try {
    const res = await aiAdminApi.listRuntimeLimitPolicies();
    if (props.tenant?.tenantId === tenantId) limitPolicies.value = res.items || [];
  } catch (error: any) {
    ElMessage.error(error?.message || "加载租户容量限制失败");
  } finally {
    policiesLoading.value = false;
  }
}

async function loadUpstreamAccess(tenantId: string) {
  upstreamAccessLoading.value = true;
  try {
    const res = await aiAdminApi.listTenantUpstreamAccess(tenantId);
    if (props.tenant?.tenantId !== tenantId) return;
    upstreamResources.value = res.items || [];
    const drafts = Object.fromEntries(upstreamResources.value.map((item) => [
      upstreamResourceKey(item),
      {
        access_granted: item.access_mode === "restricted" && item.access_granted,
        tenant_multiplier_override: item.tenant_multiplier_override ?? null
      }
    ]));
    upstreamPolicyDrafts.value = structuredClone(drafts);
    savedUpstreamPolicyDrafts.value = structuredClone(drafts);
  } catch (error: any) {
    ElMessage.error(error?.message || "加载租户上游策略失败");
  } finally {
    upstreamAccessLoading.value = false;
  }
}

async function refreshAll() {
  const tenantId = props.tenant?.tenantId;
  if (!tenantId) {
    resetPolicyState();
    return;
  }
  await Promise.all([loadPolicies(tenantId), loadUpstreamAccess(tenantId)]);
}

async function saveLimitPolicy() {
  const tenantId = props.tenant?.tenantId;
  if (!tenantId) return;
  if (!limitForm.concurrency_limit) {
    ElMessage.warning("请填写最大同时请求数");
    return;
  }
  savingPolicy.value = true;
  const payload = {
    scope_type: "tenant",
    scope_id: tenantId,
    concurrency_limit: limitForm.concurrency_limit,
    status: limitForm.status
  };
  try {
    if (currentPolicy.value) await aiAdminApi.updateRuntimeLimitPolicy(currentPolicy.value.id, payload);
    else await aiAdminApi.createRuntimeLimitPolicy(payload);
    await loadPolicies(tenantId);
    ElMessage.success("租户容量限制已保存");
  } catch (error: any) {
    ElMessage.error(error?.message || "保存失败");
  } finally {
    savingPolicy.value = false;
  }
}

function toggleUpstreamAccess(resource: TenantUpstreamAccessDTO, granted: boolean) {
  if (resource.access_mode !== "restricted") return;
  const key = upstreamResourceKey(resource);
  upstreamPolicyDrafts.value = {
    ...upstreamPolicyDrafts.value,
    [key]: { ...upstreamPolicyDrafts.value[key], access_granted: granted }
  };
}

function updateTenantMultiplierOverride(resource: TenantUpstreamAccessDTO, value: number | null) {
  const key = upstreamResourceKey(resource);
  upstreamPolicyDrafts.value = {
    ...upstreamPolicyDrafts.value,
    [key]: {
      ...upstreamPolicyDrafts.value[key],
      tenant_multiplier_override: value
    }
  };
}

async function saveUpstreamAccess() {
  const tenantId = props.tenant?.tenantId;
  if (!tenantId) return;
  const policies: TenantUpstreamPolicyRef[] = upstreamResources.value.flatMap((resource) => {
    const draft = upstreamPolicyDrafts.value[upstreamResourceKey(resource)];
    const accessGranted = resource.access_mode === "restricted" && Boolean(draft?.access_granted);
    const multiplier = draft?.tenant_multiplier_override ?? null;
    if (!accessGranted && multiplier === null) return [];
    return [{
      resource_kind: resource.resource_kind,
      resource_id: resource.resource_id,
      access_granted: accessGranted,
      ...(multiplier === null ? {} : { tenant_multiplier_override: multiplier })
    }];
  });
  savingUpstreamAccess.value = true;
  try {
    await aiAdminApi.replaceTenantUpstreamAccess(tenantId, policies);
    await loadUpstreamAccess(tenantId);
    ElMessage.success("租户上游策略已保存");
  } catch (error: any) {
    ElMessage.error(error?.message || "保存租户上游策略失败");
  } finally {
    savingUpstreamAccess.value = false;
  }
}

watch(currentPolicy, (policy) => {
  limitForm.concurrency_limit = policy?.concurrency_limit ?? null;
  limitForm.status = (policy?.status as "active" | "disabled") || "active";
}, { immediate: true });

watch(() => props.tenant?.tenantId, () => {
  resetPolicyState();
  void refreshAll();
}, { immediate: true });
</script>

<template>
  <section class="tenant-policy-panel">
    <div class="tenant-policy-panel__nav">
      <DsTabs v-model="activePolicyTab" :tabs="policyTabs" />
      <el-button :icon="Refresh" :loading="loading" @click="refreshAll">刷新</el-button>
    </div>

    <div v-show="activePolicyTab === 'capacity'" class="tenant-policy-panel__pane">
      <div class="tenant-policy-panel__heading">
        <div>
          <strong>平台保护边界</strong>
          <p>限制租户整体请求容量，不参与租户的商业定价和用户授权。</p>
        </div>
        <el-button v-if="tenant" type="primary" :loading="savingPolicy" @click="saveLimitPolicy">保存容量限制</el-button>
      </div>
      <AdminTenantLimitPanel
        :selected-tenant="tenant"
        :loading="policiesLoading"
        :configured="Boolean(currentPolicy)"
        :summary-text="limitSummaryText"
        :form="limitForm"
      />
    </div>

    <div v-show="activePolicyTab === 'upstream'" class="tenant-policy-panel__pane">
      <div class="tenant-policy-panel__heading">
        <div>
          <strong>上游访问与倍率</strong>
          <p>公开资源固定可见，专属资源按租户授权；租户倍率可覆盖资源的默认扣费倍率。</p>
        </div>
        <el-button
          v-if="tenant"
          type="primary"
          :loading="savingUpstreamAccess"
          :disabled="!upstreamAccessDirty"
          @click="saveUpstreamAccess"
        >保存上游策略</el-button>
      </div>
      <AdminTenantUpstreamAccessPanel
        :selected-tenant="tenant"
        :loading="upstreamAccessLoading"
        :resources="upstreamResources"
        :policies="upstreamPolicyDrafts"
        @toggle="toggleUpstreamAccess"
        @update-multiplier="updateTenantMultiplierOverride"
      />
    </div>
  </section>
</template>

<style scoped>
.tenant-policy-panel {
  min-width: 0;
}

.tenant-policy-panel__nav,
.tenant-policy-panel__heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.tenant-policy-panel__nav {
  align-items: center;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--ds-line);
}

.tenant-policy-panel__pane {
  padding-top: 18px;
}

.tenant-policy-panel__heading {
  margin-bottom: 16px;
}

.tenant-policy-panel__heading strong {
  color: var(--ds-ink);
}

.tenant-policy-panel__heading p {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
}

@media (max-width: 768px) {
  .tenant-policy-panel__nav,
  .tenant-policy-panel__heading {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
