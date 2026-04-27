<template>
  <div class="flex h-screen w-full bg-slate-50 overflow-hidden">
    <!-- 侧边栏 -->
    <aside class="w-64 flex-shrink-0 z-20 shadow-sm">
      <Sidebar />
    </aside>

    <!-- 右侧主区域 -->
    <div class="flex-1 flex flex-col min-w-0 relative">
      <!-- 顶部导航 -->
      <header class="h-16 flex-shrink-0 bg-white/70 backdrop-blur-xl sticky top-0 z-10 border-b border-slate-100 flex items-center justify-end px-6">
        <div class="flex items-center gap-4">
          <span class="text-sm font-medium text-slate-600">{{ authStore.username || '用户' }}</span>
          <el-button text size="small" class="!text-slate-400" @click="handleLogout">
            <el-icon><SwitchButton /></el-icon>
            退出
          </el-button>
        </div>
      </header>

      <!-- 页面主内容 -->
      <main class="flex-1 overflow-y-auto custom-scrollbar p-6">
        <div class="max-w-[1400px] mx-auto px-4">
          <router-view v-slot="{ Component, route }">
            <transition name="page-fade" mode="out-in" appear>
              <component :is="Component" :key="route.path" />
            </transition>
          </router-view>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup>
import { SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { ElMessageBox, ElMessage } from 'element-plus'
import Sidebar from './Sidebar.vue'

const authStore = useAuthStore()

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
.page-fade-enter-active,
.page-fade-leave-active {
  transition: all 0.3s ease;
}
.page-fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}
.page-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}
.custom-scrollbar {
  scrollbar-width: thin;
  scrollbar-color: #e2e8f0 transparent;
}
.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background-color: #e2e8f0;
  border-radius: 10px;
}
</style>
