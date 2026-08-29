<!-- 管理端敏感信息保护工作区：配置、规则编辑和预览归入 system-modules feature。 -->
<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Plus, RefreshCw, RotateCcw, Save, ShieldCheck, Trash2, WandSparkles } from "lucide-vue-next";

import {
  systemModulesApi,
  type PIIProtectionConfig,
  type PIIRuleConfig
} from "@/api/systemModules";
import { PortalPagePanel } from "@/platform";
import { DsButton, DsInput, DsSwitch, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

const emptyConfig = (): PIIProtectionConfig => ({
  enabled: false,
  rules: [],
  placeholderPrefix: "DAI"
});

const config = ref<PIIProtectionConfig>(emptyConfig());
const defaults = ref<PIIProtectionConfig>(emptyConfig());
const original = ref("");
const loading = ref(false);
const saving = ref(false);
const previewing = ref(false);
const previewText = ref("联系 alice@example.com，手机号 13800138000");
const previewResult = ref("");

const columns: DsTableColumn[] = [
  { key: "name", title: "规则名称", width: 190 },
  { key: "pattern", title: "匹配正则", wrap: true },
  { key: "enabled", title: "状态", width: 130 },
  { key: "actions", title: "操作", width: 96, align: "right" }
];

const dirty = computed(() => JSON.stringify(config.value) !== original.value);
const enabledRuleCount = computed(() => config.value.rules.filter((rule) => rule.enabled).length);

function cloneConfig(value: PIIProtectionConfig): PIIProtectionConfig {
  return {
    enabled: value.enabled,
    placeholderPrefix: value.placeholderPrefix || "DAI",
    rules: value.rules.map((rule) => ({ ...rule }))
  };
}

async function load() {
  loading.value = true;
  try {
    const [saved, factoryDefaults] = await Promise.all([
      systemModulesApi.getPIIProtectionConfig(),
      systemModulesApi.getPIIProtectionDefaults()
    ]);
    config.value = cloneConfig(saved);
    defaults.value = cloneConfig(factoryDefaults);
    original.value = JSON.stringify(config.value);
    previewResult.value = "";
  } catch (error: any) {
    ElMessage.error(error?.message || "敏感信息保护配置加载失败");
  } finally {
    loading.value = false;
  }
}

function updateRule(index: number, patch: Partial<PIIRuleConfig>) {
  config.value.rules[index] = { ...config.value.rules[index], ...patch };
}

function nextCustomRuleID() {
  const used = new Set(config.value.rules.map((rule) => rule.id));
  let index = 1;
  while (used.has(`custom_${index}`)) index += 1;
  return `custom_${index}`;
}

function addRule() {
  config.value.rules.push({
    id: nextCustomRuleID(),
    name: "自定义规则",
    pattern: "",
    enabled: true,
    system: false
  });
}

function removeRule(index: number) {
  config.value.rules.splice(index, 1);
}

function resetRule(index: number) {
  const current = config.value.rules[index];
  const defaultRule = defaults.value.rules.find((rule) => rule.id === current.id);
  if (defaultRule) config.value.rules[index] = { ...defaultRule };
}

async function restoreDefaults() {
  try {
    await ElMessageBox.confirm("将恢复全部预置规则并移除自定义规则，总开关状态保持不变。", "恢复预置规则", { type: "warning" });
    config.value.rules = defaults.value.rules.map((rule) => ({ ...rule }));
    config.value.placeholderPrefix = defaults.value.placeholderPrefix;
    previewResult.value = "";
  } catch {
    // Cancel keeps the current edits.
  }
}

function normalizePrefix(value: string) {
  config.value.placeholderPrefix = value.toUpperCase().replace(/[^A-Z0-9_]/g, "").slice(0, 32);
}

function validateForm() {
  if (!config.value.placeholderPrefix) return "占位符前缀不能为空";
  if (config.value.rules.length > 64) return "脱敏规则不能超过 64 条";
  const ids = new Set<string>();
  for (const rule of config.value.rules) {
    rule.id = rule.id.trim();
    rule.name = rule.name.trim();
    rule.pattern = rule.pattern.trim();
    if (!rule.id || !rule.name || !rule.pattern) return "规则标识、名称和正则不能为空";
    if (ids.has(rule.id)) return `规则标识 ${rule.id} 重复`;
    ids.add(rule.id);
  }
  return "";
}

async function save() {
  const message = validateForm();
  if (message) {
    ElMessage.warning(message);
    return;
  }
  saving.value = true;
  try {
    const saved = await systemModulesApi.updatePIIProtectionConfig(cloneConfig(config.value));
    config.value = cloneConfig(saved);
    original.value = JSON.stringify(config.value);
    ElMessage.success("敏感信息保护配置已保存并生效");
  } catch (error: any) {
    ElMessage.error(error?.message || "敏感信息保护配置保存失败");
  } finally {
    saving.value = false;
  }
}

async function preview() {
  const message = validateForm();
  if (message) {
    ElMessage.warning(message);
    return;
  }
  if (!previewText.value.trim()) {
    ElMessage.warning("请输入用于验证的文本");
    return;
  }
  previewing.value = true;
  try {
    const result = await systemModulesApi.previewPIIProtection({
      config: cloneConfig(config.value),
      text: previewText.value
    });
    previewResult.value = result.protectedText;
  } catch (error: any) {
    ElMessage.error(error?.message || "脱敏预览失败");
  } finally {
    previewing.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="pii-page">
    <PortalPagePanel
      :icon="ShieldCheck"
      :breadcrumbs="[{ label: '平台运营' }, { label: '敏感信息保护' }]"
      description="管理发送给 AI 上游前执行的敏感信息替换规则"
    >
      <template #actions>
        <DsButton :disabled="loading || saving" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </DsButton>
        <DsButton variant="primary" :disabled="loading || saving || !dirty" @click="save">
          <template #icon><Save :size="14" /></template>
          {{ saving ? "保存中" : "保存配置" }}
        </DsButton>
      </template>

      <div class="pii-workspace">
        <section class="status-band">
          <div>
            <div class="section-title-row">
              <h2>保护状态</h2>
              <DsTag :tone="config.enabled ? 'positive' : 'neutral'">{{ config.enabled ? "已开启" : "已关闭" }}</DsTag>
            </div>
            <p>开启后，请求中的命中内容会在转发前替换，响应返回客户端前恢复原文。</p>
          </div>
          <div class="status-control">
            <span>{{ config.enabled ? "停止全局保护" : "启用全局保护" }}</span>
            <DsSwitch v-model="config.enabled" :disabled="loading" />
          </div>
        </section>

        <section class="workspace-section">
          <div class="section-head">
            <div>
              <div class="section-title-row">
                <h2>替换规则</h2>
                <DsTag tone="info">{{ enabledRuleCount }}/{{ config.rules.length }} 条启用</DsTag>
              </div>
              <p>规则按列表顺序执行，正则语法由 Go RE2 引擎校验。</p>
            </div>
            <div class="section-actions">
              <DsButton size="sm" @click="restoreDefaults">
                <template #icon><RotateCcw :size="13" /></template>
                恢复预置
              </DsButton>
              <DsButton size="sm" variant="primary" @click="addRule">
                <template #icon><Plus :size="13" /></template>
                新增规则
              </DsButton>
            </div>
          </div>

          <DsTable
            :columns="columns"
            :rows="config.rules"
            row-key="id"
            :loading="loading"
            empty-title="暂无脱敏规则"
            empty-description="新增规则后可在预览区验证匹配结果。"
          >
            <template #cell-name="{ row, index }">
              <div class="rule-name-cell">
                <DsInput :model-value="row.name" size="sm" @update:model-value="updateRule(index, { name: $event })" />
                <div class="rule-id-row">
                  <DsTag v-if="row.system" tone="neutral">预置</DsTag>
                  <input
                    class="rule-id-input"
                    :value="row.id"
                    :disabled="row.system"
                    aria-label="规则标识"
                    @input="updateRule(index, { id: ($event.target as HTMLInputElement).value })"
                  />
                </div>
              </div>
            </template>
            <template #cell-pattern="{ row, index }">
              <textarea
                class="pattern-input"
                :value="row.pattern"
                rows="2"
                spellcheck="false"
                aria-label="匹配正则"
                @input="updateRule(index, { pattern: ($event.target as HTMLTextAreaElement).value })"
              />
            </template>
            <template #cell-enabled="{ row, index }">
              <div class="rule-status">
                <DsSwitch :model-value="row.enabled" size="sm" @update:model-value="updateRule(index, { enabled: $event })" />
                <span>{{ row.enabled ? "启用" : "停用" }}</span>
              </div>
            </template>
            <template #cell-actions="{ row, index }">
              <div class="row-actions">
                <DsButton v-if="row.system" size="sm" variant="ghost" title="恢复此规则" @click="resetRule(index)">
                  <template #icon><RotateCcw :size="14" /></template>
                </DsButton>
                <DsButton v-else size="sm" variant="ghost" title="删除规则" @click="removeRule(index)">
                  <template #icon><Trash2 :size="14" /></template>
                </DsButton>
              </div>
            </template>
          </DsTable>
        </section>

        <section class="workspace-section settings-grid">
          <div class="settings-copy">
            <h2>占位符前缀</h2>
            <p>仅支持大写字母、数字和下划线，最长 32 位。</p>
          </div>
          <div class="prefix-control">
            <DsInput :model-value="config.placeholderPrefix" @update:model-value="normalizePrefix" />
            <code>__{{ config.placeholderPrefix || "DAI" }}_PII_EMAIL_1__</code>
          </div>
        </section>

        <section class="workspace-section preview-section">
          <div class="section-head">
            <div>
              <h2>脱敏预览</h2>
              <p>使用当前页面中的未保存规则验证文本，预览内容不会写入配置。</p>
            </div>
            <DsButton :disabled="previewing" @click="preview">
              <template #icon><WandSparkles :size="14" /></template>
              {{ previewing ? "处理中" : "执行预览" }}
            </DsButton>
          </div>
          <div class="preview-grid">
            <label>
              <span>原始文本</span>
              <textarea v-model="previewText" rows="5" maxlength="10000" />
            </label>
            <label>
              <span>替换结果</span>
              <textarea :value="previewResult" rows="5" readonly placeholder="执行预览后显示结果" />
            </label>
          </div>
        </section>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.pii-page {
  display: flex;
  min-height: 100%;
  flex-direction: column;
}

.pii-workspace {
  display: flex;
  flex-direction: column;
}

.status-band,
.workspace-section {
  padding: 20px 24px;
}

.status-band {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  background: var(--ds-panel-muted);
}

.workspace-section {
  border-top: 1px solid var(--ds-line);
}

.section-head,
.section-title-row,
.section-actions,
.status-control,
.rule-status,
.row-actions {
  display: flex;
  align-items: center;
}

.section-head {
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 16px;
}

.section-title-row,
.section-actions,
.status-control,
.rule-status,
.row-actions {
  gap: 10px;
}

h2,
p {
  margin: 0;
}

h2 {
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 650;
}

p {
  margin-top: 5px;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.status-control {
  flex: none;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 600;
}

.rule-name-cell {
  display: grid;
  gap: 7px;
}

.rule-id-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.rule-id-input,
.pattern-input,
.preview-grid textarea {
  min-width: 0;
  border: 1px solid var(--ds-line-strong);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  color: var(--ds-ink);
  outline: none;
}

.rule-id-input:focus,
.pattern-input:focus,
.preview-grid textarea:focus {
  border-color: var(--ds-accent);
  box-shadow: 0 0 0 2px var(--ds-accent-soft);
}

.rule-id-input {
  width: 100%;
  border: none;
  color: var(--ds-muted);
  font: 11px var(--ds-font-mono);
}

.rule-id-input:disabled {
  background: transparent;
}

.pattern-input {
  width: 100%;
  min-height: 58px;
  resize: vertical;
  padding: 9px 10px;
  font: 12px/1.5 var(--ds-font-mono);
}

.rule-status {
  color: var(--ds-ink-soft);
  font-size: 12px;
}

.row-actions {
  justify-content: flex-end;
}

.settings-grid {
  display: grid;
  grid-template-columns: minmax(180px, 0.5fr) minmax(320px, 1fr);
  align-items: center;
  gap: 24px;
}

.prefix-control {
  display: grid;
  grid-template-columns: minmax(180px, 280px) minmax(0, 1fr);
  align-items: center;
  gap: 14px;
}

.prefix-control code {
  overflow: hidden;
  padding: 9px 12px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink-soft);
  font: 12px var(--ds-font-mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.preview-grid label {
  display: grid;
  gap: 7px;
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 600;
}

.preview-grid textarea {
  width: 100%;
  resize: vertical;
  padding: 10px 12px;
  font: 12px/1.6 var(--ds-font-mono);
}

.preview-grid textarea[readonly] {
  background: var(--ds-panel-muted);
}

@media (max-width: 800px) {
  .status-band,
  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .settings-grid,
  .prefix-control,
  .preview-grid {
    grid-template-columns: 1fr;
  }

  .status-band,
  .workspace-section {
    padding-inline: 16px;
  }
}
</style>
