<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantAiVisibleGroup } from "@/api/types/aiTenant";
import { formatMultiplier } from "@/platform/ai/utils";
import UserPricingGroupsPanel from "@/views/tenant/ai/user-pricing/components/UserPricingGroupsPanel.vue";

import type { UserGroupPolicyRow, UserPolicyTarget } from "./model";

const props = defineProps<{
  open: boolean;
  user: UserPolicyTarget | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const loading = shallowRef(false);
const visibleGroups = shallowRef<TenantAiVisibleGroup[]>([]);
const userBindings = reactive(new Map<string, number | null>());
let loadGeneration = 0;

const title = computed(() => props.user ? `分组策略 · ${props.user.username}` : "分组策略");
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

async function loadPolicy() {
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
    ElMessage.error(error instanceof Error ? error.message : "加载分组策略失败");
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
    } else {
      await aiTenantApi.deleteUserGroup(userId, group.id);
    }
    ElMessage.success("分组策略已更新");
    await loadPolicy();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存分组策略失败");
  }
}

async function editMultiplier(group: UserGroupPolicyRow) {
  const userId = props.user?.userId;
  if (!userId) return;
  const current = userBindings.get(group.id);
  try {
    const { value } = await ElMessageBox.prompt(
      `留空则继承分组默认倍率 ×${formatMultiplier(group.default_user_multiplier)}`,
      `设置 ${group.name} 的用户倍率`,
      {
        inputValue: current == null ? "" : String(current),
        inputPattern: /^$|^\d+(\.\d+)?$/,
        inputErrorMessage: "请输入非负数，或留空继承"
      }
    );
    await aiTenantApi.upsertUserGroup(userId, group.id, {
      multiplier_override: value.trim() === "" ? null : Number(value)
    });
    ElMessage.success("用户倍率已保存");
    await loadPolicy();
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "保存用户倍率失败");
    }
  }
}

watch(
  () => [props.open, props.user?.userId] as const,
  ([open, userId]) => {
    if (open && userId) {
      void loadPolicy();
      return;
    }
    loadGeneration += 1;
  },
  { immediate: true }
);
</script>

<template>
  <el-dialog
    v-if="open"
    :model-value="true"
    :title="title"
    width="min(760px, calc(100vw - 32px))"
    append-to-body
    destroy-on-close
    @close="emit('close')"
  >
    <UserPricingGroupsPanel
      :selected-user="user"
      :loading="loading"
      :rows="groupRows"
      :show-title="false"
      @toggle-binding="toggleBinding"
      @edit-multiplier="editMultiplier"
    />

    <template #footer>
      <el-button @click="emit('close')">关闭</el-button>
    </template>
  </el-dialog>
</template>
