<script setup lang="ts">
import { DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import type { ManagedAnnouncement } from "./types";

defineProps<{
  items: readonly ManagedAnnouncement[];
  loading: boolean;
  busyId: string;
}>();

const emit = defineEmits<{
  edit: [item: ManagedAnnouncement];
  publish: [item: ManagedAnnouncement];
  archive: [item: ManagedAnnouncement];
  delete: [item: ManagedAnnouncement];
  stats: [item: ManagedAnnouncement];
}>();

const columns: DsTableColumn[] = [
  { key: "title", title: "公告", width: "26%" },
  { key: "status", title: "状态", width: 94, align: "center" },
  { key: "displayMode", title: "展示", width: 92, align: "center" },
  { key: "audience", title: "通知范围" },
  { key: "publishedAt", title: "发布时间", width: 170 },
  { key: "actions", title: "操作", width: 210, align: "right" }
];

const statusLabels = { draft: "草稿", published: "已发布", archived: "已归档" } as const;
const statusTones = { draft: "info", published: "positive", archived: "neutral" } as const;
const categoryLabels = {
  general: "系统公告",
  maintenance: "维护通知",
  upgrade: "升级通知",
  pricing: "费率变更",
  security: "安全通知"
} as const;

function formatTime(value?: number) {
  return value ? new Date(value).toLocaleString("zh-CN") : "-";
}

function audienceLabel(item: ManagedAnnouncement) {
  if (item.publisherType === "tenant") return "本租户终端用户";
  const rules = item.audiences ?? [];
  if (rules.filter((rule) => rule.scope === "all").length === 3) return "全体用户";
  const labels = new Set<string>();
  for (const rule of rules) {
    const kind = rule.kind === "admin" ? "管理员" : rule.kind === "tenant_user" ? "租户用户" : "终端用户";
    labels.add(rule.scope === "all" ? `全部${kind}` : `指定租户${kind}`);
  }
  return [...labels].join("、") || "-";
}
</script>

<template>
  <DsTable
    :frame="false"
    :columns="columns"
    :rows="items as ManagedAnnouncement[]"
    row-key="announcementId"
    :loading="loading"
    empty-title="暂无公告"
  >
    <template #cell-title="{ row }">
      <div class="announcement-title-cell">
        <strong>{{ row.title }}</strong>
        <span>{{ categoryLabels[row.category as keyof typeof categoryLabels] }}</span>
      </div>
    </template>
    <template #cell-status="{ row }">
      <DsTag :tone="statusTones[row.status as keyof typeof statusTones]">
        {{ statusLabels[row.status as keyof typeof statusLabels] }}
      </DsTag>
    </template>
    <template #cell-displayMode="{ row }">
      {{ row.displayMode === "popup" ? "强提醒" : "公告中心" }}
    </template>
    <template #cell-audience="{ row }">
      {{ audienceLabel(row as ManagedAnnouncement) }}
    </template>
    <template #cell-publishedAt="{ row }">
      <span class="announcement-time">{{ formatTime(row.publishedAt) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <template v-if="row.status === 'draft'">
        <el-button link type="primary" @click="emit('edit', row as ManagedAnnouncement)">编辑</el-button>
        <el-button
          link
          type="primary"
          :loading="busyId === row.announcementId"
          @click="emit('publish', row as ManagedAnnouncement)"
        >
          发布
        </el-button>
        <el-button
          link
          type="danger"
          :loading="busyId === row.announcementId"
          @click="emit('delete', row as ManagedAnnouncement)"
        >
          删除
        </el-button>
      </template>
      <template v-else-if="row.status === 'published'">
        <el-button link type="primary" @click="emit('stats', row as ManagedAnnouncement)">统计</el-button>
        <el-button
          link
          type="warning"
          :loading="busyId === row.announcementId"
          @click="emit('archive', row as ManagedAnnouncement)"
        >
          归档
        </el-button>
        <el-button
          link
          type="danger"
          :loading="busyId === row.announcementId"
          @click="emit('delete', row as ManagedAnnouncement)"
        >
          删除
        </el-button>
      </template>
      <template v-else>
        <el-button link type="primary" @click="emit('stats', row as ManagedAnnouncement)">统计</el-button>
        <el-button
          link
          type="danger"
          :loading="busyId === row.announcementId"
          @click="emit('delete', row as ManagedAnnouncement)"
        >
          删除
        </el-button>
      </template>
    </template>
  </DsTable>
</template>

<style scoped>
.announcement-title-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
}

.announcement-title-cell strong {
  color: var(--ds-ink);
  overflow-wrap: anywhere;
}

.announcement-title-cell span {
  color: var(--ds-muted);
  font-size: 12px;
}

.announcement-time {
  font-size: 12px;
  color: var(--ds-faint);
}
</style>
