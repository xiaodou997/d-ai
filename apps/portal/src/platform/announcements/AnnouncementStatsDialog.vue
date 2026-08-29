<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Search } from "lucide-vue-next";

import type {
  AnnouncementManagementClient,
  AnnouncementRecipient,
  AnnouncementStats,
  ManagedAnnouncement
} from "./types";

const props = defineProps<{
  visible: boolean;
  item: ManagedAnnouncement | null;
  client: AnnouncementManagementClient;
}>();
const emit = defineEmits<{ close: [] }>();

const loading = ref(false);
const recipientsLoading = ref(false);
const stats = ref<AnnouncementStats | null>(null);
const recipients = ref<AnnouncementRecipient[]>([]);
const recipientTotal = ref(0);
const recipientPage = ref(1);
const recipientSize = 10;
const recipientSearch = ref("");
const loadError = ref("");

const readRate = computed(() => {
  const total = stats.value?.currentAudienceSize ?? 0;
  return total > 0 ? Math.round(((stats.value?.readCount ?? 0) / total) * 100) : 0;
});

watch(
  () => [props.visible, props.item?.announcementId] as const,
  ([visible]) => {
    if (!visible || !props.item) return;
    recipientPage.value = 1;
    recipientSearch.value = "";
    void loadStats();
    void loadRecipients();
  }
);

async function loadStats() {
  if (!props.item) return;
  loading.value = true;
  loadError.value = "";
  try {
    stats.value = await props.client.stats(props.item.announcementId);
  } catch (error: unknown) {
    loadError.value = error instanceof Error ? error.message : "加载统计失败";
  } finally {
    loading.value = false;
  }
}

async function loadRecipients() {
  if (!props.item) return;
  recipientsLoading.value = true;
  try {
    const page = await props.client.recipients(props.item.announcementId, {
      search: recipientSearch.value.trim() || undefined,
      page: recipientPage.value,
      size: recipientSize
    });
    recipients.value = page.items;
    recipientTotal.value = page.total;
  } catch (error: unknown) {
    loadError.value = error instanceof Error ? error.message : "加载收件人失败";
  } finally {
    recipientsLoading.value = false;
  }
}

function searchRecipients() {
  recipientPage.value = 1;
  void loadRecipients();
}

function formatTime(value?: number) {
  return value ? new Date(value).toLocaleString("zh-CN") : "未读";
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="item ? `送达统计：${item.title}` : '送达统计'"
    width="min(860px, 96vw)"
    append-to-body
    @close="emit('close')"
  >
    <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" />
    <div v-loading="loading" class="announcement-stats">
      <div class="announcement-stats__metrics">
        <div><span>发布时受众</span><strong>{{ stats?.audienceSizeAtPublish ?? 0 }}</strong></div>
        <div><span>当前受众</span><strong>{{ stats?.currentAudienceSize ?? 0 }}</strong></div>
        <div><span>已读人数</span><strong>{{ stats?.readCount ?? 0 }}</strong></div>
        <div><span>已读率</span><strong>{{ readRate }}%</strong></div>
      </div>

      <div class="announcement-stats__toolbar">
        <el-input
          v-model="recipientSearch"
          clearable
          :prefix-icon="Search"
          placeholder="搜索用户名或邮箱"
          @keyup.enter="searchRecipients"
          @clear="searchRecipients"
        />
        <el-button @click="searchRecipients">查询</el-button>
      </div>

      <el-table v-loading="recipientsLoading" :data="recipients" row-key="userId" max-height="360">
        <el-table-column prop="username" label="用户" min-width="150" />
        <el-table-column prop="email" label="邮箱" min-width="190">
          <template #default="scope">{{ scope.row.email || "-" }}</template>
        </el-table-column>
        <el-table-column label="用户类型" width="110">
          <template #default="scope">
            {{ scope.row.userType <= 2 ? "管理员" : scope.row.userType === 3 ? "租户用户" : "终端用户" }}
          </template>
        </el-table-column>
        <el-table-column label="阅读状态" min-width="170">
          <template #default="scope">
            <el-tag :type="scope.row.readAt ? 'success' : 'info'" effect="plain">
              {{ formatTime(scope.row.readAt) }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="recipientTotal > recipientSize"
        v-model:current-page="recipientPage"
        :page-size="recipientSize"
        :total="recipientTotal"
        layout="prev, pager, next"
        class="announcement-stats__pagination"
        @current-change="loadRecipients"
      />
    </div>
  </el-dialog>
</template>

<style scoped>
.announcement-stats {
  display: flex;
  min-height: 260px;
  flex-direction: column;
  gap: 16px;
}

.announcement-stats__metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-sm);
}

.announcement-stats__metrics > div {
  display: flex;
  min-height: 76px;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
  border-right: 1px solid var(--ds-line);
  padding: 12px 16px;
}

.announcement-stats__metrics > div:last-child {
  border-right: 0;
}

.announcement-stats__metrics span {
  color: var(--ds-muted);
  font-size: 12px;
}

.announcement-stats__metrics strong {
  color: var(--ds-ink);
  font-size: 22px;
  font-variant-numeric: tabular-nums;
}

.announcement-stats__toolbar {
  display: grid;
  grid-template-columns: minmax(220px, 360px) auto;
  gap: 10px;
}

.announcement-stats__pagination {
  align-self: flex-end;
}

@media (max-width: 600px) {
  .announcement-stats__metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .announcement-stats__metrics > div:nth-child(2) {
    border-right: 0;
  }
}
</style>
