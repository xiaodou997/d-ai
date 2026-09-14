<script setup lang="ts">
import { computed, shallowRef } from 'vue';
import { ElMessage } from 'element-plus';
import { DsMetricCard, DsTable, DsTag, type DsTableColumn } from '@/shared/ui';
import { PortalContentCard } from '@/platform';
import { stabilityApi, type ResourceKind } from './api';
import { useUpstreamStability } from './useUpstreamStability';
const props = defineProps<{ kind: ResourceKind; resourceId: string }>();
const runtime = useUpstreamStability(props.kind, () => props.resourceId);
const detail = runtime.detail;
const resuming = shallowRef(false);
const availabilityLabels = { available: '全部可用', partial: '部分受限', unavailable: '全部受限', unknown: '尚无运行记录' };
const phaseLabels: Record<string, string> = { available: '可用', cooling: '冷却', recovering: '恢复试用' };
const outcomeLabels: Record<string, string> = { success: '成功', server_error: '上游服务错误', timeout: '超时', network_error: '网络异常', unauthorized: '认证失败', rate_limited: '限流', model_error: '模型或权限错误', client_error: '请求参数错误（不计失败）', canceled: '已取消（不计失败）', circuit_open: '冷却跳过（不计失败）', rejected: '未发送（不计失败）' };
const stateRows = computed(() => (detail.value?.states || []).map(state => ({ ...state, key: state.scope.key, model: state.scope.model || '所有模型', endpoint: state.scope.endpoint_id || state.scope.credential_id || '账号', reasonLabel: outcomeLabels[state.reason || ''] || state.reason || '—' })));
const modelRows = computed(() => (detail.value?.models || []).map((row, key) => ({ ...row, key, result: outcomeLabels[row.outcome] || row.outcome })));
const stateColumns: DsTableColumn[] = [{ key: 'model', title: '影响模型' }, { key: 'endpoint', title: '端点 / 凭据' }, { key: 'phase', title: '运行状态' }, { key: 'reasonLabel', title: '最近原因' }, { key: 'retry_at', title: '可重新调用时间' }, { key: 'verified_at', title: '最近验证成功' }];
const modelColumns: DsTableColumn[] = [{ key: 'model', title: '模型' }, { key: 'operation', title: '操作' }, { key: 'endpoint_id', title: '端点' }, { key: 'credential_id', title: '凭据' }, { key: 'stream', title: '流式' }, { key: 'result', title: '调用结果' }, { key: 'count', title: '次数' }, { key: 'p50_ms', title: 'P50 / 毫秒' }, { key: 'p95_ms', title: 'P95 / 毫秒' }, { key: 'last_error', title: '最近错误' }];
function time(value?: number) { return value ? new Date(value).toLocaleString() : '—'; }
async function resume() {
  resuming.value = true;
  try { await stabilityApi.resume(props.kind, props.resourceId); await runtime.refresh(); ElMessage.success('已恢复业务试用，未发送探测请求'); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : '恢复失败'); }
  finally { resuming.value = false; }
}
</script>
<template>
  <PortalContentCard title="运行状态与稳定性" description="按每一次真实上游调用统计；冷却结束后自动恢复业务试用。">
    <div class="stability-toolbar">
      <el-select v-model="runtime.window.value" aria-label="稳定性统计范围">
        <el-option label="最近 1 小时" value="1h" /><el-option label="最近 24 小时" value="24h" /><el-option label="最近 7 天" value="7d" />
      </el-select>
      <DsTag v-if="detail" :tone="detail.availability === 'unavailable' ? 'danger' : detail.availability === 'partial' ? 'warning' : 'neutral'">{{ availabilityLabels[detail.availability] }}</DsTag>
      <DsTag v-if="detail?.repeated_failure" tone="danger">最近一小时反复异常</DsTag>
      <el-button :disabled="!detail || detail.config_status === 'disabled' || Boolean(detail.state_error)" :loading="resuming" @click="resume">立即恢复试用</el-button>
    </div>
    <el-alert v-if="runtime.error.value || detail?.state_error" :title="runtime.error.value || detail?.state_error" type="warning" :closable="false" />
    <div v-if="detail" class="stability-metrics">
      <DsMetricCard label="调用成功率" :value="detail.success_rate == null ? '暂无数据' : `${detail.success_rate.toFixed(1)}%`" :hint="`${detail.samples} 次有效样本${detail.samples < 20 ? ' · 样本不足' : ''}`" />
      <DsMetricCard label="上游失败" :value="String(detail.failures)" :hint="`${detail.successes} 次成功`" />
      <DsMetricCard label="最近一小时冷却次数" :value="String(detail.cooldowns_last_hour)" :hint="`${detail.excluded} 次取消或本地跳过不计入失败率`" />
    </div>
    <p class="stability-note">统计覆盖始于 {{ time(detail?.coverage_started_at) }}。流式延迟统计首个有效输出，其他请求统计完成耗时。恢复试用并不代表已验证成功。</p>
    <DsTable :columns="stateColumns" :rows="stateRows" row-key="key" empty-title="暂无故障或恢复记录">
      <template #cell-phase="{ row }"><DsTag :tone="row.phase === 'cooling' ? 'danger' : row.phase === 'recovering' ? 'warning' : 'positive'">{{ phaseLabels[row.phase] }}</DsTag></template>
      <template #cell-verified_at="{ row }">{{ row.verified_at ? time(row.verified_at) : '尚未验证成功' }}</template>
      <template #cell-retry_at="{ row }">{{ row.phase === 'cooling' ? time(row.retry_at) : row.phase === 'recovering' ? (row.busy ? '试用请求进行中' : '已到期，可参与业务选路') : '—' }}</template>
    </DsTable>
    <DsTable :columns="modelColumns" :rows="modelRows" row-key="key" empty-title="当前时间范围暂无调用记录" />
  </PortalContentCard>
</template>
<style scoped>
.stability-toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 16px; }
.stability-toolbar :deep(.el-select) { width: 160px; }
.stability-metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; margin: 16px 0; }
.stability-note { color: var(--ds-muted); font-size: 12px; margin: 16px 0; }
</style>
