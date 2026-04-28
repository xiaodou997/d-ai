<script setup>
import { onMounted, reactive, shallowRef } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Coin, Refresh, Select } from '@element-plus/icons-vue'
import { rechargeTenant } from '@/api/finance'
import { getAccountInfo, queryTenants } from '@/api/tenant'

const route = useRoute()
const formRef = shallowRef(null)
const loading = shallowRef(false)
const searchLoading = shallowRef(false)
const searchOptions = shallowRef([])
const selectedTenant = shallowRef(null)
const history = shallowRef([])

const form = reactive({
  tenantId: '',
  paidAmountYuan: null,
  creditAmount: null,
  expireTime: null,
  reason: ''
})

const rules = {
  tenantId: [{ required: true, message: '请选择租户', trigger: 'change' }],
  paidAmountYuan: [
    { required: true, message: '请输入实付金额', trigger: 'blur' },
    { type: 'number', min: 0, message: '金额不能小于 0', trigger: 'blur' }
  ],
  creditAmount: [
    { required: true, message: '请输入到账积分', trigger: 'blur' },
    { type: 'number', min: 1, message: '积分必须大于 0', trigger: 'blur' }
  ],
  reason: [
    { required: true, message: '请输入充值原因', trigger: 'blur' },
    { min: 5, message: '至少 5 个字符', trigger: 'blur' }
  ]
}

const yuan = (cents) => ((Number(cents) || 0) / 100).toFixed(2)
const credits = (value) => (Number(value) || 0).toLocaleString('zh-CN')
const formatTime = (value) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : ''

const fetchTenantBalance = async (tenantId) => {
  const data = await getAccountInfo({ accountType: 1, accountId: tenantId })
  return data?.availableCredits ?? data?.totalCredits ?? data?.credits ?? 0
}

const setSelectedTenant = async (tenant) => {
  const balance = await fetchTenantBalance(tenant.tenantId)
  selectedTenant.value = {
    id: tenant.tenantId,
    name: tenant.tenantName,
    balance
  }
}

const handleRemoteSearch = async (keyword) => {
  if (!keyword || keyword.length < 2) {
    searchOptions.value = []
    return
  }
  searchLoading.value = true
  try {
    const data = await queryTenants({ keyword, page: 1, size: 10 })
    searchOptions.value = (data.records || []).map((item) => ({
      id: item.tenantId,
      label: `${item.tenantName} · ${item.tenantId}`,
      tenant: item
    }))
  } finally {
    searchLoading.value = false
  }
}

const handleTenantSelect = async (tenantId) => {
  if (!tenantId) {
    selectedTenant.value = null
    return
  }
  const option = searchOptions.value.find((item) => item.id === tenantId)
  if (option) {
    await setSelectedTenant(option.tenant)
  }
}

const setQuickAmount = (yuanAmount, creditAmount) => {
  form.paidAmountYuan = yuanAmount
  form.creditAmount = creditAmount
}

const setExpireDays = (days) => {
  const next = new Date()
  next.setDate(next.getDate() + days)
  form.expireTime = next.getTime()
}

const resetForm = () => {
  formRef.value?.resetFields()
  selectedTenant.value = null
  searchOptions.value = []
}

const submitRecharge = async () => {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid || !selectedTenant.value) return

  const paidAmount = Math.round((Number(form.paidAmountYuan) || 0) * 100)
  const creditAmount = Number(form.creditAmount) || 0
  try {
    if (paidAmount === 0) {
      await ElMessageBox.confirm('实付金额为 0 元，确认执行免费入账？', '金额确认', {
        confirmButtonText: '确认免费充值',
        cancelButtonText: '取消',
        type: 'warning'
      })
    }

    await ElMessageBox.confirm(
      `确认给租户「${selectedTenant.value.name}」充值 ${credits(creditAmount)} 积分？`,
      '租户充值确认',
      {
        confirmButtonText: '确认入账',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }

  loading.value = true
  try {
    const result = await rechargeTenant({
      customerId: selectedTenant.value.id,
      paidAmount,
      creditAmount,
      reason: form.reason,
      expireTime: form.expireTime || null
    })
    ElMessage.success('充值成功')
    history.value.unshift({
      tenantName: selectedTenant.value.name,
      paidAmount,
      creditAmount,
      reason: form.reason,
      operationTime: result.operationTime || Date.now()
    })
    selectedTenant.value.balance = await fetchTenantBalance(selectedTenant.value.id)
    form.paidAmountYuan = null
    form.creditAmount = null
    form.expireTime = null
    form.reason = ''
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  const { tenantId, tenantName } = route.query
  if (tenantId) {
    const tenant = { tenantId: String(tenantId), tenantName: String(tenantName || tenantId) }
    form.tenantId = tenant.tenantId
    searchOptions.value = [{ id: tenant.tenantId, label: `${tenant.tenantName} · ${tenant.tenantId}`, tenant }]
    await setSelectedTenant(tenant)
  }
})
</script>

<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex items-center justify-between gap-4 mb-6">
        <div>
          <h2 class="text-lg font-black text-slate-800">租户充值</h2>
          <p class="text-sm text-slate-400 mt-1">平台管理员快捷入口，仅支持租户账户充值；退款、用户充值和全量流水仍在 URM 处理。</p>
        </div>
        <el-icon class="text-primary-500" :size="28"><Coin /></el-icon>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="目标租户" prop="tenantId">
          <el-select
            v-model="form.tenantId"
            filterable
            remote
            reserve-keyword
            clearable
            :remote-method="handleRemoteSearch"
            :loading="searchLoading"
            placeholder="搜索租户名称 / 租户 ID"
            class="w-full"
            @change="handleTenantSelect"
          >
            <el-option v-for="item in searchOptions" :key="item.id" :label="item.label" :value="item.id" />
          </el-select>
        </el-form-item>

        <div v-if="selectedTenant" class="tenant-card">
          <div>
            <span>租户</span>
            <strong>{{ selectedTenant.name }}</strong>
            <small>{{ selectedTenant.id }}</small>
          </div>
          <div class="text-right">
            <span>当前可用积分</span>
            <strong>{{ credits(selectedTenant.balance) }}</strong>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-5">
          <el-form-item label="实付金额（元）" prop="paidAmountYuan">
            <el-input-number v-model="form.paidAmountYuan" :min="0" :precision="2" :step="100" class="w-full" />
          </el-form-item>
          <el-form-item label="到账积分" prop="creditAmount">
            <el-input-number v-model="form.creditAmount" :min="1" :precision="0" :step="1000" class="w-full" />
          </el-form-item>
        </div>

        <div class="quick-row">
          <el-button @click="setQuickAmount(100, 10000)">100 元 / 10,000 积分</el-button>
          <el-button @click="setQuickAmount(500, 50000)">500 元 / 50,000 积分</el-button>
          <el-button @click="setQuickAmount(1000, 100000)">1,000 元 / 100,000 积分</el-button>
        </div>

        <el-form-item label="有效期">
          <el-date-picker
            v-model="form.expireTime"
            type="datetime"
            value-format="x"
            clearable
            placeholder="不选表示永久有效"
            class="w-full"
          />
        </el-form-item>
        <div class="quick-row">
          <el-button @click="setExpireDays(30)">30 天</el-button>
          <el-button @click="setExpireDays(90)">90 天</el-button>
          <el-button @click="setExpireDays(365)">365 天</el-button>
          <el-button @click="form.expireTime = null">永久有效</el-button>
        </div>

        <el-form-item label="充值原因" prop="reason">
          <el-input v-model="form.reason" type="textarea" :rows="4" maxlength="500" show-word-limit placeholder="请输入业务原因，便于审计追踪" />
        </el-form-item>

        <div class="flex justify-end gap-3 pt-4 border-t border-slate-50">
          <el-button :icon="Refresh" @click="resetForm">清空</el-button>
          <el-button type="primary" :icon="Select" :loading="loading" :disabled="!selectedTenant" @click="submitRecharge">确认入账</el-button>
        </div>
      </el-form>
    </section>

    <section v-if="history.length" class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <h3 class="text-base font-black text-slate-800 mb-4">本次操作记录</h3>
      <el-table :data="history" border stripe>
        <el-table-column prop="tenantName" label="租户" min-width="160" />
        <el-table-column label="实付金额" width="120" align="right">
          <template #default="{ row }">¥ {{ yuan(row.paidAmount) }}</template>
        </el-table-column>
        <el-table-column label="到账积分" width="140" align="right">
          <template #default="{ row }">{{ credits(row.creditAmount) }}</template>
        </el-table-column>
        <el-table-column prop="reason" label="原因" min-width="220" show-overflow-tooltip />
        <el-table-column label="时间" width="180">
          <template #default="{ row }">{{ formatTime(row.operationTime) }}</template>
        </el-table-column>
      </el-table>
    </section>
  </div>
</template>

<style scoped>
.tenant-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid #e2e8f0;
  border-radius: 14px;
  background: #f8fafc;
  margin-bottom: 18px;
  padding: 18px;
}

.tenant-card span,
.tenant-card small {
  display: block;
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.tenant-card strong {
  display: block;
  color: #0f172a;
  font-size: 20px;
  font-weight: 900;
}

.quick-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: -8px 0 18px;
}
</style>
