<template>
  <aside class="w-64 bg-white border-r border-slate-50 flex flex-col h-full">
    <!-- Logo 区域 -->
    <div class="h-20 flex items-center px-6 border-b border-slate-50">
      <div
        class="w-8 h-8 bg-primary-500 rounded-lg flex items-center justify-center mr-3 shadow-lg shadow-primary-200"
      >
        <el-icon color="#fff" :size="20"><OfficeBuilding /></el-icon>
      </div>
      <span class="text-xl font-bold tracking-tight text-slate-800">
        URM<span class="text-primary-500">租户中心</span>
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
        <div class="menu-divider">数据监控</div>
        <el-menu-item index="/dashboard">
          <el-icon><Odometer /></el-icon>
          <span>控制概览</span>
        </el-menu-item>

        <!-- 用户管理 -->
        <div class="menu-divider">用户管理</div>
        <el-menu-item index="/users">
          <el-icon><User /></el-icon>
          <span>终端用户</span>
        </el-menu-item>
        <el-menu-item index="/invite-codes">
          <el-icon><Ticket /></el-icon>
          <span>邀请码管理</span>
        </el-menu-item>

        <!-- 财务中心 -->
        <div class="menu-divider">财务中心</div>
        <el-menu-item index="/finance/account">
          <el-icon><Wallet /></el-icon>
          <span>我的账户</span>
        </el-menu-item>
        <el-menu-item index="/finance/transactions">
          <el-icon><DataLine /></el-icon>
          <span>交易流水</span>
        </el-menu-item>
        <el-menu-item index="/finance/user-recharge-records">
          <el-icon><Money /></el-icon>
          <span>用户充值记录</span>
        </el-menu-item>

        <!-- AI Gateway -->
        <div class="menu-divider">AI Gateway</div>
        <el-menu-item index="/ai/models">
          <el-icon><Box /></el-icon>
          <span>已授权模型</span>
        </el-menu-item>
        <el-menu-item index="/ai/prices">
          <el-icon><PriceTag /></el-icon>
          <span>租户定价</span>
        </el-menu-item>
        <el-menu-item index="/ai/api-keys">
          <el-icon><Key /></el-icon>
          <span>API Key</span>
        </el-menu-item>
        <el-menu-item index="/ai/user-consumption">
          <el-icon><TrendCharts /></el-icon>
          <span>用户消耗</span>
        </el-menu-item>
      </el-menu>
    </nav>

    <!-- 底部用户信息 -->
    <div class="p-4 border-t border-slate-50">
      <div
        class="flex items-center p-3 bg-slate-50 rounded-2xl cursor-pointer hover:bg-slate-100 transition-all group"
        @click="handleLogout"
      >
        <el-avatar
          :size="32"
          class="bg-primary-100 text-primary-600 font-bold flex-shrink-0"
        >
          {{ authStore.username?.[0]?.toUpperCase() || 'T' }}
        </el-avatar>
        <div class="ml-3 overflow-hidden flex-1 min-w-0">
          <p class="text-xs font-bold text-slate-800 truncate">
            {{ authStore.username || '租户管理员' }}
          </p>
          <p class="text-[10px] text-slate-400 truncate">
            租户 ID: {{ authStore.tenantId }}
          </p>
        </div>
        <el-icon
          class="text-slate-300 group-hover:text-rose-500 transition-colors flex-shrink-0 ml-2"
        >
          <SwitchButton />
        </el-icon>
      </div>
    </div>
  </aside>
</template>

<script setup>
import {
  OfficeBuilding,
  Odometer,
  User,
  Ticket,
  Wallet,
  DataLine,
  SwitchButton,
  Money,
  Box,
  PriceTag,
  Key,
  TrendCharts
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { ElMessageBox, ElMessage } from 'element-plus'

const authStore = useAuthStore()

const handleLogout = () => {
  ElMessageBox.confirm('您确定要退出 URM 租户中心吗？', '提示', {
    confirmButtonText: '确定退出',
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

<style scoped>
.modern-menu :deep(.el-menu-item) {
  height: 48px;
  line-height: 48px;
  margin: 4px 0;
  border-radius: 12px;
  color: #64748b;
  font-weight: 500;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}

.modern-menu :deep(.el-menu-item:hover) {
  background-color: #f8fafc;
  color: #334155;
}

.modern-menu :deep(.el-menu-item.is-active) {
  background-color: #eff6ff;
  color: #3b82f6;
  font-weight: 700;
}

.modern-menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 12px;
  bottom: 12px;
  width: 4px;
  background-color: #3b82f6;
  border-radius: 0 4px 4px 0;
}

.modern-menu :deep(.el-icon) {
  margin-right: 12px;
  font-size: 18px;
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
}

.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background-color: #e2e8f0;
  border-radius: 10px;
}
</style>
