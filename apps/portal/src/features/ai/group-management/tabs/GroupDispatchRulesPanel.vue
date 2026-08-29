<script setup lang="ts">
import { shallowRef } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Play, Plus } from "lucide-vue-next";
import type { TenantAiClientSurface, TenantAiDispatchRule, TenantAiDispatchRuleWriteRequest } from "@/api/types/aiTenant";
import { useGroupDispatchRules } from "../composables/useGroupDispatchRules";
import { errorMessage, showDispatchPriceConflict } from "../problemPresentation";
import DispatchRuleDialog from "../components/DispatchRuleDialog.vue";
import DispatchRuleTable from "../components/DispatchRuleTable.vue";
import GroupDispatchPreviewPanel from "./GroupDispatchPreviewPanel.vue";

const props = defineProps<{
  groupId: string;
  priceBookName: string;
}>();
const state = useGroupDispatchRules({ groupId: () => props.groupId });
const dialogVisible = shallowRef(false);
const previewVisible = shallowRef(false);
const editingRule = shallowRef<TenantAiDispatchRule | null>(null);

function openCreate() {
  editingRule.value = null;
  dialogVisible.value = true;
  void state.loadModels("openai_chat");
}

function openEdit(rule: TenantAiDispatchRule) {
  editingRule.value = rule;
  dialogVisible.value = true;
  void state.loadModels(rule.client_surface);
}

function loadModels(clientSurface: TenantAiClientSurface) {
  void state.loadModels(clientSurface);
}

async function save(payload: TenantAiDispatchRuleWriteRequest) {
  try {
    if (editingRule.value) await state.update(editingRule.value.id, payload);
    else await state.create(payload);
    dialogVisible.value = false;
    ElMessage.success(editingRule.value ? "调度规则已更新" : "调度规则已创建");
  } catch (error: unknown) {
    if (!await showDispatchPriceConflict(error)) {
      ElMessage.error(errorMessage(error, "保存调度规则失败"));
    }
  }
}

async function toggle(rule: TenantAiDispatchRule) {
  const status = rule.status === "active" ? "disabled" : "active";
  if (status === "active" && !rule.can_enable) {
    ElMessage.warning("当前模型没有对应能力价格，无法启用规则");
    return;
  }
  try {
    await state.updateStatus(rule.id, status);
    ElMessage.success(status === "active" ? "规则已启用" : "规则已停用");
  } catch (error: unknown) {
    ElMessage.error(errorMessage(error, "更新规则状态失败"));
  }
}

async function remove(rule: TenantAiDispatchRule) {
  try {
    await ElMessageBox.confirm(`删除规则「${rule.match_value}」？`, "确认删除", { type: "warning" });
    await state.remove(rule.id);
    ElMessage.success("调度规则已删除");
  } catch (error: unknown) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(errorMessage(error, "删除调度规则失败"));
    }
  }
}
</script>

<template>
  <section class="rules-panel">
    <header class="panel-header">
      <div>
        <h3>请求规则</h3>
        <p>{{ priceBookName }} · 将客户端模型名改写为逻辑模型后参与真实调度</p>
      </div>
      <div class="panel-actions">
        <el-button
          size="small"
          :icon="Play"
          @click="previewVisible = !previewVisible"
        >
          {{ previewVisible ? "收起预览" : "调度预览" }}
        </el-button>
        <el-button type="primary" size="small" :icon="Plus" :disabled="state.loading.value" @click="openCreate">
          新建规则
        </el-button>
      </div>
    </header>
    <section v-show="previewVisible" class="preview-card">
      <GroupDispatchPreviewPanel :group-id="groupId" />
    </section>
    <el-alert v-if="state.loadError.value" :title="state.loadError.value" type="error" :closable="false" show-icon />
    <DispatchRuleTable
      :rules="state.rules.value"
      :loading="state.loading.value"
      :saving="state.saving.value"
      :unpriced-rule-ids="state.unpricedRuleIds.value"
      @edit="openEdit"
      @remove="remove"
      @toggle="toggle"
    />
    <DispatchRuleDialog
      v-model="dialogVisible"
      :rule="editingRule"
      :models="state.models.value"
      :models-loading="state.modelsLoading.value"
      :models-error="state.modelsError.value"
      :price-book-name="priceBookName"
      :saving="state.saving.value"
      @surface-change="loadModels"
      @save="save"
    />
  </section>
</template>

<style scoped>
.rules-panel { display: flex; flex-direction: column; gap: 14px; }
.panel-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.panel-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.panel-actions :deep(.el-button + .el-button) { margin-left: 0; }
.panel-header h3 { margin: 0; color: var(--ds-ink); font-size: 16px; letter-spacing: 0; }
.panel-header p { margin: 5px 0 0; color: var(--ds-muted); font-size: 12px; }
.preview-card { overflow: hidden; padding: 16px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel); }
@media (max-width: 640px) {
  .panel-header { align-items: stretch; flex-direction: column; }
  .panel-actions { justify-content: stretch; }
  .panel-actions :deep(.el-button) { flex: 1; }
  .preview-card { padding: 12px; }
}
</style>
