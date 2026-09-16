<script setup lang="ts">
import { computed } from "vue";
import { DsTable, DsTag } from "@/shared/ui";
import { microUSD, type RequestRecord } from "../recordsApi";
import { compactCount, compactMs, copyRecordText, multiplier, recordColumns, sourceLabel, type RecordRole } from "../recordPresentation";
const props = defineProps<{ role: RecordRole; errors: boolean; rows: RequestRecord[]; busy: boolean; fixedUser?: boolean; fill?: boolean }>();
defineEmits<{ detail: [row: RequestRecord] }>();
const columns = computed(() => recordColumns(props.role, props.errors, props.fixedUser));
const positive = (value?: number | null) => value != null && value > 0;
</script>
<template>
  <DsTable class="record-table" :class="{ 'record-table--fill': fill }" :style="{ '--record-table-width': `${columns.reduce((sum, col) => sum + Number(col.width || 150), 0)}px` }" :columns="columns" :rows="rows" row-key="request_id" :loading="busy" :frame="false" empty-title="当前范围没有记录" empty-description="请调整时间或筛选条件；历史执行记录可能已过保留期">
    <template #cell-time="{ row }"><strong>{{ new Date(row.created_at).toLocaleTimeString('zh-CN', { hour12: false }) }}</strong><small>{{ new Date(row.created_at).toLocaleDateString('zh-CN') }}</small></template>
    <template #cell-subject="{ row }"><button v-if="role === 'admin'" class="record-copy" :title="`复制租户 ID：${row.tenant_id}`" @click="copyRecordText(row.tenant_id)">{{ row.profile?.tenant_name || row.tenant_id }}</button><button v-if="row.user_id" class="record-copy" :title="`复制用户 ID：${row.user_id}`" @click="copyRecordText(row.user_id)">{{ row.profile?.user_name || row.user_id }}</button><small v-else>租户调用</small></template>
    <template #cell-model="{ row }">
      <strong class="record-model" :title="row.profile?.requested_model || row.model">{{ row.profile?.requested_model || row.model || '未命中模型' }}</strong>
      <small v-if="role === 'admin' && row.admin_context?.resolved_logical_model && row.admin_context.resolved_logical_model !== (row.profile?.requested_model || row.model)">{{ row.admin_context.resolved_logical_model }}</small>
      <span class="record-meta"><DsTag tone="info">{{ sourceLabel(row.source) }}</DsTag><span v-if="row.stream">流式</span><span v-if="row.profile?.reasoning_effort" class="record-effort">{{ row.profile.reasoning_effort }}</span></span>
    </template>
    <template #cell-profile="{ row }"><strong class="record-inline" :title="row.profile?.group_name_snapshot || row.profile?.group_id"><span>{{ row.profile?.group_name_snapshot || row.profile?.group_id || '未记录分组' }}</span><span v-if="row.profile?.group_default_user_multiplier_snapshot != null" class="record-multiplier">{{ multiplier(row.profile.group_default_user_multiplier_snapshot) }}</span></strong><small class="record-key" :title="row.profile?.api_key_name || row.profile?.api_key_id">{{ row.profile?.api_key_name || row.profile?.api_key_id || '非 Key 调用' }}</small></template>
    <template #cell-upstream="{ row }"><strong class="record-inline" :title="row.admin_context?.upstream_account_name || row.admin_context?.upstream_account_id"><span>{{ row.admin_context?.upstream_account_name || row.admin_context?.upstream_account_id || '未记录账号' }}</span><span v-if="(row.admin_context?.upstream_account_name || row.admin_context?.upstream_account_id) && row.admin_context?.tenant_multiplier != null" class="record-multiplier" title="租户倍率">{{ multiplier(row.admin_context.tenant_multiplier) }}</span></strong><small class="record-provider">{{ row.admin_context?.provider_code || '—' }}</small></template>
    <template #cell-tokens="{ row }"><template v-if="row.media_units != null"><strong>{{ row.media_units }} {{ row.media_unit_type }}</strong><small>{{ row.profile?.resolution }}</small></template><template v-else><div class="record-token-line"><span><span class="record-label">输入</span> {{ compactCount(row.tokens.input) }}</span><span class="record-token-output"><span class="record-label">输出</span> {{ compactCount(row.tokens.output) }}</span></div><small v-if="positive(row.tokens.cache_read) || positive(row.tokens.cache_write)" class="record-cache"><span v-if="positive(row.tokens.cache_read)">缓存读 {{ compactCount(row.tokens.cache_read) }}</span><span v-if="positive(row.tokens.cache_write)">缓存写 {{ compactCount(row.tokens.cache_write) }}</span></small></template></template>
    <template #cell-charge="{ row }"><strong v-if="role !== 'customer'" class="record-charge-tenant">租户 {{ microUSD(row.charge.tenant_charged_micro) }}</strong><strong class="record-charge-user">{{ role === 'customer' ? '实扣' : '用户' }} {{ microUSD(row.charge.user_charged_micro) }}</strong><small v-if="row.charge.source === 'subscription'">订阅额度 {{ microUSD(row.charge.subscription_used_micro) }}</small></template>
    <template #cell-timing="{ row }"><strong>首：{{ compactMs(row.first_token_ms) }}</strong><small>总：{{ compactMs(row.total_ms) }}</small></template>
    <template #cell-error="{ row }"><span class="record-error" :title="row.error?.message">{{ row.error?.message || '错误诊断已过保留期' }}</span><small>{{ row.error?.code || '—' }} · {{ row.error?.stage || '—' }}<span v-if="row.http_status != null"> · HTTP {{ row.http_status }}</span></small></template>
    <template #cell-actions="{ row }"><el-button link type="primary" @click="$emit('detail', row)">查看详情</el-button></template>
  </DsTable>
</template>
<style scoped>
.record-table { max-height: 540px; overflow: auto; scrollbar-width: thin; scrollbar-color: var(--ds-muted) var(--ds-panel-muted); scrollbar-gutter: stable; overscroll-behavior: contain; }
.record-table--fill { max-height: none; }
.record-table :deep(td) { vertical-align: top; }
.record-table :deep(table) { table-layout: fixed; width: max(100%, var(--record-table-width)); }
.record-table::-webkit-scrollbar { width: 10px; height: 10px; }
.record-table::-webkit-scrollbar-track { background: var(--ds-panel-muted); }
.record-table::-webkit-scrollbar-thumb { border: 2px solid var(--ds-panel-muted); border-radius: var(--ds-radius-pill); background: var(--ds-muted); }
.record-table strong, .record-table small { display: block; overflow: hidden; text-overflow: ellipsis; max-width: 240px; }
.record-table strong { font-weight: 550; font-size: 12px; }
.record-table small { margin-top: 4px; font-size: 11px; color: var(--ds-muted); }
.record-model { color: var(--ds-accent); font-family: var(--ds-font-mono); }
.record-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: 6px; color: var(--ds-muted); font-size: 11px; }
.record-meta :deep(.ds-tag) { padding: 1px 7px; font-size: 10.5px; }
.record-effort { color: var(--ds-info); font-family: var(--ds-font-mono); }
.record-table .record-multiplier { color: var(--ds-accent); }
.record-table .record-inline { display: flex; align-items: baseline; gap: 6px; }
.record-inline > span:first-child { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.record-inline .record-multiplier { flex: none; font-size: 11px; }
.record-key::before { content: "Key · "; color: var(--ds-muted); }
.record-table .record-provider { color: var(--ds-info); font-family: var(--ds-font-mono); }
.record-label { color: var(--ds-muted); font-weight: 450; }
.record-table .record-token-line, .record-table .record-cache { display: flex; align-items: center; gap: 8px; white-space: nowrap; }
.record-token-line { font-size: 12px; font-weight: 550; color: var(--ds-ink-soft); }
.record-token-output { color: var(--ds-accent); }
.record-charge-tenant { color: var(--ds-ink-soft); }
.record-charge-user { color: var(--ds-positive); }
.record-copy { display: block; max-width: 155px; overflow: hidden; text-overflow: ellipsis; background: none; border: none; padding: 0; margin: 3px 0; color: var(--ds-ink); font: inherit; font-size: 12px; cursor: pointer; }
.record-copy:hover { color: var(--ds-accent); }
.record-error { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; font-size: 12px; }
</style>
