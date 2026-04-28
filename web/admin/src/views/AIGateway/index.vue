<script setup>
import { computed } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const authStore = useAuthStore()

const pageTitle = computed(() => route.meta.title || (authStore.isPlatformAdmin ? 'AI 网关' : '我的 AI 网关'))
const pageDescription = computed(() => {
  if (route.path.endsWith('/providers')) return '维护上游厂商和连接配置。'
  if (route.path.endsWith('/models')) return '维护对外模型、调用配置、销售价和上游成本价。'
  if (route.path.endsWith('/access')) return '管理模型授权与运行时 API Key。'
  if (route.path.endsWith('/usage')) return '查看调用日志、计费单位和积分消耗。'
  if (route.path.endsWith('/limits')) return '维护租户、用户和 Key 级限流策略。'
  if (route.path.endsWith('/audit')) return '追踪 AI Gateway 后台配置变更。'
  return authStore.isPlatformAdmin ? '统一维护 AI Gateway 业务能力。' : '管理你的模型授权、API Key 和调用记录。'
})
</script>

<template>
  <div class="page-container space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="gateway-head">
      <div>
        <p class="gateway-eyebrow">AI Gateway</p>
        <h2 class="gateway-title">{{ pageTitle }}</h2>
        <p class="gateway-subtitle">{{ pageDescription }}</p>
      </div>
      <el-tag type="info" effect="plain">{{ authStore.roleName }}</el-tag>
    </section>

    <RouterView />
  </div>
</template>

<style scoped>
.gateway-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  background: #ffffff;
  padding: 24px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.gateway-eyebrow {
  margin: 0;
  color: #64748b;
  font-size: 12px;
  font-weight: 900;
  text-transform: uppercase;
}

.gateway-title {
  margin: 6px 0 0;
  color: #0f172a;
  font-size: 22px;
  font-weight: 900;
  letter-spacing: 0;
}

.gateway-subtitle {
  margin: 8px 0 0;
  color: #64748b;
  font-size: 14px;
}

@media (max-width: 768px) {
  .gateway-head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
