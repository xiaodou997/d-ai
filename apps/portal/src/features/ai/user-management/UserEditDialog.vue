<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import type { FormInstance, FormRules } from "element-plus";

import { aiTenantApi } from "@/api/aiTenant";
import { platformTenantApi } from "@/api/platformTenant";
import type { EndUserItem } from "@/api/types/platformTenant";

const props = defineProps<{
  open: boolean;
  user: EndUserItem | null;
}>();

const emit = defineEmits<{
  close: [];
  saved: [];
}>();

const formRef = ref<FormInstance>();
const loading = ref(false);
const saving = ref(false);
const form = reactive<{
  email: string;
  phone: string;
  internalNote: string;
  concurrencyLimit: number | null;
}>({
  email: "",
  phone: "",
  internalNote: "",
  concurrencyLimit: null
});

const rules: FormRules = {
  email: [{ type: "email", message: "请输入有效的邮箱地址", trigger: "blur" }],
  phone: [{ max: 32, message: "手机号不能超过 32 个字符", trigger: "blur" }],
  internalNote: [{ max: 500, message: "内部备注不能超过 500 个字符", trigger: "blur" }]
};

function resetForm() {
  form.email = props.user?.email ?? "";
  form.phone = props.user?.phone ?? "";
  form.internalNote = props.user?.internalNote ?? "";
  form.concurrencyLimit = null;
  formRef.value?.clearValidate();
}

async function loadConcurrency() {
  const userId = props.user?.userId;
  if (!props.open || !userId) return;
  loading.value = true;
  try {
    const response = await aiTenantApi.listUserLimitPolicies(userId);
    form.concurrencyLimit = response.items?.[0]?.concurrency_limit ?? null;
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "加载用户并发上限失败");
  } finally {
    loading.value = false;
  }
}

async function submit() {
  const userId = props.user?.userId;
  if (!userId) return;
  const valid = await formRef.value?.validate().catch(() => false);
  if (!valid) return;

  saving.value = true;
  try {
    await platformTenantApi.updateEndUser(userId, {
      email: form.email.trim(),
      phone: form.phone.trim(),
      internalNote: form.internalNote.trim()
    });
    try {
      await aiTenantApi.upsertUserLimitPolicy(userId, {
        concurrency_limit: form.concurrencyLimit,
        status: "active"
      });
    } catch (error) {
      emit("saved");
      ElMessage.warning(error instanceof Error ? `用户资料已保存；${error.message}` : "用户资料已保存，但并发上限保存失败");
      return;
    }
    ElMessage.success("用户资料已保存");
    emit("saved");
    emit("close");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存用户资料失败");
  } finally {
    saving.value = false;
  }
}

watch(
  () => [props.open, props.user?.userId] as const,
  ([open, userId]) => {
    if (!open || !userId) return;
    resetForm();
    void loadConcurrency();
  },
  { immediate: true }
);
</script>

<template>
  <el-dialog
    v-if="open"
    :model-value="true"
    :title="user ? `编辑用户 · ${user.username}` : '编辑用户'"
    width="min(560px, calc(100vw - 32px))"
    append-to-body
    destroy-on-close
    :close-on-click-modal="false"
    @close="emit('close')"
  >
    <el-form ref="formRef" v-loading="loading" :model="form" :rules="rules" label-position="top">
      <div class="user-edit-dialog__grid">
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="未填写" clearable />
        </el-form-item>
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" placeholder="未填写" clearable maxlength="32" />
        </el-form-item>
      </div>

      <el-form-item label="内部备注" prop="internalNote">
        <el-input
          v-model="form.internalNote"
          type="textarea"
          :rows="3"
          maxlength="500"
          show-word-limit
          placeholder="仅租户内部可见"
        />
      </el-form-item>

      <el-form-item label="并发上限">
        <el-input-number
          v-model="form.concurrencyLimit"
          :min="1"
          :step="1"
          :controls="false"
          placeholder="不填表示不限"
          class="user-edit-dialog__number"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button :disabled="saving" @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.user-edit-dialog__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.user-edit-dialog__number {
  width: 100%;
}

@media (max-width: 640px) {
  .user-edit-dialog__grid {
    grid-template-columns: 1fr;
  }
}
</style>
