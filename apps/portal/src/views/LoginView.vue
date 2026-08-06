<script setup lang="ts">
import { computed, ref } from "vue";
import { Eye, EyeOff } from "lucide-vue-next";
import { useRoute, useRouter } from "vue-router";

import { useAuthStore } from "@/stores/auth";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const username = ref("");
const password = ref("");
const showPassword = ref(false);
const pending = ref(false);
const errorMessage = ref("");

const redirectPath = computed(() => {
  const value = typeof route.query.redirect === "string" ? route.query.redirect : "/overview";
  return value.startsWith("/") && !value.startsWith("//") ? value : "/overview";
});

async function startLogin() {
  pending.value = true;
  errorMessage.value = "";
  try {
    await authStore.login(username.value, password.value);
    await router.replace(redirectPath.value);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "登录失败，请重试";
  } finally {
    pending.value = false;
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-card" aria-labelledby="login-title">
      <div class="login-brand">D-AI</div>
      <p class="login-kicker">统一 AI 服务平台</p>
      <h1 id="login-title">登录</h1>
      <p class="login-description">使用账号登录，系统会根据你的身份和权限展示对应的工作区。</p>

      <form class="login-form" @submit.prevent="startLogin">
        <label class="login-field">
          <span>用户名</span>
          <input
            v-model.trim="username"
            name="username"
            type="text"
            autocomplete="username"
            placeholder="请输入用户名"
            required
            autofocus
          />
        </label>

        <label class="login-field">
          <span>密码</span>
          <span class="login-password">
            <input
              v-model="password"
              name="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
              required
            />
            <button
              type="button"
              class="password-toggle"
              :aria-label="showPassword ? '隐藏密码' : '显示密码'"
              :title="showPassword ? '隐藏密码' : '显示密码'"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="18" />
              <Eye v-else :size="18" />
            </button>
          </span>
        </label>

        <button type="submit" class="login-button" :disabled="pending">
          {{ pending ? "登录中…" : "登录" }}
        </button>
      </form>

      <p v-if="errorMessage" class="login-error" role="alert">{{ errorMessage }}</p>
    </section>
  </main>
</template>

<style scoped>
.login-page {
  min-height: 100dvh;
  display: grid;
  place-items: center;
  padding: 32px 20px;
  background: var(--ds-paper);
}

.login-card {
  width: min(100%, 520px);
  padding: 40px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-pop);
}

.login-brand {
  color: var(--ds-accent);
  font: 700 28px/1 var(--ds-font-display);
  letter-spacing: 0;
}

.login-kicker {
  margin: 18px 0 6px;
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

h1 {
  margin: 0;
  color: var(--ds-ink);
  font: 700 28px/1.2 var(--ds-font-display);
}

.login-description {
  margin: 12px 0 28px;
  color: var(--ds-muted);
  line-height: 1.6;
}

.login-form {
  display: grid;
  gap: 18px;
}

.login-field {
  display: grid;
  gap: 8px;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 650;
}

.login-field input {
  width: 100%;
  min-height: 46px;
  padding: 10px 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  outline: none;
  background: var(--ds-panel);
  color: var(--ds-ink);
  font: inherit;
  font-weight: 500;
  transition: border-color 160ms ease, box-shadow 160ms ease;
}

.login-field input:focus {
  border-color: var(--ds-accent);
  box-shadow: 0 0 0 3px var(--ds-accent-soft);
}

.login-field input::placeholder {
  color: var(--ds-muted);
  font-weight: 400;
}

.login-password {
  position: relative;
  display: block;
}

.login-password input {
  padding-right: 44px;
}

.password-toggle {
  position: absolute;
  top: 50%;
  right: 8px;
  display: grid;
  width: 32px;
  height: 32px;
  padding: 0;
  place-items: center;
  transform: translateY(-50%);
  border: 0;
  background: transparent;
  color: var(--ds-muted);
  cursor: pointer;
}

.password-toggle:hover {
  color: var(--ds-ink);
}

.login-button {
  width: 100%;
  min-height: 48px;
  padding: 12px 16px;
  border: 0;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
  font-weight: 700;
  cursor: pointer;
  transition: background 160ms ease;
}

.login-button:hover:not(:disabled) {
  background: var(--ds-accent-hover);
}

.login-button:disabled {
  cursor: wait;
  opacity: 0.65;
}

.login-error {
  margin: 18px 0 0;
  color: var(--ds-danger);
  line-height: 1.5;
}

@media (max-width: 520px) {
  .login-card {
    padding: 28px 22px;
  }
}
</style>
