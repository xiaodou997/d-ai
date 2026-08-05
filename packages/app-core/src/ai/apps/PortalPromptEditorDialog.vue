<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import type { PortalAppPromptDetailRecord, PortalAppPromptWriteInput, PortalAppScope } from "./types";

const props = defineProps<{
  visible: boolean;
  mode: "create" | "edit";
  loading: boolean;
  scope: PortalAppScope;
  detail?: PortalAppPromptDetailRecord | null;
}>();

const emit = defineEmits<{
  close: [];
  submit: [payload: PortalAppPromptWriteInput];
}>();

const form = reactive({
  name: "",
  description: "",
  status: "active" as "active" | "disabled",
  templateText: ""
});

const promptNoun = computed(() => (props.scope === "tenant" ? "租户提示词" : props.scope === "user" ? "我的提示词" : "提示词"));
const title = computed(() => `${props.mode === "create" ? "新建" : "编辑"}${promptNoun.value}`);
const nameChanged = computed(
  () => props.mode === "edit" && form.name.trim() !== (props.detail?.prompt.name || "").trim()
);
const submitDisabled = computed(() => !form.name.trim() || !form.templateText.trim());

watch(
  () => [props.visible, props.detail] as const,
  ([visible]) => {
    if (!visible) return;
    form.name = props.detail?.prompt.name || "";
    form.description = props.detail?.prompt.description || "";
    form.status = props.detail?.prompt.status || "active";
    form.templateText = props.detail?.prompt.template_text || "";
  },
  { immediate: true }
);

function handleSubmit() {
  if (submitDisabled.value) return;
  emit("submit", {
    name: form.name.trim(),
    description: form.description.trim(),
    status: form.status,
    template_text: form.templateText.trim()
  });
}
</script>

<template>
  <el-dialog :model-value="visible" :title="title" width="min(760px, 92vw)" @close="emit('close')">
    <el-form label-position="top">
      <el-form-item label="名称" required>
        <el-input v-model="form.name" placeholder="例如：客户背景" maxlength="80" show-word-limit />
      </el-form-item>
      <el-alert
        v-if="nameChanged"
        class="rename-alert"
        type="warning"
        :closable="false"
        title="重命名会同步改变动态提示词应用的占位符名称；旧名称将不再匹配。"
      />
      <el-form-item label="描述">
        <el-input v-model="form.description" type="textarea" :rows="2" placeholder="说明用途与适用范围" />
      </el-form-item>
      <el-form-item label="状态">
        <el-switch v-model="form.status" active-value="active" inactive-value="disabled" active-text="启用" inactive-text="停用" />
      </el-form-item>
      <el-form-item label="提示词内容" required>
        <el-input
          v-model="form.templateText"
          type="textarea"
          :rows="12"
          placeholder="例如：客户名称是 {{客户名称}}，请基于以下背景回答。"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="loading" :disabled="submitDisabled" @click="handleSubmit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.rename-alert {
  margin: -4px 0 18px;
}
</style>
