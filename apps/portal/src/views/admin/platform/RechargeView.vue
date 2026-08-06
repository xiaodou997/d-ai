<!--
  手动充值中心 — 1:1 搬运自 v1/platform/platform-admin/src/views/Finance/Recharge.vue。
  管理员仅对「租户账户」充值（accountType=1 / packageType=1）；完整页面（非抽屉）。
  适配：axios api → platformAdminApi（listTenants 远程搜索 / getAccountBalance / createRecharge）；
       V1 的 reason 映射到 v4 的 note 列；expireTime 走 v4 已支持的 ExpireTime。
  入口：从租户列表/详情的「充值」按钮带 query(tenantId/tenantName) 跳入预填。
  重构：页面骨架迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       表单置于面板 body）,「本次操作审计流水」小表迁移至 DsTable（客户端数组、不分页）;
       充值表单仍为 element-plus，UTC+8 手动日期解析逻辑保持不变。
-->
<template>
  <div class="recharge-page">
    <PortalPagePanel
      :icon="Wallet"
      :breadcrumbs="[
        { label: '用户中心' },
        { label: '数据监控' },
        { label: '账户全景', to: '/accounts/overview' },
        { label: '充值' }
      ]"
      description="管理员对租户账户手动入账；带有效期的充值会单独存入有效积分包，到期后自动失效。"
    >
      <div class="rc-body">
      <el-form
        ref="rechargeFormRef"
        :model="rechargeForm"
        :rules="rechargeRules"
        label-position="top"
        size="large"
      >
        <!-- 目标搜索 -->
        <el-form-item label="1. 目标账号搜索" prop="accountId">
          <el-select
            v-model="rechargeForm.accountId"
            filterable
            remote
            reserve-keyword
            placeholder="搜索租户名称 / 租户 ID"
            :remote-method="handleRemoteSearch"
            :loading="searchLoading"
            class="rc-field--full"
            @change="handleTargetSelect"
          >
            <el-option v-for="item in searchOptions" :key="item.id" :label="item.label" :value="item.id" />
          </el-select>
          <p class="rc-hint">管理员仅可对租户账户充值，用户充值由租户操作</p>
        </el-form-item>

        <!-- 身份卡片 -->
        <transition name="el-fade-in">
          <div v-if="selectedTarget" class="rc-identity">
            <div class="rc-identity__avatar">{{ selectedTarget.name?.[0]?.toUpperCase() }}</div>
            <div class="rc-identity__grid">
              <div>
                <p class="rc-identity__label">目标账户</p>
                <p class="rc-identity__name">{{ selectedTarget.name }}</p>
              </div>
              <div class="rc-identity__right">
                <p class="rc-identity__label">当前积分余额</p>
                <p class="rc-identity__credits" :class="{ 'is-negative': selectedTarget.credits < 0 }">
                  {{ selectedTarget.credits.toLocaleString() }} 积分
                </p>
              </div>
            </div>
          </div>
        </transition>

        <!-- 充值参数 -->
        <div v-if="selectedTarget" class="rc-params">
          <div class="rc-params__col">
            <el-form-item label="2. 实付金额（元）" prop="paidAmountYuan">
              <el-input-number
                v-model="rechargeForm.paidAmountYuan"
                :min="0"
                :precision="2"
                :step="100"
                :controls="false"
                class="rc-field--full"
                @change="isCreditAutoCalc = true"
              />
              <div class="rc-chips">
                <button v-for="q in [100, 500, 1000]" :key="q" type="button" class="rc-chip" @click="handleAmountQuickPick(q)">+{{ q }}</button>
              </div>
              <p class="rc-hint">实际收款金额，输入后自动计算到账积分（1元=100积分）</p>
            </el-form-item>

            <el-form-item label="3. 到账积分" prop="creditAmount">
              <el-input-number
                v-model="rechargeForm.creditAmount"
                :min="1"
                :precision="0"
                :step="1000"
                :controls="false"
                class="rc-field--full"
                @change="isCreditAutoCalc = false"
              />
              <div class="rc-chips">
                <button v-for="q in [10000, 50000, 100000]" :key="q" type="button" class="rc-chip" @click="handleCreditQuickPick(q)">+{{ q.toLocaleString() }}</button>
              </div>
              <p class="rc-hint">
                <span v-if="isCreditAutoCalc && (rechargeForm.paidAmountYuan || 0) > 0" class="rc-hint--accent">
                  已自动按 1元=100积分 计算，可手动修改
                </span>
                <span v-else>
                  实际到账积分数，可独立设置支持促销比例
                </span>
              </p>
            </el-form-item>

            <el-form-item label="4. 设定有效期 (可选)" prop="expireTime">
              <el-date-picker
                v-model="rechargeForm.expireTime"
                type="datetime"
                placeholder="永久有效"
                value-format="YYYY-MM-DD HH:mm:ss"
                class="rc-field--full"
                :disabled-date="(d: Date) => d.getTime() < Date.now()"
              />
              <!-- 快捷选项 -->
              <div class="rc-chips">
                <button v-for="days in [7, 30, 90, 180, 365]" :key="days" type="button" class="rc-chip" @click="setExpireDays(days)">{{ days }}天</button>
                <button type="button" class="rc-chip" @click="clearExpire">永久有效</button>
              </div>
              <p class="rc-hint">按北京时间填写，留空则充入永久积分账户。</p>
              <p v-if="rechargeForm.expireTime" class="rc-hint rc-hint--accent">
                <el-icon class="mr-1"><Clock /></el-icon>
                该笔积分到期后自动失效；消费时系统会优先扣减更早到期的有效积分，永久积分后扣。
              </p>
            </el-form-item>
          </div>

          <el-form-item label="5. 备注说明与凭证" prop="reason">
            <el-input
              v-model="rechargeForm.reason"
              type="textarea"
              :rows="6"
              :placeholder="isZeroAmount ? '实付金额为0，请详细说明免费充值原因（必填）' : '请输入充值原因（可选）'"
              maxlength="500"
              show-word-limit
            />
            <p v-if="isZeroAmount" class="rc-hint rc-hint--danger">
              <el-icon class="mr-1"><WarningFilled /></el-icon>
              实付金额为 ¥0 时，备注说明为必填项
            </p>
          </el-form-item>
        </div>

        <div class="rc-actions">
          <el-button @click="handleReset">清空</el-button>
          <el-button type="primary" :loading="loading" :disabled="!selectedTarget" class="px-10! font-bold" @click="handleRecharge">
            确认执行入账
          </el-button>
        </div>
      </el-form>
      </div>
    </PortalPagePanel>

    <!-- 会话操作审计 -->
    <transition name="el-zoom-in-top">
      <PortalContentCard v-if="rechargeHistory.length > 0" title="本次操作审计流水" body-padding="none">
        <DsTable
          :frame="false"
          :columns="auditColumns"
          :rows="rechargeHistory"
          row-key="seq"
          empty-title="暂无审计流水"
        >
          <template #cell-paidAmount="{ row }">
            <span class="rc-audit-amount">¥ {{ (row.paidAmount / 100).toFixed(2) }}</span>
          </template>
          <template #cell-creditAmount="{ row }">
            <span class="rc-audit-credits">{{ row.creditAmount.toLocaleString() }} 积分</span>
          </template>
          <template #cell-type="{ row }">
            <DsTag :tone="row.expireTime ? 'warning' : 'positive'">
              {{ row.expireTime ? '限时' : '永久' }}
            </DsTag>
          </template>
        </DsTable>
      </PortalContentCard>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Clock, WarningFilled } from '@element-plus/icons-vue'
import { useRoute } from 'vue-router'
import { Wallet } from 'lucide-vue-next'
import { PortalContentCard, PortalPagePanel } from '@/platform'
import { DsTable, DsTag, type DsTableColumn } from '@/shared/ui'
import { platformAdminApi } from '@/api/platformAdmin'

const route = useRoute()
const rechargeFormRef = ref<any>(null)
const loading = ref(false)
const searchLoading = ref(false)

// 标记积分是否为自动计算（用户手动修改后则不再自动同步）
const isCreditAutoCalc = ref(true)

const rechargeForm = reactive<{
  accountType: number
  accountId: string
  paidAmountYuan: number | null
  creditAmount: number | null
  reason: string
  expireTime: string | null
}>({ accountType: 1, accountId: '', paidAmountYuan: null, creditAmount: null, reason: '', expireTime: null })

const searchOptions = ref<any[]>([])
const selectedTarget = ref<any>(null)
const rechargeHistory = ref<any[]>([])
// 审计流水为客户端数组且行无天然唯一键，用自增序号作为 DsTable 的 row-key
let historySeq = 0

// 审计流水表列定义（不分页，随会话累积）
const auditColumns: DsTableColumn[] = [
  { key: 'targetName', title: '充值对象', width: 180 },
  { key: 'paidAmount', title: '实付金额', width: 130, align: 'right' },
  { key: 'creditAmount', title: '到账积分', width: 140, align: 'right' },
  { key: 'type', title: '类型', width: 100, align: 'center' },
  { key: 'reason', title: '备注' },
  { key: 'operationTime', title: '时间', width: 180 }
]

// 实付金额为 0 时备注必填
const isZeroAmount = computed(() => rechargeForm.paidAmountYuan === 0)
const UTC8_TIME_ZONE = 'Asia/Shanghai'
const utc8DateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  timeZone: UTC8_TIME_ZONE,
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
    utc8DateTimeFormatter
      .formatToParts(date)
      .filter((part) => part.type !== 'literal')
      .map((part) => [part.type, part.value])
  ) as Record<string, string>

  return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`
}

const parseUtc8DateTime = (value: string) => {
  const match = value.trim().match(/^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/)
  if (!match) return Number.NaN

  const [, year, month, day, hour, minute, second] = match
  return Date.UTC(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour) - 8,
    Number(minute),
    Number(second)
  )
}

const clearRechargeValidation = async (fields: Array<'paidAmountYuan' | 'creditAmount' | 'reason'>) => {
  await nextTick()
  rechargeFormRef.value?.clearValidate(fields)
}

// 动态验证规则
const rechargeRules = computed<any>(() => ({
  accountType: [{ required: true, message: '选择账户类型', trigger: 'change' }],
  accountId: [{ required: true, message: '请搜索并选择目标', trigger: 'change' }],
  paidAmountYuan: [{ required: true, message: '输入实付金额', trigger: ['blur', 'change'] }, { type: 'number', min: 0, message: '金额不能为负数', trigger: ['blur', 'change'] }],
  creditAmount: [{ required: true, message: '输入到账积分', trigger: ['blur', 'change'] }, { type: 'number', min: 1, message: '积分必须大于0', trigger: ['blur', 'change'] }],
  reason: isZeroAmount.value
    ? [{ required: true, message: '实付金额为0时，备注说明必填', trigger: ['blur', 'change'] }, { min: 5, message: '至少5个字符', trigger: ['blur', 'change'] }]
    : []
}))

// 监听实付金额变化，自动计算到账积分（×100）
watch(() => rechargeForm.paidAmountYuan, (newVal) => {
  if (isCreditAutoCalc.value && newVal !== null && newVal >= 0) {
    rechargeForm.creditAmount = Math.round(newVal * 100)
  }
})

watch(isZeroAmount, (value) => {
  if (!value) {
    void clearRechargeValidation(['reason'])
  }
})

// 从路由参数预填租户
onMounted(async () => {
  const tenantId = typeof route.query.tenantId === 'string' ? route.query.tenantId : ''
  const tenantName = typeof route.query.tenantName === 'string' ? route.query.tenantName : ''
  if (tenantId && tenantName) {
    rechargeForm.accountId = tenantId
    let credits = 0
    try {
      const accountData = await platformAdminApi.getAccountBalance({ accountType: 1, accountId: tenantId })
      credits = accountData?.totalCredits ?? 0
    } catch {}
    searchOptions.value = [{ id: tenantId, label: tenantName, fullData: { tenantId, tenantName, credits } }]
    selectedTarget.value = { id: tenantId, name: tenantName, credits }
  }
})

const handleRemoteSearch = async (query: string) => {
  if (query.length < 2) return
  searchLoading.value = true
  try {
    const res = await platformAdminApi.listTenants({ keyword: query, page: 1, size: 10 })
    searchOptions.value = (res.items || []).map((i) => ({ id: i.tenantId, label: i.tenantName, fullData: i }))
  } finally { searchLoading.value = false }
}

const handleTargetSelect = (val: string) => {
  const opt = searchOptions.value.find((o) => o.id === val)
  if (opt) {
    selectedTarget.value = { id: opt.id, name: opt.label, credits: opt.fullData.credits || 0 }
  }
}

// 设置有效期天数
const setExpireDays = (days: number) => {
  rechargeForm.expireTime = formatUtc8DateTime(new Date(Date.now() + days * 24 * 60 * 60 * 1000))
}

// 清除有效期
const clearExpire = () => {
  rechargeForm.expireTime = null
}

// 快速选择金额（恢复自动计算模式）
const handleAmountQuickPick = (amount: number) => {
  rechargeForm.paidAmountYuan = amount
  isCreditAutoCalc.value = true
  void clearRechargeValidation(['paidAmountYuan', 'creditAmount', 'reason'])
}

// 快速选择积分（标记为手动修改）
const handleCreditQuickPick = (amount: number) => {
  rechargeForm.creditAmount = amount
  isCreditAutoCalc.value = false
  void clearRechargeValidation(['creditAmount'])
}

const handleRecharge = async () => {
  if (!rechargeFormRef.value) return
  await rechargeFormRef.value.validate(async (v: boolean) => {
    if (!v) return
    if (rechargeForm.paidAmountYuan === 0) {
      try {
        await ElMessageBox.confirm('实付金额为 ¥0，确认执行免费充值？', '金额为零确认', { confirmButtonText: '确认免费充值', cancelButtonText: '取消', roundButton: true, type: 'warning' })
      } catch { return }
    }
    try {
      await ElMessageBox.confirm(`确认执行${rechargeForm.expireTime ? '【限时】' : '【永久】'}入账操作？`, '财务安全确认', { confirmButtonText: '确定入账', roundButton: true, type: 'warning' })
    } catch { return }

    loading.value = true
    try {
      const paidAmount = Math.round((rechargeForm.paidAmountYuan || 0) * 100)

      // 管理员只对租户充值：packageType=1，tenantId=accountId；reason 落 v4 note 列
      const result = await platformAdminApi.createRecharge({
        packageType: rechargeForm.accountType, // 1=租户
        tenantId: rechargeForm.accountType === 1 ? rechargeForm.accountId : '',
        userId: rechargeForm.accountType === 1 ? '' : rechargeForm.accountId,
        paidAmount,
        creditAmount: rechargeForm.creditAmount || 0,
        note: rechargeForm.reason,
        expireTime: rechargeForm.expireTime ? parseUtc8DateTime(rechargeForm.expireTime) : null
      })
      ElMessage.success('操作成功')
      rechargeHistory.value.unshift({
        seq: ++historySeq,
        targetName: selectedTarget.value.name,
        paidAmount,
        creditAmount: rechargeForm.creditAmount,
        expireTime: rechargeForm.expireTime,
        reason: rechargeForm.reason,
        operationTime: result.orderTime ? new Date(result.orderTime).toLocaleString() : ''
      })
      rechargeForm.paidAmountYuan = null; rechargeForm.creditAmount = null; rechargeForm.reason = ''; rechargeForm.expireTime = null; isCreditAutoCalc.value = true
    } catch (e: any) { ElMessage.error(e?.message || '操作失败') }
    finally { loading.value = false }
  })
}

const handleReset = () => {
  rechargeFormRef.value?.resetFields()
  selectedTarget.value = null
  isCreditAutoCalc.value = true
}
</script>

<style scoped>
.recharge-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.rc-field--full {
  width: 100%;
}

.rc-body {
  padding: 24px;
}

.rc-hint {
  margin: 6px 0 0;
  font-size: 11px;
  line-height: 1.5;
  color: var(--ds-faint);
}

.rc-hint--accent {
  color: var(--ds-accent);
  font-weight: 600;
}

.rc-hint--danger {
  color: var(--ds-danger);
  font-weight: 600;
}

/* 目标身份卡 */
.rc-identity {
  display: flex;
  align-items: center;
  gap: 24px;
  margin: 20px 0;
  padding: 20px 24px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel-muted);
}

.rc-identity__avatar {
  flex-shrink: 0;
  width: 56px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
  font-size: 20px;
  font-weight: 800;
  color: var(--ds-accent);
}

.rc-identity__grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  min-width: 0;
}

.rc-identity__right {
  text-align: right;
}

.rc-identity__label {
  margin: 0;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ds-faint);
}

.rc-identity__name {
  margin: 4px 0 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--ds-ink-soft);
}

.rc-identity__credits {
  margin: 4px 0 0;
  font-size: 18px;
  font-weight: 800;
  color: var(--ds-positive);
}

.rc-identity__credits.is-negative {
  color: var(--ds-danger);
}

/* 参数区 */
.rc-params {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 32px;
}

.rc-params__col {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

/* 快捷选项 chip */
.rc-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.rc-chip {
  padding: 4px 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-pill);
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.rc-chip:hover {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
}

.rc-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid var(--ds-line);
}

/* 审计表数字列 */
.rc-audit-amount {
  font-weight: 700;
  color: var(--ds-ink-soft);
  font-variant-numeric: tabular-nums;
}

.rc-audit-credits {
  font-weight: 800;
  color: var(--ds-positive);
  font-variant-numeric: tabular-nums;
}

@media (max-width: 768px) {
  .rc-params,
  .rc-identity__grid {
    grid-template-columns: 1fr;
  }

  .rc-identity__right {
    text-align: left;
  }
}
</style>
