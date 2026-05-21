<template>
  <aside class="w-64 bg-white border-r border-slate-50 flex flex-col h-full">
    <!-- Logo 区域 -->
    <div class="h-20 flex items-center px-6 border-b border-slate-50">
      <div class="w-8 h-8 bg-primary-500 rounded-lg flex items-center justify-center mr-3 shadow-lg shadow-primary-200">
        <el-icon color="#fff" :size="20"><User /></el-icon>
      </div>
      <span class="text-xl font-bold tracking-tight text-slate-800">
        URM<span class="text-primary-500">用户中心</span>
      </span>
    </div>

    <!-- 菜单导航 -->
    <nav class="flex-1 overflow-y-auto py-4 px-3 custom-scrollbar">
      <el-menu :default-active="$route.path" :router="true" class="modern-menu border-none">
        <!-- 概览 -->
        <div class="menu-divider">概览</div>
        <el-menu-item index="/dashboard">
          <el-icon><TrendCharts /></el-icon>
          <span>工作台</span>
        </el-menu-item>

        <!-- AI Gateway -->
        <div class="menu-divider">AI Gateway</div>
        <el-menu-item index="/ai/models">
          <el-icon><Grid /></el-icon>
          <span>可用模型</span>
        </el-menu-item>
        <el-menu-item index="/ai/api-keys">
          <el-icon><Key /></el-icon>
          <span>我的 API Key</span>
        </el-menu-item>

        <template v-for="group in urmMenuGroups" :key="group.title">
          <div class="menu-divider">{{ group.title }}</div>
          <el-menu-item
            v-for="item in group.items"
            :key="item.key"
            :index="getUrmCustomerPagePath(item)"
          >
            <el-icon><component :is="getMenuIcon(item.icon)" /></el-icon>
            <span>{{ item.title }}</span>
          </el-menu-item>
        </template>
      </el-menu>
    </nav>

    <!-- 底部用户信息 -->
    <div class="p-4 border-t border-slate-50">
      <div class="flex items-center p-3 bg-slate-50 rounded-2xl cursor-pointer hover:bg-slate-100 transition-all group" @click="handleLogout">
        <el-avatar :size="32" class="bg-primary-100 text-primary-600 font-bold flex-shrink-0">
          {{ authStore.username?.[0]?.toUpperCase() || 'U' }}
        </el-avatar>
        <div class="ml-3 overflow-hidden flex-1">
          <p class="text-xs font-bold text-slate-800 truncate">{{ authStore.username || '用户' }}</p>
          <p class="text-[10px] text-slate-400 truncate">终端用户</p>
        </div>
        <el-icon class="text-slate-300 group-hover:text-rose-500 transition-colors"><SwitchButton /></el-icon>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { onMounted, shallowRef } from 'vue'
import {
  User,
  Wallet,
  DataLine,
  List,
  Setting,
  SwitchButton,
  Key,
  TrendCharts,
  Grid
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { ElMessageBox, ElMessage } from 'element-plus'
import {
  getUrmCustomerMenuGroups,
  getUrmCustomerPagePath,
  loadUrmCustomerManifest
} from '@/remote/urmCustomerManifest'

const authStore = useAuthStore()
const urmMenuGroups = shallowRef([])

const menuIcons = {
  Wallet,
  DataLine,
  List,
  Setting
}

const getMenuIcon = (name) => menuIcons[name] || Setting

onMounted(async () => {
  try {
    const manifest = await loadUrmCustomerManifest()
    urmMenuGroups.value = getUrmCustomerMenuGroups(manifest)
  } catch (error) {
    console.error('加载 URM 菜单失败:', error)
  }
})

const handleLogout = () => {
  ElMessageBox.confirm('确定要退出登录吗？', '提示', {
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

<style scoped>
.modern-menu :deep(.el-menu-item) {
  height: 48px;
  line-height: 48px;
  margin: 4px 0;
  border-radius: 12px;
  color: #64748b;
  font-weight: 500;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.modern-menu :deep(.el-menu-item:hover) {
  background-color: #f8fafc;
  color: #334155;
}

.modern-menu :deep(.el-menu-item.is-active) {
  background-color: #ecfeff;
  color: #06b6d4;
  font-weight: 700;
}

.modern-menu :deep(.el-menu-item.is-active::before) {
  content: '';
  position: absolute;
  left: 0;
  top: 12px;
  bottom: 12px;
  width: 4px;
  background-color: #06b6d4;
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
