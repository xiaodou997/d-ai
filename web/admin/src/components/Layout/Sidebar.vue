<template>
  <aside
    class="w-64 bg-white border-r border-slate-50 flex flex-col h-full shadow-soft-xl z-20"
  >
    <!-- Logo 区域 -->
    <div class="h-20 flex items-center px-6 border-bottom border-slate-50">
      <div
        class="w-8 h-8 bg-primary-500 rounded-lg flex items-center justify-center mr-3 shadow-lg shadow-primary-200"
      >
        <el-icon color="#fff" :size="20"><Management /></el-icon>
      </div>
      <span class="text-xl font-bold tracking-tight text-slate-800">
        Uni<span class="text-primary-500">AI API</span>
      </span>
    </div>

    <!-- 菜单导航 -->
    <nav class="flex-1 overflow-y-auto py-4 px-3 custom-scrollbar">
      <el-menu
        :default-active="$route.path"
        :router="true"
        class="modern-menu border-none"
      >
        <!-- 数据监控 -->
        <div v-if="authStore.isPlatformAdmin" class="menu-divider">数据监控</div>
        <el-menu-item v-if="authStore.isPlatformAdmin" index="/dashboard">
          <el-icon><Odometer /></el-icon>
          <span>控制概览</span>
        </el-menu-item>
        <!-- 业务管理 -->
        <div class="menu-divider">{{ authStore.isPlatformAdmin ? '业务管理' : '工作台' }}</div>
        <el-menu-item v-if="authStore.isPlatformAdmin" index="/tenants">
          <el-icon><OfficeBuilding /></el-icon>
          <span>租户管理</span>
        </el-menu-item>
        <el-menu-item v-if="authStore.isPlatformAdmin" index="/users">
          <el-icon><User /></el-icon>
          <span>终端用户</span>
        </el-menu-item>
        <el-menu-item index="/ai-gateway">
          <el-icon><Cpu /></el-icon>
          <span>{{ authStore.isPlatformAdmin ? 'AI 网关' : '我的 AI 网关' }}</span>
        </el-menu-item>
      </el-menu>
    </nav>

    <!-- 底部用户信息 -->
    <div class="p-4 border-t border-slate-50">
      <div
        class="flex items-center p-3 bg-slate-50 rounded-2xl cursor-pointer hover:bg-slate-100 transition-all group"
        @click="handleLogout"
      >
        <el-avatar :size="32" class="bg-primary-100 text-primary-600 font-bold">
          {{ authStore.username?.[0]?.toUpperCase() || 'A' }}
        </el-avatar>
        <div class="ml-3 overflow-hidden flex-1">
          <p class="text-xs font-bold text-slate-800 truncate">
            {{ authStore.username || 'Admin' }}
          </p>
          <p class="text-[10px] text-slate-400 truncate">{{ authStore.roleName }}</p>
        </div>
        <el-icon
          class="text-slate-300 group-hover:text-rose-500 transition-colors"
        >
          <SwitchButton />
        </el-icon>
      </div>
    </div>
  </aside>
</template>

<script setup>
import {
  Management,
  Odometer,
  OfficeBuilding,
  User,
  Cpu,
  SwitchButton
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { ElMessageBox, ElMessage } from 'element-plus'

const authStore = useAuthStore()

const handleLogout = () => {
  ElMessageBox.confirm('您确定要退出 Uni AI API 管理后台吗？', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
    roundButton: true
  })
    .then(async () => {
      await authStore.logout()
      ElMessage.success('已安全退出')
    })
    .catch(() => {})
}
</script>

<style scoped lang="scss">
.modern-menu {
  :deep(.el-menu-item) {
    height: 48px;
    line-height: 48px;
    margin: 4px 0;
    border-radius: 12px;
    color: #64748b;
    font-weight: 500;
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);

    &:hover {
      background-color: #f8fafc;
      color: #334155;
    }

    &.is-active {
      background-color: #faf5ff;
      color: #8b5cf6;
      font-weight: 700;

      &::before {
        content: '';
        position: absolute;
        left: 0;
        top: 12px;
        bottom: 12px;
        width: 4px;
        background-color: #8b5cf6;
        border-radius: 0 4px 4px 0;
      }
    }

    .el-icon {
      margin-right: 12px;
      font-size: 18px;
    }
  }
}

.menu-divider {
  padding: 16px 12px 8px;
  font-size: 10px;
  font-weight: 900;
  color: #cbd5e1;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.custom-scrollbar {
  scrollbar-width: thin;
  scrollbar-color: #e2e8f0 transparent;

  &::-webkit-scrollbar {
    width: 4px;
  }
  &::-webkit-scrollbar-thumb {
    background-color: #e2e8f0;
    border-radius: 10px;
  }
}
</style>
