<script setup lang="ts">
import { DsTag } from '@/shared/ui';
import type { Stability } from './api';
const props = withDefaults(defineProps<{ value?: Stability; loading?: boolean }>(), { loading: false });
const availabilityLabels: Record<string, string> = { partial: '部分受限', unavailable: '全部受限' };
</script>
<template>
  <div v-if="value" class="stability-badge">
    <span>{{ value.success_rate == null ? '暂无有效样本' : `${value.success_rate.toFixed(1)}% 成功` }}</span>
    <span>{{ value.samples }} 次有效尝试<span v-if="value.samples > 0 && value.samples < 20"> · 样本不足</span></span>
    <DsTag v-if="!value.state_error && value.states?.some(state => state.phase === 'cooling' && state.scope.kind === 'authentication')" tone="danger">认证冷却</DsTag>
    <DsTag v-if="value.repeated_failure" tone="danger">反复异常</DsTag>
    <DsTag v-else-if="value.stability_declining" tone="warning">稳定性下降</DsTag>
    <DsTag v-if="!value.state_error && availabilityLabels[value.availability]" :tone="value.availability === 'unavailable' ? 'danger' : 'warning'">
      {{ availabilityLabels[value.availability] }}
    </DsTag>
  </div>
  <div v-else-if="!props.loading" class="stability-badge stability-badge--empty">暂无稳定性样本</div>
  <div v-else class="stability-badge stability-badge--loading">正在加载稳定性…</div>
</template>
<style scoped>
.stability-badge { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; color: var(--ds-muted); font-size: 12px; }
.stability-badge--empty,
.stability-badge--loading { color: var(--ds-faint); }
</style>
