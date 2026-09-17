<script setup lang="ts">
import { formatDuration } from "@/platform/ai/usage";
import { computed, shallowRef, watch } from 'vue';
import { ElMessage } from 'element-plus';
import { DsMetricCard, DsTable, DsTag, type DsTableColumn } from '@/shared/ui';
import { PortalContentCard } from '@/platform';
import { stabilityApi, type ResourceKind, type Stability, type StabilityWindow } from './api';
import { useUpstreamStability } from './useUpstreamStability';

const props = withDefaults(defineProps<{ kind: ResourceKind; resourceId: string; window?: StabilityWindow }>(), { window: '24h' });
const emit = defineEmits<{
  'update:window': [value: StabilityWindow];
  'update:error': [value: string];
  snapshot: [value: Stability | null];
}>();
const activeWindow = shallowRef<StabilityWindow>(props.window);
const runtime = useUpstreamStability(props.kind, () => props.resourceId, activeWindow);
const detail = runtime.detail;
const resuming = shallowRef(false);

watch(() => props.window, (value) => { if (value !== activeWindow.value) activeWindow.value = value; });
watch(activeWindow, (value) => emit('update:window', value));
watch(detail, (value) => emit('snapshot', value), { immediate: true });
watch(runtime.error, (value) => emit('update:error', value), { immediate: true });

const availabilityLabels: Record<string, string> = {
  available: '全部可用',
  partial: '部分受限',
  unavailable: '全部受限',
  unknown: '状态未知'
};
const windowLabels: Record<string, string> = { '1h': '最近 1 小时', '24h': '最近 24 小时', '7d': '最近 7 天' };
const phaseLabels: Record<string, string> = { available: '可用', cooling: '冷却中', recovering: '等待请求验证' };
const operationLabels: Record<string, string> = {
  openai_chat: 'OpenAI Chat',
  openai_responses: 'OpenAI Responses',
  openai_embeddings: 'OpenAI Embeddings',
  openai_images: 'OpenAI Images',
  anthropic_messages: 'Anthropic Messages',
  gemini_generate: 'Gemini Generate',
  gemini_embeddings: 'Gemini Embeddings'
};
const outcomeLabels: Record<string, string> = {
  success: '成功',
  server_error: '上游服务错误',
  timeout: '超时',
  network_error: '网络异常',
  unauthorized: '认证失败',
  rate_limited: '限流',
  model_error: '模型或权限错误',
  client_error: '请求参数错误（不计失败）',
  canceled: '已取消（不计失败）',
  circuit_open: '冷却跳过（不计失败）',
  rejected: '未发送（不计失败）'
};

const stateColumns: DsTableColumn[] = [
  { key: 'model', title: '影响模型', width: 180 },
  { key: 'target', title: '端点 / 凭据', width: 180 },
  { key: 'phase', title: '运行状态', width: 110 },
  { key: 'reasonLabel', title: '最近原因', width: 150 },
  { key: 'retry_at', title: '可重新调用时间', width: 180 },
  { key: 'verified_at', title: '最近验证成功', width: 180 }
];
const modelColumns: DsTableColumn[] = [
  { key: 'model', title: '模型 / 目标', width: 230, wrap: true },
  { key: 'operationLabel', title: '请求格式', width: 160 },
  { key: 'streamLabel', title: '传输', width: 78 },
  { key: 'result', title: '调用结果 / 最近错误', width: 260, wrap: true },
  { key: 'count', title: '次数', width: 70, align: 'right' },
  { key: 'latencyLabel', title: '成功延迟', width: 190, wrap: true }
];

function shortIdentifier(value?: string) {
  if (!value) return '—';
  return value.length > 14 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}

function targetLabel(value: { endpoint_id?: string; credential_id?: string }) {
  if (value.endpoint_id && value.credential_id) {
    return `端点 · ${shortIdentifier(value.endpoint_id)} / 凭据 · ${shortIdentifier(value.credential_id)}`;
  }
  if (value.endpoint_id) return `端点 · ${shortIdentifier(value.endpoint_id)}`;
  if (value.credential_id) return `凭据 · ${shortIdentifier(value.credential_id)}`;
  return '账号级';
}

function operationLabel(value?: string) {
  if (!value) return '—';
  const [base, suffix] = value.split(':', 2);
  const label = operationLabels[base] || base;
  if (suffix === 'compact') return `${label} · 紧凑请求`;
  if (suffix === 'edit') return `${label} · 图片编辑`;
  return label;
}

function durationLabel(value?: number | null) {
  if (value == null || value <= 0) return '—';
  return formatDuration(value);
}

function availabilityTone(value?: string): 'positive' | 'warning' | 'danger' | 'neutral' {
  if (value === 'available') return 'positive';
  if (value === 'partial' || value === 'unknown') return 'warning';
  if (value === 'unavailable') return 'danger';
  return 'neutral';
}

function outcomeTone(value?: string): 'positive' | 'warning' | 'danger' | 'neutral' {
  if (value === 'success') return 'positive';
  if (['client_error', 'canceled', 'circuit_open', 'rejected'].includes(value || '')) return 'neutral';
  if (value) return 'danger';
  return 'neutral';
}

const activeWindowLabel = computed(() => windowLabels[runtime.window.value] || runtime.window.value);
const stateRows = computed(() => (detail.value?.states || []).map(state => ({
  ...state,
  key: state.scope.key,
  model: state.scope.model || '所有模型',
  target: targetLabel({ endpoint_id: state.scope.endpoint_id, credential_id: state.scope.credential_id }),
  reasonLabel: outcomeLabels[state.reason || ''] || state.reason || '—'
})));
const modelRows = computed(() => (detail.value?.models || []).map((row, key) => ({
  ...row,
  key: `${row.endpoint_id || row.credential_id || row.model}|${row.operation}|${row.stream}|${row.outcome}|${key}`,
  model: row.model || '未标注模型',
  operationLabel: operationLabel(row.operation),
  target: targetLabel(row),
  streamLabel: row.stream ? '流式' : '非流式',
  result: outcomeLabels[row.outcome] || row.outcome || '—',
  p50Label: durationLabel(row.p50_ms),
  p95Label: durationLabel(row.p95_ms),
  lastError: row.last_error || '—',
  latencyLabel: row.p50_ms > 0 || row.p95_ms > 0 ? `P50 ${durationLabel(row.p50_ms)} · P95 ${durationLabel(row.p95_ms)}` : '无成功样本'
})));

function time(value?: number) { return value ? new Date(value).toLocaleString('zh-CN') : '—'; }

async function resume() {
  resuming.value = true;
  try {
    await stabilityApi.resume(props.kind, props.resourceId);
    await runtime.refresh();
    ElMessage.success('已允许新的业务请求重试，成功后将自动恢复正常调度');
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '恢复失败');
  } finally {
    resuming.value = false;
  }
}
</script>

<template>
  <PortalContentCard title="运行状态与稳定性" description="按账号、模型和结果聚合真实上游尝试，不是单条请求日志；取消、冷却跳过和本地拒绝不计入成功率。">
    <div class="stability-toolbar">
      <label class="stability-toolbar__field">
        <span>统计范围</span>
        <el-select v-model="runtime.window.value" aria-label="稳定性统计范围">
          <el-option label="最近 1 小时" value="1h" /><el-option label="最近 24 小时" value="24h" /><el-option label="最近 7 天" value="7d" />
        </el-select>
      </label>
      <DsTag v-if="runtime.loading.value" tone="info">更新中</DsTag>
      <DsTag v-if="detail && !detail.state_error" :tone="availabilityTone(detail.availability)">{{ availabilityLabels[detail.availability] }}</DsTag>
      <DsTag v-if="detail?.repeated_failure" tone="danger">最近一小时反复异常</DsTag>
      <span class="stability-toolbar__spacer" />
      <el-button :disabled="!detail || detail.config_status === 'disabled' || Boolean(detail.state_error)" :loading="resuming" :title="detail?.state_error ? '运行状态服务不可用，暂时无法执行恢复操作' : undefined" @click="resume">允许立即重试</el-button>
    </div>

    <el-alert
      v-if="detail?.state_error"
      title="运行状态暂不可读取"
      description="暂时无法读取账号的重试状态。请刷新重试；历史调用记录仍可查看，暂时不要反复停用、启用账号。"
      type="warning"
      :closable="false"
    />
    <el-alert v-else-if="runtime.error.value" :title="runtime.error.value" type="warning" :closable="false" />

    <div v-if="runtime.loading.value && !detail" class="stability-loading">正在加载运行统计…</div>
    <template v-else-if="detail">
      <div class="stability-metrics">
        <DsMetricCard label="上游尝试成功率" :value="detail.success_rate == null ? '暂无样本' : `${detail.success_rate.toFixed(1)}%`" :hint="`${detail.samples} 次有效尝试`" />
        <DsMetricCard label="有效样本" :value="String(detail.samples)" :hint="`${detail.successes} 次成功 · ${detail.failures} 次失败`" />
        <DsMetricCard label="上游失败" :value="String(detail.failures)" :hint="'仅统计真实上游失败'" />
        <DsMetricCard label="最近 1 小时进入冷却" :value="String(detail.cooldowns_last_hour)" :hint="`${detail.excluded} 次取消或本地跳过未计入成功率`" />
      </div>

      <p class="stability-note">统计窗口：{{ activeWindowLabel }} · 覆盖始于 {{ time(detail.coverage_started_at) }}。流式请求按首个有效输出统计延迟，其他请求按完成耗时统计。</p>

      <div class="stability-table-stack">
        <section class="stability-table-section">
          <div class="stability-section-heading">
            <div>
              <h3>故障与恢复</h3>
              <p>上游报错后会暂时等待，再用新的业务请求验证恢复。可点击“允许立即重试”提前结束等待。</p>
            </div>
          </div>
          <DsTable
            :columns="stateColumns"
            :rows="stateRows"
            row-key="key"
            :empty-title="detail.state_error ? '运行状态暂不可读取' : '暂无故障或恢复记录'"
            :empty-description="detail.state_error ? '历史调用统计仍可查看。' : '当前没有冷却或恢复中的运行范围。'"
          >
            <template #cell-target="{ row }"><span class="stability-ref" :title="row.scope.endpoint_id || row.scope.credential_id || undefined">{{ row.target }}</span></template>
            <template #cell-phase="{ row }"><DsTag :tone="row.phase === 'cooling' ? 'danger' : row.phase === 'recovering' ? 'warning' : 'positive'">{{ phaseLabels[row.phase] || row.phase }}</DsTag></template>
            <template #cell-verified_at="{ row }">{{ row.verified_at ? time(row.verified_at) : '尚未验证成功' }}</template>
            <template #cell-retry_at="{ row }">{{ row.phase === 'cooling' ? time(row.retry_at) : row.phase === 'recovering' ? (row.busy ? '试用请求进行中' : '已到期，可参与业务选路') : '—' }}</template>
          </DsTable>
        </section>

        <section class="stability-table-section">
          <div class="stability-section-heading">
            <div>
              <h3>调用明细</h3>
              <p>按模型、请求格式和结果汇总；P50/P95 没有成功样本时显示为 —。</p>
            </div>
          </div>
          <DsTable :columns="modelColumns" :rows="modelRows" row-key="key" empty-title="当前时间范围暂无调用记录">
            <template #cell-model="{ row }">
              <div class="stability-model-cell">
                <strong>{{ row.model }}</strong>
                <span class="stability-ref" :title="row.endpoint_id || row.credential_id || undefined">{{ row.target }}</span>
              </div>
            </template>
            <template #cell-result="{ row }">
              <div class="stability-result-cell">
                <DsTag :tone="outcomeTone(row.outcome)">{{ row.result }}</DsTag>
                <span v-if="row.lastError !== '—'" class="stability-error">{{ row.lastError }}</span>
              </div>
            </template>
          </DsTable>
        </section>
      </div>
    </template>
    <div v-else-if="!runtime.loading.value" class="stability-empty">当前账号暂无可展示的运行统计。</div>
  </PortalContentCard>
</template>

<style scoped>
.stability-toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 18px; }
.stability-toolbar__field { display: inline-flex; align-items: center; gap: 8px; color: var(--ds-muted); font-size: 12px; font-weight: 600; }
.stability-toolbar__field :deep(.el-select) { width: 150px; }
.stability-toolbar__spacer { flex: 1; }
.stability-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin: 18px 0; }
.stability-note { margin: 0 0 18px; padding: 10px 12px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); color: var(--ds-muted); font-size: 12px; line-height: 1.6; }
.stability-table-stack { display: flex; flex-direction: column; gap: 22px; margin-top: 4px; }
.stability-table-section { min-width: 0; }
.stability-section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
.stability-section-heading h3 { margin: 0; color: var(--ds-ink); font-size: 14px; }
.stability-section-heading p { margin: 4px 0 0; color: var(--ds-muted); font-size: 12px; line-height: 1.5; }
.stability-model-cell,
.stability-result-cell { display: flex; flex-direction: column; gap: 4px; min-width: 0; white-space: normal; }
.stability-model-cell strong { color: var(--ds-ink); font-weight: 650; }
.stability-ref { color: var(--ds-ink-soft); font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; }
.stability-error { display: block; max-width: 300px; color: var(--ds-ink-soft); line-height: 1.5; overflow-wrap: anywhere; white-space: normal; }
.stability-loading,
.stability-empty { padding: 36px 12px; color: var(--ds-muted); font-size: 13px; text-align: center; }
@media (max-width: 1100px) { .stability-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 640px) { .stability-metrics { grid-template-columns: 1fr; } .stability-toolbar__spacer { display: none; } .stability-toolbar__field { width: 100%; justify-content: space-between; } .stability-toolbar__field :deep(.el-select) { flex: 1; } }
</style>
