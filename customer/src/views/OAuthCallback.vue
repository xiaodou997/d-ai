<template>
  <div class="callback-container">
    <div class="callback-card">
      <template v-if="error">
        <svg class="error-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <line x1="15" y1="9" x2="9" y2="15"/>
          <line x1="9" y1="9" x2="15" y2="15"/>
        </svg>
        <p class="error-text">{{ error }}</p>
        <button class="retry-btn" @click="goLogin">返回登录</button>
      </template>
      <template v-else>
        <span class="loading-spinner"></span>
        <p class="loading-text">正在完成登录...</p>
      </template>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { exchangeCode } from '@/api/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const error = ref('')

const goLogin = () => router.replace('/login')

onMounted(async () => {
  const code = route.query.code
  const state = route.query.state
  const savedState = sessionStorage.getItem('oauth_state')
  const redirectUri = sessionStorage.getItem('oauth_redirect_uri')

  if (!code) {
    error.value = '登录失败：未收到授权码'
    return
  }
  if (savedState && state !== savedState) {
    error.value = '登录失败：state 验证不通过'
    return
  }

  sessionStorage.removeItem('oauth_state')
  sessionStorage.removeItem('oauth_redirect_uri')

  try {
    const callbackURI = redirectUri || (window.location.origin + '/oauth/callback')
    const tokenData = await exchangeCode(code, callbackURI)
    authStore.setAuth(tokenData)
    await authStore.fetchUserInfo()
    ElMessage({ message: '欢迎回来，' + authStore.username, type: 'success', plain: true, duration: 3000 })
    router.replace('/dashboard')
  } catch (err) {
    error.value = '登录失败，请重试'
  }
})
</script>

<style scoped>
.callback-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #0f766e 0%, #14b8a6 50%, #0d9488 100%);
}

.callback-card {
  background: rgba(255,255,255,0.95);
  border-radius: 20px;
  padding: 48px 56px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  box-shadow: 0 25px 50px rgba(0,0,0,0.2);
  min-width: 280px;
}

.loading-text, .error-text {
  font-size: 15px;
  color: #475569;
  margin: 0;
}

.error-icon {
  width: 48px;
  height: 48px;
  color: #ef4444;
}

.retry-btn {
  padding: 10px 24px;
  background: linear-gradient(135deg, #14b8a6, #0f766e);
  color: white;
  border: none;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
}

.loading-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(20,184,166,0.2);
  border-top-color: #14b8a6;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
