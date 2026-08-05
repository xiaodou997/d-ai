<script setup lang="ts">
import { computed, watch } from "vue";
import { Delete, Plus } from "@element-plus/icons-vue";

import { capabilityOptions } from "../../../../api/aiTenant";
import {
  isTokenPricedCapability,
  type LiteLLMPriceModel,
  type PriceBookEntryForm
} from "../pricingTypes";
import TieredPricingEditor from "./TieredPricingEditor.vue";

const props = defineProps<{
  editing: boolean;
  liteLLMOptions: LiteLLMPriceModel[];
  liteLLMLoading: boolean;
}>();

const emit = defineEmits<{
  searchLiteLLM: [query: string];
  applyLiteLLM: [modelCode: string];
  submit: [];
}>();

const visible = defineModel<boolean>("visible", { required: true });
const form = defineModel<PriceBookEntryForm>("form", { required: true });

const isTokenCap = computed(() => isTokenPricedCapability(form.value.capability_type));
const isImageCap = computed(() => form.value.capability_type === "image");
const isVideoCap = computed(() => form.value.capability_type === "video");
const isAudioCap = computed(() => form.value.capability_type === "audio_tts" || form.value.capability_type === "audio_stt");
const imagePriceTiers = [
  { value: "1k", label: "1K" },
  { value: "2k", label: "2K" },
  { value: "4k", label: "4K" }
];

function addResolution(kind: "image_prices" | "video_prices") {
  form.value[kind].push({ resolution: "", price: 0 });
}

function removeResolution(kind: "image_prices" | "video_prices", index: number) {
  form.value[kind].splice(index, 1);
}

function imageTierPrice(tier: string) {
  return form.value.image_prices.find((item) => item.resolution === tier)?.price ?? form.value.image_default_price_usd;
}

function imageTierConfigured(tier: string) {
  return form.value.image_prices.some((item) => item.resolution === tier);
}

function setImageTierConfigured(tier: string, enabled: boolean) {
  const index = form.value.image_prices.findIndex((item) => item.resolution === tier);
  if (enabled && index < 0) {
    form.value.image_prices.push({ resolution: tier, price: form.value.image_default_price_usd });
    return;
  }
  if (!enabled && index >= 0) form.value.image_prices.splice(index, 1);
}

function setImageTierPrice(tier: string, price: number | undefined) {
  const entry = form.value.image_prices.find((item) => item.resolution === tier);
  if (entry) entry.price = Number(price) || 0;
}

function normalizeImageTierPrices() {
  const prices = new Map(form.value.image_prices.map((item) => [item.resolution, item.price]));
  form.value.image_prices = imagePriceTiers.flatMap((tier) => {
    const price = prices.get(tier.value);
    return typeof price === "number" ? [{ resolution: tier.value, price }] : [];
  });
}

watch([visible, isImageCap], ([open, imageCap]) => {
  if (open && imageCap) normalizeImageTierPrices();
});

function firstTierLabel(model: LiteLLMPriceModel) {
  const tier = model.token_price_tiers[0];
  if (!tier) return model.model_code;
  return `${model.model_code} · in $${tier.input_per_1m_usd.toFixed(2)}/1M · out $${tier.output_per_1m_usd.toFixed(2)}/1M`;
}
</script>

<template>
  <el-dialog v-model="visible" :title="props.editing ? '编辑条目' : '新建条目'" width="min(920px, 94vw)">
    <el-form label-position="top">
      <el-form-item v-if="!props.editing" label="从 LiteLLM 填充">
        <el-select
          filterable
          remote
          clearable
          :remote-method="(query: string) => emit('searchLiteLLM', query)"
          :loading="props.liteLLMLoading"
          placeholder="搜索模型名"
          class="full-width"
          @change="(value: string) => emit('applyLiteLLM', value)"
          @visible-change="(open: boolean) => open && !props.liteLLMOptions.length && emit('searchLiteLLM', '')"
        >
          <el-option v-for="option in props.liteLLMOptions" :key="option.model_code" :label="firstTierLabel(option)" :value="option.model_code" />
        </el-select>
      </el-form-item>

      <div class="identity-grid">
        <el-form-item label="模型 model_code" required>
          <el-input v-model="form.model_code" :disabled="props.editing" />
        </el-form-item>
        <el-form-item label="能力类型">
          <el-select v-model="form.capability_type" class="full-width">
            <el-option v-for="capability in capabilityOptions" :key="capability.value" :label="capability.label" :value="capability.value" />
          </el-select>
        </el-form-item>
      </div>

      <el-form-item v-if="isTokenCap" label="上下文价格档位" required>
        <TieredPricingEditor v-model="form.token_price_tiers" />
      </el-form-item>

      <template v-else-if="isImageCap">
        <el-form-item label="默认价（每张 USD）" required>
          <el-input-number v-model="form.image_default_price_usd" :min="0" :precision="6" :controls="false" />
        </el-form-item>
        <el-form-item label="尺寸档位（每张 USD）">
          <div class="image-tier-list">
            <div v-for="tier in imagePriceTiers" :key="tier.value" class="image-tier-row">
              <strong>{{ tier.label }}</strong>
              <el-switch
                :model-value="imageTierConfigured(tier.value)"
                active-text="单独定价"
                inactive-text="默认价"
                @update:model-value="setImageTierConfigured(tier.value, $event)"
              />
              <el-input-number
                :model-value="imageTierPrice(tier.value)"
                :disabled="!imageTierConfigured(tier.value)"
                :min="0"
                :precision="6"
                :controls="false"
                @update:model-value="setImageTierPrice(tier.value, $event)"
              />
            </div>
          </div>
        </el-form-item>
      </template>

      <template v-else-if="isVideoCap">
        <el-form-item label="默认价（每秒 USD）" required>
          <el-input-number v-model="form.video_default_price_usd" :min="0" :precision="6" :controls="false" />
        </el-form-item>
        <el-form-item label="分辨率价格">
          <div class="resolution-list">
            <div v-for="(price, index) in form.video_prices" :key="index" class="resolution-row">
              <el-input v-model="price.resolution" placeholder="720p" />
              <el-input-number v-model="price.price" :min="0" :precision="6" :controls="false" />
              <el-button :icon="Delete" circle title="删除规格" @click="removeResolution('video_prices', index)" />
            </div>
            <el-button :icon="Plus" @click="addResolution('video_prices')">添加规格</el-button>
          </div>
        </el-form-item>
      </template>

      <div v-else-if="isAudioCap" class="identity-grid">
        <el-form-item label="TTS USD/1M 字符">
          <el-input-number v-model="form.audio_tts_per_1m_chars_usd" :min="0" :precision="6" :controls="false" />
        </el-form-item>
        <el-form-item label="STT USD/分钟">
          <el-input-number v-model="form.audio_stt_per_minute_usd" :min="0" :precision="6" :controls="false" />
        </el-form-item>
      </div>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" @click="emit('submit')">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.full-width { width: 100%; }
.identity-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(180px, 1fr); gap: 16px; }
.resolution-list { display: grid; gap: 8px; width: 100%; }
.resolution-row { display: grid; grid-template-columns: minmax(160px, 1fr) minmax(160px, 1fr) 36px; gap: 8px; }
.resolution-row :deep(.el-input-number) { width: 100%; }
.image-tier-list { display: grid; gap: 8px; width: 100%; }
.image-tier-row { display: grid; grid-template-columns: 48px 132px minmax(160px, 1fr); align-items: center; gap: 12px; }
.image-tier-row :deep(.el-input-number) { width: 100%; }
@media (max-width: 640px) {
  .identity-grid { grid-template-columns: 1fr; gap: 0; }
  .resolution-row { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 36px; }
  .image-tier-row { grid-template-columns: 44px minmax(0, 1fr); }
  .image-tier-row :deep(.el-input-number) { grid-column: 1 / -1; }
}
</style>
