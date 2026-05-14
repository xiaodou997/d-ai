<template>
  <div>
    <div class="space-y-6">
      <div class="bg-white p-6 rounded-xl border border-slate-50 shadow-soft">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h1 class="text-2xl font-black text-slate-800 tracking-tight">终端用户</h1>
            <p class="text-slate-400 text-sm font-medium mt-1">管理属于本租户的所有终端用户</p>
          </div>
          <div class="flex items-center gap-3">
            <el-input
              v-model="keyword"
              placeholder="搜索用户名 / 邮箱 / 手机"
              clearable
              class="modern-search !w-64"
              @keyup.enter="handleSearch"
              @clear="handleSearch"
            >
              <template #prefix>
                <el-icon class="text-slate-400"><Search /></el-icon>
              </template>
            </el-input>
            <el-button type="primary" class="!rounded-2xl font-bold" @click="handleSearch">搜索</el-button>
          </div>
        </div>
      </div>

      <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
        <div v-if="loading" class="flex items-center justify-center py-16">
          <el-icon class="text-slate-300 animate-spin" :size="36"><Loading /></el-icon>
        </div>

        <el-table v-else :data="userList" empty-text="暂无数据" row-class-name="table-row-hover">
          <el-table-column prop="userId" label="用户 ID" width="110" show-overflow-tooltip />
          <el-table-column prop="username" label="用户名" width="130" />
          <el-table-column prop="email" label="邮箱" min-width="160" show-overflow-tooltip />
          <el-table-column prop="status" label="状态" width="80">
            <template #default="{ row }">
              <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small" round>
                {{ row.status === 1 ? '正常' : '禁用' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="createdTime" label="注册时间" width="170">
            <template #default="{ row }">{{ formatTime(row.createdTime) }}</template>
          </el-table-column>
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <div class="flex items-center">
                <el-button
                  v-if="row.status === 1"
                  type="danger"
                  link
                  @click="handleDisable(row)"
                >停用</el-button>
                <el-button
                  v-else
                  type="success"
                  link
                  @click="handleEnable(row)"
                >启用</el-button>
                <el-button
                  type="warning"
                  link
                  @click="handleResetPassword(row)"
                >重置密码</el-button>
                <el-button
                  type="primary"
                  link
                  @click="openRecharge(row)"
                >充值</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <div class="flex justify-end px-6 py-4 border-t border-slate-50" v-if="total > 0">
          <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :total="total"
            :page-sizes="[10, 20, 50]"
            layout="total, sizes, prev, pager, next"
            background
            @change="fetchUsers"
          />
        </div>
      </div>
    </div>

    <el-dialog v-model="showRechargeDialog" title="用户积分充值" width="520px" :close-on-click-modal="false" @close="resetRechargeForm">
      <el-form ref="rechargeFormRef" :model="rechargeForm" :rules="rechargeRules" label-width="90px">
        <el-form-item label="用户">
          <span class="text-slate-700 font-semibold">{{ rechargeTarget?.username }}</span>
          <span class="text-slate-400 text-xs ml-2">{{ rechargeTarget?.userId }}</span>
        </el-form-item>

        <el-form-item label="实付金额" prop="paidAmountYuan">
          <el-input-number
            v-model="rechargeForm.paidAmountYuan"
            :min="0"
            :precision="2"
            :step="100"
            style="width: 100%"
            @change="isCreditAutoCalc = true"
          />
          <div class="flex gap-2 mt-2">
            <el-tag
              v-for="q in [100, 500, 1000]"
              :key="q"
              @click="handleAmountQuickPick(q)"
              class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-primary-500 hover:!text-white transition-all"
            >¥{{ q }}</el-tag>
          </div>
          <p class="text-[10px] text-slate-400 mt-1">输入金额（元），自动计算到账积分（1元=100积分）</p>
        </el-form-item>

        <el-form-item label="到账积分" prop="creditAmount">
          <el-input-number
            v-model="rechargeForm.creditAmount"
            :min="1"
            :precision="0"
            :step="1000"
            style="width: 100%"
            @change="isCreditAutoCalc = false"
          />
          <div class="flex gap-2 mt-2">
            <el-tag
              v-for="q in [10000, 50000, 100000]"
              :key="q"
              @click="handleCreditQuickPick(q)"
              class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-primary-500 hover:!text-white transition-all"
            >+{{ q.toLocaleString() }}</el-tag>
          </div>
          <p class="text-[10px] text-slate-400 mt-1">
            <span v-if="isCreditAutoCalc && rechargeForm.paidAmountYuan > 0" class="text-primary-500 font-medium">
              已自动按 1元=100积分 计算，可手动修改
            </span>
            <span v-else>默认按 1元 = 100积分 自动换算，可手动修改</span>
          </p>
        </el-form-item>

        <el-form-item label="有效期">
          <el-date-picker
            v-model="rechargeForm.expireDate"
            type="datetime"
            placeholder="永久有效"
            value-format="YYYY-MM-DD HH:mm:ss"
            style="width: 100%"
            :disabled-date="d => d.getTime() < Date.now()"
          />
          <div class="flex gap-2 mt-2 flex-wrap">
            <el-tag
              v-for="days in [7, 30, 90, 180, 365]"
              :key="days"
              @click="setExpireDays(days)"
              class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-primary-500 hover:!text-white transition-all"
            >{{ days }}天</el-tag>
            <el-tag
              @click="clearExpire"
              class="cursor-pointer border-none !bg-slate-100 !text-slate-500 hover:!bg-slate-300 hover:!text-white transition-all"
            >永久有效</el-tag>
          </div>
          <p class="text-[10px] text-primary-500 mt-1 font-bold" v-if="rechargeForm.expireDate">
            该笔积分将在到期后自动失效，扣费时优先消耗。
          </p>
        </el-form-item>

        <el-form-item label="备注" prop="reason">
          <el-input v-model="rechargeForm.reason" :placeholder="isZeroAmount ? '实付金额为0，请详细说明免费充值原因（必填）' : '充值备注（可选）'" maxlength="200" show-word-limit />
          <p v-if="isZeroAmount" class="text-[10px] text-rose-500 mt-1 font-bold">实付金额为 ¥0 时，备注说明为必填项</p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRechargeDialog = false">取消</el-button>
        <el-button type="primary" :loading="rechargeLoading" @click="submitRecharge">确认充值</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive, watch, computed } from 'vue'
import { Search, Loading } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getUsers, updateUserStatus, resetUserPassword, rechargeUser } from '@/api/tenant'
import dayjs from 'dayjs'

const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const userList = ref([])

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const fetchUsers = async () => {
  loading.value = true
  try {
    const res = await getUsers({ keyword: keyword.value, page: page.value, size: pageSize.value })
    userList.value = Array.isArray(res) ? res : (res?.list || res?.records || [])
    total.value = res?.total || userList.value.length
  } catch (e) {
    console.error('获取用户列表失败:', e)
    userList.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  page.value = 1
  fetchUsers()
}

const handleDisable = async (row) => {
  try {
    await ElMessageBox.confirm(`确定要停用用户「${row.username}」吗？停用后该用户将立即无法登录。`, '确认停用', {
      confirmButtonText: '确认停用',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    })
    await updateUserStatus(row.userId, 'disabled')
    ElMessage.success('用户已停用')
    fetchUsers()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e?.message || '操作失败')
  }
}

const handleEnable = async (row) => {
  try {
    await ElMessageBox.confirm(`确定要启用用户「${row.username}」吗？`, '确认启用', {
      confirmButtonText: '确认启用',
      cancelButtonText: '取消',
      type: 'info'
    })
    await updateUserStatus(row.userId, 'active')
    ElMessage.success('用户已启用')
    fetchUsers()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e?.message || '操作失败')
  }
}

const handleResetPassword = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要将用户「${row.username}」的密码重置为 123456 吗？`,
      '确认重置密码',
      {
        confirmButtonText: '确认重置',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    await resetUserPassword(row.userId)
    ElMessage.success('密码已重置为 123456')
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e?.message || '操作失败')
  }
}

const showRechargeDialog = ref(false)
const rechargeLoading = ref(false)
const rechargeTarget = ref(null)
const rechargeFormRef = ref(null)
const isCreditAutoCalc = ref(true)
const rechargeForm = reactive({
  paidAmountYuan: null,
  creditAmount: null,
  expireDate: null,
  reason: ''
})
const isZeroAmount = computed(() => rechargeForm.paidAmountYuan === 0)
const rechargeRules = computed(() => ({
  paidAmountYuan: [{ required: true, type: 'number', min: 0, message: '请填写实付金额（元）', trigger: 'blur' }],
  creditAmount: [{ required: true, type: 'number', min: 1, message: '到账积分至少为 1', trigger: 'blur' }],
  reason: isZeroAmount.value
    ? [{ required: true, message: '实付金额为0时，备注说明必填', trigger: 'blur' }, { min: 5, message: '至少5个字符', trigger: 'blur' }]
    : []
}))

watch(() => rechargeForm.paidAmountYuan, (val) => {
  if (isCreditAutoCalc.value && val != null && val >= 0) {
    rechargeForm.creditAmount = Math.round(val * 100)
  }
})

const openRecharge = (row) => {
  rechargeTarget.value = row
  showRechargeDialog.value = true
}

const setExpireDays = (days) => {
  const d = new Date()
  d.setDate(d.getDate() + days)
  rechargeForm.expireDate = d.toISOString().slice(0, 19).replace('T', ' ')
}
const clearExpire = () => { rechargeForm.expireDate = null }

const handleAmountQuickPick = (amount) => {
  rechargeForm.paidAmountYuan = amount
  isCreditAutoCalc.value = true
}

const handleCreditQuickPick = (amount) => {
  rechargeForm.creditAmount = amount
  isCreditAutoCalc.value = false
}

const resetRechargeForm = () => {
  rechargeForm.paidAmountYuan = null
  rechargeForm.creditAmount = null
  rechargeForm.expireDate = null
  rechargeForm.reason = ''
  isCreditAutoCalc.value = true
  rechargeFormRef.value?.clearValidate()
}

const submitRecharge = async () => {
  const valid = await rechargeFormRef.value?.validate().catch(() => false)
  if (!valid) return
  if (rechargeForm.paidAmountYuan === 0) {
    try {
      await ElMessageBox.confirm(
        `实付金额为 ¥0，确认对「${rechargeTarget.value.username}」执行免费充值？`,
        '金额为零确认',
        { confirmButtonText: '确认免费充值', cancelButtonText: '取消', roundButton: true, type: 'warning' }
      )
    } catch { return }
  }
  rechargeLoading.value = true
  try {
    const payload = {
      userId: rechargeTarget.value.userId,
      paidAmount: Math.round(rechargeForm.paidAmountYuan * 100),
      creditAmount: rechargeForm.creditAmount,
      reason: rechargeForm.reason || undefined,
      expireTime: rechargeForm.expireDate ? new Date(rechargeForm.expireDate).getTime() : null
    }
    await rechargeUser(payload)
    ElMessage.success(`已成功为「${rechargeTarget.value.username}」充值 ${rechargeForm.creditAmount} 积分`)
    showRechargeDialog.value = false
    fetchUsers()
  } catch (e) {
    ElMessage.error(e?.message || '充值失败')
  } finally {
    rechargeLoading.value = false
  }
}

onMounted(() => {
  fetchUsers()
})
</script>

<style scoped>
:deep(.modern-search .el-input__wrapper) {
  border-radius: 14px !important;
  border: 1px solid #f1f5f9 !important;
  box-shadow: none !important;
  background: #f8fafc;
}
:deep(.modern-search .el-input__wrapper.is-focus) {
  border-color: #0ea5e9 !important;
  background: #fff;
  box-shadow: 0 0 0 3px rgba(14, 165, 233, 0.1) !important;
}
:deep(.table-row-hover:hover td) {
  background-color: #f8fafc !important;
}
.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
