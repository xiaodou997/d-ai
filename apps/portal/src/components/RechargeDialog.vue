<template>
  <el-dialog
    :model-value="modelValue"
    :title="title"
    width="min(640px, 94vw)"
    :close-on-click-modal="false"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
    @closed="resetForm"
  >
    <div class="recharge-target">
      <div class="recharge-target__main">
        <span class="recharge-target__label">{{ targetTypeLabel }}</span>
        <strong>{{ targetName }}</strong>
        <span class="recharge-target__identity">{{ targetIdentity }}</span>
      </div>
      <div v-if="targetBalanceUsd != null" class="recharge-target__balance">
        <span>当前 USD 余额</span>
        <strong>${{ targetBalanceUsd.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 }) }}</strong>
      </div>
    </div>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <div class="recharge-fields">
        <el-form-item label="实付金额（USD）" prop="paidAmountUsd">
          <el-input-number
            v-model="form.paidAmountUsd"
            :min="0"
            :precision="2"
            :step="100"
            :controls="false"
            class="recharge-field"
            @change="handlePaidAmountChange"
          />
          <div class="recharge-quick-list">
            <button v-for="amount in [100, 500, 1000]" :key="amount" type="button" class="recharge-quick" @click="pickAmount(amount)">
              ${{ amount }}
            </button>
          </div>
          <p class="recharge-hint">支付渠道金额按 USD cents 保存</p>
        </el-form-item>

        <el-form-item label="到账金额（USD）" prop="amountUsd">
          <el-input-number
            v-model="form.amountUsd"
            :min="0.000001"
            :precision="6"
            :step="1"
            :controls="false"
            class="recharge-field"
            @change="isAmountAutoCalc = false"
          />
          <div class="recharge-quick-list">
            <button v-for="amount in [100, 500, 1000]" :key="amount" type="button" class="recharge-quick" @click="pickCreditedAmount(amount)">
              +${{ amount.toLocaleString() }}
            </button>
          </div>
          <p class="recharge-hint" :class="{ 'recharge-hint--accent': isAmountAutoCalc && (form.paidAmountUsd ?? 0) > 0 }">
            {{ isAmountAutoCalc && (form.paidAmountUsd ?? 0) > 0 ? '默认与实付金额相同，可单独增加赠送金额' : '可独立设置到账金额' }}
          </p>
        </el-form-item>
      </div>

      <el-form-item label="有效期（可选）">
        <el-date-picker
          v-model="form.expireTime"
          type="datetime"
          placeholder="永久有效"
          value-format="YYYY-MM-DD HH:mm:ss"
          class="recharge-field"
          :disabled-date="(date: Date) => date.getTime() < Date.now()"
        />
        <div class="recharge-quick-list recharge-quick-list--wrap">
          <button v-for="days in [7, 30, 90, 180, 365]" :key="days" type="button" class="recharge-quick" @click="setExpireDays(days)">
            {{ days }} 天
          </button>
          <button type="button" class="recharge-quick" @click="form.expireTime = null">永久有效</button>
        </div>
        <p class="recharge-hint">按北京时间填写；留空则长期有效，到期后余额自动失效。</p>
      </el-form-item>

      <el-form-item label="备注" prop="reason">
        <el-input
          v-model="form.reason"
          type="textarea"
          :rows="3"
          :placeholder="isZeroAmount ? '实付金额为 $0，请详细说明赠送原因（必填）' : '充值备注（可选）'"
          maxlength="500"
          show-word-limit
        />
        <p v-if="isZeroAmount" class="recharge-hint recharge-hint--danger">实付金额为 $0 时，备注至少填写 5 个字符</p>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button :disabled="submitting" @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">确认充值</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue'
import { ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import type { RechargeFormPayload } from './recharge'

withDefaults(defineProps<{
  modelValue: boolean
  title?: string
  targetTypeLabel: string
  targetName: string
  targetIdentity: string
  targetBalanceUsd?: number | null
  submitting?: boolean
}>(), {
  title: '账户充值',
  targetBalanceUsd: null,
  submitting: false
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  submit: [payload: RechargeFormPayload]
}>()

const formRef = ref<FormInstance>()
const isAmountAutoCalc = ref(true)
const form = reactive({
  paidAmountUsd: null as number | null,
  amountUsd: null as number | null,
  expireTime: null as string | null,
  reason: ''
})

const isZeroAmount = computed(() => form.paidAmountUsd === 0)
const rules = computed<FormRules>(() => ({
  paidAmountUsd: [{ required: true, type: 'number', min: 0, message: '请填写实付金额（USD）', trigger: ['blur', 'change'] }],
  amountUsd: [{ required: true, type: 'number', min: 0.000001, message: '到账金额至少为 $0.000001', trigger: ['blur', 'change'] }],
  reason: isZeroAmount.value
    ? [
        { required: true, message: '实付金额为 0 时备注必填', trigger: ['blur', 'change'] },
        { min: 5, message: '至少填写 5 个字符', trigger: ['blur', 'change'] }
      ]
    : []
}))

const utc8Formatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23'
})

const formatUtc8DateTime = (date: Date) => {
  const parts = Object.fromEntries(
    utc8Formatter.formatToParts(date).filter((part) => part.type !== 'literal').map((part) => [part.type, part.value])
  ) as Record<string, string>
  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
}

const parseUtc8DateTime = (value: string) => {
  const match = value.trim().match(/^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/)
  if (!match) return Number.NaN
  const [, year, month, day, hour, minute, second] = match
  return Date.UTC(Number(year), Number(month) - 1, Number(day), Number(hour) - 8, Number(minute), Number(second))
}

const clearValidation = async (fields: Array<'paidAmountUsd' | 'amountUsd' | 'reason'>) => {
  await nextTick()
  formRef.value?.clearValidate(fields)
}

const handlePaidAmountChange = (value: number | undefined) => {
  isAmountAutoCalc.value = true
  if (value != null && value >= 0) form.amountUsd = value
  if (value !== 0) void clearValidation(['reason'])
}

const pickAmount = (amount: number) => {
  form.paidAmountUsd = amount
  form.amountUsd = amount
  isAmountAutoCalc.value = true
  void clearValidation(['paidAmountUsd', 'amountUsd', 'reason'])
}

const pickCreditedAmount = (amount: number) => {
  form.amountUsd = amount
  isAmountAutoCalc.value = false
  void clearValidation(['amountUsd'])
}

const setExpireDays = (days: number) => {
  form.expireTime = formatUtc8DateTime(new Date(Date.now() + days * 24 * 60 * 60 * 1000))
}

const submit = async () => {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  if (isZeroAmount.value) {
    try {
      await ElMessageBox.confirm('实付金额为 $0，确认执行赠送入账？', '金额为零确认', {
        confirmButtonText: '确认赠送',
        cancelButtonText: '取消',
        roundButton: true,
        type: 'warning'
      })
    } catch {
      return
    }
  }

  emit('submit', {
    paidAmountMinor: Math.round((form.paidAmountUsd ?? 0) * 100),
    amountMicroUsd: Math.round((form.amountUsd ?? 0) * 1_000_000),
    note: form.reason || undefined,
    expireTime: form.expireTime ? parseUtc8DateTime(form.expireTime) : null
  })
}

const resetForm = () => {
  form.paidAmountUsd = null
  form.amountUsd = null
  form.expireTime = null
  form.reason = ''
  isAmountAutoCalc.value = true
  formRef.value?.clearValidate()
}
</script>

<style scoped>
.recharge-target {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 20px;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.recharge-target__main,
.recharge-target__balance {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 3px;
}

.recharge-target__label,
.recharge-target__identity,
.recharge-target__balance span,
.recharge-hint {
  color: var(--ds-faint);
  font-size: 11px;
  line-height: 1.5;
}

.recharge-target__main strong {
  overflow: hidden;
  color: var(--ds-ink);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recharge-target__balance {
  flex-shrink: 0;
  align-items: flex-end;
}

.recharge-target__balance strong {
  color: var(--ds-accent);
  font-variant-numeric: tabular-nums;
}

.recharge-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 16px;
}

.recharge-field {
  width: 100%;
}

.recharge-quick-list {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}

.recharge-quick-list--wrap {
  flex-wrap: wrap;
}

.recharge-quick {
  min-height: 26px;
  padding: 3px 9px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-muted);
  cursor: pointer;
  font-size: 12px;
}

.recharge-quick:hover {
  border-color: var(--ds-accent);
  color: var(--ds-accent);
}

.recharge-hint {
  margin: 5px 0 0;
}

.recharge-hint--accent {
  color: var(--ds-accent);
  font-weight: 600;
}

.recharge-hint--danger {
  color: var(--ds-danger);
  font-weight: 600;
}

@media (max-width: 640px) {
  .recharge-fields {
    grid-template-columns: minmax(0, 1fr);
    gap: 0;
  }
}
</style>
