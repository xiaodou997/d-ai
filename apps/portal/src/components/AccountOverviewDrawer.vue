<template>
  <DsDrawer
    :open="open"
    :title="`${accountTypeLabel}账户`"
    :subtitle="`${accountName || accountId} · ${accountId}`"
    width="720px"
    @close="emit('close')"
  >
    <div v-loading="loading" class="account-drawer">
      <DsEmpty v-if="error" title="账户信息加载失败" :description="error">
        <template #action>
          <el-button type="primary" @click="fetchBalance">重新加载</el-button>
        </template>
      </DsEmpty>

      <template v-else-if="balance">
        <section class="account-drawer__hero" :class="{ 'is-debt': hasDebt }">
          <div>
            <p>净余额</p>
            <strong>{{ formatUSD(netBalance) }}</strong>
          </div>
          <DsTag :tone="hasDebt ? 'danger' : 'positive'">
            {{ hasDebt ? '存在未结透支' : '账户正常' }}
          </DsTag>
        </section>

        <dl class="account-drawer__metrics">
          <div><dt>可用余额</dt><dd>{{ formatUSD(balance.availableUsd) }}</dd></div>
          <div><dt>长期有效</dt><dd>{{ formatUSD(balance.permanentUsd) }}</dd></div>
          <div><dt>限时余额</dt><dd>{{ formatUSD(balance.timedUsd) }}</dd></div>
          <div><dt>剩余余额</dt><dd>{{ formatUSD(balance.remainingUsd) }}</dd></div>
          <div><dt>累计消耗</dt><dd>{{ formatUSD(balance.usedUsd) }}</dd></div>
        </dl>

        <div v-if="hasDebt" class="account-drawer__debt">
          <span>未结透支</span>
          <strong>-{{ formatMicroUSD(balance.outstandingDebtMicroUsd) }}</strong>
          <p>后续充值会优先抵扣该透支，结清后的剩余金额进入额度包。</p>
        </div>

        <section class="account-drawer__packages">
          <div class="account-drawer__section-head">
            <div>
              <h3>有效额度包</h3>
              <p>按到期时间排序，扣费时优先扣减更早到期的额度。</p>
            </div>
            <el-button :icon="RefreshRight" circle text aria-label="刷新账户" @click="fetchBalance" />
          </div>
          <DsTable
            :frame="false"
            :columns="packageColumns"
            :rows="balance.balanceLots || []"
            row-key="balanceLotId"
            empty-title="暂无有效额度包"
          >
            <template #cell-balance="{ row }">
              <span class="account-drawer__package-balance">
                {{ formatUSD(row.remainingUsd) }} / {{ formatUSD(row.totalUsd) }}
              </span>
            </template>
            <template #cell-expiresAt="{ row }">
              <span>{{ row.expiresAt ? formatTime(row.expiresAt) : '永久有效' }}</span>
            </template>
            <template #cell-source="{ row }">
              <span>{{ sourceLabel(row.source) }}</span>
            </template>
          </DsTable>
        </section>
      </template>
    </div>
  </DsDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RefreshRight } from '@element-plus/icons-vue'
import { platformAdminApi } from '@/api/platformAdmin'
import type { AccountBalanceOutput } from '@/api/types/admin'
import { DsDrawer, DsEmpty, DsTable, DsTag, type DsTableColumn } from '@/shared/ui'

const props = defineProps<{
  open: boolean
  accountType: 1 | 2
  accountId: string
  accountName?: string
}>()

const emit = defineEmits<{ close: [] }>()
const loading = ref(false)
const error = ref('')
const balance = ref<AccountBalanceOutput | null>(null)
let requestVersion = 0

const accountTypeLabel = computed(() => props.accountType === 1 ? '租户' : '用户')
const debtUsd = computed(() => (balance.value?.outstandingDebtMicroUsd || 0) / 1_000_000)
const netBalance = computed(() => (balance.value?.availableUsd || 0) - debtUsd.value)
const hasDebt = computed(() => debtUsd.value > 0)

const packageColumns: DsTableColumn[] = [
  { key: 'balanceLotId', title: '额度包 ID', mono: true },
  { key: 'balance', title: '剩余 / 总量', align: 'right', width: 150 },
  { key: 'expiresAt', title: '有效期', width: 170 },
  { key: 'source', title: '来源', width: 110 }
]

const formatUSD = (value?: number | null) => `$${Number(value || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`
const formatMicroUSD = (value?: number | null) => formatUSD(Number(value || 0) / 1_000_000)
const formatTime = (value: string | number) => new Date(value).toLocaleString('zh-CN', { hour12: false })
const sourceLabel = (source: string) => ({
  ADMIN_RECHARGE: '平台充值',
  TENANT_RECHARGE: '租户充值',
  ONLINE_TOPUP: '在线充值',
  USER_TOPUP_INCOME: '用户充值收入',
  REFUND: '退款返还'
} as Record<string, string>)[source] || source || '其他'

const fetchBalance = async () => {
  if (!props.open || !props.accountId) return
  const currentVersion = ++requestVersion
  loading.value = true
  error.value = ''
  balance.value = null
  try {
    const result = await platformAdminApi.getAccountBalance({
      accountType: props.accountType,
      accountId: props.accountId,
      detail: true
    })
    if (currentVersion === requestVersion) balance.value = result
  } catch (caught) {
    if (currentVersion === requestVersion) {
      error.value = caught instanceof Error ? caught.message : '请稍后重试'
    }
  } finally {
    if (currentVersion === requestVersion) loading.value = false
  }
}

watch(
  () => [props.open, props.accountType, props.accountId] as const,
  ([open]) => {
    if (open) void fetchBalance()
    else requestVersion++
  },
  { immediate: true }
)
</script>

<style scoped>
.account-drawer {
  min-height: 260px;
}

.account-drawer__hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 20px;
  border-left: 3px solid var(--ds-positive);
  background: var(--ds-positive-soft);
}

.account-drawer__hero.is-debt {
  border-left-color: var(--ds-danger);
  background: var(--ds-danger-soft);
}

.account-drawer__hero p,
.account-drawer__section-head p,
.account-drawer__debt p {
  margin: 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.account-drawer__hero strong {
  display: block;
  margin-top: 4px;
  color: var(--ds-ink);
  font-size: 30px;
  font-variant-numeric: tabular-nums;
}

.account-drawer__hero.is-debt strong {
  color: var(--ds-danger);
}

.account-drawer__hero small {
  margin-left: 5px;
  color: var(--ds-muted);
  font-size: 13px;
}

.account-drawer__metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 20px 0 0;
  border-top: 1px solid var(--ds-line);
  border-left: 1px solid var(--ds-line);
}

.account-drawer__metrics div {
  padding: 13px 15px;
  border-right: 1px solid var(--ds-line);
  border-bottom: 1px solid var(--ds-line);
}

.account-drawer__metrics dt {
  color: var(--ds-muted);
  font-size: 11px;
}

.account-drawer__metrics dd {
  margin: 5px 0 0;
  color: var(--ds-ink);
  font-size: 15px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.account-drawer__debt {
  margin-top: 18px;
  padding: 13px 15px;
  border: 1px solid var(--ds-danger);
  border-radius: var(--ds-radius-control);
  background: var(--ds-danger-soft);
}

.account-drawer__debt span {
  color: var(--ds-muted);
  font-size: 11px;
}

.account-drawer__debt strong {
  display: block;
  margin: 2px 0;
  color: var(--ds-danger);
}

.account-drawer__packages {
  margin-top: 24px;
}

.account-drawer__section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 12px;
}

.account-drawer__section-head h3 {
  margin: 0 0 2px;
  color: var(--ds-ink);
  font-size: 14px;
}

.account-drawer__package-balance {
  font-variant-numeric: tabular-nums;
}

@media (max-width: 640px) {
  .account-drawer__metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
