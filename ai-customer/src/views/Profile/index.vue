<template>
  <div class="space-y-6">
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <h1 class="text-2xl font-black text-slate-800 tracking-tight">个人中心</h1>
      <p class="text-slate-400 text-sm font-medium mt-1">查看个人信息、修改密码</p>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div class="p-6 border-b border-slate-50">
        <h2 class="text-base font-bold text-slate-800">基本信息</h2>
      </div>
      <div class="p-6 space-y-4">
        <div class="flex items-center justify-between py-3 border-b border-slate-50">
          <span class="text-sm text-slate-500">用户名</span>
          <span class="text-sm font-semibold text-slate-800">{{ userInfo.username || '—' }}</span>
        </div>
        <div class="flex items-center justify-between py-3 border-b border-slate-50">
          <span class="text-sm text-slate-500">用户 ID</span>
          <span class="text-sm font-mono text-slate-600">{{ userInfo.userId || '—' }}</span>
        </div>
        <div class="flex items-center justify-between py-3 border-b border-slate-50">
          <span class="text-sm text-slate-500">注册时间</span>
          <span class="text-sm text-slate-600">{{ userInfo.createdTime ? formatTime(userInfo.createdTime) : '—' }}</span>
        </div>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <div class="p-6 border-b border-slate-50">
        <h2 class="text-base font-bold text-slate-800">修改密码</h2>
      </div>
      <div class="p-6">
        <el-form
          ref="passwordFormRef"
          :model="passwordForm"
          :rules="passwordRules"
          label-width="100px"
          class="max-w-md"
        >
          <el-form-item label="旧密码" prop="oldPassword">
            <el-input
              v-model="passwordForm.oldPassword"
              type="password"
              placeholder="请输入旧密码"
              show-password
            />
          </el-form-item>
          <el-form-item label="新密码" prop="newPassword">
            <el-input
              v-model="passwordForm.newPassword"
              type="password"
              placeholder="请输入新密码（至少6位）"
              show-password
            />
          </el-form-item>
          <el-form-item label="确认密码" prop="confirmPassword">
            <el-input
              v-model="passwordForm.confirmPassword"
              type="password"
              placeholder="请再次输入新密码"
              show-password
            />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" class="rounded-2xl! font-bold" @click="handleChangePassword">
              确认修改
            </el-button>
          </el-form-item>
        </el-form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { changePassword } from '@/api/auth'
import dayjs from 'dayjs'

const authStore = useAuthStore()
const loading = ref(false)
const passwordFormRef = ref(null)

const userInfo = reactive({
  username: '',
  userId: '',
  createdTime: null
})

const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const validateConfirmPassword = (rule, value, callback) => {
  if (value !== passwordForm.newPassword) {
    callback(new Error('两次输入的密码不一致'))
  } else {
    callback()
  }
}

const passwordRules = {
  oldPassword: [{ required: true, message: '请输入旧密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码长度至少为6位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
}

const formatTime = (ts) => {
  if (!ts) return '—'
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss')
}

const handleChangePassword = async () => {
  if (!passwordFormRef.value) return

  await passwordFormRef.value.validate(async (valid) => {
    if (!valid) return

    loading.value = true
    try {
      await changePassword(passwordForm.oldPassword, passwordForm.newPassword)
      ElMessage.success('密码修改成功，请重新登录')
      passwordForm.oldPassword = ''
      passwordForm.newPassword = ''
      passwordForm.confirmPassword = ''
      await authStore.logout()
    } catch (e) {
      ElMessage.error(e?.message || '修改失败，请检查旧密码是否正确')
    } finally {
      loading.value = false
    }
  })
}

onMounted(() => {
  userInfo.username = authStore.username || ''
  userInfo.userId = authStore.userId || ''
})
</script>
