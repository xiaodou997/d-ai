<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import { formatMultiplier } from "../utils";
import type {
  PortalApiKeyGroupRecord,
  PortalApiKeyRecord,
  PortalApiKeyWriteInput
} from "./types";

const props = withDefaults(
  defineProps<{
    visible: boolean;
    loading: boolean;
    apiKey?: PortalApiKeyRecord | null;
    groups: PortalApiKeyGroupRecord[];
    statusOptions: Array<{ label: string; value: string }>;
  }>(),
  {
    apiKey: null,
    groups: () => []
  }
);

const emit = defineEmits<{
  close: [];
  submit: [payload: PortalApiKeyWriteInput];
}>();

const form = reactive({
  name: "",
  quota_limit_usd: null as number | null,
  group_id: "",
  status: "active",
  limit_status: "disabled" as "active" | "disabled",
  concurrency_limit: null as number | null
});

const isEditing = computed(() => Boolean(props.apiKey));
const title = computed(() => (isEditing.value ? "编辑模型 API 密钥" : "创建模型 API 密钥"));
const isAPIKeyActive = computed({
  get: () => form.status === "active",
  set: (value: boolean) => {
    form.status = value ? "active" : "disabled";
  }
});
const limitEnabled = computed({
  get: () => form.limit_status === "active",
  set: (value: boolean) => {
    form.limit_status = value ? "active" : "disabled";
  }
});

watch(
  () => [props.visible, props.apiKey] as const,
  ([visible]) => {
    if (!visible) return;
    form.name = props.apiKey?.name || "";
    form.quota_limit_usd = props.apiKey?.quota_limit_micro_usd == null ? null : props.apiKey.quota_limit_micro_usd / 1_000_000;
    form.group_id = props.apiKey?.group_id || "";
    form.status = props.apiKey?.status || "active";
    form.limit_status = (props.apiKey?.limit_policy?.status as "active" | "disabled") || "disabled";
    form.concurrency_limit = props.apiKey?.limit_policy?.concurrency_limit ?? null;
  },
  { immediate: true }
);

function handleSubmit() {
  emit("submit", {
    name: form.name.trim(),
    group_id: form.group_id,
    quota_limit_micro_usd: form.quota_limit_usd == null ? null : Math.round(form.quota_limit_usd * 1_000_000),
    status: form.status,
    limit_policy: {
      concurrency_limit: form.concurrency_limit === undefined ? null : form.concurrency_limit,
      status: form.limit_status
    }
  });
}

function groupMultiplierLabel(group: PortalApiKeyGroupRecord) {
  const value = group.effective_user_multiplier;
  if (value == null) return "-";
  return `${formatMultiplier(value)}x`;
}

</script>

<template>
  <el-dialog :model-value="visible" :title="title" width="760px" append-to-body @close="emit('close')">
    <el-form :model="form" label-position="top">
      <section class="form-section">
        <div class="section-copy">
          <strong>基础信息</strong>
        </div>
        <div class="form-grid form-grid--top">
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="给模型 API 密钥命名" />
          </el-form-item>
          <el-form-item label="状态">
            <div class="switch-field">
              <el-switch v-model="isAPIKeyActive" />
              <span class="switch-state">{{ isAPIKeyActive ? "启用" : "停用" }}</span>
            </div>
          </el-form-item>
        </div>

        <el-form-item label="配额限制">
          <el-input-number
            v-model="form.quota_limit_usd"
            :min="0"
            :precision="6"
            :step="0.01"
            :controls="false"
            placeholder="不填则无限制"
            class="w-full"
          />
          <p class="field-hint">USD 消费上限，精确到 0.000001 USD，超过后将无法使用</p>
        </el-form-item>
      </section>

      <section class="form-section group-section">
        <div class="section-copy">
          <strong>绑定分组</strong>
          <p class="field-hint">每个密钥只绑定一个分组；故障转移由该分组内的多上游目标完成。</p>
        </div>

        <el-form-item label="分组" required>
          <el-select v-model="form.group_id" filterable class="group-select" placeholder="搜索并选择分组">
            <el-option
              v-for="group in groups"
              :key="group.id"
              :label="`${group.name} · ${groupMultiplierLabel(group)}`"
              :value="group.id"
            />
          </el-select>
        </el-form-item>
      </section>

      <section class="form-section limit-section">
        <div class="section-header">
          <div class="section-copy">
            <strong>限流策略</strong>
            <p class="field-hint">限制该 API key 可以同时占用的请求数。</p>
          </div>
          <div class="switch-field">
            <el-switch v-model="limitEnabled" />
            <span class="switch-state">{{ limitEnabled ? "启用" : "关闭" }}</span>
          </div>
        </div>

        <div v-if="limitEnabled" class="form-grid limit-grid">
          <el-form-item label="最大同时请求数">
            <el-input-number v-model="form.concurrency_limit" :min="1" :step="1" :controls="false" class="w-full" />
            <p class="field-hint">留空表示不限制并发</p>
          </el-form-item>
        </div>
        <div v-else class="limit-disabled">
          <strong>未启用限流</strong>
          <p class="field-hint">留空表示该维度不限制。停用后保留配置但不生效。</p>
        </div>
      </section>
    </el-form>

    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="loading" @click="handleSubmit">{{ isEditing ? "保存" : "创建" }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.form-grid--top {
  align-items: start;
}

.form-section {
  padding: 16px;
  border: 1px solid var(--ds-line);
  border-radius: 8px;
  background: var(--ds-panel);
}

.form-section + .form-section {
  margin-top: 14px;
}

.limit-section {
  background: var(--ds-panel-muted);
}

.group-section {
  background: var(--ds-panel-muted);
}

.section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.section-copy {
  margin-bottom: 8px;
}

.section-copy strong {
  color: var(--ds-ink);
  font-size: 14px;
}

.switch-field {
  min-height: 32px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.switch-state {
  color: var(--ds-muted);
  font-size: 13px;
}

.group-select {
  width: 100%;
}

.limit-grid {
  grid-template-columns: minmax(0, 1fr);
  margin-top: 8px;
}

.limit-disabled {
  margin-top: 10px;
  padding: 14px;
  border: 1px dashed var(--ds-line-strong);
  border-radius: 8px;
  background: var(--ds-panel);
}

.limit-disabled strong {
  color: var(--ds-muted);
  font-size: 13px;
}

.field-hint {
  margin: 4px 0 0;
  color: var(--ds-faint);
  font-size: 12px;
}

@media (max-width: 900px) {
  .form-grid {
    grid-template-columns: 1fr;
  }

  .limit-grid {
    grid-template-columns: 1fr;
  }

  .section-header {
    flex-direction: column;
  }
}
</style>
