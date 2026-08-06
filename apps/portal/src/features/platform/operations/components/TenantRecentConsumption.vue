<!--
  工作台右下「近期用户消费」面板。
  重构:el-table → DsTable(:frame="false" 嵌入面板,loading 骨架由 DsTable 自带),
       颜色全部改用 var(--ds-*) token;数据与 props 不变。
-->
<script setup lang="ts">
import { computed } from "vue";
import { ReceiptText } from "lucide-vue-next";
import { DsTable, type DsTableColumn } from "@/shared/ui";

import type { AccountTransactionItem } from "@/api/types/platformTenant";

const props = defineProps<{
  items: readonly AccountTransactionItem[];
  rangeLabel: string;
  loading: boolean;
}>();

const columns: DsTableColumn[] = [
  { key: "username", title: "用户" },
  { key: "userCredits", title: "用户消费积分", align: "right", width: 120 },
  { key: "appName", title: "消费场景" },
  { key: "createdTime", title: "时间", width: 110 }
];

// DsTable rows 类型为 any[],readonly props 在此做一次断言,不改数据本身
const tableRows = computed(() => props.items as AccountTransactionItem[]);

const emit = defineEmits<{
  openDetails: [];
}>();

const numberFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 });

function formatTime(timestamp?: number | null) {
  if (!timestamp) return "—";
  return new Date(timestamp).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}
</script>

<template>
  <section class="recent-panel" aria-labelledby="recent-consumption-title">
    <header class="recent-panel__head">
      <div class="recent-panel__heading">
        <span class="recent-panel__icon" aria-hidden="true"><ReceiptText :size="19" /></span>
        <div>
          <h2 id="recent-consumption-title" class="recent-panel__title">近期用户消费</h2>
          <p class="recent-panel__desc">{{ rangeLabel }}最近成功消费记录</p>
        </div>
      </div>
      <button class="recent-panel__action" type="button" @click="emit('openDetails')">查看全部</button>
    </header>

    <div class="recent-panel__table">
      <DsTable
        :frame="false"
        :columns="columns"
        :rows="tableRows"
        row-key="eventId"
        :loading="loading"
        empty-title="当前时间范围内暂无用户消费"
      >
        <template #cell-username="{ row }">
          <strong class="recent-panel__user">{{ row.username || "未知用户" }}</strong>
        </template>
        <template #cell-userCredits="{ row }">
          <strong class="recent-panel__credits">{{ numberFormatter.format(row.userCredits ?? 0) }}</strong>
        </template>
        <template #cell-appName="{ row }">{{ row.appName || row.description || "—" }}</template>
        <template #cell-createdTime="{ row }">{{ formatTime(row.createdTime) }}</template>
      </DsTable>
    </div>
  </section>
</template>

<style scoped>
.recent-panel {
  min-width: 0;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.recent-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--ds-line);
  padding: 18px 20px;
}

.recent-panel__heading {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 11px;
}

.recent-panel__icon {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border-radius: 8px;
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.recent-panel__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 750;
  letter-spacing: 0;
}

.recent-panel__desc {
  margin: 3px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
}

.recent-panel__action {
  border: 0;
  background: transparent;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
  cursor: pointer;
}

.recent-panel__table {
  min-height: 374px;
  overflow-x: auto;
  padding: 8px 12px 12px;
}

.recent-panel__user {
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.recent-panel__credits {
  color: var(--ds-positive);
  font-size: 12px;
  font-weight: 800;
}

@media (max-width: 640px) {
  .recent-panel__head {
    align-items: start;
    padding: 16px;
  }

  .recent-panel__table {
    padding-inline: 8px;
  }
}
</style>
