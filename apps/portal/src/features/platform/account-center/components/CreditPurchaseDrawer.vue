<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { Coins, WalletCards } from "lucide-vue-next";
import { PortalQrPayDialog, type QrPayPollResult } from "@/platform";

import type { TenantCashAccount, TenantTopupConfig, TenantTopupOrderCreated, TopupPackage } from "@/api/types/tenant";
import { formatCents, formatCredits, type PurchaseMethod } from "../model";

const props = defineProps<{
  visible: boolean;
  method: PurchaseMethod;
  cash: TenantCashAccount;
  config: TenantTopupConfig | null;
  configLoading: boolean;
  submitting: boolean;
  activeOrder: TenantTopupOrderCreated | null;
  qrVisible: boolean;
  poll: () => Promise<QrPayPollResult>;
}>();

const emit = defineEmits<{
  close: [];
  "update:method": [value: PurchaseMethod];
  balancePurchase: [amountYuan: number];
  wechatPurchase: [body: { amount?: number; packageId?: string }];
  qrClose: [];
  qrSuccess: [result: QrPayPollResult];
}>();

const balanceAmountYuan = shallowRef<number | null>(null);
const customAmountYuan = shallowRef<number | null>(null);
const selectedPackage = shallowRef<TopupPackage | null>(null);

const enabledPackages = computed(() => (props.config?.packages ?? []).filter((item) => item.enabled));
const balancePreviewCredits = computed(() => Math.floor(Number(balanceAmountYuan.value ?? 0) * props.cash.creditsPerCny));
const balanceAmountFen = computed(() => Math.round(Number(balanceAmountYuan.value ?? 0) * 100));
const balanceInsufficient = computed(() => balanceAmountFen.value > props.cash.available);
const canBuyWithBalance = computed(() => balanceAmountFen.value > 0 && !balanceInsufficient.value);
const customPreview = computed(() => {
  if (!props.config || !customAmountYuan.value) return { gross: 0, fee: 0, net: 0, amountFen: 0 };
  const amountFen = Math.round(customAmountYuan.value * 100);
  const gross = Math.floor(customAmountYuan.value * props.config.exchangeRate);
  const fee = Math.ceil((gross * props.config.feeRateBp) / 10000);
  return { gross, fee, net: Math.max(0, gross - fee), amountFen };
});
const canBuyCustom = computed(() => Boolean(
  props.config
  && customPreview.value.amountFen >= props.config.min
  && customPreview.value.amountFen <= props.config.max
  && customPreview.value.net > 0
));

function choosePackage(pkg: TopupPackage) {
  selectedPackage.value = pkg;
  customAmountYuan.value = null;
}

function selectCustom() {
  selectedPackage.value = null;
}

function useAllBalance() {
  balanceAmountYuan.value = Number((props.cash.available / 100).toFixed(2));
}

function submitPackage() {
  if (!selectedPackage.value) return;
  emit("wechatPurchase", { packageId: selectedPackage.value.id });
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return;
    balanceAmountYuan.value = null;
    customAmountYuan.value = null;
    selectedPackage.value = null;
  }
);
</script>

<template>
  <el-drawer :model-value="visible" title="购买积分" size="min(560px, 100vw)" append-to-body destroy-on-close @close="emit('close')">
    <div class="purchase-drawer">
      <div class="purchase-method" role="group" aria-label="支付方式">
        <el-radio-group :model-value="method" @update:model-value="emit('update:method', $event as PurchaseMethod)">
          <el-radio-button value="balance" :disabled="cash.available <= 0">
            <WalletCards :size="15" />余额支付
          </el-radio-button>
          <el-radio-button value="wechat">微信支付</el-radio-button>
        </el-radio-group>
      </div>

      <section v-if="method === 'balance'" class="purchase-section">
        <div class="purchase-summary">
          <span>可用余额</span>
          <strong>¥{{ formatCents(cash.available) }}</strong>
          <em>余额购买不收手续费，1 元可购买 {{ cash.creditsPerCny }} 积分</em>
        </div>

        <label class="amount-field">
          <span>购买金额</span>
          <el-input-number v-model="balanceAmountYuan" :min="0" :precision="2" :controls="false" placeholder="输入金额" class="amount-field__input" />
          <button type="button" class="amount-field__all" @click="useAllBalance">全部余额</button>
        </label>

        <div class="purchase-preview" :class="{ 'purchase-preview--error': balanceInsufficient }">
          <Coins :size="18" />
          <span v-if="balanceInsufficient">可用余额不足</span>
          <span v-else>预计到账 <b>{{ formatCredits(balancePreviewCredits) }}</b> 积分</span>
        </div>

        <el-button type="primary" size="large" :loading="submitting" :disabled="!canBuyWithBalance" @click="emit('balancePurchase', Number(balanceAmountYuan))">
          确认购买
        </el-button>
      </section>

      <section v-else class="purchase-section">
        <div v-if="configLoading" class="purchase-loading" v-loading="true" />
        <el-empty v-else-if="!config?.enabled" description="微信支付暂未开放，可使用账户余额购买积分" />
        <template v-else-if="config">
          <div class="purchase-summary">
            <span>微信支付</span>
            <strong>1 元 = {{ config.exchangeRate }} 积分</strong>
            <em>自定义金额手续费 {{ (config.feeRateBp / 100).toFixed(2) }}%</em>
          </div>

          <div v-if="enabledPackages.length" class="package-options" aria-label="积分套餐">
            <button
              v-for="pkg in enabledPackages"
              :key="pkg.id"
              type="button"
              :class="['package-option', { 'package-option--active': selectedPackage?.id === pkg.id }]"
              @click="choosePackage(pkg)"
            >
              <span v-if="pkg.badge" class="package-option__badge">{{ pkg.badge }}</span>
              <strong>¥{{ formatCents(pkg.amount) }}</strong>
              <span>{{ pkg.name }}</span>
              <em>{{ formatCredits(pkg.credits) }} 积分</em>
            </button>
          </div>

          <label class="amount-field amount-field--custom">
            <span>自定义金额</span>
            <el-input-number
              v-model="customAmountYuan"
              :min="config.min / 100"
              :max="config.max / 100"
              :precision="2"
              :controls="false"
              placeholder="输入金额"
              class="amount-field__input"
              @focus="selectCustom"
            />
          </label>

          <div v-if="selectedPackage" class="purchase-preview">
            <Coins :size="18" />套餐到账 <b>{{ formatCredits(selectedPackage.credits) }}</b> 积分
          </div>
          <div v-else class="purchase-preview">
            <Coins :size="18" />
            <span>预计到账 <b>{{ formatCredits(customPreview.net) }}</b> 积分</span>
            <small v-if="customPreview.fee">已扣除 {{ formatCredits(customPreview.fee) }} 积分手续费</small>
          </div>

          <el-button
            type="primary"
            size="large"
            :loading="submitting"
            :disabled="selectedPackage ? false : !canBuyCustom"
            @click="selectedPackage ? submitPackage() : emit('wechatPurchase', { amount: customPreview.amountFen })"
          >
            去支付
          </el-button>
        </template>
      </section>
    </div>
  </el-drawer>

  <PortalQrPayDialog
    v-if="activeOrder"
    :visible="qrVisible"
    :order-id="activeOrder.orderId"
    :code-url="activeOrder.codeUrl"
    :amount="activeOrder.amount"
    :credit-amount="activeOrder.creditAmount"
    :expires-at="activeOrder.expiresAt"
    :poll="poll"
    @close="emit('qrClose')"
    @success="emit('qrSuccess', $event)"
  />
</template>

<style scoped>
.purchase-drawer,
.purchase-section {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.purchase-method :deep(.el-radio-group) {
  display: grid;
  width: 100%;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.purchase-method :deep(.el-radio-button__inner) {
  display: flex;
  width: 100%;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.purchase-summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 4px 12px;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
  padding: 14px 16px;
  background: var(--ds-panel-muted);
}

.purchase-summary span,
.purchase-summary em {
  color: var(--ds-muted);
  font-size: 12px;
  font-style: normal;
}

.purchase-summary strong {
  color: var(--ds-ink);
  font-size: 16px;
}

.purchase-summary em {
  grid-column: 1 / -1;
}

.amount-field {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
}

.amount-field > span {
  grid-column: 1 / -1;
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 700;
}

.amount-field__input {
  width: 100%;
}

.amount-field__all {
  height: 32px;
  border: 0;
  background: transparent;
  color: var(--ds-accent-hover);
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
}

.amount-field--custom {
  grid-template-columns: minmax(0, 1fr);
}

.purchase-preview {
  display: flex;
  min-height: 46px;
  align-items: center;
  gap: 7px;
  border-radius: 8px;
  padding: 10px 12px;
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
  font-size: 13px;
}

.purchase-preview small {
  margin-left: auto;
  color: var(--ds-muted);
  font-size: 11px;
}

.purchase-preview--error {
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.package-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.package-option {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 112px;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 3px;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
  padding: 14px;
  background: var(--ds-panel);
  color: var(--ds-ink-soft);
  cursor: pointer;
  text-align: left;
}

.package-option--active {
  border-color: var(--ds-accent);
  box-shadow: 0 0 0 2px var(--ds-accent-soft);
}

.package-option strong {
  color: var(--ds-ink);
  font-size: 20px;
}

.package-option span,
.package-option em {
  font-size: 12px;
  font-style: normal;
}

.package-option em {
  color: var(--ds-accent-hover);
  font-weight: 700;
}

.package-option__badge {
  position: absolute;
  top: 8px;
  right: 8px;
  color: var(--ds-positive) !important;
  font-size: 10px !important;
  font-weight: 700;
}

.purchase-loading {
  min-height: 220px;
}

@media (max-width: 520px) {
  .package-options {
    grid-template-columns: minmax(0, 1fr);
  }

  .purchase-preview {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .purchase-preview small {
    width: 100%;
    margin-left: 25px;
  }
}
</style>
