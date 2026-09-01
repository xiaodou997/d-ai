<script setup lang="ts">
import { computed } from "vue";
import { Delete, Plus } from "@element-plus/icons-vue";

import { validateTokenPriceTiers, type TokenPriceTier } from "../pricingTypes";

const tiers = defineModel<TokenPriceTier[]>({ required: true });

const commonLimits = [64_000, 128_000, 200_000, 272_000, 500_000, 1_000_000];
const priceStep = 0.000001;
const validationError = computed(() => validateTokenPriceTiers(tiers.value));

function emptyTier(): TokenPriceTier {
  return {
    up_to_input_tokens: null,
    input_per_1m_usd: 0,
    output_per_1m_usd: 0,
    cache_write_per_1m_usd: 0,
    cache_read_per_1m_usd: 0
  };
}

function updateTier(index: number, patch: Partial<TokenPriceTier>) {
  tiers.value = tiers.value.map((tier, tierIndex) => tierIndex === index ? { ...tier, ...patch } : tier);
}

function addTier() {
  const current = tiers.value.length ? tiers.value : [emptyTier()];
  const terminal = current.at(-1) ?? emptyTier();
  const previousLimit = current.length > 1 ? current.at(-2)?.up_to_input_tokens ?? 0 : 0;
  const nextLimit = commonLimits.find((limit) => limit > previousLimit) ?? previousLimit + 500_000;
  tiers.value = [
    ...current.slice(0, -1),
    { ...terminal, up_to_input_tokens: nextLimit },
    { ...terminal, up_to_input_tokens: null }
  ];
}

function removeTier(index: number) {
  if (tiers.value.length <= 1) return;
  const next = tiers.value.filter((_, tierIndex) => tierIndex !== index);
  tiers.value = next.map((tier, tierIndex) => tierIndex === next.length - 1
    ? { ...tier, up_to_input_tokens: null }
    : tier);
}

function rangeLabel(index: number, tier: TokenPriceTier) {
  const lower = index === 0 ? 0 : (tiers.value[index - 1]?.up_to_input_tokens ?? 0) + 1;
  const upper = tier.up_to_input_tokens === null ? "无上限" : tier.up_to_input_tokens.toLocaleString("zh-CN");
  return `${lower.toLocaleString("zh-CN")} - ${upper}`;
}
</script>

<template>
  <div class="tiered-pricing-editor">
    <div v-for="(tier, index) in tiers" :key="index" class="tier-row">
      <div class="tier-heading">
        <div>
          <strong>档位 {{ index + 1 }}</strong>
          <span>{{ rangeLabel(index, tier) }}</span>
        </div>
        <div class="tier-limit">
          <span>输入上限</span>
          <el-select
            v-if="index < tiers.length - 1"
            :model-value="tier.up_to_input_tokens"
            allow-create
            filterable
            default-first-option
            @update:model-value="(value: number | string) => updateTier(index, { up_to_input_tokens: Number(value) })"
          >
            <el-option v-for="limit in commonLimits" :key="limit" :label="limit.toLocaleString('zh-CN')" :value="limit" />
          </el-select>
          <span v-else class="unbounded-label">无上限</span>
          <el-button v-if="tiers.length > 1" link type="danger" :icon="Delete" title="删除档位" @click="removeTier(index)" />
        </div>
      </div>

      <div class="price-grid">
        <label>
          <span>输入 $/1M</span>
          <el-input-number :model-value="tier.input_per_1m_usd" :min="0" :step="priceStep" step-strictly :controls="false" @update:model-value="(value?: number) => updateTier(index, { input_per_1m_usd: value ?? 0 })" />
        </label>
        <label>
          <span>输出 $/1M</span>
          <el-input-number :model-value="tier.output_per_1m_usd" :min="0" :step="priceStep" step-strictly :controls="false" @update:model-value="(value?: number) => updateTier(index, { output_per_1m_usd: value ?? 0 })" />
        </label>
        <label>
          <span>缓存写 $/1M</span>
          <el-input-number :model-value="tier.cache_write_per_1m_usd" :min="0" :step="priceStep" step-strictly :controls="false" @update:model-value="(value?: number) => updateTier(index, { cache_write_per_1m_usd: value ?? 0 })" />
        </label>
        <label>
          <span>缓存读 $/1M</span>
          <el-input-number :model-value="tier.cache_read_per_1m_usd" :min="0" :step="priceStep" step-strictly :controls="false" @update:model-value="(value?: number) => updateTier(index, { cache_read_per_1m_usd: value ?? 0 })" />
        </label>
      </div>
    </div>

    <el-alert v-if="validationError" :title="validationError" type="error" :closable="false" show-icon />
    <el-button :icon="Plus" plain @click="addTier">添加档位</el-button>
  </div>
</template>

<style scoped>
.tiered-pricing-editor { display: grid; gap: 12px; width: 100%; }
.tier-row { padding: 12px; border: 1px solid var(--el-border-color); border-radius: var(--ds-radius-sm); background: var(--el-fill-color-lighter); }
.tier-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 12px; }
.tier-heading > div { display: flex; align-items: center; gap: 10px; }
.tier-heading strong { color: var(--el-text-color-primary); font-size: 13px; }
.tier-heading span { color: var(--el-text-color-secondary); font-size: 12px; }
.tier-limit { min-width: 240px; justify-content: flex-end; }
.tier-limit :deep(.el-select) { width: 150px; }
.unbounded-label { width: 150px; text-align: center; }
.price-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.price-grid label { display: grid; gap: 5px; min-width: 0; color: var(--el-text-color-regular); font-size: 12px; }
.price-grid :deep(.el-input-number) { width: 100%; }
@media (max-width: 900px) {
  .tier-heading { align-items: flex-start; flex-direction: column; }
  .tier-limit { width: 100%; justify-content: flex-start; }
  .price-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
