<!--
  个人资料 — 查看个人信息、修改密码。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行),
       不再使用 PortalProfileWorkspace(customer 变体含硬编码色值);基本信息/修改密码
       两个分区收进同卡 body 的 24px 容器,色值全部改用 --ds-* token;密码表单仍为
       element-plus,校验规则、接口调用与改密后退出登录的逻辑保持不变。
-->
<template>
  <div class="page-container profile-page">
    <PortalPagePanel
      fill
      :icon="UserRound"
      :breadcrumbs="[{ label: '用户中心' }, { label: '账户设置' }, { label: '个人资料' }]"
      description="查看个人信息、修改密码"
    >
      <div class="profile-body">
        <section class="profile-section">
          <h2 class="profile-section__title">基本信息</h2>
          <div class="profile-rows">
            <div v-for="field in profileFields" :key="field.label" class="profile-row">
              <span class="profile-row__label">{{ field.label }}</span>
              <span :class="fieldClass(field)">{{ field.value || "—" }}</span>
            </div>
          </div>
        </section>

        <section class="profile-section">
          <h2 class="profile-section__title">修改密码</h2>
          <el-form
            ref="passwordFormRef"
            :model="passwordForm"
            :rules="passwordRules"
            label-width="100px"
            class="max-w-md"
          >
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
            <p v-if="passwordPolicy" class="profile-password-policy">{{ passwordPolicy.description }}</p>
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
        </section>
      </div>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, shallowRef } from "vue";
import type { FormInstance, FormRules } from "element-plus";
import { UserRound } from "lucide-vue-next";
import { PortalPagePanel, notifyError, notifySuccess } from "@/platform";
import { useAuthStore } from "@/stores/auth";
import { platformCustomerApi } from "@/api/platformCustomer";
import { usePasswordPolicy, validatePasswordAgainstPolicy } from "@/platform/auth/passwordPolicy";

interface ProfileField {
  label: string;
  value: string;
  tone?: "default" | "strong" | "mono";
}

const authStore = useAuthStore();

const profileFields = computed<ProfileField[]>(() => [
  { label: "用户名", value: authStore.username || "", tone: "strong" },
  { label: "用户 ID", value: authStore.userInfo?.sub || "", tone: "mono" },
  { label: "注册时间", value: "—" }
]);

const loading = shallowRef(false);
const passwordFormRef = ref<FormInstance>();
const passwordPolicy = usePasswordPolicy();

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
        const message = validatePasswordAgainstPolicy(value, authStore.username || "", passwordPolicy.value);
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

const fieldClass = computed(() => (field: ProfileField) => ({
  "profile-value": true,
  "profile-value--strong": field.tone === "strong",
  "profile-value--mono": field.tone === "mono"
}));

const handlePasswordChanged = async () => {
  const redirected = await authStore.logout();
  if (!redirected) {
    window.location.reload();
  }
};

async function handleChangePassword() {
  if (!passwordFormRef.value) return;
  await passwordFormRef.value.validate(async (valid) => {
    if (!valid) return;
    loading.value = true;
    try {
      await platformCustomerApi.changePassword({
        oldPassword: passwordForm.oldPassword,
        newPassword: passwordForm.newPassword
      });
      notifySuccess("密码修改成功，请重新登录");
      passwordForm.oldPassword = "";
      passwordForm.newPassword = "";
      passwordForm.confirmPassword = "";
      await handlePasswordChanged();
    } catch (error) {
      notifyError(error instanceof Error ? error.message : "修改失败，请检查旧密码是否正确");
    } finally {
      loading.value = false;
    }
  });
}
</script>

<style scoped>
.profile-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.profile-password-policy {
  margin: -8px 0 16px 100px;
  color: var(--ds-muted);
  font-size: 12px;
}

/* PortalPagePanel body 无内边距,用 24px 容器排布基本信息与修改密码两个分区 */
.profile-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.profile-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.profile-section + .profile-section {
  padding-top: 24px;
  border-top: 1px solid var(--ds-line);
}

.profile-section__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--ds-ink);
}

.profile-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--ds-line);
}

.profile-row:last-child {
  border-bottom: 0;
}

.profile-row__label {
  font-size: 14px;
  color: var(--ds-muted);
}

.profile-value {
  font-size: 14px;
  color: var(--ds-ink-soft);
}

.profile-value--strong {
  font-weight: 600;
  color: var(--ds-ink);
}

.profile-value--mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>
