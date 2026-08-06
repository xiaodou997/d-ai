<!--
  用户管理 — 为单个终端用户配置 AI 服务权限和调用限额,并查看使用记录与使用统计。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       用户列表/配置区两栏收进同卡 body 的 24px 容器,fill 撑满视口);
       el-tabs → DsTabs(面板按访问懒挂载、v-show 切换,保持原懒加载语义),
       el-tag → DsTag;业务逻辑、请求参数与路由同步保持不变。弹窗/表单仍为 element-plus。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Users } from "lucide-vue-next";
import { PortalContentCard, PortalPagePanel } from "@/platform";
import { formatMultiplier } from "@/platform/ai/utils";
import { DsEmpty, DsTabs, DsTag } from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import { tenantApi } from "@/api/tenant";
import type { TenantAiVisibleGroup } from "@/api/types/aiTenant";
import type { TenantEndUserItem } from "@/api/types/tenant";
import { END_USER_LOOKUP_PAGE_SIZE, findEndUserById } from "@/utils/endUsers";
import UserPricingGroupsPanel from "@/views/tenant/ai/user-pricing/components/UserPricingGroupsPanel.vue";
import UserPricingUsersPanel from "@/views/tenant/ai/user-pricing/components/UserPricingUsersPanel.vue";
import UserLimitPolicyPanel from "./components/UserLimitPolicyPanel.vue";
import UserUsageFilters from "./components/UserUsageFilters.vue";
import UserUsageRecordsPanel from "./components/UserUsageRecordsPanel.vue";
import UserUsageSummaryPanel from "./components/UserUsageSummaryPanel.vue";
import {
  defaultUserUsageFilters,
  type UserGroupPolicyRow,
  type UserUsageFilters as UserUsageFiltersState
} from "./model";

const route = useRoute();
const router = useRouter();
const usersLoading = shallowRef(false);
const users = shallowRef<TenantEndUserItem[]>([]);
const keyword = shallowRef("");
const selectedUser = shallowRef<TenantEndUserItem | null>(null);
const activeTab = shallowRef("access");

const groupsLoading = shallowRef(false);
const visibleGroups = shallowRef<TenantAiVisibleGroup[]>([]);
const userBindings = reactive(new Map<string, number | null>());
const usageFilters = reactive<UserUsageFiltersState>(defaultUserUsageFilters());
const usageReloadKey = shallowRef(0);
let bindingsGeneration = 0;

// DsTabs 只是标签条,内容区自行按标签切换;visitedTabs 记录已访问标签,
// 未访问的懒挂载、已访问的 v-show 保活,对齐原 el-tabs 的懒渲染语义
const tabs = [
  { key: "access", label: "服务权限" },
  { key: "limits", label: "调用限额" },
  { key: "usage-records", label: "使用记录" },
  { key: "usage-summary", label: "使用统计" }
];
const visitedTabs = reactive(new Set<string>(["access"]));
watch(activeTab, (tab) => visitedTabs.add(tab));

const routedUserId = computed(() => (typeof route.params.userId === "string" ? route.params.userId : ""));
const showUsageFilters = computed(() => activeTab.value === "usage-records" || activeTab.value === "usage-summary");
const filteredUsers = computed(() => {
  const value = keyword.value.trim().toLowerCase();
  if (!value) return users.value;
  return users.value.filter((user) => user.username.toLowerCase().includes(value) || (user.email || "").toLowerCase().includes(value));
});
const customUserGroupCount = computed(() => userBindings.size);
const groupRows = computed<UserGroupPolicyRow[]>(() => visibleGroups.value.map((group) => {
  const bound = userBindings.has(group.id);
  const defaultVisible = Boolean(group.user_default_visible);
  return {
    id: group.id,
    name: group.name,
    default_user_multiplier: group.default_user_multiplier,
    user_default_visible: defaultVisible,
    user_bound: bound,
    user_multiplier_override: bound ? (userBindings.get(group.id) ?? null) : null,
    availability_state: bound ? "custom" : defaultVisible ? "default" : "unavailable"
  };
}));

async function loadUsers() {
  usersLoading.value = true;
  try {
    const response = await tenantApi.listEndUsers({ page: 1, size: END_USER_LOOKUP_PAGE_SIZE });
    users.value = response.items ?? [];
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "加载终端用户失败");
  } finally {
    usersLoading.value = false;
  }
}

function resolveUserById(userId: string) {
  return findEndUserById<TenantEndUserItem>((params) => tenantApi.listEndUsers(params), userId);
}

async function loadGroups() {
  try {
    const response = await aiTenantApi.listMyGroups();
    visibleGroups.value = response.items ?? [];
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "加载分组失败");
  }
}

async function loadUserBindings(userId: string) {
  const requestGeneration = ++bindingsGeneration;
  groupsLoading.value = true;
  try {
    const response = await aiTenantApi.listUserGroups(userId);
    if (requestGeneration !== bindingsGeneration) return;
    userBindings.clear();
    for (const binding of response.items ?? []) userBindings.set(binding.group_id, binding.multiplier_override ?? null);
  } catch (error) {
    if (requestGeneration !== bindingsGeneration) return;
    ElMessage.error(error instanceof Error ? error.message : "加载用户服务权限失败");
    userBindings.clear();
  } finally {
    if (requestGeneration === bindingsGeneration) groupsLoading.value = false;
  }
}

async function selectUser(user: TenantEndUserItem, syncRoute = true) {
  selectedUser.value = user;
  if (syncRoute && routedUserId.value !== user.userId) {
    await router.replace({ name: "ai-user-management", params: { userId: user.userId } });
  }
  await loadUserBindings(user.userId);
}

function openOverview() {
  if (!selectedUser.value) return;
  void router.push(`/tenant/users/directory/${encodeURIComponent(selectedUser.value.userId)}`);
}

async function toggleBinding(group: Pick<UserGroupPolicyRow, "id" | "name">, bind: boolean) {
  if (!selectedUser.value) return;
  const userId = selectedUser.value.userId;
  try {
    if (bind) {
      await aiTenantApi.upsertUserGroup(userId, group.id, { multiplier_override: null });
      ElMessage.success(`已为该用户打开「${group.name}」`);
    } else {
      await aiTenantApi.deleteUserGroup(userId, group.id);
      ElMessage.success(`已移除该用户的「${group.name}」例外配置`);
    }
    await loadUserBindings(userId);
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存用户服务权限失败");
  }
}

async function editMultiplier(group: UserGroupPolicyRow) {
  if (!selectedUser.value) return;
  const current = userBindings.get(group.id);
  const inherited = group.default_user_multiplier;
  try {
    const { value } = await ElMessageBox.prompt(
      `设置该用户在「${group.name}」的扣费倍率。留空则继承分组默认用户倍率 ×${formatMultiplier(inherited)}。`,
      "用户扣费倍率",
      {
        inputValue: current == null ? "" : String(current),
        inputPattern: /^$|^\d+(\.\d+)?$/,
        inputErrorMessage: "请输入非负数，或留空继承"
      }
    );
    await aiTenantApi.upsertUserGroup(selectedUser.value.userId, group.id, {
      multiplier_override: value.trim() === "" ? null : Number(value)
    });
    ElMessage.success("用户扣费倍率已保存");
    await loadUserBindings(selectedUser.value.userId);
  } catch {
    // 用户取消时不需要反馈。
  }
}

function refreshAll() {
  void Promise.all([loadUsers(), loadGroups()]);
  if (selectedUser.value) {
    void loadUserBindings(selectedUser.value.userId);
    usageReloadKey.value += 1;
  }
}

function refreshUsage() {
  usageReloadKey.value += 1;
}

onMounted(() => {
  void Promise.all([loadUsers(), loadGroups()]);
});

watch(
  [users, routedUserId],
  async ([items, userId]) => {
    if (!userId || selectedUser.value?.userId === userId) return;
    let target = items.find((item) => item.userId === userId);
    if (!target) {
      const resolved = await resolveUserById(userId);
      if (resolved) {
        users.value = users.value.some((item) => item.userId === resolved.userId) ? users.value : [resolved, ...users.value];
      }
      target = resolved ?? undefined;
    }
    if (target) void selectUser(target, false);
  },
  { immediate: true }
);
</script>

<template>
  <div class="page-container user-management-page">
    <PortalPagePanel
      fill
      :icon="Users"
      :breadcrumbs="[{ label: '智能服务' }, { label: '用户与订阅' }, { label: '用户管理' }]"
      description="为单个终端用户配置 AI 服务权限和调用限额，并查看该用户的使用记录与使用统计。"
    >
      <template #actions>
        <el-button :icon="Refresh" @click="refreshAll">刷新</el-button>
      </template>

      <!-- 主从布局:body 无内边距,用 24px 容器承载用户列表 + 配置区两栏 -->
      <div class="management-body">
        <UserPricingUsersPanel
          v-model:keyword="keyword"
          :users="filteredUsers"
          :loading="usersLoading"
          :selected-user-id="selectedUser?.userId || ''"
          @select-user="selectUser"
        />

        <PortalContentCard class="user-detail-card">
          <template #header>
            <div class="detail-card-copy">
              <span class="card-title">{{ selectedUser ? `AI 服务 · ${selectedUser.username}` : "AI 服务设置" }}</span>
              <p class="card-desc">{{ selectedUser ? "管理该用户的 AI 服务权限、调用限额和使用情况。" : "选择一个终端用户后开始管理。" }}</p>
            </div>
          </template>
          <template #actions>
            <DsTag v-if="selectedUser" tone="neutral">服务例外 {{ customUserGroupCount }}</DsTag>
            <el-button v-if="selectedUser" link type="primary" @click="openOverview">查看用户详情</el-button>
          </template>

          <template v-if="selectedUser">
            <UserUsageFilters v-if="showUsageFilters" v-model="usageFilters" :loading="false" @search="refreshUsage" />

            <DsTabs v-model="activeTab" :tabs="tabs" class="management-tabs" />

            <!-- 各面板均为单根组件:未访问懒挂载,访问后 v-show 保活 -->
            <UserPricingGroupsPanel
              v-if="visitedTabs.has('access')"
              v-show="activeTab === 'access'"
              :selected-user="selectedUser || undefined"
              :loading="groupsLoading"
              :rows="groupRows"
              @toggle-binding="toggleBinding"
              @edit-multiplier="editMultiplier"
            />
            <UserLimitPolicyPanel
              v-if="visitedTabs.has('limits')"
              v-show="activeTab === 'limits'"
              :user="selectedUser"
            />
            <UserUsageRecordsPanel
              v-if="visitedTabs.has('usage-records')"
              v-show="activeTab === 'usage-records'"
              :user="selectedUser"
              :filters="usageFilters"
              :reload-key="usageReloadKey"
            />
            <UserUsageSummaryPanel
              v-if="visitedTabs.has('usage-summary')"
              v-show="activeTab === 'usage-summary'"
              :user="selectedUser"
              :filters="usageFilters"
              :reload-key="usageReloadKey"
            />
          </template>

          <DsEmpty v-else title="未选择用户" description="从左侧选择一个终端用户" />
        </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.user-management-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.management-body {
  display: grid;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  align-items: start;
  gap: 20px;
  padding: 24px;
  flex: 1;
  min-height: 0;
}

.detail-card-copy { display: flex; flex-direction: column; gap: 4px; }
.card-title { color: var(--ds-ink); font-weight: 700; }
.card-desc { margin: 0; color: var(--ds-muted); font-size: 13px; }
.user-detail-card { min-height: 100%; }
.management-tabs { margin-top: 14px; }

@media (max-width: 960px) {
  .management-body {
    grid-template-columns: 1fr;
  }
}
</style>
