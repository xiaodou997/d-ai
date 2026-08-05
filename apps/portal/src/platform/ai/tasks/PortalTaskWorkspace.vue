<!--
  任务中心工作区(用户端「我的任务」/租户端「任务中心」共用)。
  重构:迁移至 DsUI 一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       面包屑由 eyebrow 拆分 + 页面标题组成,条数徽章 DsTag 放入 #actions,
       筛选条 DsFilterBar 体系,列表 DsTable,分页脚 DsPagination「共 N 条」;
       任务为 load-more 加载、无总条数,分页只读展示已加载条数)。
       详情抽屉内 el-tag 换 DsTag,el-descriptions/el-alert 仍为 element-plus(过渡期)。
-->
<script setup lang="ts">
import { computed, reactive, shallowRef } from "vue";
import { ListChecks } from "lucide-vue-next";
import { DsPagination, DsTag } from "@/shared/ui";

import PortalPagePanel from "../../page/PortalPagePanel.vue";

import {
  formatPortalTaskCredits,
  formatPortalTaskDuration,
  formatPortalTaskTime,
  portalTaskSourceLabel,
  portalTaskStatusLabel,
  portalTaskStatusTone,
  portalTaskTypeLabel
} from "./formatters";
import PortalTaskFilters from "./PortalTaskFilters.vue";
import PortalTaskTable from "./PortalTaskTable.vue";
import type {
  PortalTaskApi,
  PortalTaskOwnerScope,
  PortalTaskPortalMode,
  PortalTaskQuery,
  PortalTaskRecord,
  PortalTaskStatus,
  PortalTaskType
} from "./types";
import { usePortalTasks } from "./usePortalTasks";

const props = withDefaults(
  defineProps<{
    api: PortalTaskApi;
    mode: PortalTaskPortalMode;
    pollIntervalMs?: number;
    /** 菜单分组路径(按 "/" 拆分作面包屑父级),不同端菜单分组名不同 */
    eyebrow?: string;
    notifySuccess?: (message: string) => void;
    notifyError?: (message: string) => void;
    confirmDelete?: (task: PortalTaskRecord) => boolean | Promise<boolean>;
  }>(),
  { pollIntervalMs: 20_000, eyebrow: "智能服务 / 工作台" }
);

const filters = reactive<{
  ownerScope: "" | PortalTaskOwnerScope;
  userID: string;
  status: "" | PortalTaskStatus;
  taskType: "" | PortalTaskType;
}>({ ownerScope: "", userID: "", status: "", taskType: "" });

const selectedTask = shallowRef<PortalTaskRecord>();
const drawerOpen = shallowRef(false);
const detailLoading = shallowRef(false);

const {
  tasks,
  loading,
  loadingMore,
  hasMore,
  activeCount,
  operationTaskID,
  refresh,
  loadMore,
  getTask,
  cancelTask,
  deleteTask
} = usePortalTasks({
  api: props.api,
  pollIntervalMs: props.pollIntervalMs,
  notifyError: props.notifyError
});

const title = computed(() => (props.mode === "tenant" ? "任务中心" : "我的任务"));

// 面包屑 = eyebrow(按 "/" 拆分的菜单分组) + 页面标题(末级)
const breadcrumbs = computed(() => [
  ...props.eyebrow.split("/").map((item) => ({ label: item.trim() })),
  { label: title.value }
]);

// 任务接口为 load-more 增量加载、无总条数:分页只读展示「共 N 条(已加载)」
const loadedPageSize = computed(() => Math.max(20, tasks.value.length));

function currentQuery(): PortalTaskQuery {
  return {
    owner_scope: props.mode === "tenant" ? filters.ownerScope : undefined,
    user_id: props.mode === "tenant" && filters.ownerScope === "user" ? filters.userID.trim() : undefined,
    status: filters.status,
    type: filters.taskType,
    limit: 20
  };
}

async function openDetail(task: PortalTaskRecord): Promise<void> {
  drawerOpen.value = true;
  selectedTask.value = task;
  detailLoading.value = true;
  try {
    selectedTask.value = await getTask(task.id);
  } catch (error) {
    props.notifyError?.((error as Error).message || "任务详情加载失败");
  } finally {
    detailLoading.value = false;
  }
}

async function handleCancel(task: PortalTaskRecord): Promise<void> {
  if (!task.permissions.can_cancel) return;
  const updated = await cancelTask(task);
  if (!updated) return;
  if (selectedTask.value?.id === updated.id) selectedTask.value = updated;
  props.notifySuccess?.("任务已取消");
}

async function handleDelete(task: PortalTaskRecord): Promise<void> {
  if (!task.permissions.can_delete) return;
  const confirmed = props.confirmDelete
    ? await props.confirmDelete(task)
    : window.confirm("确定删除该任务记录吗？此操作不可恢复。");
  if (!confirmed || !(await deleteTask(task))) return;
  if (selectedTask.value?.id === task.id) drawerOpen.value = false;
  props.notifySuccess?.("任务已删除");
}

function formattedResult(task?: PortalTaskRecord): string {
  if (!task?.result) return "";
  try {
    return JSON.stringify(task.result, null, 2);
  } catch {
    return String(task.result);
  }
}
</script>

<template>
  <div class="task-workspace">
    <PortalPagePanel :icon="ListChecks" :breadcrumbs="breadcrumbs" fill>
      <template #actions>
        <DsTag tone="neutral">{{ tasks.length }} 条</DsTag>
        <DsTag v-if="activeCount" tone="accent">{{ activeCount }} 个进行中</DsTag>
      </template>

      <template #filters>
        <PortalTaskFilters
          v-model:owner-scope="filters.ownerScope"
          v-model:user-id="filters.userID"
          v-model:status="filters.status"
          v-model:task-type="filters.taskType"
          :mode="props.mode"
          :loading="loading"
          @search="refresh(currentQuery())"
          @refresh="refresh(currentQuery())"
        />
      </template>

      <PortalTaskTable
        :tasks="tasks"
        :mode="props.mode"
        :loading="loading"
        :operation-task-id="operationTaskID"
        @view="openDetail"
        @cancel="handleCancel"
        @delete="handleDelete"
      />

      <template #pagination>
        <div class="task-workspace__pagination">
          <DsPagination :page="1" :page-size="loadedPageSize" :page-sizes="[loadedPageSize]" :total="tasks.length" />
          <el-button v-if="hasMore" :loading="loadingMore" @click="loadMore">加载更多</el-button>
        </div>
      </template>
    </PortalPagePanel>

    <el-drawer v-model="drawerOpen" title="任务详情" size="min(560px, 92vw)" append-to-body destroy-on-close>
      <div v-loading="detailLoading" class="task-detail">
        <template v-if="selectedTask">
          <div class="task-detail__status">
            <DsTag :tone="portalTaskStatusTone(selectedTask.status)">
              {{ portalTaskStatusLabel(selectedTask.status) }}
            </DsTag>
            <DsTag v-if="selectedTask.permissions.read_only" tone="neutral">用户任务 · 只读</DsTag>
          </div>

          <el-descriptions :column="1" border>
            <el-descriptions-item label="任务编号">{{ selectedTask.id }}</el-descriptions-item>
            <el-descriptions-item v-if="props.mode === 'tenant'" label="归属">
              {{ selectedTask.owner.scope === "user" ? `用户 ${selectedTask.owner.user_id || '-'}` : "租户" }}
            </el-descriptions-item>
            <el-descriptions-item label="任务类型">{{ portalTaskTypeLabel(selectedTask.type) }}</el-descriptions-item>
            <el-descriptions-item label="调用来源">{{ portalTaskSourceLabel(selectedTask.source) }}</el-descriptions-item>
            <el-descriptions-item label="模型">{{ selectedTask.model || "-" }}</el-descriptions-item>
            <el-descriptions-item label="请求编号">{{ selectedTask.request_id || "-" }}</el-descriptions-item>
            <el-descriptions-item label="执行次数">{{ selectedTask.attempt }}</el-descriptions-item>
            <el-descriptions-item label="提交时间">{{ formatPortalTaskTime(selectedTask.created_at) }}</el-descriptions-item>
            <el-descriptions-item label="开始时间">{{ formatPortalTaskTime(selectedTask.started_at) }}</el-descriptions-item>
            <el-descriptions-item label="完成时间">{{ formatPortalTaskTime(selectedTask.completed_at) }}</el-descriptions-item>
            <el-descriptions-item label="执行耗时">{{ formatPortalTaskDuration(selectedTask) }}</el-descriptions-item>
            <el-descriptions-item label="消耗积分">{{ formatPortalTaskCredits(selectedTask.usage?.cost_credits) }}</el-descriptions-item>
          </el-descriptions>

          <el-alert
            v-if="selectedTask.error"
            class="task-detail__section"
            type="error"
            :title="selectedTask.error.code || '任务失败'"
            :description="selectedTask.error.message"
            :closable="false"
            show-icon
          />

          <section v-if="selectedTask.result_available" class="task-detail__section">
            <h3 class="task-detail__heading">结果</h3>
            <div v-if="selectedTask.result_summary" class="task-detail__summary">
              <DsTag v-if="selectedTask.result_summary.image_count" tone="positive">
                {{ selectedTask.result_summary.image_count }} 张图片
              </DsTag>
              <DsTag v-if="selectedTask.result_summary.choice_count" tone="positive">
                {{ selectedTask.result_summary.choice_count }} 个回复
              </DsTag>
            </div>
            <pre v-if="selectedTask.result" class="task-detail__result">{{ formattedResult(selectedTask) }}</pre>
            <el-alert
              v-else-if="selectedTask.permissions.read_only"
              type="info"
              title="用户任务内容已脱敏"
              :closable="false"
            />
          </section>
        </template>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.task-workspace {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}

/* 分页脚:左侧「共 N 条」,右侧「加载更多」(load-more 增量加载,无总条数) */
.task-workspace__pagination {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.task-detail__status,
.task-detail__summary {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.task-detail {
  min-height: 180px;
}

.task-detail__status {
  margin-bottom: 16px;
}

.task-detail__section {
  margin-top: 18px;
}

.task-detail__heading {
  margin: 0 0 10px;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 650;
}

.task-detail__result {
  max-height: 360px;
  margin: 12px 0 0;
  padding: 12px;
  overflow: auto;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
