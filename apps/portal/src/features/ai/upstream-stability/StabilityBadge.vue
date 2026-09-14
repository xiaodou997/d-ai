<script setup lang="ts">
import { DsTag } from '@/shared/ui';
import type { Stability } from './api';
defineProps<{ value?: Stability }>();
</script>
<template>
  <div class="stability-badge">
    <span>{{ value?.success_rate == null ? '暂无数据' : `${value.success_rate.toFixed(1)}% 成功` }}</span>
    <span v-if="value">{{ value.samples }} 次<span v-if="value.samples > 0 && value.samples < 20"> · 样本不足</span></span>
    <DsTag v-if="value?.state_error" tone="warning">状态服务不可用</DsTag>
 <DsTag v-else-if="value?.states?.some(state => state.phase === 'cooling' && state.scope.kind === 'authentication')" tone="danger">认证冷却</DsTag>
 <DsTag v-if="value?.repeated_failure" tone="danger">反复异常</DsTag>
    <DsTag v-else-if="value?.stability_declining" tone="warning">稳定性下降</DsTag>
  </div>
</template>
<style scoped>
.stability-badge { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; color: var(--ds-muted); font-size: 12px; }
</style>
