<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";

import type {
  TenantSubPurchaseCalendarUnit,
  TenantSubPurchasePeriodType,
  TenantSubPurchasePolicyInput
} from "../../../types/aiTenant";

const policy = defineModel<TenantSubPurchasePolicyInput>({ required: true });
const rollingUnit = shallowRef<"hour" | "day">("day");
const expandedSettings = shallowRef<string[]>([]);

const lifetimeLimited = computed({
  get: () => policy.value.lifetime_max_purchases != null,
  set: (enabled: boolean) => {
    policy.value.lifetime_max_purchases = enabled
      ? (policy.value.lifetime_max_purchases ?? 1)
      : null;
  }
});

const rollingAmount = computed({
  get: () => {
    const hours = policy.value.rolling_window_hours ?? 24;
    return rollingUnit.value === "day" ? Math.max(1, Math.round(hours / 24)) : hours;
  },
  set: (value: number | undefined) => {
    const amount = Math.max(1, Number(value) || 1);
    policy.value.rolling_window_hours = rollingUnit.value === "day" ? amount * 24 : amount;
  }
});

const rollingExample = computed(() => {
  const amount = rollingAmount.value;
  const unit = rollingUnit.value === "day" ? "天" : "小时";
  return `从每次购买时间开始计算，满 ${amount} ${unit}后恢复名额。`;
});

const calendarUnitOptions: Array<{
  value: TenantSubPurchaseCalendarUnit;
  label: string;
}> = [
  { value: "day", label: "每天" },
  { value: "week", label: "每周" },
  { value: "month", label: "每月" }
];

const timezoneOptions = [
  "UTC",
  "Asia/Shanghai",
  "Asia/Singapore",
  "Australia/Perth",
  "Europe/London",
  "America/Los_Angeles"
];

watch(
  () => policy.value,
  (value) => {
    const hours = value.rolling_window_hours ?? 24;
    rollingUnit.value = hours % 24 === 0 ? "day" : "hour";
  },
  { immediate: true }
);

function changePeriodType(value: string | number | boolean | undefined) {
  const type = value as TenantSubPurchasePeriodType;
  policy.value.period_type = type;
  if (type === "none") {
    policy.value.period_max_purchases = null;
    policy.value.rolling_window_hours = null;
    policy.value.calendar_unit = "";
    policy.value.calendar_timezone = "";
    return;
  }
  policy.value.period_max_purchases ??= 1;
  if (type === "rolling") {
    policy.value.rolling_window_hours ??= 24;
    rollingUnit.value = (policy.value.rolling_window_hours ?? 24) % 24 === 0 ? "day" : "hour";
    policy.value.calendar_unit = "";
    policy.value.calendar_timezone = "";
    return;
  }
  policy.value.rolling_window_hours = null;
  policy.value.calendar_unit ||= "month";
  policy.value.calendar_timezone ||=
    Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function changeRollingUnit(unit: "hour" | "day") {
  const hours = policy.value.rolling_window_hours ?? 24;
  rollingUnit.value = unit;
  if (unit === "day") {
    policy.value.rolling_window_hours = Math.max(1, Math.ceil(hours / 24)) * 24;
  }
}
</script>

<template>
  <div class="policy-editor">
    <section class="policy-section">
      <div class="section-heading">
        <strong>每位用户最多可以买多少</strong>
        <span>这是单个用户的总购买次数，不影响套餐的全局销售数量</span>
      </div>
      <div class="sentence-row">
        <el-switch v-model="lifetimeLimited" />
        <span v-if="!lifetimeLimited">不限制每位用户的总购买次数</span>
        <template v-else>
          <span>每位用户总共最多购买</span>
          <el-input-number
            v-model="policy.lifetime_max_purchases"
            :min="1"
            :max="2147483647"
            :precision="0"
            :step="1"
          />
          <span>份</span>
        </template>
      </div>
    </section>

    <section class="policy-section">
      <div class="section-heading">
        <strong>多久可以买一次</strong>
        <span>用于控制短时间内的重复购买</span>
      </div>
      <el-radio-group
        v-model="policy.period_type"
        class="period-type"
        @change="changePeriodType"
      >
        <el-radio-button value="none">不限频率</el-radio-button>
        <el-radio-button value="rolling">连续时间内</el-radio-button>
        <el-radio-button value="calendar">按日 / 周 / 月</el-radio-button>
      </el-radio-group>

      <div v-if="policy.period_type === 'rolling'" class="rule-builder">
        <span>每位用户在任意连续</span>
        <el-input-number
          v-model="rollingAmount"
          :min="1"
          :max="rollingUnit === 'day' ? 36500 : 876000"
          :precision="0"
          :step="1"
        />
        <el-select
          :model-value="rollingUnit"
          class="unit-select"
          @update:model-value="changeRollingUnit"
        >
          <el-option value="hour" label="小时" />
          <el-option value="day" label="天" />
        </el-select>
        <span>内最多购买</span>
        <el-input-number
          v-model="policy.period_max_purchases"
          :min="1"
          :max="2147483647"
          :precision="0"
          :step="1"
        />
        <span>份</span>
        <small>{{ rollingExample }}</small>
      </div>

      <div v-if="policy.period_type === 'calendar'" class="rule-builder">
        <span>每位用户</span>
        <el-select v-model="policy.calendar_unit" class="calendar-select">
          <el-option
            v-for="option in calendarUnitOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </el-select>
        <span>最多购买</span>
        <el-input-number
          v-model="policy.period_max_purchases"
          :min="1"
          :max="2147483647"
          :precision="0"
          :step="1"
        />
        <span>份，到下一个周期自动重新计算</span>
        <small>例如选择“每月”，会在每月 1 日 00:00 恢复购买名额。</small>

        <el-collapse v-model="expandedSettings" class="advanced-collapse">
          <el-collapse-item name="timezone" title="高级设置：周期时区">
            <el-form-item label="按以下时区计算周期开始时间">
              <el-select
                v-model="policy.calendar_timezone"
                allow-create
                filterable
                default-first-option
              >
                <el-option
                  v-for="timezone in timezoneOptions"
                  :key="timezone"
                  :label="timezone"
                  :value="timezone"
                />
              </el-select>
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </div>
    </section>

    <section class="policy-section">
      <div class="section-heading">
        <strong>套餐未到期时能否再次购买</strong>
        <span>再次购买的套餐会排队，并在当前套餐结束后自动生效</span>
      </div>
      <div class="sentence-row">
        <el-switch v-model="policy.allow_advance_purchase" />
        <span>{{ policy.allow_advance_purchase ? "允许提前购买下一份" : "必须等当前套餐结束后再购买" }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.policy-editor,
.policy-section {
  display: flex;
  flex-direction: column;
}

.policy-editor {
  gap: 28px;
}

.policy-section {
  gap: 14px;
}

.section-heading {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--ds-line);
}

.section-heading strong {
  color: var(--ds-ink);
  font-size: 15px;
}

.section-heading span,
.policy-editor small {
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.sentence-row,
.rule-builder {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  color: var(--ds-ink-soft);
  font-size: 14px;
}

.sentence-row :deep(.el-input-number),
.rule-builder :deep(.el-input-number) {
  width: 128px;
}

.rule-builder {
  padding: 14px 0 0;
}

.rule-builder small,
.advanced-collapse {
  flex-basis: 100%;
}

.unit-select {
  width: 94px;
}

.calendar-select {
  width: 110px;
}

.period-type {
  width: 100%;
}

.period-type :deep(.el-radio-button) {
  flex: 1;
}

.period-type :deep(.el-radio-button__inner) {
  width: 100%;
}

.advanced-collapse {
  border-top: 0;
}

.advanced-collapse :deep(.el-collapse-item__header) {
  color: var(--ds-ink-soft);
  font-size: 13px;
}

.policy-editor :deep(.el-form-item) {
  margin-bottom: 0;
}

.advanced-collapse :deep(.el-select) {
  width: 100%;
}

@media (max-width: 720px) {
  .period-type {
    align-items: stretch;
    flex-direction: column;
  }

  .period-type :deep(.el-radio-button__inner) {
    border-left: var(--el-border);
  }
}
</style>
