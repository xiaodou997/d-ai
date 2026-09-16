<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ScrollText } from "lucide-vue-next";
import { useAuthStore } from "@/stores/auth";
import { PortalPagePanel, PortalMetricGrid } from "@/platform";
import { DsTabs } from "@/shared/ui";
import RecordRangeSelector from "./components/RecordRangeSelector.vue";
import RecordFilters from "./components/RecordFilters.vue";
import RecordTable from "./components/RecordTable.vue";
import { useRecordList } from "./composables/useRecordList";
import { recordRole, recordPath, recordTitle, timestamp } from "./recordPresentation";
import type { RequestRecord } from "./recordsApi";
const props = defineProps<{ userId?: string; embedded?: boolean }>();
const auth = useAuthStore(), route = useRoute(), router = useRouter();
const role = computed(() => recordRole(auth.userInfo?.userType));
const container = ref<HTMLElement | null>(null);
const stateKey = `usage-records:v2:${auth.userInfo?.sub}:${auth.userInfo?.tenantId}:${role.value}:${route.path}:${props.userId || ""}`;
const { filters, range, customRange, tab, pageSize, rows, busy, failure, metrics, applied, history, nextCursor, search, refresh, reset, changeTab, changePageSize, next, previous, save } = useRecordList(role, stateKey, props.userId, container);
const tabs = [{ key: "requests", label: "请求记录" }, { key: "errors", label: "错误请求" }];
function show(row: RequestRecord) {
  save();
  void router.push({ path: `${recordPath(role.value)}/${encodeURIComponent(row.request_id)}`, query: { returnTo: route.fullPath } });
}
</script>
<template>
  <div class="record-page-root">
  <component :is="embedded ? 'section' : PortalPagePanel" :fill="!embedded" :icon="ScrollText" :breadcrumbs="[{ label: '智能服务' }, { label: recordTitle(role) }]" description="按请求查看调用用量与实际扣款">
    <div ref="container" class="records-workspace" :class="{ 'records-workspace--embedded': embedded }">
      <RecordRangeSelector v-model="range" v-model:custom-range="customRange" />
      <PortalMetricGrid class="record-metrics" :class="{ 'record-metrics--customer': role === 'customer' }" :metrics="metrics" min-col-width="180px" />
      <p class="records-note">整体概览 · {{ timestamp(applied.from) }} 至 {{ timestamp(applied.to) }} · 包含正常与错误请求，切换 Tab 不改变统计口径。</p>
      <DsTabs :tabs="tabs" :model-value="tab" @update:model-value="changeTab" />
      <RecordFilters v-model="filters" :role="role" :fixed-user="Boolean(userId)" :busy="busy" @search="search" @reset="reset" @refresh="refresh" />
      <p v-if="failure" class="records-failure" role="alert">{{ failure }} <el-button link type="primary" @click="refresh">重试</el-button></p>
      <p v-if="tab === 'errors'" class="records-note">明确报错的请求，费用与计费额度均免收；诊断记录按现有保留策略查询。</p>
      <RecordTable :role="role" :errors="tab === 'errors'" :rows="rows" :busy="busy" :fixed-user="Boolean(userId)" @detail="show" />
      <div class="records-pager"><span>第 {{ history.length + 1 }} 页</span><el-select :model-value="pageSize" aria-label="每页条数" @update:model-value="changePageSize"><el-option v-for="size in [20, 50, 100]" :key="size" :value="size" :label="`${size} 条 / 页`" /></el-select><el-button :disabled="busy || !history.length" @click="previous">上一页</el-button><el-button :disabled="busy || !nextCursor" @click="next">下一页</el-button></div>
      <p class="records-note">统计基于可查询的结算记录。执行记录、错误诊断与财务记录保留期不同，历史统计总量可能大于当前可浏览记录数。</p>
    </div>
  </component>
  </div>
</template>
<style scoped>
.record-page-root { display: flex; flex: 1; flex-direction: column; min-width: 0; min-height: 0; }
.records-workspace { display: flex; flex-direction: column; gap: 18px; padding: 22px; min-width: 0; }
.records-workspace--embedded { padding: 0; }
.records-workspace .record-metrics { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.records-workspace .record-metrics--customer { grid-template-columns: repeat(4, minmax(0, 1fr)); }
@media (min-width: 1700px) { .records-workspace .record-metrics:not(.records-workspace .record-metrics--customer) { grid-template-columns: repeat(6, minmax(0, 1fr)); } }
@media (max-width: 900px) { .records-workspace .record-metrics, .records-workspace .record-metrics--customer { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 480px) { .records-workspace { padding: 14px; } .records-workspace .record-metrics, .records-workspace .record-metrics--customer { grid-template-columns: minmax(0, 1fr); } }
.records-note { margin: 0; font-size: 12px; line-height: 1.6; color: var(--ds-muted); }
.records-failure { color: var(--ds-danger); margin: 0; }
.records-pager { display: flex; flex-wrap: wrap; justify-content: flex-end; align-items: center; gap: 10px; font-size: 12px; color: var(--ds-muted); }
.records-pager :deep(.el-select) { width: 125px; }
</style>
