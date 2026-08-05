<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute } from "vue-router";

import {
  beginSSOAuthorize,
  currentRedirectUri,
  rememberPortalClientType,
  type PortalClientType
} from "@/platform";
import { portalEnv } from "@/env";

const route = useRoute();
const pending = ref<PortalClientType | null>(null);
const errorMessage = ref("");

const redirectPath = computed(() => {
  const value = typeof route.query.redirect === "string" ? route.query.redirect : "/overview";
  return value.startsWith("/") && !value.startsWith("//") ? value : "/overview";
});

const loginOptions: Array<{ type: PortalClientType; title: string; description: string }> = [
  { type: "admin", title: "管理端", description: "平台管理员和系统管理员" },
  { type: "tenant", title: "租户端", description: "租户运营、成员和额度管理" },
  { type: "customer", title: "用户端", description: "AI 工作台、账户和个人服务" }
];

async function startLogin(clientType: PortalClientType) {
  pending.value = clientType;
  errorMessage.value = "";
  rememberPortalClientType(clientType);
  portalEnv.clientTypeHeader = clientType;
  try {
    const authorizeUrl = await beginSSOAuthorize(
      portalEnv,
      currentRedirectUri(redirectPath.value),
      "",
      "urm"
    );
    if (!authorizeUrl) {
      errorMessage.value = "SSO 尚未配置，请检查 Portal 的 VITE_SSO_AUTHORIZE_URL。";
      return;
    }
    window.location.assign(authorizeUrl);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "无法发起登录";
  } finally {
    pending.value = null;
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-card" aria-labelledby="login-title">
      <div class="login-brand">D-AI</div>
      <p class="login-kicker">统一 AI 服务平台</p>
      <h1 id="login-title">选择登录入口</h1>
      <p class="login-description">同一个 Portal 根据登录身份展示对应的工作区、菜单和主题。</p>

      <div class="login-options">
        <button
          v-for="option in loginOptions"
          :key="option.type"
          type="button"
          class="login-option"
          :disabled="pending !== null"
          @click="startLogin(option.type)"
        >
          <span class="login-option__copy">
            <strong>{{ option.title }}</strong>
            <span>{{ option.description }}</span>
          </span>
          <span class="login-option__action">{{ pending === option.type ? "跳转中…" : "登录" }}</span>
        </button>
      </div>

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
  letter-spacing: 0.08em;
}

.login-kicker {
  margin: 18px 0 6px;
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.12em;
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

.login-options {
  display: grid;
  gap: 12px;
}

.login-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
  padding: 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  color: var(--ds-ink);
  cursor: pointer;
  text-align: left;
  transition: border-color 160ms ease, background 160ms ease;
}

.login-option:hover:not(:disabled) {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
}

.login-option:disabled {
  cursor: wait;
  opacity: 0.65;
}

.login-option__copy {
  display: grid;
  gap: 4px;
}

.login-option__copy strong {
  font-size: 15px;
}

.login-option__copy span {
  color: var(--ds-muted);
  font-size: 13px;
}

.login-option__action {
  flex: none;
  color: var(--ds-accent);
  font-weight: 700;
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
