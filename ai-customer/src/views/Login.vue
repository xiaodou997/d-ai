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

    <!-- 登录/注册卡片 -->
    <div class="login-card">
      <div class="card-content">
        <!-- 徽章 -->
        <div class="role-badge">
          <span class="badge">
            <svg class="badge-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" />
              <circle cx="9" cy="7" r="4" />
              <path d="M23 21v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75" />
            </svg>
            {{ isRegister ? '用户注册' : '用户登录' }}
          </span>
        </div>

        <!-- Logo 区域 -->
        <div class="logo-section">
          <div class="logo-wrapper">
            <div class="logo-avatar">
              <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" width="32" height="32">
                <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                <circle cx="9" cy="7" r="4" stroke="white" stroke-width="2"/>
                <path d="M23 21v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </div>
            <div class="logo-glow"></div>
          </div>
          <h1 class="title">{{ isRegister ? '创建账户' : '欢迎回来' }}</h1>
          <p class="subtitle">URM 用户中心</p>
        </div>

        <!-- 登录：SSO 自动跳转 -->
        <div v-if="!isRegister" class="sso-section">
          <span class="loading-spinner"></span>
          <p class="sso-tip">正在跳转到统一登录...</p>
        </div>

        <!-- 注册表单 -->
        <form v-else @submit.prevent="handleRegister" class="login-form">
          <div class="form-item">
            <div class="input-wrapper">
              <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2"/>
                <circle cx="12" cy="7" r="4"/>
              </svg>
              <input v-model="registerForm.username" type="text" required placeholder="请输入用户名" class="form-input" />
            </div>
          </div>

          <div class="form-item">
            <div class="input-wrapper">
              <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                <path d="M7 11V7a5 5 0 0110 0v4"/>
              </svg>
              <input v-model="registerForm.password" type="password" required placeholder="请输入密码" class="form-input" />
            </div>
          </div>

          <div class="form-item">
            <div class="input-wrapper">
              <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                <path d="M7 11V7a5 5 0 0110 0v4"/>
              </svg>
              <input v-model="registerForm.confirmPassword" type="password" required placeholder="请确认密码" class="form-input" />
            </div>
          </div>

          <div class="form-item">
            <div class="input-wrapper">
              <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
              </svg>
              <input v-model="registerForm.inviteCode" type="text" required placeholder="请输入邀请码" class="form-input" />
            </div>
          </div>

          <div v-if="registerError" class="error-tip">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
              <circle cx="12" cy="12" r="10"/>
              <line x1="12" y1="8" x2="12" y2="12"/>
              <line x1="12" y1="16" x2="12.01" y2="16"/>
            </svg>
            {{ registerError }}
          </div>

          <button type="submit" :disabled="registerLoading" class="login-button">
            <span v-if="registerLoading" class="loading-spinner"></span>
            {{ registerLoading ? '注册中...' : '注册' }}
          </button>
        </form>

        <!-- 切换登录/注册 -->
        <div class="toggle-section">
          <span class="toggle-text">
            {{ isRegister ? '已有账户？' : '还没有账户？' }}
          </span>
          <button type="button" class="toggle-btn" @click="isRegister = !isRegister">
            {{ isRegister ? '立即登录' : '立即注册' }}
          </button>
        </div>

        <!-- 功能说明 -->
        <div v-if="!isRegister" class="tips-section">
          <div class="tip-title">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
              <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
            </svg>
            <span>用户服务功能</span>
          </div>
          <div class="tip-list">
            <div class="tip-item">
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
              </svg>
              <span>查看积分余额</span>
            </div>
            <div class="tip-item">
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
              </svg>
              <span>消费记录查询</span>
            </div>
            <div class="tip-item">
              <svg class="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/>
              </svg>
              <span>账户安全管理</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref, reactive, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { register } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()

const AUTHORIZE_URL = import.meta.env.VITE_SSO_AUTHORIZE_URL || ''
const CLIENT_ID = import.meta.env.VITE_SSO_CLIENT_ID || ''
const CLIENT_TYPE = import.meta.env.VITE_SSO_CLIENT_TYPE || 'customer'

const isRegister = ref(false)

// 注册
const registerForm = reactive({ username: '', password: '', confirmPassword: '', inviteCode: '' })
const registerLoading = ref(false)
const registerError = ref('')

const redirectToSSO = () => {
  if (!AUTHORIZE_URL || !CLIENT_ID) return
  const callbackURL = window.location.origin + '/oauth/callback'
  const state = crypto.randomUUID()
  sessionStorage.setItem('oauth_state', state)
  sessionStorage.setItem('oauth_redirect_uri', callbackURL)
  const params = new URLSearchParams({
    client_id: CLIENT_ID,
    redirect_uri: callbackURL,
    state,
    client_type: CLIENT_TYPE
  })
  window.location.href = AUTHORIZE_URL + '?' + params.toString()
}

onMounted(() => {
  if (authStore.isAuthenticated() || isRegister.value) return
  redirectToSSO()
})

watch(isRegister, (value) => {
  if (!value && !authStore.isAuthenticated()) {
    redirectToSSO()
  }
})

// 注册处理
const handleRegister = async () => {
  registerError.value = ''

  if (registerForm.password !== registerForm.confirmPassword) {
    registerError.value = '两次输入的密码不一致'
    return
  }
  if (registerForm.password.length < 6) {
    registerError.value = '密码长度至少为6位'
    return
  }

  registerLoading.value = true

  try {
    await register({
      username: registerForm.username,
      password: registerForm.password,
      inviteCode: registerForm.inviteCode
    })

    ElMessage.success('注册成功，请登录')
    registerForm.username = ''
    registerForm.password = ''
    registerForm.confirmPassword = ''
    registerForm.inviteCode = ''
    isRegister.value = false
  } catch (err) {
    registerError.value = err.response?.data?.message || err.message || '注册失败'
  } finally {
    registerLoading.value = false
  }
}
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
  background: linear-gradient(135deg, #0d9488 0%, #14b8a6 50%, #06b6d4 100%);
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
  background: rgba(6,182,212,0.1);
  color: #0e7490;
  border-radius: 20px;
  font-size: 13px;
  font-weight: 500;
  border: 1px solid rgba(6,182,212,0.2);
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
  background: linear-gradient(135deg, #14b8a6, #0d9488);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 8px 24px rgba(6,182,212,0.4);
}

.logo-glow {
  position: absolute;
  inset: -10px;
  background: radial-gradient(circle, rgba(6,182,212,0.3) 0%, transparent 70%);
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

/* SSO 登录区 */
.sso-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
  padding: 8px 0;
}

.sso-tip {
  font-size: 13px;
  color: #64748b;
  margin: 0;
}

/* 表单 */
.login-form {
  margin-bottom: 24px;
}

.form-item {
  margin-bottom: 16px;
}

.input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.input-icon {
  position: absolute;
  left: 14px;
  width: 18px;
  height: 18px;
  color: #94a3b8;
  pointer-events: none;
  z-index: 1;
}

.form-input {
  width: 100%;
  padding: 12px 16px 12px 44px;
  border: 1.5px solid #e2e8f0;
  border-radius: 12px;
  font-size: 14px;
  color: #1e293b;
  background: #fff;
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
  box-sizing: border-box;
}

.form-input::placeholder {
  color: #94a3b8;
}

.form-input:focus {
  border-color: #06b6d4;
  box-shadow: 0 0 0 3px rgba(6,182,212,0.15);
}

/* 错误提示 */
.error-tip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 10px;
  color: #dc2626;
  font-size: 13px;
  margin-bottom: 16px;
}

/* 登录按钮 */
.login-button {
  width: 100%;
  padding: 13px;
  background: linear-gradient(135deg, #06b6d4 0%, #0d9488 100%);
  color: white;
  font-size: 15px;
  font-weight: 600;
  border: none;
  border-radius: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  box-shadow: 0 4px 15px rgba(6,182,212,0.4);
  transition: transform 0.2s, box-shadow 0.2s;
}

.login-button:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(6,182,212,0.5);
}

.login-button:active:not(:disabled) {
  transform: translateY(0);
}

.login-button:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.loading-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255,255,255,0.4);
  border-top-color: white;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* 切换登录/注册 */
.toggle-section {
  text-align: center;
  margin-bottom: 24px;
}

.toggle-text {
  font-size: 13px;
  color: #64748b;
}

.toggle-btn {
  background: none;
  border: none;
  color: #06b6d4;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  margin-left: 4px;
  transition: color 0.2s;
}

.toggle-btn:hover {
  color: #0e7490;
  text-decoration: underline;
}

/* 功能提示 */
.tips-section {
  padding: 20px;
  background: rgba(6,182,212,0.05);
  border-radius: 12px;
  border: 1px solid rgba(6,182,212,0.1);
}

.tip-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #0e7490;
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
  color: #06b6d4;
  flex-shrink: 0;
}

@media (max-width: 480px) {
  .login-card { max-width: 100%; }
  .card-content { padding: 28px 24px; }
  .title { font-size: 24px; }
}
</style>
