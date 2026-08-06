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
      <div v-if="targetCredits != null" class="recharge-target__balance">
        <span>当前积分</span>
        <strong>{{ targetCredits.toLocaleString() }}</strong>
      </div>
    </div>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <div class="recharge-fields">
        <el-form-item label="实付金额（元）" prop="paidAmountYuan">
          <el-input-number
            v-model="form.paidAmountYuan"
            :min="0"
            :precision="2"
            :step="100"
            :controls="false"
            class="recharge-field"
            @change="handlePaidAmountChange"
          />
          <div class="recharge-quick-list">
            <button v-for="amount in [100, 500, 1000]" :key="amount" type="button" class="recharge-quick" @click="pickAmount(amount)">
              ¥{{ amount }}
            </button>
          </div>
          <p class="recharge-hint">输入金额后按 1 元 = 100 积分自动换算</p>
        </el-form-item>

        <el-form-item label="到账积分" prop="creditAmount">
          <el-input-number
            v-model="form.creditAmount"
            :min="1"
            :precision="0"
            :step="1000"
            :controls="false"
            class="recharge-field"
            @change="isCreditAutoCalc = false"
          />
          <div class="recharge-quick-list">
            <button v-for="amount in [10000, 50000, 100000]" :key="amount" type="button" class="recharge-quick" @click="pickCredits(amount)">
              +{{ amount.toLocaleString() }}
            </button>
          </div>
          <p class="recharge-hint" :class="{ 'recharge-hint--accent': isCreditAutoCalc && (form.paidAmountYuan ?? 0) > 0 }">
            {{ isCreditAutoCalc && (form.paidAmountYuan ?? 0) > 0 ? '已自动换算，可手动修改' : '可独立设置到账积分' }}
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
        <p class="recharge-hint">按北京时间填写；留空则永久有效，限时积分到期后自动失效。</p>
      </el-form-item>

      <el-form-item label="备注" prop="reason">
        <el-input
          v-model="form.reason"
          type="textarea"
          :rows="3"
          :placeholder="isZeroAmount ? '实付金额为 0，请详细说明免费充值原因（必填）' : '充值备注（可选）'"
          maxlength="500"
          show-word-limit
        />
        <p v-if="isZeroAmount" class="recharge-hint recharge-hint--danger">实付金额为 ¥0 时，备注至少填写 5 个字符</p>
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
  targetCredits?: number | null
  submitting?: boolean
}>(), {
  title: '账户充值',
  targetCredits: null,
  submitting: false
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  submit: [payload: RechargeFormPayload]
}>()

const formRef = ref<FormInstance>()
const isCreditAutoCalc = ref(true)
const form = reactive({
  paidAmountYuan: null as number | null,
  creditAmount: null as number | null,
  expireTime: null as string | null,
  reason: ''
})

const isZeroAmount = computed(() => form.paidAmountYuan === 0)
const rules = computed<FormRules>(() => ({
  paidAmountYuan: [{ required: true, type: 'number', min: 0, message: '请填写实付金额（元）', trigger: ['blur', 'change'] }],
  creditAmount: [{ required: true, type: 'number', min: 1, message: '到账积分至少为 1', trigger: ['blur', 'change'] }],
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

const clearValidation = async (fields: Array<'paidAmountYuan' | 'creditAmount' | 'reason'>) => {
  await nextTick()
  formRef.value?.clearValidate(fields)
}

const handlePaidAmountChange = (value: number | undefined) => {
  isCreditAutoCalc.value = true
  if (value != null && value >= 0) form.creditAmount = Math.round(value * 100)
  if (value !== 0) void clearValidation(['reason'])
}

const pickAmount = (amount: number) => {
  form.paidAmountYuan = amount
  form.creditAmount = amount * 100
  isCreditAutoCalc.value = true
  void clearValidation(['paidAmountYuan', 'creditAmount', 'reason'])
}

const pickCredits = (amount: number) => {
  form.creditAmount = amount
  isCreditAutoCalc.value = false
  void clearValidation(['creditAmount'])
}

const setExpireDays = (days: number) => {
  form.expireTime = formatUtc8DateTime(new Date(Date.now() + days * 24 * 60 * 60 * 1000))
}

const submit = async () => {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  if (isZeroAmount.value) {
    try {
      await ElMessageBox.confirm('实付金额为 ¥0，确认执行免费充值？', '金额为零确认', {
        confirmButtonText: '确认免费充值',
        cancelButtonText: '取消',
        roundButton: true,
        type: 'warning'
      })
    } catch {
      return
    }
  }

  emit('submit', {
    paidAmount: Math.round((form.paidAmountYuan ?? 0) * 100),
    creditAmount: form.creditAmount ?? 0,
    note: form.reason || undefined,
    expireTime: form.expireTime ? parseUtc8DateTime(form.expireTime) : null
  })
}

const resetForm = () => {
  form.paidAmountYuan = null
  form.creditAmount = null
  form.expireTime = null
  form.reason = ''
  isCreditAutoCalc.value = true
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
