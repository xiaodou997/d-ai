<script setup lang="ts">
import { PortalContentCard } from "@/platform";
import { DsTag } from "@/shared/ui";

import type { TenantAiUpstreamResource } from "@/api/types/aiTenant";
import { formatMultiplier, protocolTagTone, resourceProtocolLabels } from "../presentation";

const props = defineProps<{
  accounts: TenantAiUpstreamResource[];
  selectedId: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  select: [resourceId: string];
}>();
</script>

<template>
  <PortalContentCard title="上游资源" body-padding="none" class="account-card">
    <div v-loading="props.loading" class="account-list">
      <button
        v-for="account in props.accounts"
        :key="account.id"
        type="button"
        class="account-option"
        :class="{ 'account-option--selected': props.selectedId === account.id }"
        @click="emit('select', account.id)"
      >
        <span class="account-option__name" :title="account.name">{{ account.name }}</span>
        <DsTag
          v-for="protocol in resourceProtocolLabels(account)"
          :key="protocol"
          :tone="protocolTagTone(protocol)"
          class="account-option__protocol"
        >
          {{ protocol }}
        </DsTag>
        <span class="account-option__multiplier">{{ formatMultiplier(account.tenant_multiplier) }}</span>
      </button>

      <div v-if="!props.loading && props.accounts.length === 0" class="account-list__empty">
        当前租户暂无可选上游资源
      </div>
    </div>
  </PortalContentCard>
</template>

<style scoped>
/* 卡片随 grid 行拉伸,内部 flex 列让列表吃掉剩余高度 */
.account-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.account-card :deep(.portal-content-card__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.account-list {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 120px;
  overflow-y: auto;
}

.account-option {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 48px;
  padding: 10px 14px;
  border: 0;
  border-bottom: 1px solid var(--ds-line);
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.account-option:last-child {
  border-bottom: 0;
}

.account-option:hover {
  background: var(--ds-panel-muted);
}

.account-option--selected {
  background: var(--ds-accent-soft);
  box-shadow: var(--ds-shadow-inset-accent-wide);
}

.account-option--selected:hover {
  background: var(--ds-accent-soft);
}

.account-option__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  color: var(--ds-ink);
  font-size: 13px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-option__protocol {
  flex: 0 0 auto;
  padding-inline: 7px;
  font-size: 10px;
  font-weight: 700;
}

.account-option__multiplier {
  flex: 0 0 auto;
  min-width: 47px;
  color: var(--ds-muted);
  font-family: var(--ds-font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  text-align: right;
}

.account-list__empty {
  padding: 24px 16px;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.6;
}

@media (max-width: 960px) {
  .account-list {
    max-height: 320px;
  }
}
</style>
