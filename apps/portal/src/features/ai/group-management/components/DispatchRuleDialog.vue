<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { ElMessage } from "element-plus";
import type { TenantAiClientSurface, TenantAiDispatchModel, TenantAiDispatchRule, TenantAiDispatchRuleWriteRequest } from "@/api/types/aiTenant";
import { capabilityLabels, clientSurfaceOptions } from "../catalog";
import { selectableDispatchModels, validateMatchPattern } from "../dispatchRulePolicy";
import { capabilityDescription, dispatchMatchOptions, matchPresentation, surfacePresentation, type DispatchMatchType } from "../dispatchRulePresentation";

export interface DispatchRuleDraft {
  client_surface: TenantAiClientSurface;
  match_type: DispatchMatchType;
  match_value: string;
  target_model_code: string;
  priority: number;
  notes: string;
}

const props = defineProps<{
  modelValue: boolean;
  rule?: TenantAiDispatchRule | null;
  models: readonly TenantAiDispatchModel[];
  modelsLoading: boolean;
  modelsError: string;
  priceBookName: string;
  saving: boolean;
}>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  "surface-change": [surface: TenantAiClientSurface];
  save: [payload: TenantAiDispatchRuleWriteRequest];
}>();
const form = reactive<DispatchRuleDraft>({
  client_surface: "openai_chat",
  match_type: "exact",
  match_value: "",
  target_model_code: "",
  priority: 100,
  notes: "",
});
const editing = computed(() => Boolean(props.rule));
const effectiveStatus = computed<"active" | "disabled">(() => props.rule?.status === "disabled" ? "disabled" : "active");
const surface = computed(() => surfacePresentation(form.client_surface));
const match = computed(() => matchPresentation(form.match_type));
const selectableModels = computed(() => selectableDispatchModels(props.models));
const selectedPriced = computed(() => props.models.some((item) => item.model_code === form.target_model_code));
const selectedHasCandidate = computed(() => selectableModels.value.some((item) => item.model_code === form.target_model_code));

function reset(rule?: TenantAiDispatchRule | null) {
  Object.assign(
    form,
    rule
      ? {
          client_surface: rule.client_surface as TenantAiClientSurface,
          match_type: rule.match_type as DispatchMatchType,
          match_value: rule.match_value,
          target_model_code: rule.target_model_code,
          priority: rule.priority,
          notes: rule.notes || "",
        }
      : {
          client_surface: "openai_chat",
          match_type: "exact",
          match_value: "",
          target_model_code: "",
          priority: 100,
          notes: "",
        },
  );
}
watch(
  () => [props.modelValue, props.rule] as const,
  ([visible]) => {
    if (visible) reset(props.rule);
  },
);
watch(
  () => form.client_surface,
  (value) => {
    if (props.modelValue) emit("surface-change", value);
  },
);
function submit() {
  const error = validateMatchPattern(form.match_type, form.match_value);
  if (error) {
    ElMessage.warning(error);
    return;
  }
  if (!form.target_model_code.trim()) {
    ElMessage.warning("请选择或输入目标逻辑模型");
    return;
  }
  emit("save", {
    client_surface: form.client_surface,
    match_type: form.match_type,
    match_value: form.match_value.trim(),
    target_model_code: form.target_model_code.trim(),
    priority: form.priority,
    notes: form.notes.trim() || undefined,
  });
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="editing ? '编辑调度规则' : '新建调度规则'"
    width="760px"
    class="dispatch-dialog"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="rule-intro">
      <span class="rule-intro-title">这条规则做的是一次“模型名改写”：</span>
      <ol class="rule-intro-list">
        <li>用户传入模型名，例如 <code>claude-opus-4-8</code></li>
        <li>命中规则后改写成平台逻辑模型，例如 <code>gpt-5.5</code></li>
        <li>系统再去找可用上游，路由到真实模型</li>
      </ol>
    </div>
    <el-form label-width="108px" class="dispatch-form">
      <el-form-item label="客户端请求格式" required>
        <el-select v-model="form.client_surface" class="full-field">
          <el-option
            v-for="item in clientSurfaceOptions"
            :key="item.id"
            :value="item.id"
            :label="`${item.name} · ${item.endpoint}`"
          />
        </el-select>
        <span class="hint">{{ surface.endpoint }} · {{ capabilityDescription(form.client_surface) }}</span>
      </el-form-item>
      <el-form-item label="匹配方式" required>
        <div class="match-type-row">
          <button
            v-for="item in dispatchMatchOptions"
            :key="item.value"
            type="button"
            class="match-type-chip"
            :class="{ 'is-active': form.match_type === item.value }"
            @click="form.match_type = item.value"
          >
            {{ item.label }}
          </button>
        </div>
        <span class="hint">{{ match.tip }}</span>
      </el-form-item>
      <el-form-item label="匹配值" required>
        <el-input v-model="form.match_value" :placeholder="match.placeholder" />
        <div v-if="match.examples.length" class="example-strip">
          <span class="example-strip-label">常见写法</span>
          <button
            v-for="example in match.examples"
            :key="example"
            type="button"
            class="example-chip"
            @click="form.match_value = example"
          >
            {{ example }}
          </button>
        </div>
        <span class="hint">
          {{ match.description }} 这里匹配的是用户传入的模型名，不是最终上游模型名。
        </span>
      </el-form-item>
      <el-form-item label="逻辑模型" required>
        <el-select
          v-model="form.target_model_code"
          class="full-field"
          filterable
          allow-create
          default-first-option
          clearable
          :loading="modelsLoading"
          placeholder="优先从当前分组价格表中选择，也可直接输入"
        >
          <el-option
            v-for="item in selectableModels"
            :key="item.model_code"
            :label="`${item.model_code} · ${capabilityLabels[surface.capability] || surface.capability} · ${item.available_targets} 个候选`"
            :value="item.model_code"
          />
        </el-select>
        <span class="hint">
          来自当前分组价格表「{{ priceBookName || "当前价格表" }}」，当前有
          {{ selectableModels.length }} 个具备可执行候选的{{ capabilityLabels[surface.capability] || surface.capability
          }}模型；也可直接输入平台 logical model_code。
        </span>
        <el-alert v-if="modelsError" :title="modelsError" type="error" :closable="false" />
        <el-alert
          v-else-if="effectiveStatus === 'active' && form.target_model_code && !selectedPriced"
          title="该模型没有当前入口所需的能力价格，启用前请更换模型。"
          type="warning"
          :closable="false"
        />
        <el-alert
          v-else-if="form.target_model_code && selectedPriced && !selectedHasCandidate"
          title="该模型当前没有可执行上游候选，规则仍可保存，建议保存后打开调度预览检查。"
          type="warning"
          :closable="false"
        />
      </el-form-item>
      <el-form-item label="优先级">
        <el-input-number v-model="form.priority" :min="0" :step="10" />
        <span class="hint">越小越先命中；同一客户端 API 格式下按 priority 升序匹配第一条规则。</span>
      </el-form-item>
      <el-form-item label="备注">
        <el-input
          v-model="form.notes"
          type="textarea"
          :rows="2"
          maxlength="200"
          show-word-limit
          placeholder="说明这条规则用于哪个客户端/场景"
        />
        <span class="hint">保存后可打开“调度预览”，输入真实模型名测试是否命中。</span>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button
        type="primary"
        :loading="saving"
        :disabled="effectiveStatus === 'active' && !selectedPriced"
        @click="submit"
      >
        保存规则
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.full-field { width: 100%; }
.dispatch-form :deep(.el-form-item__content) { display: flex; min-width: 0; flex-direction: column; align-items: stretch; }
.rule-intro { margin-bottom: 18px; padding: 10px 14px; border-left: 3px solid var(--ds-accent); border-radius: 6px; background: color-mix(in srgb, var(--ds-accent) 5%, var(--ds-white)); }
.rule-intro-title { color: var(--ds-ink); font-size: 13px; font-weight: 600; }
.rule-intro-list { margin: 4px 0 0; padding-left: 20px; color: var(--ds-muted); font-size: 12px; line-height: 1.7; }
.rule-intro-list code { padding: 1px 5px; border-radius: 4px; background: color-mix(in srgb, var(--ds-accent) 10%, var(--ds-white)); color: var(--ds-ink); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.match-type-row { display: flex; flex-wrap: wrap; gap: 8px; }
.match-type-chip, .example-chip { border: 1px solid var(--ds-line); border-radius: 999px; cursor: pointer; }
.match-type-chip { padding: 5px 14px; background: var(--ds-white); color: var(--ds-muted); font-size: 13px; }
.match-type-chip:hover, .example-chip:hover { border-color: color-mix(in srgb, var(--ds-accent) 40%, var(--ds-line)); color: var(--ds-ink); }
.match-type-chip.is-active { border-color: var(--ds-accent); background: color-mix(in srgb, var(--ds-accent) 10%, var(--ds-white)); color: var(--ds-accent); font-weight: 600; }
.example-strip { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.example-strip-label, .hint { color: var(--ds-faint); font-size: 12px; }
.example-strip-label { align-self: center; }
.example-chip { padding: 4px 10px; background: color-mix(in srgb, var(--ds-accent) 4%, var(--ds-white)); color: var(--ds-ink); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
.hint { display: block; margin-top: 8px; line-height: 1.5; }
:global(.el-dialog.dispatch-dialog) { width: min(760px, calc(100vw - 24px)) !important; margin: 12px auto; }
@media (max-width: 640px) {
  :global(.el-dialog.dispatch-dialog) { width: calc(100vw - 16px) !important; margin: 8px auto; }
  :global(.el-dialog.dispatch-dialog .el-dialog__header) { padding: 16px 16px 10px; }
  :global(.el-dialog.dispatch-dialog .el-dialog__body) { padding: 8px 16px; }
  :global(.el-dialog.dispatch-dialog .el-dialog__footer) { padding: 10px 16px 16px; }
  .dispatch-form :deep(.el-form-item) { display: block; }
  .dispatch-form :deep(.el-form-item__label) { display: block; width: auto !important; height: auto; margin-bottom: 6px; line-height: 20px; text-align: left; }
  .dispatch-form :deep(.el-form-item__content) { margin-left: 0 !important; }
  .match-type-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); width: 100%; }
  .match-type-chip { width: 100%; padding-inline: 8px; text-align: center; }
  .example-strip { min-width: 0; }
  .example-chip { max-width: 100%; overflow-wrap: anywhere; }
}
</style>
