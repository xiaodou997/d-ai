<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { ElMessage } from "element-plus";
import { DsNumberInput } from "@/shared/ui";

import type {
  TenantAiGroupWriteRequest,
  TenantAiPriceBook,
  TenantAiVisibleGroup
} from "@/api/types/aiTenant";

const props = defineProps<{
  group?: TenantAiVisibleGroup | null;
  priceBooks: TenantAiPriceBook[];
  submitting: boolean;
}>();

const emit = defineEmits<{
  submit: [payload: TenantAiGroupWriteRequest];
}>();

const visible = defineModel<boolean>({ required: true });
const editing = computed(() => Boolean(props.group?.id));
const selectablePriceBooks = computed(() => props.priceBooks.filter((book) =>
  book.status === "active" || book.id === props.group?.retail_price_book_id
));

interface GroupFormState {
  name: string;
  description: string;
  retail_price_book_id: string;
  default_user_multiplier: number;
  user_default_visible: boolean;
  allow_protocol_conversion: boolean;
  sort_order: number;
  status: "active" | "disabled";
}

const form = reactive<GroupFormState>({
  name: "",
  description: "",
  retail_price_book_id: "",
  default_user_multiplier: 1,
  user_default_visible: true,
  allow_protocol_conversion: true,
  sort_order: 100,
  status: "active"
});

const exclusiveGroup = computed({
  get: () => !form.user_default_visible,
  set: (exclusive: boolean) => {
    form.user_default_visible = !exclusive;
  }
});

function resetForm() {
  const group = props.group;
  Object.assign(form, {
    name: group?.name || "",
    description: group?.description || "",
    retail_price_book_id: group?.retail_price_book_id || selectablePriceBooks.value.find((book) => book.status === "active")?.id || "",
    default_user_multiplier: group?.default_user_multiplier ?? 1,
    user_default_visible: group?.user_default_visible ?? true,
    allow_protocol_conversion: group?.allow_protocol_conversion ?? true,
    sort_order: group?.sort_order ?? 100,
    status: group?.status === "disabled" ? "disabled" : "active"
  });
}

watch(() => [visible.value, props.group, props.priceBooks], ([nextVisible]) => {
  if (nextVisible) resetForm();
}, { immediate: true });

function submit() {
  if (!form.name.trim()) {
    ElMessage.warning("请填写分组名称");
    return;
  }
  if (!form.retail_price_book_id) {
    ElMessage.warning("请选择零售价格表");
    return;
  }
  emit("submit", {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    retail_price_book_id: form.retail_price_book_id,
    default_user_multiplier: form.default_user_multiplier,
    user_default_visible: form.user_default_visible,
    allow_protocol_conversion: form.allow_protocol_conversion,
    sort_order: form.sort_order,
    status: form.status
  });
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="editing ? '编辑分组' : '新建分组'"
    width="min(640px, calc(100vw - 24px))"
    destroy-on-close
  >
    <el-form label-position="top">
      <div class="form-grid">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" maxlength="80" show-word-limit />
        </el-form-item>
        <el-form-item label="零售价格表" required>
          <el-select v-model="form.retail_price_book_id" filterable class="full-field">
            <el-option
              v-for="book in selectablePriceBooks"
              :key="book.id"
              :value="book.id"
              :disabled="book.status !== 'active'"
              :label="`${book.name} · ${book.owner_type === 'platform' ? '平台' : '租户'}${book.status === 'active' ? '' : ' · 已停用'}`"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="默认用户倍率" required>
          <DsNumberInput v-model="form.default_user_multiplier" :min="0" :step="0.1" :precision="4" class="full-field" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" :step="10" :controls="false" class="full-field" />
        </el-form-item>
        <el-form-item label="专属分组">
          <el-switch v-model="exclusiveGroup" inline-prompt active-text="专属" inactive-text="公开" />
          <p class="field-hint">开启后仅指定用户可见；关闭后所有用户可见。</p>
        </el-form-item>
        <el-form-item label="协议转换">
          <el-switch v-model="form.allow_protocol_conversion" />
        </el-form-item>
      </div>
      <el-form-item label="描述">
        <el-input v-model="form.description" type="textarea" :rows="3" maxlength="240" show-word-limit />
      </el-form-item>
      <el-form-item label="状态">
        <el-segmented v-model="form.status" :options="[{ label: '启用', value: 'active' }, { label: '停用', value: 'disabled' }]" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}

.full-field {
  width: 100%;
}

.field-hint {
  width: 100%;
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
