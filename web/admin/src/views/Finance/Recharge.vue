<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <el-card class="!rounded-3xl border-none shadow-soft">
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center">
            <div class="w-8 h-8 rounded-2xl bg-primary-50 flex items-center justify-center mr-3 text-primary-500">
              <el-icon :size="18"><Coin /></el-icon>
            </div>
            <span class="text-lg font-bold text-slate-800">手动充值中心</span>
          </div>
          <el-tooltip content="有有效期的充值将作为'资源包'存入，系统扣费时会自动优先消耗最快到期的资产。" placement="left">
            <el-icon class="text-slate-300 cursor-help"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>
      </template>

      <el-form
        ref="rechargeFormRef"
        :model="rechargeForm"
        :rules="rechargeRules"
        label-position="top"
        size="large"
        class="modern-form"
      >
        <!-- 目标搜索 -->
        <div class="mb-8">
          <el-form-item label="1. 目标账号搜索" prop="accountId">
            <el-select
              v-model="rechargeForm.accountId"
              filterable
              remote
              reserve-keyword
              placeholder="搜索租户名称 / 租户 ID"
              :remote-method="handleRemoteSearch"
              :loading="searchLoading"
              class="w-full modern-select"
              @change="handleTargetSelect"
            >
              <el-option v-for="item in searchOptions" :key="item.id" :label="item.label" :value="item.id" />
            </el-select>
            <p class="text-[10px] text-slate-400 mt-1">管理员仅可对租户账户充值，用户充值由租户操作</p>
          </el-form-item>
        </div>

        <!-- 身份卡片 -->
        <transition name="el-fade-in">
          <div v-if="selectedTarget" class="bg-slate-50 border border-slate-100 rounded-[24px] p-6 mb-8 flex items-center gap-6">
            <div class="w-14 h-14 rounded-2xl bg-white shadow-sm flex items-center justify-center text-xl font-black text-primary-500">
              {{ selectedTarget.name?.[0]?.toUpperCase() }}
            </div>
            <div class="flex-1 grid grid-cols-2 gap-4">
              <div>
                <p class="text-[10px] font-bold text-slate-400 uppercase tracking-widest">Target Account</p>
                <p class="text-sm font-bold text-slate-700">{{ selectedTarget.name }}</p>
              </div>
              <div class="text-right">
                <p class="text-[10px] font-bold text-slate-400 uppercase tracking-widest">当前积分余额</p>
                <p class="text-lg font-black" :class="selectedTarget.credits >= 0 ? 'text-emerald-600' : 'text-rose-600'">
                  {{ selectedTarget.credits.toLocaleString() }} 积分
                </p>
              </div>
            </div>
          </div>
        </transition>

        <!-- 充值参数 -->
        <div v-if="selectedTarget" class="grid grid-cols-1 md:grid-cols-2 gap-8 animate-in fade-in duration-500">
          <div class="space-y-6">
            <el-form-item label="2. 实付金额（元）" prop="paidAmountYuan">
              <el-input-number v-model="rechargeForm.paidAmountYuan" :min="0" :precision="2" :step="100" style="width: 100%" />
              <div class="flex gap-2 mt-2">
                <el-tag v-for="q in [100, 500, 1000]" :key="q" @click="rechargeForm.paidAmountYuan = q" class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-primary-500 hover:!text-white transition-all">+{{ q }}</el-tag>
              </div>
              <p class="text-[10px] text-slate-400 mt-1">实际收款金额，发送时自动 ×100 转为分</p>
            </el-form-item>

            <el-form-item label="3. 到账积分" prop="creditAmount">
              <el-input-number v-model="rechargeForm.creditAmount" :min="1" :precision="0" :step="1000" style="width: 100%" />
              <div class="flex gap-2 mt-2">
                <el-tag v-for="q in [10000, 50000, 100000]" :key="q" @click="rechargeForm.creditAmount = q" class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-primary-500 hover:!text-white transition-all">+{{ q.toLocaleString() }}</el-tag>
              </div>
              <p class="text-[10px] text-slate-400 mt-1">实际到账积分数，与金额独立设置可支持促销比例</p>
            </el-form-item>

            <el-form-item label="4. 设定有效期 (可选)" prop="expireTime">
              <el-date-picker
                v-model="rechargeForm.expireTime"
                type="datetime"
                placeholder="永久有效"
                value-format="YYYY-MM-DD HH:mm:ss"
                style="width: 100%"
                :disabled-date="d => d.getTime() < Date.now()"
              />
              <!-- 快捷选项 -->
              <div class="flex gap-2 mt-2 flex-wrap">
                <el-tag
                  v-for="days in [7, 30, 90, 180, 365]"
                  :key="days"
                  @click="setExpireDays(days)"
                  class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-primary-500 hover:!text-white transition-all"
                >
                  {{ days }}天
                </el-tag>
                <el-tag
                  @click="clearExpire"
                  class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-slate-300 hover:!text-white transition-all"
                >
                  永久有效
                </el-tag>
              </div>
              <p class="text-[10px] text-primary-500 mt-2 font-bold" v-if="rechargeForm.expireTime">
                <el-icon class="mr-1"><Clock /></el-icon> 该笔积分将在到期后自动失效，扣费时优先消耗。
              </p>
              <p class="text-[10px] text-slate-400 mt-2 font-medium" v-else>
                不选则充入永久积分账户。
              </p>
            </el-form-item>
          </div>

          <el-form-item label="5. 备注说明与凭证" prop="reason">
            <el-input v-model="rechargeForm.reason" type="textarea" :rows="6" placeholder="请输入充值原因（审计必填）" maxlength="500" show-word-limit />
          </el-form-item>
        </div>

        <div class="flex justify-end gap-3 mt-8 pt-6 border-t border-slate-50">
          <el-button @click="handleReset" class="!rounded-2xl">清空</el-button>
          <el-button type="primary" :loading="loading" :disabled="!selectedTarget" @click="handleRecharge" class="!rounded-2xl !px-10 font-bold shadow-lg shadow-primary-100">
            确认执行入账
          </el-button>
        </div>
      </el-form>
    </el-card>

    <!-- 会话操作审计 -->
    <transition name="el-zoom-in-top">
      <el-card v-if="rechargeHistory.length > 0" class="!rounded-3xl border-none shadow-soft overflow-hidden">
        <template #header>
          <span class="text-base font-bold text-slate-700">本次操作审计流水</span>
        </template>
        <el-table :data="rechargeHistory" border stripe>
          <el-table-column prop="targetName" label="充值对象" width="180" />
          <el-table-column label="实付金额" width="130" align="right">
            <template #default="{ row }">
              <span class="font-bold text-slate-700">¥ {{ (row.paidAmount / 100).toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="到账积分" width="140" align="right">
            <template #default="{ row }">
              <span class="font-black text-emerald-600">{{ row.creditAmount.toLocaleString() }} 积分</span>
            </template>
          </el-table-column>
          <el-table-column label="类型" width="100" align="center">
            <template #default="{ row }">
              <el-tag :type="row.expireTime ? 'warning' : 'success'" size="small" class="font-bold rounded-2xl">
                {{ row.expireTime ? '限时' : '永久' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="reason" label="备注" show-overflow-tooltip />
          <el-table-column prop="operationTime" label="时间" width="180" />
        </el-table>
      </el-card>
    </transition>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Coin, Clock, QuestionFilled } from '@element-plus/icons-vue'
import { useRoute } from 'vue-router'
import { recharge } from '@/api/user'
import { queryTenants, getAccountInfo } from '@/api/tenant'

const route = useRoute()
const rechargeFormRef = ref(null)
const loading = ref(false)
const searchLoading = ref(false)

const rechargeForm = reactive({ accountType: 1, accountId: '', paidAmountYuan: null, creditAmount: null, reason: '', expireTime: null })
const searchOptions = ref([]); const selectedTarget = ref(null); const rechargeHistory = ref([])

const rechargeRules = {
  accountType: [{ required: true, message: '选择账户类型', trigger: 'change' }],
  accountId: [{ required: true, message: '请搜索并选择目标', trigger: 'change' }],
  paidAmountYuan: [{ required: true, message: '输入实付金额', trigger: 'blur' }, { type: 'number', min: 0, message: '金额不能为负数', trigger: 'blur' }],
  creditAmount: [{ required: true, message: '输入到账积分', trigger: 'blur' }, { type: 'number', min: 1, message: '积分必须大于0', trigger: 'blur' }],
  reason: [{ required: true, message: '说明原因', trigger: 'blur' }, { min: 5, message: '至少5个字符', trigger: 'blur' }]
}

// 从路由参数预填租户
onMounted(async () => {
  const { tenantId, tenantName } = route.query
  if (tenantId && tenantName) {
    rechargeForm.accountId = tenantId
    let credits = 0
    try {
      const accountData = await getAccountInfo({ accountType: 1, accountId: tenantId })
      credits = accountData?.totalCredits ?? 0
    } catch {}
    searchOptions.value = [{ id: tenantId, label: tenantName, fullData: { tenantId, tenantName, credits } }]
    selectedTarget.value = { id: tenantId, name: tenantName, credits }
  }
})

const handleRemoteSearch = async (query) => {
  if (query.length < 2) return
  searchLoading.value = true
  try {
    const res = await queryTenants({ keyword: query, page: 1, size: 10 })
    searchOptions.value = res.records.map(i => ({ id: i.tenantId, label: i.tenantName, fullData: i }))
  } finally { searchLoading.value = false }
}

const handleTargetSelect = (val) => {
  const opt = searchOptions.value.find(o => o.id === val)
  if (opt) {
    selectedTarget.value = { id: opt.id, name: opt.label, credits: opt.fullData.credits || 0 }
  }
}

// 设置有效期天数
const setExpireDays = (days) => {
  const date = new Date()
  date.setDate(date.getDate() + days)
  rechargeForm.expireTime = date.toISOString().slice(0, 19).replace('T', ' ')
}

// 清除有效期
const clearExpire = () => {
  rechargeForm.expireTime = null
}

const handleRecharge = async () => {
  if (!rechargeFormRef.value) return
  await rechargeFormRef.value.validate(async (v) => {
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
      const paidAmount = Math.round(rechargeForm.paidAmountYuan * 100)
      
      // 根据账户类型正确组装后端期望的参数
      const requestData = {
        packageType: rechargeForm.accountType,  // 1=租户，2=用户
        customerId: rechargeForm.accountType === 1 ? rechargeForm.accountId : rechargeForm.accountId,
        tenantId: rechargeForm.accountType === 1 ? rechargeForm.accountId : '',  // 租户充值时传租户ID
        paidAmount,
        creditAmount: rechargeForm.creditAmount,
        reason: rechargeForm.reason,
        expireTime: rechargeForm.expireTime ? new Date(rechargeForm.expireTime).getTime() : null,
      }
      
      const result = await recharge(requestData)
      ElMessage.success('操作成功')
      rechargeHistory.value.unshift({
        targetName: selectedTarget.value.name,
        paidAmount,
        creditAmount: rechargeForm.creditAmount,
        expireTime: rechargeForm.expireTime,
        reason: rechargeForm.reason,
        operationTime: result.operationTime
      })
      rechargeForm.paidAmountYuan = null; rechargeForm.creditAmount = null; rechargeForm.reason = ''; rechargeForm.expireTime = null
    } catch (e) { ElMessage.error(e.message || '操作失败') }
    finally { loading.value = false }
  })
}

const handleReset = () => { rechargeFormRef.value?.resetFields(); selectedTarget.value = null }
</script>
