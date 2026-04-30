<template>
  <div class="login-container">
    <!-- 背景装饰 -->
    <div class="background">
      <div class="bg-gradient"></div>
      <div class="bg-pattern"></div>
      <div class="bg-circles">
        <div class="circle circle-1"></div>
        <div class="circle circle-2"></div>
        <div class="circle circle-3"></div>
        <div class="circle circle-4"></div>
        <div class="circle circle-5"></div>
      </div>
    </div>

    <!-- 登录卡片 -->
    <div class="login-card">
      <div class="card-content">
        <!-- 管理员徽章 -->
        <div class="role-badge">
          <span class="badge">
            <svg class="badge-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            管理员专用通道
          </span>
        </div>

        <!-- Logo 区域 -->
        <div class="logo-section">
          <div class="logo-wrapper">
            <div class="logo-avatar">
              <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" width="32" height="32">
                <path d="M13 10V3L4 14h7v7l9-11h-7z" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </div>
            <div class="logo-glow"></div>
          </div>
          <h1 class="title">Uni AI API</h1>
          <p class="subtitle">AI Gateway 管理后台</p>
        </div>

        <!-- SSO 跳转提示 -->
        <div class="sso-redirect">
          <span class="loading-spinner"></span>
          <span>正在跳转到统一登录...</span>
        </div>

        <!-- 权限说明 -->
        <div class="tips-section">
          <div class="tip-title">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            <span>管理员权限</span>
          </div>
          <div class="tip-list">
            <div class="tip-item">
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
              </svg>
              <span>系统管理与配置</span>
            </div>
            <div class="tip-item">
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
              </svg>
              <span>用户与租户治理</span>
            </div>
            <div class="tip-item">
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
              </svg>
              <span>审计日志与监控</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()

const URM_BASE_URL = import.meta.env.VITE_URM_BASE_URL || ''
const APP_KEY = import.meta.env.VITE_URM_APP_KEY || ''

onMounted(() => {
  if (authStore.isAuthenticated()) return
  if (!URM_BASE_URL || !APP_KEY) return

  const callbackURL = window.location.origin + '/oauth/callback'
  const state = crypto.randomUUID()
  sessionStorage.setItem('oauth_state', state)
  sessionStorage.setItem('oauth_redirect_uri', callbackURL)

  const params = new URLSearchParams({
    client_id: APP_KEY,
    redirect_uri: callbackURL,
    state,
  })
  window.location.href = URM_BASE_URL + '/login?' + params.toString()
})
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  position: relative;
  overflow: hidden;
}

/* 背景 */
.background {
  position: absolute;
  inset: 0;
  z-index: 0;
}

.bg-gradient {
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, #6b21a8 0%, #8b5cf6 50%, #7c3aed 100%);
}

.bg-pattern {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255,255,255,0.05) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,0.05) 1px, transparent 1px);
  background-size: 50px 50px;
}

.bg-circles {
  position: absolute;
  inset: 0;
  overflow: hidden;
}

.circle {
  position: absolute;
  border-radius: 50%;
  background: rgba(255,255,255,0.1);
  animation: float 20s infinite ease-in-out;
}
.circle-1 { width: 300px; height: 300px; top: -100px; left: -100px; animation-delay: 0s; }
.circle-2 { width: 200px; height: 200px; top: 20%; right: -50px; animation-delay: 4s; }
.circle-3 { width: 150px; height: 150px; bottom: 10%; left: 10%; animation-delay: 8s; }
.circle-4 { width: 100px; height: 100px; bottom: 30%; right: 20%; animation-delay: 12s; }
.circle-5 { width: 250px; height: 250px; top: 50%; left: 50%; animation-delay: 16s; }

@keyframes float {
  0%, 100% { transform: translate(0, 0) scale(1); }
  25%       { transform: translate(20px, -20px) scale(1.05); }
  50%       { transform: translate(-10px, 10px) scale(0.95); }
  75%       { transform: translate(15px, 15px) scale(1.02); }
}

/* 卡片 */
.login-card {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 440px;
  background: rgba(255,255,255,0.95);
  backdrop-filter: blur(20px);
  border-radius: 24px;
  box-shadow: 0 25px 50px rgba(0,0,0,0.2), 0 0 0 1px rgba(255,255,255,0.1);
  animation: slideUp 0.6s ease-out;
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(30px); }
  to   { opacity: 1; transform: translateY(0); }
}

.card-content {
  padding: 40px;
}

/* 角色徽章 */
.role-badge {
  display: flex;
  justify-content: center;
  margin-bottom: 24px;
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  background: rgba(139,92,246,0.12);
  color: #7c3aed;
  border-radius: 20px;
  font-size: 13px;
  font-weight: 500;
  border: 1px solid rgba(139,92,246,0.2);
}

.badge-icon {
  width: 14px;
  height: 14px;
}

/* Logo 区域 */
.logo-section {
  text-align: center;
  margin-bottom: 32px;
}

.logo-wrapper {
  position: relative;
  width: 80px;
  height: 80px;
  margin: 0 auto 20px;
}

.logo-avatar {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: linear-gradient(135deg, #8b5cf6, #6b21a8);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 8px 24px rgba(139,92,246,0.4);
}

.logo-glow {
  position: absolute;
  inset: -10px;
  background: radial-gradient(circle, rgba(139,92,246,0.3) 0%, transparent 70%);
  border-radius: 50%;
  animation: pulse 3s infinite ease-in-out;
}

@keyframes pulse {
  0%, 100% { opacity: 0.5; transform: scale(1); }
  50%       { opacity: 0.9; transform: scale(1.15); }
}

.title {
  font-size: 28px;
  font-weight: 700;
  color: #1e293b;
  margin: 0 0 6px;
  letter-spacing: -0.5px;
}

.subtitle {
  font-size: 14px;
  color: #64748b;
  margin: 0;
}

/* SSO 跳转 */
.sso-redirect {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 20px 0;
  font-size: 14px;
  color: #64748b;
  margin-bottom: 24px;
}

.loading-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(139,92,246,0.3);
  border-top-color: #8b5cf6;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* 功能提示 */
.tips-section {
  padding: 20px;
  background: rgba(139,92,246,0.05);
  border-radius: 12px;
  border: 1px solid rgba(139,92,246,0.1);
}

.tip-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #6b21a8;
  margin-bottom: 14px;
}

.tip-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.tip-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: #475569;
}

.check-icon {
  width: 15px;
  height: 15px;
  color: #8b5cf6;
  flex-shrink: 0;
}

@media (max-width: 480px) {
  .login-card { max-width: 100%; }
  .card-content { padding: 28px 24px; }
  .title { font-size: 24px; }
}
</style>
