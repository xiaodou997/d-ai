<!-- Public invitation registration workspace. -->
<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import {
  ArrowRight,
  CheckCircle2,
  CircleAlert,
  Eye,
  EyeOff,
  LoaderCircle,
  RefreshCw,
  ShieldCheck,
  UserPlus
} from "lucide-vue-next";

import { platformPublicApi } from "@/api/platformPublic";
import type {
  PasswordPolicy,
  PublicInvitation,
  PublicRegistrationPayload
} from "@/api/types/platformPublic";
import { PortalLegalFooter } from "@/platform";
import { validatePasswordAgainstPolicy } from "@/platform/auth/passwordPolicy";

type RegisterState = "loading" | "ready" | "invalid" | "error" | "success";

const invitationCodePattern = /^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{8}$/;

const route = useRoute();
const invitation = ref<PublicInvitation | null>(null);
const passwordPolicy = ref<PasswordPolicy | null>(null);
const state = ref<RegisterState>("loading");
const loadError = ref("");
const submitError = ref("");
const pending = ref(false);
const showPassword = ref(false);
const showConfirmPassword = ref(false);
const faviconVisible = ref(true);
const loadGeneration = ref(0);

const form = reactive({
  username: "",
  password: "",
  confirmPassword: "",
  email: "",
  phone: "",
  accepted: false
});

const code = computed(() => String(route.params.code || "").trim().toUpperCase());
const siteName = computed(
  () => invitation.value?.customerSiteName || invitation.value?.tenantName || "D-AI"
);
const canSubmit = computed(
  () => state.value === "ready" && Boolean(invitation.value) && Boolean(passwordPolicy.value) && !pending.value
);

watch(code, () => {
  void loadInvitation();
}, { immediate: true });

async function loadInvitation() {
  const generation = loadGeneration.value + 1;
  loadGeneration.value = generation;
  resetForm();
  invitation.value = null;
  passwordPolicy.value = null;
  loadError.value = "";
  submitError.value = "";
  faviconVisible.value = true;
  state.value = "loading";

  if (!invitationCodePattern.test(code.value)) {
    state.value = "invalid";
    loadError.value = "注册链接无效，请向邀请方索取新的注册链接。";
    return;
  }

  try {
    const [result, policy] = await Promise.all([
      platformPublicApi.getInvitation(code.value),
      platformPublicApi.getPasswordPolicy()
    ]);
    if (generation !== loadGeneration.value) return;

    invitation.value = result;
    passwordPolicy.value = policy;
    state.value = result.canRegister && result.status === "active" ? "ready" : "invalid";
  } catch (error) {
    if (generation !== loadGeneration.value) return;

    if (statusOf(error) === 400) {
      state.value = "invalid";
      loadError.value = "注册链接无效，请向邀请方索取新的注册链接。";
    } else {
      state.value = "error";
      loadError.value = errorMessage(error, "注册链接加载失败，请稍后重试。");
    }
  }
}

function resetForm() {
  Object.assign(form, {
    username: "",
    password: "",
    confirmPassword: "",
    email: "",
    phone: "",
    accepted: false
  });
  showPassword.value = false;
  showConfirmPassword.value = false;
}

async function submitRegistration() {
  if (!canSubmit.value || !invitation.value) return;

  submitError.value = validateForm();
  if (submitError.value) return;

  pending.value = true;
  try {
    const payload: PublicRegistrationPayload = {
      username: form.username.trim(),
      password: form.password,
      termsVersion: invitation.value.legal.termsVersion,
      privacyVersion: invitation.value.legal.privacyVersion
    };
    const email = form.email.trim();
    const phone = form.phone.trim();
    if (email) payload.email = email;
    if (phone) payload.phone = phone;

    await platformPublicApi.registerInvitation(code.value, payload);
    state.value = "success";
  } catch (error) {
    submitError.value = errorMessage(error, "注册失败，请检查填写内容后重试。");
  } finally {
    pending.value = false;
  }
}

function validateForm(): string {
  if (!form.username.trim()) return "请输入用户名。";
  if (!passwordPolicy.value) return "密码策略尚未加载。";
  const passwordError = validatePasswordAgainstPolicy(
    form.password,
    form.username,
    passwordPolicy.value
  );
  if (passwordError) return passwordError;
  if (form.password !== form.confirmPassword) return "两次输入的密码不一致。";
  if (!form.accepted) return "请先阅读并同意服务条款和隐私政策。";
  return "";
}

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "object" && error !== null && "detail" in error) {
    const detail = (error as { detail?: unknown }).detail;
    if (typeof detail === "string" && detail) return detail;
  }
  return fallback;
}

function statusOf(error: unknown): number | undefined {
  if (typeof error !== "object" || error === null || !("status" in error)) return undefined;
  const status = Number((error as { status?: unknown }).status);
  return Number.isFinite(status) ? status : undefined;
}
</script>

<template>
  <main class="register-page ds-theme-customer">
    <div class="register-shell">
      <header class="register-header">
        <div class="register-brand" aria-label="D-AI">
          <span class="register-brand__mark"><UserPlus :size="18" :stroke-width="2.2" /></span>
          <span>D-AI</span>
        </div>
        <RouterLink class="register-login-link" to="/login">已有账号？登录</RouterLink>
      </header>

      <section class="register-card" aria-live="polite">
        <div v-if="state === 'loading'" class="register-state register-state--loading" aria-busy="true">
          <LoaderCircle class="register-state__icon register-state__icon--spin" :size="34" />
          <h1>正在验证邀请链接</h1>
          <p>请稍候，我们正在确认该邀请码是否仍然有效。</p>
        </div>

        <div v-else-if="state === 'error'" class="register-state">
          <CircleAlert class="register-state__icon register-state__icon--danger" :size="40" />
          <h1>注册链接加载失败</h1>
          <p>{{ loadError }}</p>
          <button type="button" class="register-button register-button--secondary" @click="loadInvitation">
            <RefreshCw :size="17" />
            重新加载
          </button>
        </div>

        <div v-else-if="state === 'invalid'" class="register-state">
          <CircleAlert class="register-state__icon register-state__icon--warning" :size="40" />
          <h1>暂时无法注册</h1>
          <p>{{ invitation?.message || loadError || "该注册链接不可用。" }}</p>
          <RouterLink class="register-button register-button--secondary" to="/login">
            返回登录
            <ArrowRight :size="17" />
          </RouterLink>
        </div>

        <div v-else-if="state === 'success'" class="register-state register-state--success">
          <CheckCircle2 class="register-state__icon register-state__icon--success" :size="44" />
          <h1>注册成功</h1>
          <p>
            账号 <strong>{{ form.username.trim() }}</strong> 已创建成功，现在可以使用该账号登录。
          </p>
          <RouterLink class="register-button" to="/login">
            去登录
            <ArrowRight :size="17" />
          </RouterLink>
        </div>

        <form v-else class="register-form" @submit.prevent="submitRegistration">
          <div class="register-intro">
            <div class="register-intro__identity">
              <img
                v-if="invitation?.faviconPath && faviconVisible"
                class="register-intro__favicon"
                :src="invitation.faviconPath"
                alt=""
                @error="faviconVisible = false"
              />
              <div>
                <p class="register-kicker">邀请注册</p>
                <h1>加入 {{ siteName }}</h1>
              </div>
            </div>
            <p class="register-description">
              {{ invitation?.description || "创建终端用户账号，开始使用平台服务。" }}
            </p>
          </div>

          <label class="register-field">
            <span>用户名</span>
            <input
              v-model.trim="form.username"
              name="username"
              type="text"
              autocomplete="username"
              placeholder="请输入用户名"
              maxlength="64"
              required
            />
          </label>

          <label class="register-field">
            <span>密码</span>
            <span class="register-password">
              <input
                v-model="form.password"
                name="password"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="new-password"
                :placeholder="passwordPolicy?.description"
                :minlength="passwordPolicy?.minLength"
                required
              />
              <button
                type="button"
                class="register-password__toggle"
                :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                :title="showPassword ? '隐藏密码' : '显示密码'"
                @click="showPassword = !showPassword"
              >
                <EyeOff v-if="showPassword" :size="18" />
                <Eye v-else :size="18" />
              </button>
            </span>
          </label>

          <label class="register-field">
            <span>确认密码</span>
            <span class="register-password">
              <input
                v-model="form.confirmPassword"
                name="confirmPassword"
                :type="showConfirmPassword ? 'text' : 'password'"
                autocomplete="new-password"
                placeholder="请再次输入密码"
                :minlength="passwordPolicy?.minLength"
                required
              />
              <button
                type="button"
                class="register-password__toggle"
                :aria-label="showConfirmPassword ? '隐藏确认密码' : '显示确认密码'"
                :title="showConfirmPassword ? '隐藏确认密码' : '显示确认密码'"
                @click="showConfirmPassword = !showConfirmPassword"
              >
                <EyeOff v-if="showConfirmPassword" :size="18" />
                <Eye v-else :size="18" />
              </button>
            </span>
          </label>

          <div class="register-optional-fields">
            <label class="register-field">
              <span>邮箱 <em>可选</em></span>
              <input
                v-model.trim="form.email"
                name="email"
                type="email"
                autocomplete="email"
                placeholder="用于接收账户通知"
              />
            </label>

            <label class="register-field">
              <span>手机号 <em>可选</em></span>
              <input
                v-model.trim="form.phone"
                name="phone"
                type="tel"
                autocomplete="tel"
                placeholder="请输入手机号"
              />
            </label>
          </div>

          <label class="register-consent">
            <input v-model="form.accepted" name="accepted" type="checkbox" />
            <span>
              我已阅读并同意
              <a :href="invitation?.legal.termsUrl" target="_blank" rel="noreferrer">服务条款</a>
              和
              <a :href="invitation?.legal.privacyUrl" target="_blank" rel="noreferrer">隐私政策</a>
            </span>
          </label>

          <p v-if="submitError" class="register-error" role="alert">{{ submitError }}</p>

          <button type="submit" class="register-button" :disabled="!canSubmit">
            <LoaderCircle v-if="pending" class="register-state__icon--spin" :size="17" />
            <ShieldCheck v-else :size="17" />
            {{ pending ? "注册中…" : "创建账号" }}
          </button>
        </form>
      </section>

      <PortalLegalFooter />
    </div>
  </main>
</template>

<style scoped>
.register-page {
  min-height: 100dvh;
  padding: 28px 20px 24px;
  background:
    radial-gradient(circle at 12% 0%, color-mix(in srgb, var(--ds-accent-soft) 82%, transparent), transparent 34%),
    var(--ds-paper);
}

.register-shell {
  width: min(100%, 560px);
  min-height: calc(100dvh - 52px);
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.register-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 36px;
}

.register-brand {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  color: var(--ds-ink);
  font: 800 18px/1 var(--ds-font-display);
}

.register-brand__mark {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  border-radius: var(--ds-radius-control);
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
}

.register-login-link {
  color: var(--ds-muted);
  font-size: 13px;
  text-decoration: none;
}

.register-login-link:hover {
  color: var(--ds-accent);
}

.register-card {
  padding: 36px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-pop);
}

.register-form {
  display: grid;
  gap: 18px;
}

.register-intro {
  padding-bottom: 24px;
  border-bottom: 1px solid var(--ds-line);
}

.register-intro__identity {
  display: flex;
  align-items: center;
  gap: 14px;
}

.register-intro__favicon {
  width: 44px;
  height: 44px;
  flex: 0 0 44px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  object-fit: cover;
}

.register-kicker {
  margin: 0 0 7px;
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

h1 {
  margin: 0;
  color: var(--ds-ink);
  font: 700 28px/1.2 var(--ds-font-display);
}

.register-description {
  margin: 16px 0 0;
  color: var(--ds-muted);
  line-height: 1.65;
}

.register-field {
  display: grid;
  gap: 8px;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 650;
}

.register-field em {
  margin-left: 5px;
  color: var(--ds-muted);
  font-size: 12px;
  font-style: normal;
  font-weight: 500;
}

.register-field input {
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

.register-field input:focus {
  border-color: var(--ds-accent);
  box-shadow: var(--ds-shadow-focus-strong);
}

.register-field input::placeholder {
  color: var(--ds-muted);
  font-weight: 400;
}

.register-password {
  position: relative;
  display: block;
}

.register-password input {
  padding-right: 44px;
}

.register-password__toggle {
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

.register-password__toggle:hover {
  color: var(--ds-ink);
}

.register-optional-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.register-consent {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.register-consent input {
  width: 16px;
  height: 16px;
  flex: 0 0 16px;
  margin: 2px 0 0;
  accent-color: var(--ds-accent);
}

.register-consent a {
  color: var(--ds-accent);
  font-weight: 650;
}

.register-error {
  margin: -4px 0 0;
  color: var(--ds-danger);
  line-height: 1.5;
}

.register-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  min-height: 48px;
  padding: 12px 16px;
  border: 0;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-accent);
  color: var(--ds-accent-contrast);
  font-weight: 700;
  text-decoration: none;
  cursor: pointer;
  transition: background 160ms ease, opacity 160ms ease;
}

.register-button:hover:not(:disabled) {
  background: var(--ds-accent-hover);
}

.register-button:disabled {
  cursor: wait;
  opacity: 0.65;
}

.register-button--secondary {
  width: auto;
  min-width: 150px;
  margin-top: 8px;
  border: 1px solid var(--ds-line-strong);
  background: var(--ds-panel);
  color: var(--ds-ink-soft);
}

.register-button--secondary:hover:not(:disabled) {
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
}

.register-state {
  min-height: 380px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  text-align: center;
}

.register-state h1 {
  margin-top: 4px;
  font-size: 26px;
}

.register-state p {
  max-width: 400px;
  margin: 0;
  color: var(--ds-muted);
  line-height: 1.65;
}

.register-state p strong {
  color: var(--ds-ink);
}

.register-state__icon {
  color: var(--ds-accent);
}

.register-state__icon--spin {
  animation: register-spin 900ms linear infinite;
}

.register-state__icon--danger {
  color: var(--ds-danger);
}

.register-state__icon--warning {
  color: var(--ds-warning);
}

.register-state__icon--success {
  color: var(--ds-positive);
}

.register-state--success .register-button {
  width: auto;
  min-width: 150px;
  margin-top: 8px;
}

@keyframes register-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 560px) {
  .register-page {
    padding: 20px 14px;
  }

  .register-shell {
    min-height: calc(100dvh - 40px);
  }

  .register-card {
    padding: 28px 22px;
  }

  .register-optional-fields {
    grid-template-columns: 1fr;
  }
}
</style>
