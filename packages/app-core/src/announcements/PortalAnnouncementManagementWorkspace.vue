<script setup lang="ts">
import { onMounted, ref, type Component } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { DsPagination } from "@dai/ui";

import PortalPagePanel, { type PortalPagePanelBreadcrumb } from "../page/PortalPagePanel.vue";
import AnnouncementEditorDialog from "./AnnouncementEditorDialog.vue";
import AnnouncementManagementFilters from "./AnnouncementManagementFilters.vue";
import AnnouncementManagementTable from "./AnnouncementManagementTable.vue";
import AnnouncementStatsDialog from "./AnnouncementStatsDialog.vue";
import type {
  AnnouncementDraftPayload,
  AnnouncementManagementClient,
  AnnouncementStatus,
  AnnouncementTenantLoader,
  ManagedAnnouncement
} from "./types";

const props = defineProps<{
  mode: "platform" | "tenant";
  client: AnnouncementManagementClient;
  loadTenants?: AnnouncementTenantLoader;
  /** 页头身份图标(lucide 组件),由消费端按自身主题传入 */
  icon?: Component;
  /** 面包屑路径,末级即页面标题;管理端/租户端各自传入 */
  breadcrumbs: PortalPagePanelBreadcrumb[];
  /** 页面描述,以 "·" 尾随面包屑同行 */
  description?: string;
}>();

defineSlots<{ actions?(): unknown }>();

const items = ref<ManagedAnnouncement[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const keyword = ref("");
const status = ref<"all" | AnnouncementStatus>("all");
const loading = ref(false);
const saving = ref(false);
const busyId = ref("");
const editorVisible = ref(false);
const editingItem = ref<ManagedAnnouncement | null>(null);
const statsItem = ref<ManagedAnnouncement | null>(null);
const loadError = ref("");

onMounted(loadAnnouncements);

async function loadAnnouncements() {
  loading.value = true;
  loadError.value = "";
  try {
    const result = await props.client.list({
      status: status.value === "all" ? undefined : status.value,
      search: keyword.value.trim() || undefined,
      page: page.value,
      size: size.value
    });
    items.value = result.items;
    total.value = result.total;
  } catch (error: unknown) {
    loadError.value = errorMessage(error, "加载公告列表失败");
  } finally {
    loading.value = false;
  }
}

function search() {
  page.value = 1;
  void loadAnnouncements();
}

function resetFilters() {
  keyword.value = "";
  status.value = "all";
  search();
}

function handlePageChange(nextPage: number) {
  page.value = nextPage;
  void loadAnnouncements();
}

function handlePageSizeChange(nextSize: number) {
  size.value = nextSize;
  page.value = 1;
  void loadAnnouncements();
}

function createAnnouncement() {
  editingItem.value = null;
  editorVisible.value = true;
}

function editAnnouncement(item: ManagedAnnouncement) {
  editingItem.value = item;
  editorVisible.value = true;
}

async function saveDraft(payload: AnnouncementDraftPayload) {
  saving.value = true;
  try {
    if (editingItem.value) {
      await props.client.update(editingItem.value.announcementId, payload);
      ElMessage.success("公告草稿已更新");
    } else {
      await props.client.create(payload);
      ElMessage.success("公告草稿已创建");
    }
    editorVisible.value = false;
    await loadAnnouncements();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "保存公告草稿失败"));
  } finally {
    saving.value = false;
  }
}

async function publish(item: ManagedAnnouncement) {
  try {
    await ElMessageBox.confirm(
      "公告发布后内容和受众不可修改。确认发布？",
      "发布公告",
      { type: "warning", confirmButtonText: "确认发布", cancelButtonText: "取消" }
    );
  } catch {
    return;
  }
  busyId.value = item.announcementId;
  try {
    await props.client.publish(item.announcementId);
    ElMessage.success("公告已发布");
    await loadAnnouncements();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "发布公告失败"));
  } finally {
    busyId.value = "";
  }
}

async function archive(item: ManagedAnnouncement) {
  try {
    await ElMessageBox.confirm(
      "归档后公告将立即从收件人的公告中心和强提醒中下线。确认归档？",
      "归档公告",
      { type: "warning", confirmButtonText: "确认归档", cancelButtonText: "取消" }
    );
  } catch {
    return;
  }
  busyId.value = item.announcementId;
  try {
    await props.client.archive(item.announcementId);
    ElMessage.success("公告已归档");
    await loadAnnouncements();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "归档公告失败"));
  } finally {
    busyId.value = "";
  }
}

async function deleteDraft(item: ManagedAnnouncement) {
  try {
    await ElMessageBox.confirm("删除后无法恢复。确认删除该草稿？", "删除草稿", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消"
    });
  } catch {
    return;
  }
  try {
    await props.client.deleteDraft(item.announcementId);
    ElMessage.success("公告草稿已删除");
    await loadAnnouncements();
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "删除公告草稿失败"));
  }
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

// 页头（PortalPagePanel #actions）由消费方渲染，通过该暴露方法触发"新建公告"
defineExpose({ openCreate: createAnnouncement });
</script>

<template>
  <section class="announcement-workspace">
    <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" />

    <PortalPagePanel :icon="icon" :breadcrumbs="breadcrumbs" :description="description">
      <template v-if="$slots.actions" #actions>
        <slot name="actions" />
      </template>

      <template #filters>
        <AnnouncementManagementFilters
          v-model:keyword="keyword"
          v-model:status="status"
          :loading="loading"
          @search="search"
          @reset="resetFilters"
        />
      </template>

      <AnnouncementManagementTable
        :items="items"
        :loading="loading"
        :busy-id="busyId"
        @edit="editAnnouncement"
        @publish="publish"
        @archive="archive"
        @delete="deleteDraft"
        @stats="statsItem = $event"
      />

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="size"
          :total="total"
          :page-sizes="[20, 50, 100]"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </PortalPagePanel>

    <AnnouncementEditorDialog
      :visible="editorVisible"
      :mode="mode"
      :item="editingItem"
      :saving="saving"
      :load-tenants="loadTenants"
      @close="editorVisible = false"
      @save="saveDraft"
    />
    <AnnouncementStatsDialog
      :visible="statsItem !== null"
      :item="statsItem"
      :client="client"
      @close="statsItem = null"
    />
  </section>
</template>

<style scoped>
.announcement-workspace {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 14px;
}
</style>
