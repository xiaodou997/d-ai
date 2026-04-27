<template>
  <div class="w-full h-full flex justify-between items-center px-6">
    <!-- 左侧：面包屑 / 标题 -->
    <div class="flex items-center">
      <div class="flex items-center space-x-2 text-slate-400 mr-4">
        <el-icon :size="18"><HomeFilled /></el-icon>
        <span>/</span>
      </div>
      <h2 class="text-sm font-semibold text-slate-700 tracking-tight">
        {{ currentRouteTitle }}
      </h2>
    </div>

    <!-- 右侧：用户信息 + 退出 -->
    <div class="flex items-center space-x-4">
      <!-- 租户名称标签 -->
      <div class="hidden md:flex items-center bg-primary-50 border border-primary-100 rounded-full px-3 py-1.5">
        <el-icon class="text-primary-400 mr-1.5" :size="12"><OfficeBuilding /></el-icon>
        <span class="text-xs font-bold text-primary-600">{{ authStore.tenantName || authStore.tenantId }}</span>
      </div>

      <div class="w-px h-6 bg-slate-100"></div>

      <!-- 用户下拉 -->
      <el-dropdown @command="handleCommand" trigger="click">
        <div class="flex items-center cursor-pointer hover:bg-slate-50 p-1.5 rounded-xl transition-colors">
          <div class="w-8 h-8 rounded-full bg-gradient-to-tr from-primary-500 to-sky-400 flex items-center justify-center text-white text-xs font-bold mr-2 shadow-sm">
            {{ authStore.username?.[0]?.toUpperCase() || 'T' }}
          </div>
          <span class="text-sm font-medium text-slate-700 hidden sm:inline-block mr-1">
            {{ authStore.username || '租户管理员' }}
          </span>
          <el-icon class="text-slate-400"><ArrowDown /></el-icon>
        </div>
        <template #dropdown>
          <el-dropdown-menu class="modern-dropdown">
            <div class="px-4 py-3 border-b border-slate-50">
              <p class="text-xs text-slate-400">当前身份</p>
              <p class="text-sm font-semibold text-slate-700">租户管理员</p>
              <p class="text-xs text-slate-400 mt-0.5">{{ authStore.tenantName || authStore.tenantId }}</p>
            </div>
            <el-dropdown-item command="changePassword" class="mt-1">
              <el-icon><Lock /></el-icon>修改密码
            </el-dropdown-item>
            <el-dropdown-item command="logout" class="text-danger-500">
              <el-icon><SwitchButton /></el-icon>退出登录
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>

  <!-- 修改密码对话框 -->
  <el-dialog
    v-model="showPasswordDialog"
    title="修改密码"
    width="400px"
    :close-on-click-modal="false"
    @close="resetPasswordForm"
  >
    <el-form
      ref="passwordFormRef"
      :model="passwordForm"
      :rules="passwordRules"
      label-width="90px"
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
    </el-form>
    <template #footer>
      <el-button @click="showPasswordDialog = false">取消</el-button>
      <el-button type="primary" :loading="passwordLoading" @click="handleChangePassword">确认修改</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, reactive } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowDown,
  SwitchButton,
  HomeFilled,
  OfficeBuilding,
  Lock
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { changePassword } from '@/api/auth'

const route = useRoute()
const authStore = useAuthStore()

const currentRouteTitle = computed(() => {
  return route.meta.title || '控制台'
})

const showPasswordDialog = ref(false)
const passwordLoading = ref(false)
const passwordFormRef = ref(null)
const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})
const passwordRules = {
  oldPassword: [{ required: true, message: '请输入旧密码', trigger: 'blur' }],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少6位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== passwordForm.newPassword) {
          callback(new Error('两次密码输入不一致'))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ]
}

const resetPasswordForm = () => {
  passwordForm.oldPassword = ''
  passwordForm.newPassword = ''
  passwordForm.confirmPassword = ''
  passwordFormRef.value?.clearValidate()
}

const handleChangePassword = async () => {
  const valid = await passwordFormRef.value?.validate().catch(() => false)
  if (!valid) return
  passwordLoading.value = true
  try {
    await changePassword(passwordForm.oldPassword, passwordForm.newPassword)
    ElMessage.success('密码修改成功，请重新登录')
    showPasswordDialog.value = false
    await authStore.logout()
  } catch (error) {
    ElMessage.error(error?.message || '密码修改失败')
  } finally {
    passwordLoading.value = false
  }
}

const handleCommand = (command) => {
  if (command === 'logout') {
    handleLogout()
  } else if (command === 'changePassword') {
    showPasswordDialog.value = true
  }
}

const handleLogout = async () => {
  try {
    await ElMessageBox.confirm('您确定要退出 URM 租户中心吗？', '确认退出', {
      confirmButtonText: '立即退出',
      cancelButtonText: '再想想',
      type: 'warning',
      roundButton: true
    })
    await authStore.logout()
  } catch (error) {
    if (error !== 'cancel' && error?.message) {
      ElMessage.error('退出失败：' + error.message)
    }
  }
}
</script>

<style scoped>
.text-danger-500 {
  color: #f43f5e !important;
}

:deep(.modern-dropdown) {
  border-radius: 16px;
  padding: 8px;
  border: 1px solid #f1f5f9;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}
</style>
