<script setup lang="ts">
import { computed } from "vue";
import { ArrowRight, CircleAlert, WalletCards } from "lucide-vue-next";
import type { AccountBalance } from "@/api/types/platformTenant";
import { formatMicroUSD, formatTime, formatUSD } from "../model";

const props = defineProps<{
  balance: AccountBalance;
  balanceError?: string;
  nearestExpiry?: { remainingUsd: number; expiresAt?: string | null } | null;
}>();
const emit = defineEmits<{ purchase: []; lots: []; retry: [] }>();
const expiryText = computed(() => props.nearestExpiry?.expiresAt
  ? `${formatUSD(props.nearestExpiry.remainingUsd)} 将于 ${formatTime(props.nearestExpiry.expiresAt)} 到期`
  : "当前没有即将到期的余额");
</script>

<template>
  <section class="asset-overview" aria-label="USD 账户余额">
    <header class="asset-overview__head">
      <span class="asset-overview__icon"><WalletCards :size="19" /></span>
      <div><strong>USD 额度</strong><span>充值收入、服务消费和透支统一归集到一个额度账户</span></div>
      <el-button type="primary" size="small" @click="emit('purchase')">充值</el-button>
      <el-button text size="small" :icon="ArrowRight" @click="emit('lots')">有效期</el-button>
    </header>
    <div v-if="balanceError" class="asset-overview__error">
      <CircleAlert :size="15" /><span>{{ balanceError }}</span><button type="button" @click="emit('retry')">重试</button>
    </div>
    <div v-else class="asset-overview__figures">
      <strong class="asset-overview__value">{{ formatUSD(balance.availableUsd) }}</strong>
      <dl>
        <div><dt>总余额</dt><dd>{{ formatUSD(balance.remainingUsd) }}</dd></div>
        <div><dt>长期有效</dt><dd>{{ formatUSD(balance.permanentUsd) }}</dd></div>
        <div><dt>限时</dt><dd>{{ formatUSD(balance.timedUsd) }}</dd></div>
        <div><dt>当前透支</dt><dd>{{ formatMicroUSD(balance.outstandingDebtMicroUsd) }}</dd></div>
      </dl>
    </div>
    <p>{{ expiryText }}</p>
  </section>
</template>

<style scoped>
.asset-overview { display:flex; flex-direction:column; gap:12px; padding:14px 16px; border:1px solid var(--ds-line); border-left:3px solid var(--ds-accent); border-radius:var(--ds-radius-panel); background:var(--ds-panel); box-shadow:var(--ds-shadow-sm); }
.asset-overview__head { display:flex; align-items:center; gap:10px; }
.asset-overview__head > div { display:flex; min-width:0; flex:1; flex-direction:column; }
.asset-overview__head span, .asset-overview p, dt { color:var(--ds-muted); font-size:12px; }
.asset-overview__icon { display:grid; width:34px; height:34px; place-items:center; border-radius:8px; background:var(--ds-accent-soft); color:var(--ds-accent); }
.asset-overview__figures { display:flex; align-items:baseline; flex-wrap:wrap; gap:14px 28px; }
.asset-overview__value { font-size:28px; font-variant-numeric:tabular-nums; }
dl { display:flex; flex:1; flex-wrap:wrap; justify-content:flex-end; gap:8px 20px; margin:0; }
dl > div { display:flex; align-items:baseline; gap:6px; } dd { margin:0; font-weight:700; font-variant-numeric:tabular-nums; }
.asset-overview p { margin:0; }
.asset-overview__error { display:flex; align-items:center; gap:8px; color:var(--ds-danger); }
.asset-overview__error button { border:0; background:none; color:inherit; cursor:pointer; }
@media (max-width:760px) { .asset-overview__head { flex-wrap:wrap; } dl { justify-content:flex-start; } }
</style>
