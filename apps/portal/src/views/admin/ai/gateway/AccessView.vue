<!--
  智能服务 — 租户策略(平台侧租户容量保护 / 上游可见范围 / 结算倍率覆盖)。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行),
       左右分栏(租户列表 + DsTabs 策略配置)收进同卡,只用 1px 分隔线分区;
       子面板 el-table/el-pagination/el-tag 换为 DsTable/DsPagination/DsTag,
       业务逻辑与请求参数不变,表单仍为 element-plus。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef, watch } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { SlidersHorizontal } from "lucide-vue-next";

import { PortalPagePanel } from "@dai/app-core";
import { DsTabs } from "@dai/ui";
import { aiAdminApi } from "../../../api/aiAdmin";
import { urmAdminApi } from "../../../api/urmAdmin";
import type { RuntimeLimitPolicyDTO, TenantUpstreamAccessDTO, TenantUpstreamPolicyRef } from "../../../types/ai";
import type { TenantListItem } from "../../../types/admin";
import AdminTenantLimitPanel from "./tenant-management/components/AdminTenantLimitPanel.vue";
import AdminTenantListPanel from "./tenant-management/components/AdminTenantListPanel.vue";
import AdminTenantUpstreamAccessPanel from "./tenant-management/components/AdminTenantUpstreamAccessPanel.vue";
import type { AdminTenantLimitForm, AdminTenantUpstreamPolicyDraft } from "./tenant-management/types";

const tenantsLoading = shallowRef(false);
const tenants = shallowRef<TenantListItem[]>([]);
const tenantTotal = shallowRef(0);
const tenantFilters = reactive({ page: 1, size: 20, keyword: "" });
const selectedTenant = shallowRef<TenantListItem | null>(null);
const activePolicyTab = shallowRef("capacity");
const policyTabs = [
  { key: "capacity", label: "容量限制" },
  { key: "upstream", label: "上游权限" }
];

const policiesLoading = shallowRef(false);
const limitPolicies = shallowRef<RuntimeLimitPolicyDTO[]>([]);
const savingPolicy = shallowRef(false);
const upstreamAccessLoading = shallowRef(false);
const savingUpstreamAccess = shallowRef(false);
const upstreamResources = shallowRef<TenantUpstreamAccessDTO[]>([]);
const upstreamPolicyDrafts = shallowRef<Record<string, AdminTenantUpstreamPolicyDraft>>({});
const savedUpstreamPolicyDrafts = shallowRef<Record<string, AdminTenantUpstreamPolicyDraft>>({});
const limitForm = reactive<AdminTenantLimitForm>({
  concurrency_limit: null,
  status: "active"
});

const currentPolicy = computed(() => {
  const tenantId = selectedTenant.value?.tenantId;
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

function upstreamResourceKey(resource: TenantUpstreamAccessDTO) {
  return `${resource.resource_kind}:${resource.resource_id}`;
}

function syncSelectedTenant(items: TenantListItem[]) {
  const currentId = selectedTenant.value?.tenantId;
  selectedTenant.value = items.find((item) => item.tenantId === currentId) || items[0] || null;
}

async function loadTenants() {
  tenantsLoading.value = true;
  try {
    const res = await urmAdminApi.listTenants({
      page: tenantFilters.page,
      size: tenantFilters.size,
      keyword: tenantFilters.keyword.trim() || undefined
    });
    tenants.value = res.items || [];
    tenantTotal.value = res.total || 0;
    syncSelectedTenant(tenants.value);
  } catch (error: any) {
    ElMessage.error(error?.message || "加载租户失败");
  } finally {
    tenantsLoading.value = false;
  }
}

async function loadPolicies() {
  policiesLoading.value = true;
  try {
    const res = await aiAdminApi.listRuntimeLimitPolicies({ limit: 500 });
    limitPolicies.value = res.items || [];
  } catch (error: any) {
    ElMessage.error(error?.message || "加载租户容量限制失败");
  } finally {
    policiesLoading.value = false;
  }
}

async function loadUpstreamAccess() {
  const tenantId = selectedTenant.value?.tenantId;
  if (!tenantId) {
    upstreamResources.value = [];
    upstreamPolicyDrafts.value = {};
    savedUpstreamPolicyDrafts.value = {};
    return;
  }
  upstreamAccessLoading.value = true;
  try {
    const res = await aiAdminApi.listTenantUpstreamAccess(tenantId);
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
  await Promise.all([loadTenants(), loadPolicies()]);
  await loadUpstreamAccess();
}

async function changePage(value: number) {
  // DsPagination 在切换 pageSize 时会连带派发 update:page(1),与 changeSize 去重
  if (value === tenantFilters.page) return;
  tenantFilters.page = value;
  await loadTenants();
}

async function changeSize(value: number) {
  tenantFilters.size = value;
  tenantFilters.page = 1;
  await loadTenants();
}

async function saveLimitPolicy() {
  if (!selectedTenant.value) return;
  if (!limitForm.concurrency_limit) {
    ElMessage.warning("请填写最大同时请求数");
    return;
  }
  savingPolicy.value = true;
  const payload = {
    scope_type: "tenant",
    scope_id: selectedTenant.value.tenantId,
    concurrency_limit: limitForm.concurrency_limit ?? undefined,
    status: limitForm.status
  };
  try {
    if (currentPolicy.value) await aiAdminApi.updateRuntimeLimitPolicy(currentPolicy.value.id, payload);
    else await aiAdminApi.createRuntimeLimitPolicy(payload);
    await loadPolicies();
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

// 改的是租户扣费倍率的按租户覆盖值，不是资源上的默认倍率。
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
  const tenantId = selectedTenant.value?.tenantId;
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
    await loadUpstreamAccess();
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

watch(() => selectedTenant.value?.tenantId, () => {
  void loadUpstreamAccess();
});

onMounted(refreshAll);
</script>

<template>
  <div class="page-container tenant-policy-page">
    <PortalPagePanel
      :icon="SlidersHorizontal"
      :breadcrumbs="[{ label: '智能服务' }, { label: '运营管理' }, { label: '租户策略' }]"
      description="集中维护平台侧的租户容量保护、上游可见范围和结算倍率覆盖；分组、零售倍率和用户策略仍由租户自行负责。"
      fill
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="tenantsLoading || policiesLoading || upstreamAccessLoading" @click="refreshAll">刷新</el-button>
      </template>

      <div class="policy-body">
        <aside class="policy-side">
          <AdminTenantListPanel
            :loading="tenantsLoading"
            :tenants="tenants"
            :total="tenantTotal"
            :page="tenantFilters.page"
            :size="tenantFilters.size"
            :keyword="tenantFilters.keyword"
            :selected-tenant-id="selectedTenant?.tenantId || ''"
            @update:keyword="tenantFilters.keyword = $event"
            @change-page="changePage"
            @change-size="changeSize"
            @select-tenant="selectedTenant = $event"
            @refresh="loadTenants"
          />
        </aside>

        <div class="policy-main">
          <DsTabs v-model="activePolicyTab" :tabs="policyTabs" />

          <div v-show="activePolicyTab === 'capacity'" class="policy-pane">
            <div class="tab-heading">
              <div class="tab-heading__text">
                <strong>平台保护边界{{ selectedTenant ? ` · ${selectedTenant.tenantName}` : "" }}</strong>
                <p class="card-desc">限制租户整体请求容量，不参与租户的商业定价和用户授权。</p>
              </div>
              <!-- 保存按钮提到卡片头部右上角,loading/选中租户逻辑与原右下角按钮一致 -->
              <el-button v-if="selectedTenant" type="primary" :loading="savingPolicy" @click="saveLimitPolicy">保存租户策略</el-button>
            </div>
            <AdminTenantLimitPanel
              :selected-tenant="selectedTenant"
              :loading="policiesLoading"
              :configured="Boolean(currentPolicy)"
              :summary-text="limitSummaryText"
              :form="limitForm"
            />
          </div>

          <div v-show="activePolicyTab === 'upstream'" class="policy-pane">
            <div class="tab-heading">
              <div class="tab-heading__text">
                <strong>上游访问与倍率{{ selectedTenant ? ` · ${selectedTenant.tenantName}` : "" }}</strong>
                <p class="card-desc">公开资源固定可见，专属资源按租户授权；租户倍率可覆盖资源的默认扣费倍率。</p>
              </div>
              <!-- 保存按钮提到卡片头部右上角,loading/dirty 禁用逻辑与原右下角按钮一致 -->
              <el-button
                v-if="selectedTenant"
                type="primary"
                :loading="savingUpstreamAccess"
                :disabled="!upstreamAccessDirty"
                @click="saveUpstreamAccess"
              >保存租户策略</el-button>
            </div>
            <AdminTenantUpstreamAccessPanel
              :selected-tenant="selectedTenant"
              :loading="upstreamAccessLoading"
              :resources="upstreamResources"
              :policies="upstreamPolicyDrafts"
              @toggle="toggleUpstreamAccess"
              @update-multiplier="updateTenantMultiplierOverride"
            />
          </div>
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.tenant-policy-page { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 20px; }

/* 面板 body 无内边距:左右分栏收进同卡,只用 1px 分隔线分区;fill 模式下分栏区伸展撑满 */
.policy-body {
  flex: 1;
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  min-height: 520px;
}

.policy-side {
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: 20px;
  border-right: 1px solid var(--ds-line);
}

.policy-main {
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 16px 24px 24px;
}

.policy-pane {
  flex: 1;
  min-height: 0;
  padding-top: 16px;
}

/* 卡片头:标题/描述居左,保存按钮固定在右上角 */
.tab-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}
.tab-heading__text { min-width: 0; }
.card-desc { margin: 6px 0 0; color: var(--ds-muted); font-size: 13px; }

@media (max-width: 1199px) {
  .policy-body { grid-template-columns: minmax(0, 1fr); }
  .policy-side { border-right: none; border-bottom: 1px solid var(--ds-line); }
}
</style>
