<script setup lang="ts">
import { PortalContentCard } from "@/platform";
import { formatMultiplier } from "@/platform/ai/utils";

import type { TenantAiVisibleGroup } from "@/api/types/aiTenant";

defineProps<{
  group: TenantAiVisibleGroup;
  priceBookName: string;
  busy?: boolean;
}>();

defineEmits<{
  edit: [];
  toggle: [];
  remove: [];
}>();
</script>

<template>
  <PortalContentCard class="summary-card">
    <template #header>
      <div class="summary-heading">
        <div class="title-line">
          <h2>{{ group.name }}</h2>
          <el-tag :type="group.status === 'active' ? 'success' : 'info'" effect="plain">
            {{ group.status === "active" ? "启用" : "停用" }}
          </el-tag>
        </div>
        <p>{{ group.description || "未填写描述" }}</p>
      </div>
    </template>
    <template #actions>
      <el-button type="warning" :disabled="busy" @click="$emit('toggle')">{{ group.status === "active" ? "停用" : "启用" }}</el-button>
      <el-button type="primary" :disabled="busy" @click="$emit('edit')">编辑</el-button>
      <el-button type="danger" :disabled="busy" @click="$emit('remove')">删除</el-button>
    </template>

    <dl class="summary-grid">
      <div><dt>零售价格表</dt><dd>{{ priceBookName }}</dd></div>
      <div><dt>默认用户倍率</dt><dd>×{{ formatMultiplier(group.default_user_multiplier) }}</dd></div>
      <div><dt>专属分组</dt><dd>{{ group.user_default_visible ? "公开，所有用户可见" : "专属，仅指定用户可见" }}</dd></div>
      <div><dt>协议转换</dt><dd>{{ group.allow_protocol_conversion ? "允许" : "仅同协议" }}</dd></div>
      <div><dt>排序</dt><dd>{{ group.sort_order }}</dd></div>
    </dl>
  </PortalContentCard>
</template>

<style scoped>
.summary-heading,
.title-line {
  min-width: 0;
}

.title-line {
  display: flex;
  align-items: center;
  gap: 10px;
}

.title-line h2 {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--ds-ink);
  font-size: 18px;
  letter-spacing: 0;
}

.summary-heading p {
  margin: 5px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 1px;
  margin: 0;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-sm);
  background: var(--ds-line);
}

.summary-grid div {
  min-width: 0;
  padding: 12px;
  background: var(--ds-panel);
}

.summary-grid dt {
  color: var(--ds-muted);
  font-size: 11px;
}

.summary-grid dd {
  margin: 5px 0 0;
  overflow-wrap: anywhere;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 650;
}

@media (max-width: 900px) {
  .summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
