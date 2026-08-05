<script setup lang="ts">
import { computed, reactive, ref, shallowRef } from "vue";
import type { FormInstance, FormRules } from "element-plus";

import { notifyError, notifySuccess } from "./feedback";

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
    title?: string;
    subtitle?: string;
  }>(),
  {
    title: "个人中心",
    subtitle: "查看个人信息、修改密码",
    afterPasswordChanged: undefined
  }
);

const loading = shallowRef(false);
const passwordFormRef = ref<FormInstance>();

const passwordForm = reactive({
  oldPassword: "",
  newPassword: "",
  confirmPassword: ""
});

const passwordRules: FormRules = {
  oldPassword: [{ required: true, message: "请输入旧密码", trigger: "blur" }],
  newPassword: [
    { required: true, message: "请输入新密码", trigger: "blur" },
    { min: 6, message: "密码长度至少为6位", trigger: "blur" }
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
              placeholder="请输入新密码（至少6位）"
              show-password
            />
          </el-form-item>
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
    </main>
  </div>
</template>

<style scoped>
.page-container {
  padding: 4px;
}

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
