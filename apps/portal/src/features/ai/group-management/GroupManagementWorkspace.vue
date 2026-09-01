<!--
  分组管理 — 管理租户零售分组及其价格、入口、调度和上游关联。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       筛选/表格/分页同卡);el-table 改 DsTable,空态 DsEmpty,接口无分页改为前端分页
       (DsPagination 始终渲染);行点击进详情保留(容器事件委托,跳过开关/按钮);
       业务逻辑、请求与弹窗(element-plus)保持不变。
-->
<script setup lang="ts">
import { computed, onMounted, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { ElMessage, ElMessageBox } from "element-plus";
import { Eye, Layers, Plus, RefreshCw } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { formatMultiplier } from "@/platform/ai/utils";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  type DsTableColumn
} from "@/shared/ui";

import { aiTenantApi } from "@/api/aiTenant";
import type {
  TenantAiGroupWriteRequest,
  TenantAiPriceBook,
  TenantAiVisibleGroup
} from "@/api/types/aiTenant";
import GroupFormDialog from "./components/GroupFormDialog.vue";
import GroupModelPreviewDialog from "./components/GroupModelPreviewDialog.vue";
import {
  errorMessage,
  isGroupRoutePolicyConflict,
  showDispatchPriceConflict,
  showGroupDependencies
} from "./problemPresentation";

const router = useRouter();
const loading = shallowRef(false);
const submitting = shallowRef(false);
const groups = shallowRef<TenantAiVisibleGroup[]>([]);
const priceBooks = shallowRef<TenantAiPriceBook[]>([]);
const keyword = shallowRef("");
const dialogVisible = shallowRef(false);
const editingGroup = shallowRef<TenantAiVisibleGroup | null>(null);
const previewGroup = shallowRef<TenantAiVisibleGroup | null>(null);
const previewVisible = shallowRef(false);
const updatingGroupId = shallowRef("");
let loadGeneration = 0;

const priceBookNames = computed(() => new Map(priceBooks.value.map((book) => [book.id, book.name])));
const visibleGroups = computed(() => {
  const query = keyword.value.trim().toLowerCase();
  const items = query
    ? groups.value.filter((group) => [
      group.name,
      group.description,
      group.retail_price_book_name,
      priceBookNames.value.get(group.retail_price_book_id)
    ].filter(Boolean).some((value) => String(value).toLowerCase().includes(query)))
    : groups.value;
  return [...items].sort((left, right) => left.name.localeCompare(right.name, "zh-CN", { numeric: true }));
});

const page = shallowRef(1);
const pageSize = shallowRef(20);
const pagedGroups = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return visibleGroups.value.slice(start, start + pageSize.value);
});
watch([keyword, pageSize], () => { page.value = 1; });

const columns: DsTableColumn[] = [
  { key: "name", title: "名称" },
  { key: "priceBook", title: "零售价格表", width: 240 },
  { key: "exclusive", title: "专属分组", width: 128, align: "center" },
  { key: "multiplier", title: "默认倍率", width: 120, align: "center" },
  { key: "protocol", title: "协议转换", width: 120, align: "center" },
  { key: "status", title: "状态", width: 108, align: "center" },
  { key: "actions", title: "操作", width: 230, align: "right" }
];

async function load() {
  const generation = ++loadGeneration;
  loading.value = true;
  try {
    const [groupResponse, bookResponse] = await Promise.all([
      aiTenantApi.listMyGroups(),
      aiTenantApi.listPriceBooks()
    ]);
    if (generation !== loadGeneration) return;
    groups.value = groupResponse.items || [];
    priceBooks.value = bookResponse.items || [];
  } catch (error: unknown) {
    if (generation === loadGeneration) ElMessage.error(errorMessage(error, "加载分组列表失败"));
  } finally {
    if (generation === loadGeneration) loading.value = false;
  }
}

function openCreate() {
  editingGroup.value = null;
  dialogVisible.value = true;
}

function openEdit(group: TenantAiVisibleGroup) {
  editingGroup.value = group;
  dialogVisible.value = true;
}

function openPreview(group: TenantAiVisibleGroup) {
  previewGroup.value = group;
  previewVisible.value = true;
}

function openDetail(group: TenantAiVisibleGroup) {
  void router.push({ name: "ai-group-detail", params: { groupId: group.id } });
}

// DsTable 无行点击事件,用容器委托还原原 el-table 的整行进详情;开关/按钮内点击不触发。
function handleTableClick(event: MouseEvent) {
  if (loading.value) return;
  const target = event.target as HTMLElement;
  if (target.closest("button, input, .el-switch, a")) return;
  const row = target.closest("tbody tr");
  if (!row || !row.parentElement) return;
  const index = Array.from(row.parentElement.children).indexOf(row);
  const group = pagedGroups.value[index];
  if (group) openDetail(group);
}

async function submit(payload: TenantAiGroupWriteRequest) {
  submitting.value = true;
  try {
    const saved = editingGroup.value
      ? await aiTenantApi.updateGroup(editingGroup.value.id, payload)
      : await aiTenantApi.createGroup(payload);
    dialogVisible.value = false;
    ElMessage.success(editingGroup.value ? "分组已更新" : "分组已创建");
    if (!editingGroup.value) {
      await router.push({ name: "ai-group-detail", params: { groupId: saved.id } });
      return;
    }
    await load();
  } catch (error: unknown) {
    if (isGroupRoutePolicyConflict(error)) {
      ElMessage.warning("路由配置已被其他窗口修改，已重新加载最新版本");
      await load();
    } else if (!await showDispatchPriceConflict(error)) ElMessage.error(errorMessage(error, "保存分组失败"));
  } finally {
    submitting.value = false;
  }
}

async function remove(group: TenantAiVisibleGroup) {
  try {
    await ElMessageBox.confirm(`删除分组「${group.name}」？API 入口策略、调度规则和上游关联会一并清理。`, "确认删除", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消"
    });
    await aiTenantApi.deleteGroup(group.id);
    ElMessage.success("分组已删除");
    await load();
  } catch (error: unknown) {
    if (error === "cancel" || error === "close") return;
    if (!await showGroupDependencies(error)) ElMessage.error(errorMessage(error, "删除分组失败"));
  }
}

function groupWriteRequest(group: TenantAiVisibleGroup, patch: Partial<TenantAiGroupWriteRequest> = {}): TenantAiGroupWriteRequest {
  return {
    name: group.name,
    description: group.description || undefined,
    retail_price_book_id: group.retail_price_book_id,
    default_user_multiplier: group.default_user_multiplier,
    user_default_visible: group.user_default_visible,
    allow_protocol_conversion: group.allow_protocol_conversion,
    route_policy: group.route_policy,
    route_policy_version: group.route_policy_version,
    sort_order: group.sort_order,
    status: group.status === "disabled" ? "disabled" : "active",
    ...patch
  };
}

async function updateVisibility(group: TenantAiVisibleGroup, exclusive: boolean) {
  updatingGroupId.value = group.id;
  try {
    const saved = await aiTenantApi.updateGroup(group.id, groupWriteRequest(group, {
      user_default_visible: !exclusive
    }));
    groups.value = groups.value.map((item) => item.id === saved.id ? saved : item);
    ElMessage.success(exclusive ? "分组已设为专属" : "分组已设为公开");
  } catch (error: unknown) {
    if (isGroupRoutePolicyConflict(error)) {
      ElMessage.warning("路由配置已被其他窗口修改，已重新加载最新版本");
      await load();
    } else if (!await showDispatchPriceConflict(error)) ElMessage.error(errorMessage(error, "更新分组可见范围失败"));
  } finally {
    updatingGroupId.value = "";
  }
}

async function updateStatus(group: TenantAiVisibleGroup, active: boolean) {
  updatingGroupId.value = group.id;
  try {
    const saved = await aiTenantApi.updateGroupStatus(group.id, active ? "active" : "disabled");
    groups.value = groups.value.map((item) => item.id === saved.id ? saved : item);
    ElMessage.success(active ? "分组已启用" : "分组已停用");
  } catch (error: unknown) {
    if (!await showDispatchPriceConflict(error)) ElMessage.error(errorMessage(error, "更新分组状态失败"));
  } finally {
    updatingGroupId.value = "";
  }
}

onMounted(load);
</script>

<template>
  <div class="page-container group-list-page">
    <PortalPagePanel
      fill
      :icon="Layers"
      :breadcrumbs="[{ label: '智能服务' }, { label: '商业配置' }, { label: '分组管理' }]"
      description="管理租户零售分组及其价格、入口、调度和上游关联。"
    >
      <template #actions>
        <el-button :icon="RefreshCw" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建分组</el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input v-model="keyword" clearable placeholder="搜索名称、描述或价格表" class="search-input" />
          </DsFilterField>
          <template #actions>
            <span class="group-count">{{ visibleGroups.length }} 个分组</span>
          </template>
        </DsFilterBar>
      </template>

      <div class="groups-table-wrap" @click="handleTableClick">
        <DsTable
          :frame="false"
          :columns="columns"
          :rows="pagedGroups"
          row-key="id"
          :loading="loading"
        >
          <template #empty>
            <DsEmpty title="暂无分组" description="还没有零售分组,先新建一个吧">
              <template #action>
                <el-button type="primary" :icon="Plus" @click="openCreate">新建分组</el-button>
              </template>
            </DsEmpty>
          </template>
          <template #cell-name="{ row }">
            <div class="name-cell"><strong>{{ row.name }}</strong><span>{{ row.description || "未填写描述" }}</span></div>
          </template>
          <template #cell-priceBook="{ row }">
            {{ row.retail_price_book_name || priceBookNames.get(row.retail_price_book_id) || row.retail_price_book_id }}
          </template>
          <template #cell-exclusive="{ row }">
            <el-switch
              :model-value="!row.user_default_visible"
              inline-prompt
              active-text="专属"
              inactive-text="公开"
              :loading="updatingGroupId === row.id"
              :disabled="Boolean(updatingGroupId) && updatingGroupId !== row.id"
              @update:model-value="updateVisibility(row, Boolean($event))"
            />
          </template>
          <template #cell-multiplier="{ row }">×{{ formatMultiplier(row.default_user_multiplier) }}</template>
          <template #cell-protocol="{ row }">{{ row.allow_protocol_conversion ? "允许" : "禁止" }}</template>
          <template #cell-status="{ row }">
            <el-switch
              :model-value="row.status === 'active'"
              inline-prompt
              active-text="启用"
              inactive-text="停用"
              :loading="updatingGroupId === row.id"
              :disabled="Boolean(updatingGroupId) && updatingGroupId !== row.id"
              @update:model-value="updateStatus(row, Boolean($event))"
            />
          </template>
          <template #cell-actions="{ row }">
            <el-button link type="primary" size="small" :icon="Eye" @click="openPreview(row)">预览</el-button>
            <el-button link type="primary" size="small" @click="openDetail(row)">详情</el-button>
            <el-button link type="warning" size="small" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
          </template>
        </DsTable>
      </div>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="visibleGroups.length"
          @update:page="page = $event"
          @update:page-size="pageSize = $event"
        />
      </template>
    </PortalPagePanel>

    <GroupFormDialog v-model="dialogVisible" :group="editingGroup" :price-books="priceBooks" :submitting="submitting" @submit="submit" />
    <GroupModelPreviewDialog
      v-model="previewVisible"
      :group="previewGroup"
      :load-models="aiTenantApi.getMyGroupEffectivePrices"
    />
  </div>
</template>

<style scoped>
.group-list-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.groups-table-wrap {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.groups-table-wrap :deep(.ds-table__row) {
  cursor: pointer;
}

.search-input {
  width: min(420px, 100%);
}

.group-count {
  color: var(--ds-muted);
  font-size: 12px;
}

.name-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.name-cell strong {
  overflow-wrap: anywhere;
  color: var(--ds-ink);
  font-size: 13px;
}

.name-cell span {
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
