<script setup lang="ts">
import { DsFilterBar, DsFilterField } from "@/shared/ui";
import { Search } from "lucide-vue-next";

import type { AnnouncementStatus } from "./types";

defineProps<{ loading: boolean }>();
const emit = defineEmits<{ search: []; reset: [] }>();
const keyword = defineModel<string>("keyword", { required: true });
const status = defineModel<"all" | AnnouncementStatus>("status", { required: true });
</script>

<template>
  <DsFilterBar>
    <DsFilterField label="关键词">
      <el-input
        v-model="keyword"
        clearable
        placeholder="搜索公告标题或内容"
        class="announcement-filters__search"
        @keyup.enter="emit('search')"
        @clear="emit('search')"
      >
        <template #prefix>
          <Search class="announcement-filters__search-icon" />
        </template>
      </el-input>
    </DsFilterField>
    <DsFilterField label="状态">
      <el-select v-model="status" aria-label="公告状态" class="announcement-filters__status" @change="emit('search')">
        <el-option label="全部状态" value="all" />
        <el-option label="草稿" value="draft" />
        <el-option label="已发布" value="published" />
        <el-option label="已归档" value="archived" />
      </el-select>
    </DsFilterField>

    <template #actions>
      <el-button type="primary" :loading="loading" @click="emit('search')">查询</el-button>
      <el-button @click="emit('reset')">重置</el-button>
    </template>
  </DsFilterBar>
</template>

<style scoped>
.announcement-filters__search {
  width: min(260px, 100%);
}

.announcement-filters__status {
  width: 160px;
}

.announcement-filters__search-icon {
  width: 16px;
  height: 16px;
  color: var(--ds-faint);
}
</style>
