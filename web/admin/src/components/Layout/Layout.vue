<template>
  <div class="flex h-screen w-full bg-slate-50 overflow-hidden">
    <!-- 侧边栏固定宽度 -->
    <aside class="w-64 flex-shrink-0 z-20 shadow-sm">
      <Sidebar />
    </aside>

    <!-- 右侧主区域 -->
    <div class="flex-1 flex flex-col min-w-0 relative">
      <!-- 顶部导航：磨砂玻璃效果 + 吸顶 -->
      <header
        class="h-16 flex-shrink-0 glass-effect sticky top-0 z-10 border-b border-slate-100 flex items-center"
      >
        <Header />
      </header>

      <!-- 页面主内容 -->
      <main class="flex-1 overflow-y-auto custom-scrollbar p-6">
        <div class="max-w-[1800px] mx-auto px-4">
          <router-view v-slot="{ Component }">
            <transition name="page-fade" mode="out-in" appear>
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup>
import Header from './Header.vue'
import Sidebar from './Sidebar.vue'
</script>

<style scoped lang="scss">
/* 页面切换动画 */
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

  &::-webkit-scrollbar {
    width: 6px;
  }
}

.glass-effect {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

/* Tailwind 动画扩展补丁 */
.animate-in {
  animation-fill-mode: both;
}

@keyframes fade-in {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

@keyframes slide-in-from-bottom {
  from {
    transform: translateY(1rem);
  }
  to {
    transform: translateY(0);
  }
}

.fade-in {
  animation-name: fade-in;
}
.slide-in-from-bottom-4 {
  animation-name: slide-in-from-bottom;
}
</style>
