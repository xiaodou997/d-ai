<!--
  关联上游目标 — 分组详情页签:勾选/解除上游资源关联,草稿态统一保存。
  重构:el-table 改 DsTable(行键用草稿 key),el-tag 改 DsTag,空态走 DsTable 空槽;
       协议标记硬编码颜色换 accent token;租户只维护关联和启停状态。
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { CheckSquare, RefreshCw, RotateCcw, Save, Search } from "lucide-vue-next";
import { formatMultiplier } from "@/platform/ai/utils";
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import { useGroupTargets } from "../composables/useGroupTargets";
import type { GroupTargetDraft, GroupTargetSaveFailure } from "../groupTargets";
import { errorMessage } from "../problemPresentation";

const props = defineProps<{ groupId: string }>();
const emit = defineEmits<{ changed: [] }>();
const state = useGroupTargets({ groupId: () => props.groupId });

const UNAVAILABLE_REASON_LABELS: Record<"inactive" | "access_revoked" | "missing", string> = {
  inactive: "资源已停用",
  access_revoked: "已撤销授权",
  missing: "资源已删除"
};

// DsTable 的 cell 插槽 row 是 any,用一个显式收窄的 helper 查表,避免 any 索引类型。
function unavailableLabel(reason: string | null | undefined): string {
  if (!reason) return "";
  return UNAVAILABLE_REASON_LABELS[reason as keyof typeof UNAVAILABLE_REASON_LABELS] ?? "不可用";
}
const keyword = shallowRef("");
const linkFilter = shallowRef<"all" | "linked" | "unlinked">("all");
const failures = shallowRef<GroupTargetSaveFailure[]>([]);
const hasConflict = computed(() => failures.value.some((failure) => failure.code === "group_route_policy_conflict"));

const columns: DsTableColumn[] = [
  { key: "link", title: "关联", width: 58, align: "center" },
  { key: "resource", title: "上游资源" },
  { key: "priceModel", title: "价格模型", width: 104 },
  { key: "status", title: "状态", width: 104 },
  { key: "change", title: "变更", width: 82 }
];

const visibleRows = computed(() => {
  const query = keyword.value.trim().toLowerCase();
  return state.rows.value.filter((row) => {
    if (linkFilter.value === "linked" && !row.linked) return false;
    if (linkFilter.value === "unlinked" && row.linked) return false;
    if (!query) return true;
    return [row.name, row.targetId, ...row.protocols].some((value) => value.toLowerCase().includes(query));
  });
});

async function confirmDiscardChanges(message: string) {
  if (state.saving.value) {
    ElMessage.warning("关联变更正在保存");
    return false;
  }
  if (!state.hasChanges.value) return true;
  try {
    await ElMessageBox.confirm(message, "存在未保存变更", { type: "warning", confirmButtonText: "放弃变更", cancelButtonText: "继续编辑" });
    state.discard();
    failures.value = [];
    return true;
  } catch {
    return false;
  }
}

async function refresh() {
  if (!await confirmDiscardChanges("刷新将放弃尚未保存的关联变更，是否继续？")) return;
  failures.value = [];
  try {
    await state.load();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "加载上游关联失败"));
  }
}

function selectVisible() {
  for (const row of visibleRows.value) if (row.selectable) state.setSelected(row.key, true);
}

function updateDraft(key: string, patch: Partial<GroupTargetDraft>) {
  failures.value = [];
  state.updateDraft(key, patch);
}

async function save() {
  if (state.removals.value.length) {
    try {
      await ElMessageBox.confirm(`将解除 ${state.removals.value.length} 个上游关联，是否继续？`, "确认关联变更", { type: "warning" });
    } catch {
      return;
    }
  }
  const result = await state.save();
  failures.value = result.failures;
  if (result.failures.length) {
    ElMessage.warning(`已新增 ${result.added}、更新 ${result.updated}、解除 ${result.removed}，${result.failures.length} 项失败`);
  } else {
    ElMessage.success(`关联已更新：新增 ${result.added}、更新 ${result.updated}、解除 ${result.removed}`);
    emit("changed");
  }
}

async function reloadAfterConflict() {
  failures.value = [];
  await state.load();
  ElMessage.info("已重新加载最新分组目标配置，请重新确认变更");
}

function beforeUnload(event: BeforeUnloadEvent) {
  if (!state.hasChanges.value) return;
  event.preventDefault();
  event.returnValue = "";
}

onMounted(() => window.addEventListener("beforeunload", beforeUnload));
onBeforeUnmount(() => window.removeEventListener("beforeunload", beforeUnload));
defineExpose({ confirmDiscardChanges });
</script>

<template>
  <section class="targets-panel">
    <header class="panel-header">
      <h3>关联上游目标</h3>
      <el-button :icon="RefreshCw" :loading="state.loading.value" @click="refresh">刷新</el-button>
    </header>

    <el-alert v-if="state.loadError.value" :title="state.loadError.value" type="error" show-icon :closable="false" />

    <div class="filters">
      <el-input v-model="keyword" clearable :prefix-icon="Search" placeholder="搜索名称或资源 ID" />
      <el-select v-model="linkFilter" aria-label="关联状态"><el-option label="全部关联状态" value="all" /><el-option label="已关联" value="linked" /><el-option label="未关联" value="unlinked" /></el-select>
      <el-button :icon="CheckSquare" :disabled="state.saving.value || !visibleRows.length" @click="selectVisible">全选当前结果</el-button>
    </div>

    <div class="defaults-row">
      <span>勾选上游后会自动加入路由候选；停用的上游不会接收请求。</span>
      <span class="count">{{ visibleRows.length }} / {{ state.rows.value.length }}</span>
    </div>

    <DsTable
      :columns="columns"
      :rows="visibleRows"
      row-key="key"
      :loading="state.loading.value"
      empty-title="没有匹配的上游资源"
    >
      <template #cell-link="{ row }">
        <el-checkbox
          :model-value="row.selected"
          :disabled="state.saving.value || (!row.selectable && !row.selected)"
          :aria-label="`${row.selected ? '解除' : '关联'}${row.name}`"
          @update:model-value="state.setSelected(row.key, Boolean($event))"
        />
      </template>
      <template #cell-resource="{ row }">
        <div class="resource-line">
          <strong>{{ row.name }}</strong>
          <span v-for="protocol in row.protocols" :key="protocol" class="protocol-mark">{{ protocol }}</span>
          <span v-if="row.tenantMultiplier !== null" class="multiplier">×{{ formatMultiplier(row.tenantMultiplier) }}</span>
          <DsTag
            v-if="row.linked && row.bindingUnavailableReason"
            tone="danger"
            :title="`该关联当前不可用(${unavailableLabel(row.bindingUnavailableReason)}),请求会被网关拒绝`"
          >{{ unavailableLabel(row.bindingUnavailableReason) }}</DsTag>
        </div>
      </template>
      <template #cell-priceModel="{ row }">
        <DsTag :tone="row.resourceState === 'available' ? 'positive' : 'danger'">
          {{ row.resourceState === "available" ? `${row.availableModels} 个` : row.resourceState === "missing" ? "资源失效" : "无可用价格" }}
        </DsTag>
      </template>
      <template #cell-status="{ row }">
        <el-switch v-if="row.selected" :model-value="row.status === 'active'" inline-prompt active-text="启用" inactive-text="停用" size="small" :disabled="state.saving.value" @update:model-value="updateDraft(row.key, { status: $event ? 'active' : 'disabled' })" />
        <span v-else class="muted">未关联</span>
      </template>
      <template #cell-change="{ row }">
        <DsTag v-if="row.change" :tone="row.change === 'add' ? 'positive' : row.change === 'update' ? 'warning' : 'danger'">
          {{ row.change === "add" ? "待新增" : row.change === "update" ? "待更新" : "待解除" }}
        </DsTag>
        <span v-else class="muted">{{ row.linked ? "已保存" : "-" }}</span>
      </template>
    </DsTable>

    <el-alert v-for="failure in failures" :key="`${failure.action}:${failure.targetKey}`" :title="`${failure.targetName}：${failure.message}`" type="error" show-icon :closable="false" />
    <el-alert v-if="hasConflict" type="warning" title="配置版本已变化" show-icon :closable="false">
      <template #default>
        <span>其他窗口已修改了这个分组的路由配置，当前草稿没有写入。</span>
        <el-button link type="primary" :disabled="state.loading.value" @click="reloadAfterConflict">重新加载</el-button>
      </template>
    </el-alert>

    <footer class="save-bar">
      <span>已选 {{ state.rows.value.filter((row) => row.selected).length }}，新增 {{ state.additions.value.length }}，更新 {{ state.updates.value.length }}，解除 {{ state.removals.value.length }}</span>
      <div>
        <el-button :icon="RotateCcw" :disabled="!state.hasChanges.value || state.saving.value" @click="state.discard">放弃</el-button>
        <el-button type="primary" :icon="Save" :loading="state.saving.value" :disabled="!state.hasChanges.value" @click="save">统一保存</el-button>
      </div>
    </footer>
  </section>
</template>

<style scoped>
.targets-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.panel-header,
.defaults-row,
.defaults-row label,
.resource-line,
.save-bar {
  display: flex;
  align-items: center;
}

.panel-header,
.save-bar {
  justify-content: space-between;
  gap: 16px;
}

.panel-header h3 {
  margin: 0;
  color: var(--ds-ink);
  font-size: 16px;
  letter-spacing: 0;
}

.filters {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 160px auto;
  gap: 10px;
}

.defaults-row {
  min-height: 42px;
  flex-wrap: wrap;
  gap: 18px;
  padding: 8px 10px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-sm);
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 12px;
}

.defaults-row label {
  gap: 7px;
}

.defaults-row :deep(.el-input-number) {
  width: 90px;
}

.count {
  margin-left: auto;
}

.resource-line {
  min-width: 0;
  gap: 8px;
}

.resource-line strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.protocol-mark {
  display: inline-flex;
  height: 20px;
  align-items: center;
  justify-content: center;
  padding: 0 6px;
  border-radius: var(--ds-radius-xs);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
  font-size: 10px;
  font-weight: 700;
}

.multiplier,
.muted {
  color: var(--ds-muted);
  font-size: 12px;
}

.save-bar {
  position: sticky;
  bottom: 0;
  min-height: 52px;
  padding: 8px 12px;
  border-top: 1px solid var(--ds-line);
  background: var(--ds-panel);
  color: var(--ds-muted);
  font-size: 12px;
}

@media (max-width: 900px) {
  .filters {
    grid-template-columns: 1fr 1fr;
  }
}

@media (max-width: 640px) {
  .filters {
    grid-template-columns: 1fr;
  }

  .count {
    width: 100%;
    margin-left: 0;
  }

  .save-bar {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
