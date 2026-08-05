<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import type { ServiceSourceItem } from "@/api/types/admin";

const props = defineProps<{ modelValue: boolean; source?: ServiceSourceItem | null; saving?: boolean }>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  save: [value: { sourceCidr: string; description: string }];
}>();

const form = reactive({ sourceCidr: "", description: "" });
const title = computed(() => (props.source ? "编辑外部来源" : "添加外部来源"));

watch(
  () => [props.modelValue, props.source] as const,
  ([open, source]) => {
    if (open) Object.assign(form, { sourceCidr: source?.sourceCidr || "", description: source?.description || "" });
  },
  { immediate: true }
);

function submit() {
  if (!form.sourceCidr.trim()) return;
  emit("save", { sourceCidr: form.sourceCidr.trim(), description: form.description.trim() });
}
</script>

<template>
  <el-dialog :model-value="modelValue" :title="title" width="min(520px, 92vw)" append-to-body @update:model-value="emit('update:modelValue', $event)">
    <el-form label-position="top" @submit.prevent="submit">
      <el-form-item label="IP / CIDR" required>
        <el-input v-model="form.sourceCidr" placeholder="203.0.113.0/24" />
      </el-form-item>
      <el-form-item label="备注">
        <el-input v-model="form.description" type="textarea" :rows="3" placeholder="例如：悉尼生产集群出口" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" :disabled="!form.sourceCidr.trim()" @click="submit">保存来源</el-button>
    </template>
  </el-dialog>
</template>
