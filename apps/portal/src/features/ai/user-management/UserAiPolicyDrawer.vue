<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantAiVisibleGroup } from "@/api/types/aiTenant";
import { formatMultiplier } from "@/platform/ai/utils";
import { DsDrawer, DsTag } from "@/shared/ui";
import UserPricingGroupsPanel from "@/views/tenant/ai/user-pricing/components/UserPricingGroupsPanel.vue";

import UserLimitPolicyPanel from "./components/UserLimitPolicyPanel.vue";
import type { UserAiPolicyTarget, UserGroupPolicyRow } from "./model";

const props = defineProps<{
  open: boolean;
  user: UserAiPolicyTarget | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const loading = shallowRef(false);
const visibleGroups = shallowRef<TenantAiVisibleGroup[]>([]);
const userBindings = reactive(new Map<string, number | null>());
let loadGeneration = 0;

const customGroupCount = computed(() => userBindings.size);
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

async function loadPolicyContext() {
  const userId = props.user?.userId;
  if (!props.open || !userId) return;
  const requestGeneration = ++loadGeneration;
  loading.value = true;
  try {
    const [groupsResponse, bindingsResponse] = await Promise.all([
      aiTenantApi.listMyGroups(),
      aiTenantApi.listUserGroups(userId)
    ]);
    if (requestGeneration !== loadGeneration) return;
    visibleGroups.value = groupsResponse.items ?? [];
    userBindings.clear();
    for (const binding of bindingsResponse.items ?? []) {
      userBindings.set(binding.group_id, binding.multiplier_override ?? null);
    }
  } catch (error) {
    if (requestGeneration !== loadGeneration) return;
    visibleGroups.value = [];
    userBindings.clear();
    ElMessage.error(error instanceof Error ? error.message : "加载用户 AI 策略失败");
  } finally {
    if (requestGeneration === loadGeneration) loading.value = false;
  }
}

async function toggleBinding(group: Pick<UserGroupPolicyRow, "id" | "name">, bind: boolean) {
  const userId = props.user?.userId;
  if (!userId) return;
  try {
    if (bind) {
      await aiTenantApi.upsertUserGroup(userId, group.id, { multiplier_override: null });
      ElMessage.success(`已为该用户打开「${group.name}」`);
    } else {
      await aiTenantApi.deleteUserGroup(userId, group.id);
      ElMessage.success(`已移除该用户的「${group.name}」例外配置`);
    }
    await loadPolicyContext();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存用户服务权限失败");
  }
}

async function editMultiplier(group: UserGroupPolicyRow) {
  const userId = props.user?.userId;
  if (!userId) return;
  const current = userBindings.get(group.id);
  try {
    const { value } = await ElMessageBox.prompt(
      `设置该用户在「${group.name}」的扣费倍率。留空则继承分组默认用户倍率 ×${formatMultiplier(group.default_user_multiplier)}。`,
      "用户扣费倍率",
      {
        inputValue: current == null ? "" : String(current),
        inputPattern: /^$|^\d+(\.\d+)?$/,
        inputErrorMessage: "请输入非负数，或留空继承"
      }
    );
    await aiTenantApi.upsertUserGroup(userId, group.id, {
      multiplier_override: value.trim() === "" ? null : Number(value)
    });
    ElMessage.success("用户扣费倍率已保存");
    await loadPolicyContext();
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "保存用户扣费倍率失败");
    }
  }
}

watch(
  () => [props.open, props.user?.userId] as const,
  ([open, userId]) => {
    if (open && userId) {
      void loadPolicyContext();
      return;
    }
    loadGeneration += 1;
  },
  { immediate: true }
);
</script>

<template>
  <DsDrawer
    v-if="open"
    :open="true"
    title="AI 策略"
    :subtitle="user ? `${user.username} · ${user.userId}` : '选择用户后配置策略'"
    width="920px"
    @close="emit('close')"
  >
    <div v-if="user" class="policy-drawer">
      <header class="policy-drawer__summary">
        <div>
          <strong>{{ user.username }}</strong>
          <p>策略仅作用于当前用户，未设置的项目继续继承租户默认配置。</p>
        </div>
        <div class="policy-drawer__summary-actions">
          <DsTag tone="neutral">服务例外 {{ customGroupCount }}</DsTag>
          <el-tooltip content="刷新策略" placement="top">
            <el-button
              circle
              text
              :icon="Refresh"
              :loading="loading"
              aria-label="刷新 AI 策略"
              @click="loadPolicyContext"
            />
          </el-tooltip>
        </div>
      </header>

      <section class="policy-drawer__section">
        <UserPricingGroupsPanel
          :selected-user="user"
          :loading="loading"
          :rows="groupRows"
          @toggle-binding="toggleBinding"
          @edit-multiplier="editMultiplier"
        />
      </section>

      <section class="policy-drawer__section">
        <header class="policy-drawer__section-head">
          <h3>调用限额</h3>
          <p>为该用户设置专属并发限制；未配置时只受租户级策略约束。</p>
        </header>
        <UserLimitPolicyPanel :user="user" />
      </section>
    </div>
  </DsDrawer>
</template>

<style scoped>
.policy-drawer {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.policy-drawer__summary {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 20px;
  border-bottom: 1px solid var(--ds-line);
}

.policy-drawer__summary strong {
  color: var(--ds-ink);
  font-size: 16px;
}

.policy-drawer__summary p,
.policy-drawer__section-head p {
  margin: 5px 0 0;
  color: var(--ds-muted);
  font-size: 13px;
  line-height: 1.6;
}

.policy-drawer__summary-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.policy-drawer__section + .policy-drawer__section {
  padding-top: 24px;
  border-top: 1px solid var(--ds-line);
}

.policy-drawer__section-head {
  margin-bottom: 16px;
}

.policy-drawer__section-head h3 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
}

@media (max-width: 640px) {
  .policy-drawer__summary {
    flex-direction: column;
  }
}
</style>
