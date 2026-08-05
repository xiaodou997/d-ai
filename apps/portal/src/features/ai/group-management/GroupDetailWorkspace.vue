<!--
  分组详情 — 配置客户端入口、模型调度和上游目标。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       面包屑「分组管理」可回跳列表,返回按钮随之移除);el-tabs 改 DsTabs(置于 24px
       容器内,页签切换前的未保存确认拦截保留);空态 DsEmpty;业务逻辑与请求保持不变。
-->
<script setup lang="ts">
import { computed, shallowRef, useTemplateRef, watch } from "vue";
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from "vue-router";
import { ElMessage, ElMessageBox } from "element-plus";
import { Layers, RefreshCw } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsEmpty, DsTabs } from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import type {
  TenantAiGroupWriteRequest,
  TenantAiPriceBook,
  TenantAiVisibleGroup
} from "@/api/types/aiTenant";
import GroupFormDialog from "./components/GroupFormDialog.vue";
import GroupSummaryCard from "./components/GroupSummaryCard.vue";
import GroupClientSurfacePolicyPanel from "./tabs/GroupClientSurfacePolicyPanel.vue";
import GroupDispatchRulesPanel from "./tabs/GroupDispatchRulesPanel.vue";
import GroupTargetsWorkspace from "./tabs/GroupTargetsWorkspace.vue";
import {
  errorMessage,
  showDispatchPriceConflict,
  showGroupDependencies
} from "./problemPresentation";

type DetailTab = "client-surfaces" | "dispatch-rules" | "targets";
interface DirtyHandle { confirmDiscardChanges(message: string): Promise<boolean> }

const validTabs: DetailTab[] = ["client-surfaces", "dispatch-rules", "targets"];
const detailTabs: { key: DetailTab; label: string }[] = [
  { key: "client-surfaces", label: "API 入口" },
  { key: "dispatch-rules", label: "请求规则" },
  { key: "targets", label: "关联上游目标" }
];
const route = useRoute();
const router = useRouter();
const loading = shallowRef(false);
const busy = shallowRef(false);
const group = shallowRef<TenantAiVisibleGroup | null>(null);
const priceBooks = shallowRef<TenantAiPriceBook[]>([]);
const dialogVisible = shallowRef(false);
const surfacePanel = useTemplateRef<DirtyHandle>("surfacePanel");
const targetsPanel = useTemplateRef<DirtyHandle>("targetsPanel");
let loadGeneration = 0;

const groupId = computed(() => String(route.params.groupId || ""));
const priceBookName = computed(() => group.value?.retail_price_book_name || priceBooks.value.find((book) => book.id === group.value?.retail_price_book_id)?.name || group.value?.retail_price_book_id || "-");
const activeTab = computed<DetailTab>({
  get: () => {
    const requestedTab = String(route.query.tab || "client-surfaces");
    const tab = requestedTab === "dispatch-preview" ? "dispatch-rules" : requestedTab as DetailTab;
    return validTabs.includes(tab) ? tab : "client-surfaces";
  },
  set: (tab) => {
    void router.replace({ query: { ...route.query, tab } });
  }
});

async function load() {
  const id = groupId.value;
  const generation = ++loadGeneration;
  if (!id) {
    group.value = null;
    return;
  }
  loading.value = true;
  try {
    const [nextGroup, books] = await Promise.all([
      aiTenantApi.getGroup(id),
      aiTenantApi.listPriceBooks()
    ]);
    if (generation !== loadGeneration || id !== groupId.value) return;
    group.value = nextGroup;
    priceBooks.value = books.items || [];
  } catch (error: unknown) {
    if (generation === loadGeneration) {
      group.value = null;
      ElMessage.error(errorMessage(error, "加载分组详情失败"));
    }
  } finally {
    if (generation === loadGeneration) loading.value = false;
  }
}

async function saveGroup(payload: TenantAiGroupWriteRequest) {
  if (!group.value) return;
  busy.value = true;
  try {
    group.value = await aiTenantApi.updateGroup(group.value.id, payload);
    dialogVisible.value = false;
    ElMessage.success("分组已更新");
  } catch (error: unknown) {
    if (!await showDispatchPriceConflict(error)) ElMessage.error(errorMessage(error, "保存分组失败"));
  } finally {
    busy.value = false;
  }
}

async function toggleGroup() {
  if (!group.value) return;
  busy.value = true;
  try {
    group.value = await aiTenantApi.updateGroupStatus(group.value.id, group.value.status === "active" ? "disabled" : "active");
    ElMessage.success("分组状态已更新");
  } catch (error: unknown) {
    if (!await showDispatchPriceConflict(error)) ElMessage.error(errorMessage(error, "更新分组状态失败"));
  } finally {
    busy.value = false;
  }
}

async function removeGroup() {
  if (!group.value) return;
  try {
    await ElMessageBox.confirm(`删除分组「${group.value.name}」？API 入口策略、调度规则和上游关联会一并清理。`, "确认删除", { type: "warning" });
    await aiTenantApi.deleteGroup(group.value.id);
    ElMessage.success("分组已删除");
    await router.push({ name: "ai-groups" });
  } catch (error: unknown) {
    if (error === "cancel" || error === "close") return;
    if (!await showGroupDependencies(error)) ElMessage.error(errorMessage(error, "删除分组失败"));
  }
}

async function confirmAllDirty(message: string) {
  for (const handle of [surfacePanel.value, targetsPanel.value]) {
    if (handle && !await handle.confirmDiscardChanges(message)) return false;
  }
  return true;
}

async function beforeTabLeave(next: string | number, previous: string | number) {
  if (next === previous) return true;
  if (previous === "client-surfaces" && surfacePanel.value) {
    return surfacePanel.value.confirmDiscardChanges("切换页签将放弃尚未保存的 API 入口变更，是否继续？");
  }
  if (previous === "targets" && targetsPanel.value) {
    return targetsPanel.value.confirmDiscardChanges("切换页签将放弃尚未保存的上游关联变更，是否继续？");
  }
  return true;
}

// DsTabs 无 before-leave,切换前先跑未保存确认,放弃则回滚选中态。
async function selectTab(next: string) {
  const previous = activeTab.value;
  if (next === previous) return;
  if (await beforeTabLeave(next, previous)) activeTab.value = next as DetailTab;
}

watch(groupId, load, { immediate: true });
onBeforeRouteLeave(() => confirmAllDirty("离开页面将放弃尚未保存的变更，是否继续？"));
onBeforeRouteUpdate((to, from) => to.params.groupId === from.params.groupId || confirmAllDirty("切换分组将放弃尚未保存的变更，是否继续？"));
</script>

<template>
  <div v-loading="loading" class="page-container group-detail-page">
    <PortalPagePanel
      :icon="Layers"
      :breadcrumbs="[
        { label: '智能服务' },
        { label: '分组管理', to: '/workspace/groups' },
        { label: '分组详情' }
      ]"
      description="配置客户端入口、模型调度和上游目标。"
    >
      <template #actions>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">刷新</el-button>
      </template>

      <!-- 详情主体:body 无内边距,用 24px 容器承载概要卡与标签页 -->
      <div v-if="group" class="detail-body">
        <GroupSummaryCard :group="group" :price-book-name="priceBookName" :busy="busy" @edit="dialogVisible = true" @toggle="toggleGroup" @remove="removeGroup" />

        <DsTabs :model-value="activeTab" :tabs="detailTabs" @update:model-value="selectTab" />

        <GroupClientSurfacePolicyPanel v-show="activeTab === 'client-surfaces'" ref="surfacePanel" :group-id="group.id" />
        <GroupDispatchRulesPanel v-show="activeTab === 'dispatch-rules'" :group-id="group.id" :price-book-name="priceBookName" />
        <GroupTargetsWorkspace v-show="activeTab === 'targets'" ref="targetsPanel" :group-id="group.id" />
      </div>
      <div v-else class="detail-body">
        <DsEmpty title="分组不存在或已删除" description="请返回分组管理列表重新选择" />
      </div>
    </PortalPagePanel>

    <GroupFormDialog v-if="group" v-model="dialogVisible" :group="group" :price-books="priceBooks" :submitting="busy" @submit="saveGroup" />
  </div>
</template>

<style scoped>
.group-detail-page {
  display: flex;
  flex-direction: column;
}

.detail-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
}

@media (max-width: 640px) {
  .detail-body {
    padding: 12px;
  }
}
</style>
