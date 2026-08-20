<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { CheckCircle2, Eye, EyeOff, KeyRound, LoaderCircle } from "lucide-vue-next";

import { platformPublicApi } from "@/api/platformPublic";
import type { PasswordPolicy } from "@/api/types/platformPublic";
import { validatePasswordAgainstPolicy } from "@/platform/auth/passwordPolicy";

type ActivationState = "loading" | "ready" | "submitting" | "success" | "error";

const route = useRoute();
const router = useRouter();
const state = ref<ActivationState>("loading");
const policy = ref<PasswordPolicy | null>(null);
const error = ref("");
const showPassword = ref(false);
const showConfirmation = ref(false);
const fragment = new URLSearchParams(route.hash.replace(/^#/, ""));
const form = reactive({
  token: String(fragment.get("token") || route.query.token || "").trim(),
  password: "",
  confirmation: ""
});

if (form.token) {
  const { token: _token, ...query } = route.query;
  void router.replace({ query, hash: "" });
}

const canSubmit = computed(
  () => state.value === "ready" && Boolean(policy.value) && Boolean(form.token.trim())
);

onMounted(async () => {
  try {
    policy.value = await platformPublicApi.getPasswordPolicy();
    state.value = "ready";
  } catch (cause) {
    state.value = "error";
    error.value = errorMessage(cause, "密码策略加载失败，请稍后重试。");
  }
});

async function activate() {
  if (!canSubmit.value || !policy.value) return;
  error.value = validatePasswordAgainstPolicy(form.password, "", policy.value);
  if (!error.value && form.password !== form.confirmation) error.value = "两次输入的密码不一致。";
  if (error.value) return;

  state.value = "submitting";
  try {
    await platformPublicApi.activateAccount({ token: form.token.trim(), password: form.password });
    state.value = "success";
  } catch (cause) {
    state.value = "ready";
    error.value = errorMessage(cause, "激活失败，请确认链接有效后重试。");
  }
}

function errorMessage(cause: unknown, fallback: string): string {
  if (cause instanceof Error && cause.message) return cause.message;
  if (typeof cause === "object" && cause !== null && "detail" in cause) {
    const detail = (cause as { detail?: unknown }).detail;
    if (typeof detail === "string" && detail) return detail;
  }
  return fallback;
}
</script>

<template>
  <main class="activation-page ds-theme-customer">
    <section class="activation-panel" aria-live="polite">
      <div class="activation-brand"><KeyRound :size="20" /> D-AI</div>

      <div v-if="state === 'loading'" class="activation-state" aria-busy="true">
        <LoaderCircle class="activation-spin" :size="36" />
        <h1>正在加载</h1>
      </div>

      <div v-else-if="state === 'success'" class="activation-state">
        <CheckCircle2 class="activation-success" :size="44" />
        <h1>账号已激活</h1>
        <p>新密码已经生效，可以登录使用。</p>
        <RouterLink class="activation-submit" to="/login">去登录</RouterLink>
      </div>

      <form v-else class="activation-form" @submit.prevent="activate">
        <header>
          <h1>激活账号</h1>
          <p>{{ policy?.description || "设置登录密码以完成账号激活。" }}</p>
        </header>

        <label class="activation-field">
          <span>激活令牌</span>
          <input v-model="form.token" autocomplete="one-time-code" required />
        </label>

        <label class="activation-field">
          <span>新密码</span>
          <span class="activation-password">
            <input
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="new-password"
              :minlength="policy?.minLength"
              required
            />
            <button type="button" :title="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword">
              <EyeOff v-if="showPassword" :size="18" />
              <Eye v-else :size="18" />
            </button>
          </span>
        </label>

        <label class="activation-field">
          <span>确认密码</span>
          <span class="activation-password">
            <input
              v-model="form.confirmation"
              :type="showConfirmation ? 'text' : 'password'"
              autocomplete="new-password"
              :minlength="policy?.minLength"
              required
            />
            <button type="button" :title="showConfirmation ? '隐藏密码' : '显示密码'" @click="showConfirmation = !showConfirmation">
              <EyeOff v-if="showConfirmation" :size="18" />
              <Eye v-else :size="18" />
            </button>
          </span>
        </label>

        <p v-if="error" class="activation-error" role="alert">{{ error }}</p>
        <button class="activation-submit" type="submit" :disabled="!canSubmit">
          <LoaderCircle v-if="state === 'submitting'" class="activation-spin" :size="17" />
          <KeyRound v-else :size="17" />
          {{ state === "submitting" ? "激活中…" : "激活账号" }}
        </button>
      </form>
    </section>
  </main>
</template>

<style scoped>
.activation-page { min-height: 100dvh; display: grid; place-items: center; padding: 24px; background: var(--ds-paper); }
.activation-panel { width: min(100%, 480px); padding: 32px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-shell); background: var(--ds-panel); box-shadow: var(--ds-shadow-pop); }
.activation-brand { display: flex; align-items: center; gap: 8px; margin-bottom: 28px; color: var(--ds-accent); font: 800 18px/1 var(--ds-font-display); }
.activation-form { display: grid; gap: 18px; }
.activation-form header { margin-bottom: 4px; }
h1 { margin: 0; color: var(--ds-ink); font: 700 26px/1.2 var(--ds-font-display); }
.activation-form header p, .activation-state p { margin: 10px 0 0; color: var(--ds-muted); font-size: 13px; line-height: 1.6; }
.activation-field { display: grid; gap: 8px; color: var(--ds-ink); font-size: 14px; font-weight: 650; }
.activation-field input { width: 100%; min-height: 46px; padding: 10px 12px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); outline: none; background: var(--ds-panel); color: var(--ds-ink); font: inherit; }
.activation-field input:focus { border-color: var(--ds-accent); box-shadow: 0 0 0 3px var(--ds-accent-soft); }
.activation-password { position: relative; }
.activation-password input { padding-right: 44px; }
.activation-password button { position: absolute; top: 7px; right: 7px; display: grid; width: 32px; height: 32px; padding: 0; place-items: center; border: 0; background: transparent; color: var(--ds-muted); cursor: pointer; }
.activation-error { margin: -4px 0 0; color: var(--ds-danger); font-size: 13px; line-height: 1.5; }
.activation-submit { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 48px; padding: 12px 16px; border: 0; border-radius: var(--ds-radius-panel); background: var(--ds-accent); color: var(--ds-accent-contrast); font-weight: 700; text-decoration: none; cursor: pointer; }
.activation-submit:disabled { cursor: wait; opacity: 0.65; }
.activation-state { min-height: 300px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 14px; text-align: center; }
.activation-state .activation-submit { min-width: 140px; margin-top: 8px; }
.activation-success { color: var(--ds-positive); }
.activation-spin { animation: activation-spin 900ms linear infinite; }
@keyframes activation-spin { to { transform: rotate(360deg); } }
@media (max-width: 520px) { .activation-page { padding: 14px; } .activation-panel { padding: 26px 20px; } }
</style>
