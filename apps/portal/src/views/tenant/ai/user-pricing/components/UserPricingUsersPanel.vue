<!--
  终端用户选择面板 — 用户管理/用户限流共用的左侧用户列表(搜索 + 单选)。
  重构:空态 → DsEmpty;卡片结构、搜索输入与 props/emits 保持不变(表单控件仍为 element-plus)。
-->
<script setup lang="ts">
import { PortalContentCard } from "@dai/app-core";
import { DsEmpty } from "@dai/ui";

import type { TenantEndUserItem } from "../../../../types/tenant";

const keyword = defineModel<string>("keyword", { required: true });

defineProps<{
  users: TenantEndUserItem[];
  loading: boolean;
  selectedUserId: string;
}>()

defineEmits<{
  (e: "select-user", user: TenantEndUserItem): void;
}>()
</script>

<template>
  <PortalContentCard v-loading="loading" body-padding="none" class="user-card">
    <template #header>
      <span class="card-title">终端用户</span>
    </template>
    <template #actions>
      <el-input
        v-model="keyword"
        size="small"
        placeholder="搜索用户名/邮箱"
        clearable
        style="width: 160px"
      />
    </template>

    <div class="user-list">
      <button
        v-for="user in users"
        :key="user.userId"
        type="button"
        class="user-item"
        :class="{ 'is-active': selectedUserId === user.userId }"
        @click="$emit('select-user', user)"
      >
        <span class="user-name">{{ user.username }}</span>
        <span class="user-sub">{{ user.email || user.userId }}</span>
      </button>
      <DsEmpty v-if="!users.length" title="暂无终端用户" />
    </div>
  </PortalContentCard>
</template>

<style scoped>
.card-title {
  font-weight: 700;
  color: var(--ds-ink);
}

.user-list {
  max-height: 60vh;
  overflow-y: auto;
}

.user-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  padding: 10px 14px;
  border: none;
  border-bottom: 1px solid var(--ds-line);
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: background-color 140ms ease;
}

.user-item:hover {
  background: var(--ds-panel-muted);
}

.user-item.is-active {
  background: var(--ds-accent-soft);
}

.user-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ds-ink);
}

.user-sub {
  font-size: 11px;
  color: var(--ds-faint);
}
</style>
