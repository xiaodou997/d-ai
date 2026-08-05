<script setup lang="ts">
import { computed } from "vue";
import { CircleCheck, Clock } from "@element-plus/icons-vue";

import type {
  AiSubPlan,
  AiSubPurchaseBlockReason
} from "../../../types/aiCustomer";

const props = defineProps<{
  plan: AiSubPlan;
}>();

const reasonLabels: Record<AiSubPurchaseBlockReason, string> = {
  purchase_order_processing: "该套餐已有订单正在处理",
  purchase_plan_already_queued: "该套餐已在待生效队列中",
  subscription_queue_full: "订阅待生效队列已满",
  advance_purchase_not_allowed: "当前套餐权益结束前不可再次购买",
  purchase_lifetime_limit_reached: "已达到累计购买上限",
  purchase_rolling_limit_reached: "尚未到下一次可购买时间",
  purchase_calendar_limit_reached: "已达到当前自然周期购买上限"
};

const policyLabel = computed(() => {
  const policy = props.plan.purchase_policy;
  if (!policy) return "不限购买频次";
  const parts: string[] = [];
  if (policy.lifetime_max_purchases != null) {
    parts.push(`累计最多购买 ${policy.lifetime_max_purchases} 次`);
  }
  if (policy.period_type === "rolling") {
    parts.push(
      `每 ${policy.rolling_window_hours} 小时最多购买 ${policy.period_max_purchases} 次`
    );
  } else if (policy.period_type === "calendar") {
    const units: Record<string, string> = {
      day: "自然日",
      week: "自然周",
      month: "自然月"
    };
    parts.push(
      `每${units[policy.calendar_unit || ""] || "自然周期"}最多购买 ${policy.period_max_purchases} 次`
    );
  }
  if (!policy.allow_advance_purchase) {
    parts.push("生效中的同套餐结束后才可再次购买");
  }
  return parts.length ? parts.join("；") : "不限购买频次";
});

const eligibility = computed(() => props.plan.purchase_eligibility);
const allowed = computed(() => !props.plan.sold_out && eligibility.value?.allowed !== false);
const statusLabel = computed(() => {
  if (props.plan.sold_out) return "套餐已售罄";
  const reasons = eligibility.value?.blocking_rules
    ?.map((rule) => reasonLabels[rule.reason])
    .filter((label, index, all) => all.indexOf(label) === index);
  if (reasons?.length) return reasons.join("；");
  const primary = eligibility.value?.primary_reason;
  return primary ? reasonLabels[primary] : "当前可购买";
});
const retryLabel = computed(() => {
  const retryAt = eligibility.value?.retry_at;
  if (!retryAt) return "";
  return `${new Date(retryAt).toLocaleString("zh-CN", { hour12: false })} 后可重试`;
});
</script>

<template>
  <div class="purchase-policy" :class="{ 'is-blocked': !allowed }">
    <div class="purchase-policy__status">
      <el-icon><CircleCheck v-if="allowed" /><Clock v-else /></el-icon>
      <div>
        <strong>{{ statusLabel }}</strong>
        <span v-if="retryLabel">{{ retryLabel }}</span>
      </div>
    </div>
    <p>{{ policyLabel }}</p>
  </div>
</template>

<style scoped>
.purchase-policy {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding-top: 10px;
  border-top: 1px solid var(--ds-line);
}

.purchase-policy__status {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  color: var(--ds-positive);
  font-size: 12px;
}

.purchase-policy__status > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.purchase-policy__status span {
  color: var(--ds-muted);
}

.purchase-policy.is-blocked .purchase-policy__status {
  color: var(--ds-warning);
}

.purchase-policy p {
  margin: 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.45;
}
</style>
