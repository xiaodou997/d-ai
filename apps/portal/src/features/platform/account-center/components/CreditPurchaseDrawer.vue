<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { BadgeDollarSign, Gift } from "lucide-vue-next";
import { PortalQrPayDialog, type QrPayPollResult } from "@/platform";

import type { TenantTopupConfig, TenantTopupOrderCreated, TopupPackage } from "@/api/types/tenant";
import { formatMicroUSD, MICRO_USD_PER_USD } from "../model";

const props = defineProps<{
  visible: boolean;
  config: TenantTopupConfig | null;
  configLoading: boolean;
  submitting: boolean;
  activeOrder: TenantTopupOrderCreated | null;
  qrVisible: boolean;
  poll: () => Promise<QrPayPollResult>;
}>();

const emit = defineEmits<{
  close: [];
  topup: [body: { amountMicroUsd?: number; packageId?: string }];
  qrClose: [];
  qrSuccess: [result: QrPayPollResult];
}>();

const customAmountUsd = shallowRef<number | null>(null);
const selectedPackage = shallowRef<TopupPackage | null>(null);

const enabledPackages = computed(() => (props.config?.packages ?? []).filter((item) => item.enabled));
const customPreview = computed(() => {
  const gross = Math.round(Number(customAmountUsd.value ?? 0) * MICRO_USD_PER_USD);
  const fee = props.config ? Math.ceil((gross * props.config.feeRateBp) / 10_000) : 0;
  return { gross, fee, credited: Math.max(0, gross - fee) };
});
const canSubmitCustom = computed(() => Boolean(
  props.config
  && customPreview.value.gross >= props.config.minMicroUsd
  && customPreview.value.gross <= props.config.maxMicroUsd
  && customPreview.value.credited > 0
));

function choosePackage(pkg: TopupPackage) {
  selectedPackage.value = pkg;
  customAmountUsd.value = null;
}

function selectCustom() {
  selectedPackage.value = null;
}

function submit() {
  if (selectedPackage.value) emit("topup", { packageId: selectedPackage.value.id });
  else if (canSubmitCustom.value) emit("topup", { amountMicroUsd: customPreview.value.gross });
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return;
    customAmountUsd.value = null;
    selectedPackage.value = null;
  }
);
</script>

<template>
  <el-drawer :model-value="visible" title="充值 USD 额度" size="min(560px, 100vw)" append-to-body destroy-on-close @close="emit('close')">
    <div class="topup-drawer">
      <div v-if="configLoading" class="topup-loading" v-loading="true" />
      <el-empty v-else-if="!config?.enabled" description="在线充值暂未开放" />
      <template v-else-if="config">
        <div class="topup-summary">
          <BadgeDollarSign :size="20" />
          <div><strong>USD 额度充值</strong><span>订单将分别记录支付、手续费、赠送和到账金额</span></div>
          <em v-if="config.feeRateBp">自定义充值手续费 {{ (config.feeRateBp / 100).toFixed(2) }}%</em>
        </div>

        <div v-if="enabledPackages.length" class="package-options" aria-label="额度包">
          <button
            v-for="pkg in enabledPackages"
            :key="pkg.id"
            type="button"
            :class="['package-option', { 'package-option--active': selectedPackage?.id === pkg.id }]"
            @click="choosePackage(pkg)"
          >
            <span v-if="pkg.badge" class="package-option__badge">{{ pkg.badge }}</span>
            <strong>{{ formatMicroUSD(pkg.paymentAmountMicroUsd) }}</strong>
            <span>{{ pkg.name }}</span>
            <em v-if="pkg.giftAmountMicroUsd > 0"><Gift :size="13" />赠送 {{ formatMicroUSD(pkg.giftAmountMicroUsd) }}</em>
            <small>到账 {{ formatMicroUSD(pkg.paymentAmountMicroUsd + pkg.giftAmountMicroUsd) }}</small>
          </button>
        </div>

        <label class="amount-field">
          <span>自定义充值金额（USD）</span>
          <el-input-number
            v-model="customAmountUsd"
            :min="config.minMicroUsd / MICRO_USD_PER_USD"
            :max="config.maxMicroUsd / MICRO_USD_PER_USD"
            :precision="6"
            :controls="false"
            placeholder="输入 USD 金额"
            class="amount-field__input"
            @focus="selectCustom"
          />
        </label>

        <div v-if="selectedPackage" class="topup-preview">
          <span>支付 <b>{{ formatMicroUSD(selectedPackage.paymentAmountMicroUsd) }}</b></span>
          <span>赠送 <b>{{ formatMicroUSD(selectedPackage.giftAmountMicroUsd) }}</b></span>
          <strong>到账 {{ formatMicroUSD(selectedPackage.paymentAmountMicroUsd + selectedPackage.giftAmountMicroUsd) }}</strong>
        </div>
        <div v-else class="topup-preview">
          <span>充值金额 <b>{{ formatMicroUSD(customPreview.gross) }}</b></span>
          <span>手续费 <b>{{ formatMicroUSD(customPreview.fee) }}</b></span>
          <strong>到账 {{ formatMicroUSD(customPreview.credited) }}</strong>
        </div>

        <el-button type="primary" size="large" :loading="submitting" :disabled="selectedPackage ? false : !canSubmitCustom" @click="submit">
          去支付
        </el-button>
      </template>
    </div>
  </el-drawer>

  <PortalQrPayDialog
    v-if="activeOrder"
    :visible="qrVisible"
    :order-id="activeOrder.orderId"
    :code-url="activeOrder.codeUrl"
    :payment-amount-minor="activeOrder.paymentAmountMinor"
    :credited-amount-micro-usd="activeOrder.creditedAmountMicroUsd"
    :expires-at="activeOrder.expiresAt"
    :poll="poll"
    @close="emit('qrClose')"
    @success="emit('qrSuccess', $event)"
  />
</template>

<style scoped>
.topup-drawer { display:flex; flex-direction:column; gap:18px; }
.topup-loading { min-height:180px; }
.topup-summary { display:grid; grid-template-columns:auto minmax(0,1fr); align-items:center; gap:6px 10px; border:1px solid var(--ds-line); border-radius:var(--ds-radius-control); padding:14px 16px; background:var(--ds-panel-muted); color:var(--ds-accent); }
.topup-summary > div { display:flex; flex-direction:column; gap:2px; }
.topup-summary strong { color:var(--ds-ink); font-size:14px; }
.topup-summary span, .topup-summary em { color:var(--ds-muted); font-size:12px; font-style:normal; }
.topup-summary em { grid-column:2; }
.package-options { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px; }
.package-option { position:relative; display:flex; min-height:126px; flex-direction:column; align-items:flex-start; gap:5px; overflow:hidden; border:1px solid var(--ds-line); border-radius:var(--ds-radius-control); padding:14px; background:var(--ds-panel); color:var(--ds-ink); cursor:pointer; text-align:left; }
.package-option:hover, .package-option--active { border-color:var(--ds-accent); box-shadow: var(--ds-shadow-accent-outline); }
.package-option strong { font-size:20px; font-variant-numeric:tabular-nums; }
.package-option span, .package-option small { color:var(--ds-muted); font-size:12px; }
.package-option em { display:flex; align-items:center; gap:4px; color:var(--ds-positive); font-size:12px; font-style:normal; }
.package-option__badge { position:absolute; top:0; right:0; padding:3px 7px; border-bottom-left-radius:var(--ds-radius-sm); background:var(--ds-accent-soft); color:var(--ds-accent) !important; }
.amount-field { display:flex; flex-direction:column; gap:7px; color:var(--ds-ink-soft); font-size:12px; font-weight:700; }
.amount-field__input { width:100%; }
.topup-preview { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:8px 12px; border:1px solid var(--ds-line); border-radius:var(--ds-radius-control); padding:12px 14px; background:var(--ds-panel-muted); color:var(--ds-muted); font-size:12px; }
.topup-preview span { display:flex; justify-content:space-between; gap:8px; }
.topup-preview b { color:var(--ds-ink); }
.topup-preview strong { grid-column:1/-1; border-top:1px solid var(--ds-line); padding-top:8px; color:var(--ds-positive); font-size:15px; }
@media (max-width:520px) { .package-options { grid-template-columns:1fr; } }
</style>
