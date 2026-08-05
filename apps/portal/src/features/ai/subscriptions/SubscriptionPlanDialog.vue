<script setup lang="ts">
import { computed, reactive, shallowRef, watch, useTemplateRef } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'

import { aiTenantApi } from '../../../api/aiTenant'
import type {
  TenantAiVisibleGroup,
  TenantSubPlan,
  TenantSubPlanGroupInput,
  TenantSubPurchasePolicyInput,
  TenantSubPlanWriteRequest
} from '../../../types/aiTenant'
import SubscriptionGroupPricingTable from './SubscriptionGroupPricingTable.vue'
import SubscriptionPlanBasicsEditor from './SubscriptionPlanBasicsEditor.vue'
import SubscriptionPurchasePolicyEditor from './SubscriptionPurchasePolicyEditor.vue'
import { defaultSubscriptionPurchasePolicy } from './subscriptionPurchasePolicy'
import {
  estimateSubscriptionPaygValue,
  MICRO_PER_CREDIT,
  type SubscriptionPricingGroup
} from './subscriptionPricing'

const props = defineProps<{
  plan: TenantSubPlan | null
  groups: TenantAiVisibleGroup[]
  creditsPerUsd: number
}>()

const emit = defineEmits<{
  saved: []
}>()

const visible = defineModel<boolean>({ required: true })
const formRef = useTemplateRef<FormInstance>('formRef')
const DEFAULT_PLAN_PRICE_CREDITS = 100
const activeSection = shallowRef('basics')

const form = reactive({
  name: '',
  description: '',
  price_credits: DEFAULT_PLAN_PRICE_CREDITS,
  duration_days: 7,
  total_limit_credits: DEFAULT_PLAN_PRICE_CREDITS,
  window_5h_limit_credits: null as number | null,
  window_7d_limit_credits: null as number | null,
  sale_limit: null as number | null,
  groups: [] as TenantSubPlanGroupInput[],
  purchase_policy: defaultSubscriptionPurchasePolicy(),
  submitting: false
})

const rules: FormRules = {
  name: [{ required: true, message: '请输入套餐名称', trigger: 'blur' }],
  price_credits: [{ required: true, message: '请输入售价', trigger: 'blur' }],
  total_limit_credits: [
    { required: true, message: '请输入总额度', trigger: 'blur' }
  ]
}

const title = computed(() => (props.plan ? '编辑订阅套餐' : '新建订阅套餐'))
const supports7d = computed(() => form.duration_days >= 7)
const availableGroups = computed<TenantAiVisibleGroup[]>(() => {
  const items = [...props.groups]
  const known = new Set(items.map((group) => group.id))
  for (const group of props.plan?.groups ?? []) {
    if (known.has(group.id)) continue
    items.push({
      id: group.id,
      tenant_id: '',
      name: group.name,
      description: '',
      retail_price_book_id: '',
      default_user_multiplier: 0,
      user_default_visible: false,
      allow_protocol_conversion: false,
      sort_order: 0,
      status: 'disabled'
    })
  }
  return items
})
const groupById = computed(
  () => new Map(availableGroups.value.map((group) => [group.id, group]))
)
const pricingGroups = computed<SubscriptionPricingGroup[]>(() => {
  const resolved = form.groups.map((selected) => ({
    selected,
    group: groupById.value.get(selected.group_id)
  }))
  if (resolved.some(({ group }) => group?.status !== 'active')) return []
  return resolved.map(({ selected, group }) => ({
    groupId: selected.group_id,
    paygMultiplier: group!.default_user_multiplier,
    quotaDebitMultiplier: selected.quota_debit_multiplier
  }))
})

function microToCredits(value?: number | null): number | null {
  return value == null ? null : value / MICRO_PER_CREDIT
}

function creditsToMicro(value?: number | null): number | null {
  return value == null ? null : Math.round(Number(value) * MICRO_PER_CREDIT)
}

function yuanLabel(credits: number): string {
  const rate = props.creditsPerUsd > 0 ? props.creditsPerUsd : 100
  return `¥${(credits / rate).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function priceYuanHint(credits: number): string {
  return `按当前积分汇率约 ${yuanLabel(credits)}`
}

function paygValueHint(credits?: number | null): string {
  if (credits == null) return '无额外限制'
  const valueMicro = estimateSubscriptionPaygValue(
    creditsToMicro(credits) ?? 0,
    pricingGroups.value
  )
  if (valueMicro == null) return '选择可售分组后显示按量购买价值'
  const valueCredits = valueMicro / MICRO_PER_CREDIT
  return `最低相当于按量购买 ${valueCredits.toLocaleString('zh-CN', { maximumFractionDigits: 4 })} 积分（约 ${yuanLabel(valueCredits)}）`
}

function resetForm() {
  const plan = props.plan
  const defaultPriceCredits = plan?.price_credits ?? DEFAULT_PLAN_PRICE_CREDITS
  Object.assign(form, {
    name: plan?.name ?? '',
    description: plan?.description ?? '',
    price_credits: defaultPriceCredits,
    duration_days: plan?.duration_days ?? 7,
    total_limit_credits: plan
      ? (microToCredits(plan.total_limit_micro) ?? defaultPriceCredits)
      : defaultPriceCredits,
    window_5h_limit_credits: microToCredits(plan?.window_5h_limit_micro),
    window_7d_limit_credits: microToCredits(plan?.window_7d_limit_micro),
    sale_limit: plan?.sale_limit ?? null,
    groups: (plan?.groups ?? []).map((group) => ({
      group_id: group.id,
      quota_debit_multiplier: group.quota_debit_multiplier
    })),
    purchase_policy: plan?.purchase_policy
      ? {
          lifetime_max_purchases:
            plan.purchase_policy.lifetime_max_purchases ?? null,
          period_type: plan.purchase_policy.period_type,
          period_max_purchases:
            plan.purchase_policy.period_max_purchases ?? null,
          rolling_window_hours:
            plan.purchase_policy.rolling_window_hours ?? null,
          calendar_unit: plan.purchase_policy.calendar_unit ?? '',
          calendar_timezone: plan.purchase_policy.calendar_timezone ?? '',
          allow_advance_purchase:
            plan.purchase_policy.allow_advance_purchase
        }
      : defaultSubscriptionPurchasePolicy(),
    submitting: false
  })
}

watch(visible, (isOpen) => {
  if (isOpen) {
    activeSection.value = 'basics'
    resetForm()
  }
})

watch(
  () => form.duration_days,
  () => {
    if (!supports7d.value) form.window_7d_limit_credits = null
  }
)

function purchasePolicyError(
  policy: TenantSubPurchasePolicyInput
): string | null {
  if (
    policy.lifetime_max_purchases != null &&
    policy.lifetime_max_purchases < 1
  ) {
    return '累计购买上限必须至少为 1 次'
  }
  if (policy.period_type === 'rolling') {
    if (!policy.period_max_purchases || policy.period_max_purchases < 1) {
      return '请填写滚动窗口内的购买次数上限'
    }
    if (!policy.rolling_window_hours || policy.rolling_window_hours < 1) {
      return '请填写至少 1 小时的滚动窗口'
    }
  }
  if (policy.period_type === 'calendar') {
    if (!policy.period_max_purchases || policy.period_max_purchases < 1) {
      return '请填写自然周期内的购买次数上限'
    }
    if (!policy.calendar_unit || !policy.calendar_timezone?.trim()) {
      return '请选择自然周期和周期时区'
    }
  }
  return null
}

async function submit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    activeSection.value = 'basics'
    return
  }
  if (!form.groups.length) {
    activeSection.value = 'groups'
    ElMessage.error('请至少启用一个分组')
    return
  }
  if (form.groups.some((group) => !(group.quota_debit_multiplier > 0))) {
    activeSection.value = 'groups'
    ElMessage.error('套餐扣额倍率必须大于 0')
    return
  }
  const policyError = purchasePolicyError(form.purchase_policy)
  if (policyError) {
    activeSection.value = 'rules'
    ElMessage.error(policyError)
    return
  }
  const total = creditsToMicro(form.total_limit_credits) ?? 0
  const window5h = creditsToMicro(form.window_5h_limit_credits)
  const window7d = supports7d.value
    ? creditsToMicro(form.window_7d_limit_credits)
    : null
  if (
    (window5h != null && window5h > total) ||
    (window7d != null && window7d > total)
  ) {
    ElMessage.error('窗口额度不能超过总额度')
    return
  }
  const body: TenantSubPlanWriteRequest = {
    name: form.name.trim(),
    description: form.description.trim(),
    price_credits: Math.round(form.price_credits),
    duration_days: form.duration_days,
    total_limit_micro: total,
    window_5h_limit_micro: window5h,
    window_7d_limit_micro: window7d,
    sale_limit: form.sale_limit == null ? null : Math.round(form.sale_limit),
    groups: form.groups.map((group) => ({ ...group })),
    purchase_policy: {
      ...form.purchase_policy,
      calendar_timezone: form.purchase_policy.calendar_timezone?.trim()
    }
  }

  form.submitting = true
  try {
    if (props.plan) {
      await aiTenantApi.updateSubscriptionPlan(props.plan.id, body)
      ElMessage.success('套餐已保存')
    } else {
      await aiTenantApi.createSubscriptionPlan(body)
      ElMessage.success('草稿已创建')
    }
    visible.value = false
    emit('saved')
  } catch (error) {
    const detail = error as { detail?: string; message?: string }
    ElMessage.error(detail.detail || detail.message || '保存失败')
  } finally {
    form.submitting = false
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="min(920px, calc(100vw - 32px))"
    top="5vh"
    class="subscription-plan-dialog"
    append-to-body
    :close-on-click-modal="false"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <el-tabs v-model="activeSection" class="plan-tabs" stretch>
        <el-tab-pane label="基本与额度" name="basics">
          <div class="tab-content">
            <SubscriptionPlanBasicsEditor
              v-model:name="form.name"
              v-model:description="form.description"
              v-model:duration-days="form.duration_days"
              v-model:price-credits="form.price_credits"
              v-model:sale-limit="form.sale_limit"
              v-model:total-limit-credits="form.total_limit_credits"
              v-model:window5h-limit-credits="form.window_5h_limit_credits"
              v-model:window7d-limit-credits="form.window_7d_limit_credits"
              :supports7d="supports7d"
              :sold-count="plan?.sold_count ?? 0"
              :reserved-count="plan?.reserved_count ?? 0"
              :price-hint="priceYuanHint(form.price_credits)"
              :total-hint="paygValueHint(form.total_limit_credits)"
              :window5h-hint="paygValueHint(form.window_5h_limit_credits)"
              :window7d-hint="paygValueHint(form.window_7d_limit_credits)"
            />
          </div>
        </el-tab-pane>

        <el-tab-pane label="适用分组" name="groups">
          <div class="tab-content">
            <section class="dialog-section">
              <div class="section-heading">
                <strong>套餐适用范围</strong>
                <span>至少选择一个可售分组，并设置套餐额度的扣减倍率</span>
              </div>
              <SubscriptionGroupPricingTable
                v-model="form.groups"
                :groups="availableGroups"
              />
            </section>

            <el-alert
              class="settlement-note"
              type="info"
              :closable="false"
              show-icon
              title="套餐请求按实际命中的上游账号向租户结算"
              description="套餐售价和用户额度由租户自行制定；账期利润按套餐销售收入减去期间租户结算扣费核算。"
            />
          </div>
        </el-tab-pane>

        <el-tab-pane label="购买规则" name="rules">
          <div class="tab-content">
            <SubscriptionPurchasePolicyEditor v-model="form.purchase_policy" />
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <span class="footer-summary">
          {{ form.name || '未命名套餐' }} · {{ form.duration_days }} 天 · {{ form.price_credits }} 积分
        </span>
        <div>
          <el-button @click="visible = false">取消</el-button>
          <el-button type="primary" :loading="form.submitting" @click="submit">保存</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.tab-content {
  padding: 18px 4px 8px;
}

.dialog-section,
.section-heading {
  display: flex;
  flex-direction: column;
}

.dialog-section {
  gap: 16px;
}

.section-heading {
  gap: 3px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--ds-line);
}

.section-heading strong {
  color: var(--ds-ink);
  font-size: 15px;
}

.section-heading span,
.footer-summary {
  color: var(--ds-muted);
  font-size: 12px;
}

.settlement-note {
  margin-top: 24px;
}

.dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.footer-summary {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:global(.subscription-plan-dialog) {
  display: flex;
  max-height: 90vh;
  flex-direction: column;
  margin-bottom: 5vh;
}

:global(.subscription-plan-dialog .el-dialog__body) {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding-top: 4px;
}

:global(.subscription-plan-dialog .el-dialog__footer) {
  border-top: 1px solid var(--ds-line);
}

@media (max-width: 720px) {
  .dialog-footer {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
