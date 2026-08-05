<script setup lang="ts">
import { computed } from "vue";
import { ArrowRight, CircleAlert, Coins, Landmark, WalletCards } from "lucide-vue-next";

import type { AccountBalance } from "@/api/types/urmTenant";
import type { TenantCashAccount } from "@/api/types/tenant";
import { formatCents, formatCredits, formatTime } from "../model";

const props = defineProps<{
  points: AccountBalance;
  cash: TenantCashAccount;
  pointsError?: string;
  cashError?: string;
  nearestExpiry?: { remainingCredits: number; expiresAt?: string | null } | null;
}>();

const emit = defineEmits<{
  purchase: [];
  withdraw: [];
  packages: [];
  retry: [];
}>();

const pointsExpiryText = computed(() => {
  if (!props.nearestExpiry?.expiresAt) return "没有即将到期的积分";
  return `${formatCredits(props.nearestExpiry.remainingCredits)} 积分将于 ${formatTime(props.nearestExpiry.expiresAt)} 到期`;
});
</script>

<template>
  <!-- 紧凑资产卡:标题层级由面板页头承担,此处不再另起二级标题;
       数值与分项统计同行,操作按钮收到卡片右上角,整体高度约为旧版的一半 -->
  <section class="asset-overview" aria-label="账户资产">
    <article class="asset-card asset-card--points">
      <header class="asset-card__head">
        <span class="asset-card__icon"><Coins :size="18" /></span>
        <div class="asset-card__identity">
          <span class="asset-card__label">可用积分</span>
          <span class="asset-card__hint">使用平台服务时会扣除</span>
        </div>
        <div class="asset-card__actions">
          <el-button type="primary" size="small" :icon="Coins" @click="emit('purchase')">购买积分</el-button>
          <el-button text size="small" :icon="ArrowRight" @click="emit('packages')">有效期</el-button>
        </div>
      </header>

      <div v-if="pointsError" class="asset-card__error">
        <CircleAlert :size="15" />
        <span>{{ pointsError }}</span>
        <button type="button" @click="emit('retry')">重试</button>
      </div>
      <div v-else class="asset-card__figures">
        <strong class="asset-card__value">
          {{ formatCredits(points.availableCredits) }}<small>积分</small>
        </strong>
        <dl class="asset-card__stats">
          <div><dt>总积分</dt><dd>{{ formatCredits(points.totalCredits) }}</dd></div>
          <div><dt>冻结</dt><dd>{{ formatCredits(points.frozenCredits) }}</dd></div>
          <div><dt>限时</dt><dd>{{ formatCredits(points.timedCredits) }}</dd></div>
        </dl>
      </div>

      <p class="asset-card__footnote">{{ pointsExpiryText }}</p>
    </article>

    <article class="asset-card asset-card--cash">
      <header class="asset-card__head">
        <span class="asset-card__icon"><WalletCards :size="18" /></span>
        <div class="asset-card__identity">
          <span class="asset-card__label">可用余额</span>
          <span class="asset-card__hint">用户充值后到账的钱</span>
        </div>
        <div class="asset-card__actions">
          <el-button type="primary" size="small" :disabled="cash.available <= 0" :icon="Coins" @click="emit('purchase')">
            用余额买积分
          </el-button>
          <el-button text size="small" @click="emit('withdraw')">提现</el-button>
        </div>
      </header>

      <div v-if="cashError" class="asset-card__error">
        <CircleAlert :size="15" />
        <span>{{ cashError }}</span>
        <button type="button" @click="emit('retry')">重试</button>
      </div>
      <div v-else class="asset-card__figures">
        <strong class="asset-card__value">
          <span class="asset-card__currency">¥</span>{{ formatCents(cash.available) }}
        </strong>
        <dl class="asset-card__stats">
          <div><dt>总余额</dt><dd>¥{{ formatCents(cash.balance) }}</dd></div>
          <div><dt>提现中</dt><dd>¥{{ formatCents(cash.frozen) }}</dd></div>
          <div><dt>兑换比例</dt><dd>1 元 = {{ cash.creditsPerCny }} 积分</dd></div>
        </dl>
      </div>

      <p class="asset-card__footnote"><Landmark :size="13" />余额可以购买积分，也可以提现到银行卡</p>
    </article>
  </section>
</template>

<style scoped>
.asset-overview {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.asset-card {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-left: 3px solid var(--ds-accent);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.asset-card--cash {
  border-left-color: var(--ds-positive);
}

.asset-card__head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.asset-card__icon {
  display: grid;
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 9px;
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
}

.asset-card--cash .asset-card__icon {
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.asset-card__identity {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 1px;
}

.asset-card__label {
  color: var(--ds-ink);
  font-size: 13.5px;
  font-weight: 700;
}

.asset-card__hint,
.asset-card__footnote {
  color: var(--ds-muted);
  font-size: 12px;
}

.asset-card__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
}

/* 数值与分项统计同行:占位从「上下堆叠」压成一行 */
.asset-card__figures {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 10px 20px;
}

.asset-card__value {
  color: var(--ds-ink);
  font-size: 26px;
  font-weight: 800;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}

.asset-card__value small,
.asset-card__currency {
  margin-left: 3px;
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 600;
}

.asset-card__currency {
  margin: 0 1px 0 0;
}

.asset-card__stats {
  display: flex;
  flex: 1;
  align-items: baseline;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px 18px;
  margin: 0;
}

.asset-card__stats > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 5px;
}

.asset-card__stats dt {
  color: var(--ds-faint);
  font-size: 11.5px;
}

.asset-card__stats dd {
  margin: 0;
  color: var(--ds-ink-soft);
  font-size: 12.5px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}

.asset-card__footnote {
  display: flex;
  align-items: center;
  gap: 5px;
  margin: 0;
  padding-top: 10px;
  border-top: 1px solid var(--ds-line);
}

.asset-card__error {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--ds-danger);
  font-size: 12px;
}

.asset-card__error button {
  border: 0;
  background: transparent;
  color: var(--ds-accent-hover);
  cursor: pointer;
  font-weight: 700;
}

@media (max-width: 960px) {
  .asset-overview {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 640px) {
  .asset-card__head {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .asset-card__stats {
    justify-content: flex-start;
  }
}
</style>
