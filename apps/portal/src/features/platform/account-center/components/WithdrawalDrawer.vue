<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { Landmark } from "lucide-vue-next";

import type { TenantCashAccount } from "@/api/types/tenant";
import { formatCents } from "../model";

const props = defineProps<{
  visible: boolean;
  cash: TenantCashAccount;
  submitting: boolean;
}>();

const emit = defineEmits<{
  close: [];
  submit: [value: { amountYuan: number; accountName: string; bankName: string; accountNo: string; note?: string }];
}>();

const form = reactive({
  amountYuan: null as number | null,
  accountName: "",
  bankName: "",
  accountNo: "",
  note: ""
});

const preview = computed(() => {
  const amount = Math.round(Number(form.amountYuan ?? 0) * 100);
  const fee = Math.ceil((amount * props.cash.withdrawFeeBp) / 10000);
  return { amount, fee, payout: Math.max(0, amount - fee) };
});

const canSubmit = computed(() => (
  preview.value.amount > 0
  && preview.value.amount <= props.cash.available
  && Boolean(form.accountName.trim() && form.bankName.trim() && form.accountNo.trim())
));

function useAllBalance() {
  form.amountYuan = Number((props.cash.available / 100).toFixed(2));
}

function submit() {
  if (!canSubmit.value || !form.amountYuan) return;
  emit("submit", {
    amountYuan: form.amountYuan,
    accountName: form.accountName.trim(),
    bankName: form.bankName.trim(),
    accountNo: form.accountNo.trim(),
    note: form.note.trim() || undefined
  });
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return;
    form.amountYuan = null;
    form.accountName = "";
    form.bankName = "";
    form.accountNo = "";
    form.note = "";
  }
);
</script>

<template>
  <el-drawer :model-value="visible" title="提现" size="min(500px, 100vw)" append-to-body destroy-on-close @close="emit('close')">
    <div class="withdrawal-drawer">
      <div class="withdrawal-balance">
        <Landmark :size="20" />
        <div><span>可提现余额</span><strong>¥{{ formatCents(cash.available) }}</strong></div>
      </div>

      <el-form label-position="top" class="withdrawal-form">
        <el-form-item label="提现金额">
          <div class="amount-row">
            <el-input-number v-model="form.amountYuan" :min="0" :precision="2" :controls="false" placeholder="输入金额" class="amount-row__input" />
            <el-button text @click="useAllBalance">全部提现</el-button>
          </div>
        </el-form-item>

        <div class="withdrawal-preview">
          <span>申请金额 <b>¥{{ formatCents(preview.amount) }}</b></span>
          <span>手续费 <b>¥{{ formatCents(preview.fee) }}</b></span>
          <span class="withdrawal-preview__payout">预计到账 <strong>¥{{ formatCents(preview.payout) }}</strong></span>
        </div>

        <el-form-item label="收款户名"><el-input v-model="form.accountName" autocomplete="name" /></el-form-item>
        <el-form-item label="开户行"><el-input v-model="form.bankName" /></el-form-item>
        <el-form-item label="银行卡号"><el-input v-model="form.accountNo" inputmode="numeric" autocomplete="off" /></el-form-item>
        <el-form-item label="备注（选填）"><el-input v-model="form.note" type="textarea" :rows="2" /></el-form-item>
      </el-form>

      <el-button type="primary" size="large" :loading="submitting" :disabled="!canSubmit" @click="submit">提交提现申请</el-button>
    </div>
  </el-drawer>
</template>

<style scoped>
.withdrawal-drawer,
.withdrawal-form {
  display: flex;
  flex-direction: column;
}

.withdrawal-drawer {
  gap: 18px;
}

.withdrawal-form {
  gap: 2px;
}

.withdrawal-balance {
  display: flex;
  align-items: center;
  gap: 10px;
  border-radius: 8px;
  padding: 14px 16px;
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.withdrawal-balance > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.withdrawal-balance span {
  font-size: 11px;
}

.withdrawal-balance strong {
  color: color-mix(in srgb, var(--ds-positive) 75%, var(--ds-ink));
  font-size: 20px;
}

.amount-row {
  display: grid;
  width: 100%;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
}

.amount-row__input {
  width: 100%;
}

.withdrawal-preview {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
  margin: 0 0 18px;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 12px;
}

.withdrawal-preview span {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}

.withdrawal-preview b,
.withdrawal-preview strong {
  color: var(--ds-ink);
}

.withdrawal-preview__payout {
  grid-column: 1 / -1;
  border-top: 1px solid var(--ds-line);
  padding-top: 8px;
}

.withdrawal-preview__payout strong {
  color: var(--ds-positive);
  font-size: 15px;
}
</style>
