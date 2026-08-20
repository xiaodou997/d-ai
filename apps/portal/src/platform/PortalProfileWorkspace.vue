<script setup lang="ts">
import { computed, reactive, ref, shallowRef, watch } from "vue";
import type { FormInstance, FormRules } from "element-plus";

import { notifyError, notifySuccess } from "./feedback";
import { usePasswordPolicy, validatePasswordAgainstPolicy } from "./auth/passwordPolicy";

export interface PortalProfileField {
  label: string;
  value: string;
  tone?: "default" | "strong" | "mono";
}

const props = withDefaults(
  defineProps<{
    fields: PortalProfileField[];
    changePassword: (payload: { oldPassword: string; newPassword: string }) => Promise<unknown>;
    afterPasswordChanged?: () => Promise<void> | void;
    updateProfile?: (payload: { username?: string; email?: string }) => Promise<unknown>;
    initialUsername?: string;
    initialEmail?: string;
    afterProfileChanged?: () => Promise<void> | void;
    mfa?: {
      enabled?: boolean;
      enroll: () => Promise<{ secret: string; otpauthUrl: string }>;
      confirm: (code: string) => Promise<unknown>;
    };
    title?: string;
    subtitle?: string;
  }>(),
  {
    title: "个人中心",
    subtitle: "查看个人信息、修改密码",
    afterPasswordChanged: undefined,
    updateProfile: undefined,
    initialUsername: "",
    initialEmail: "",
    afterProfileChanged: undefined,
    mfa: undefined
  }
);

const loading = shallowRef(false);
const profileLoading = shallowRef(false);
const passwordFormRef = ref<FormInstance>();
const passwordPolicy = usePasswordPolicy();
const profileFormRef = ref<FormInstance>();
const mfaSecret = ref("");
const mfaURL = ref("");
const mfaCode = ref("");
const mfaLoading = ref(false);
const mfaError = ref("");
const mfaEnabled = ref(Boolean(props.mfa?.enabled));

watch(
  () => props.mfa?.enabled,
  (enabled) => {
    if (enabled !== undefined) mfaEnabled.value = enabled;
  }
);

const profileForm = reactive({
  username: props.initialUsername,
  email: props.initialEmail
});

const profileRules: FormRules = {
  username: [{ required: true, message: "请输入用户名", trigger: "blur" }],
  email: [{ type: "email", message: "邮箱格式不正确", trigger: "blur" }]
};

const passwordForm = reactive({
  oldPassword: "",
  newPassword: "",
  confirmPassword: ""
});

const passwordRules: FormRules = {
  oldPassword: [{ required: true, message: "请输入旧密码", trigger: "blur" }],
  newPassword: [
    { required: true, message: "请输入新密码", trigger: "blur" },
    {
      validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
        if (!passwordPolicy.value) return callback(new Error("密码策略尚未加载"));
        const message = validatePasswordAgainstPolicy(value, props.initialUsername, passwordPolicy.value);
        callback(message ? new Error(message) : undefined);
      },
      trigger: "blur"
    }
  ],
  confirmPassword: [
    { required: true, message: "请再次输入新密码", trigger: "blur" },
    {
      validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
        if (value !== passwordForm.newPassword) {
          callback(new Error("两次输入的密码不一致"));
          return;
        }
        callback();
      },
      trigger: "blur"
    }
  ]
};

const fieldClass = computed(() => (field: PortalProfileField) => ({
  "field-value": true,
  "field-value-strong": field.tone === "strong",
  "field-value-mono": field.tone === "mono"
}));

async function handleChangePassword() {
  if (!passwordFormRef.value) return;
  await passwordFormRef.value.validate(async (valid) => {
    if (!valid) return;
    loading.value = true;
    try {
      await props.changePassword({
        oldPassword: passwordForm.oldPassword,
        newPassword: passwordForm.newPassword
      });
      notifySuccess("密码修改成功，请重新登录");
      passwordForm.oldPassword = "";
      passwordForm.newPassword = "";
      passwordForm.confirmPassword = "";
      await props.afterPasswordChanged?.();
    } catch (error) {
      notifyError(error instanceof Error ? error.message : "修改失败，请检查旧密码是否正确");
    } finally {
      loading.value = false;
    }
  });
}

async function handleUpdateProfile() {
  if (!props.updateProfile || !profileFormRef.value) return;
  await profileFormRef.value.validate(async (valid) => {
    if (!valid) return;
    const payload: { username?: string; email?: string } = {};
    if (profileForm.username.trim() !== props.initialUsername) {
      payload.username = profileForm.username.trim();
    }
    if (profileForm.email.trim() !== props.initialEmail) {
      payload.email = profileForm.email.trim();
    }
    if (payload.username === undefined && payload.email === undefined) {
      notifyError("用户名和邮箱均未变更");
      return;
    }
    profileLoading.value = true;
    try {
      await props.updateProfile!(payload);
      notifySuccess("资料已更新，请重新登录");
      await props.afterProfileChanged?.();
    } catch (error) {
      notifyError(error instanceof Error ? error.message : "资料更新失败，请稍后重试");
    } finally {
      profileLoading.value = false;
    }
  });
}

async function startMFAEnrollment() {
  if (!props.mfa) return;
  mfaLoading.value = true;
  mfaError.value = "";
  try {
    const enrollment = await props.mfa.enroll();
    mfaSecret.value = enrollment.secret;
    mfaURL.value = enrollment.otpauthUrl;
  } catch (error) {
    mfaError.value = error instanceof Error ? error.message : "MFA 注册失败，请稍后重试";
  } finally {
    mfaLoading.value = false;
  }
}

async function confirmMFAEnrollment() {
  if (!props.mfa || mfaCode.value.length !== 6) return;
  mfaLoading.value = true;
  mfaError.value = "";
  try {
    await props.mfa.confirm(mfaCode.value);
    mfaEnabled.value = true;
    mfaSecret.value = "";
    mfaURL.value = "";
    mfaCode.value = "";
  } catch (error) {
    mfaError.value = error instanceof Error ? error.message : "MFA 验证失败，请检查验证码";
  } finally {
    mfaLoading.value = false;
  }
}
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div class="page-title">
        <h2>{{ title }}</h2>
        <p>{{ subtitle }}</p>
      </div>
    </header>

    <main class="page-main">
      <el-card shadow="never">
        <template #header><span class="card-title">基本信息</span></template>
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item v-for="field in fields" :key="field.label" :label="field.label">
            <span :class="fieldClass(field)">{{ field.value || "—" }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card v-if="updateProfile" shadow="never" class="mt-4">
        <template #header><span class="card-title">修改用户名 / 邮箱</span></template>
        <el-form ref="profileFormRef" :model="profileForm" :rules="profileRules" label-width="96px" class="max-w-md">
          <el-form-item label="用户名" prop="username">
            <el-input v-model="profileForm.username" placeholder="请输入用户名" maxlength="50" />
          </el-form-item>
          <el-form-item label="邮箱" prop="email">
            <el-input v-model="profileForm.email" placeholder="请输入邮箱（可留空）" maxlength="100" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="profileLoading" @click="handleUpdateProfile">保存资料</el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <el-card shadow="never" class="mt-4">
        <template #header><span class="card-title">修改密码</span></template>
        <el-form ref="passwordFormRef" :model="passwordForm" :rules="passwordRules" label-width="96px" class="max-w-md">
          <el-form-item label="旧密码" prop="oldPassword">
            <el-input v-model="passwordForm.oldPassword" type="password" placeholder="请输入旧密码" show-password />
          </el-form-item>
          <el-form-item label="新密码" prop="newPassword">
            <el-input
              v-model="passwordForm.newPassword"
              type="password"
              :placeholder="passwordPolicy?.description || '请输入新密码'"
              show-password
            />
          </el-form-item>
          <p v-if="passwordPolicy" class="password-policy">{{ passwordPolicy.description }}</p>
          <el-form-item label="确认密码" prop="confirmPassword">
            <el-input
              v-model="passwordForm.confirmPassword"
              type="password"
              placeholder="请再次输入新密码"
              show-password
            />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" @click="handleChangePassword">确认修改</el-button>
          </el-form-item>
        </el-form>
      </el-card>

      <el-card v-if="mfa" shadow="never" class="mt-4">
        <template #header><span class="card-title">管理员 MFA</span></template>
        <template v-if="mfaEnabled">
          <p class="password-policy">MFA 已启用。之后登录和敏感操作会要求验证动态验证码。</p>
        </template>
        <template v-else-if="mfaSecret">
          <p class="password-policy">请将以下密钥添加到身份验证器，并输入当前 6 位验证码完成启用。</p>
          <code class="mfa-secret">{{ mfaSecret }}</code>
          <p class="mfa-url">{{ mfaURL }}</p>
          <div class="mfa-confirm-row">
            <el-input v-model="mfaCode" inputmode="numeric" maxlength="6" placeholder="6 位验证码" />
            <el-button type="primary" :loading="mfaLoading" @click="confirmMFAEnrollment">启用 MFA</el-button>
          </div>
        </template>
        <template v-else>
          <p class="password-policy">启用 TOTP 后可显著降低管理员账号被盗用的风险。</p>
          <el-button type="primary" :loading="mfaLoading" @click="startMFAEnrollment">开始设置 MFA</el-button>
        </template>
        <p v-if="mfaError" class="profile-error" role="alert">{{ mfaError }}</p>
      </el-card>
    </main>
  </div>
</template>

<style scoped>
.page-container {
  padding: 4px;
}

.mfa-secret { display: block; margin: 12px 0; padding: 10px 12px; border-radius: var(--ds-radius-control); background: var(--ds-paper); color: var(--ds-ink); font-family: var(--ds-font-mono); letter-spacing: 0.08em; }
.mfa-url { color: var(--ds-muted); font-size: 12px; word-break: break-all; }
.mfa-confirm-row { display: flex; gap: 10px; align-items: center; max-width: 440px; }
.profile-error { color: var(--ds-danger); font-size: 13px; }

.page-header {
  margin-bottom: 16px;
}

.page-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 800;
  color: var(--ds-ink);
}

.page-title p {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--ds-muted);
}

.card-title {
  font-weight: 700;
  color: var(--ds-ink);
}

.password-policy {
  margin: -8px 0 16px 96px;
  color: var(--ds-muted);
  font-size: 12px;
}

.field-value {
  font-size: 14px;
  color: var(--ds-ink-soft);
}

.field-value-strong {
  font-weight: 600;
  color: var(--ds-ink);
}

.field-value-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>
