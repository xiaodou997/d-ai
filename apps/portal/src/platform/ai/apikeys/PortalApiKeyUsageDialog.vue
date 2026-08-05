<script setup lang="ts">
import { computed, onBeforeUnmount, shallowRef, watch } from "vue";
import { Copy, Download, Info } from "lucide-vue-next";
import { DsSkeleton, DsTabs } from "@/shared/ui";

import {
  buildApiKeyUsageFiles,
  downloadApiKeyUsageFile,
  type ApiKeyUsagePlatform,
  type ApiKeyUsageTool
} from "./apiKeyUsageConfig";
import type { PortalApiKeyRecord } from "./types";

const props = defineProps<{
  visible: boolean;
  loading?: boolean;
  apiKey: PortalApiKeyRecord | null;
  plaintextKey: string;
  baseUrl: string;
}>();

const emit = defineEmits<{
  close: [];
  copied: [];
  downloaded: [filename: string];
}>();

const activeTool = shallowRef<ApiKeyUsageTool>("codex");
const activePlatform = shallowRef<ApiKeyUsagePlatform>("macos");
const copiedFileID = shallowRef("");
let copiedTimer: number | undefined;

const toolTabs = [
  { key: "codex", label: "Codex CLI" },
  { key: "claude", label: "Claude Code" },
  { key: "openai", label: "OpenAI 兼容" }
];

const platformTabs = [
  { key: "macos", label: "macOS / Linux" },
  { key: "windows", label: "Windows" }
];

const files = computed(() => {
  if (!props.plaintextKey) return [];
  return buildApiKeyUsageFiles(activeTool.value, {
    baseUrl: props.baseUrl,
    apiKey: props.plaintextKey,
    platform: activePlatform.value
  });
});

const description = computed(() => {
  if (activeTool.value === "codex") return "将下列文件保存到 Codex 配置目录后重新启动 Codex CLI。";
  if (activeTool.value === "claude") return "将配置文件保存到 Claude Code 用户目录后重新启动 Claude Code。";
  return "将环境变量写入项目 .env 或当前终端会话，再启动 OpenAI 兼容客户端。";
});

function clearCopiedState() {
  copiedFileID.value = "";
  if (copiedTimer) window.clearTimeout(copiedTimer);
  copiedTimer = undefined;
}

async function copyFile(fileID: string, content: string) {
  try {
    await navigator.clipboard.writeText(content);
    copiedFileID.value = fileID;
    if (copiedTimer) window.clearTimeout(copiedTimer);
    copiedTimer = window.setTimeout(clearCopiedState, 1600);
    emit("copied");
  } catch {
    // 浏览器拒绝剪贴板权限时仍保留可选中的配置文本。
  }
}

function downloadFile(file: ReturnType<typeof buildApiKeyUsageFiles>[number]) {
  downloadApiKeyUsageFile(file);
  emit("downloaded", file.filename);
}

function selectTool(value: string) {
  activeTool.value = value as ApiKeyUsageTool;
}

function selectPlatform(value: string) {
  activePlatform.value = value as ApiKeyUsagePlatform;
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) clearCopiedState();
  }
);

onBeforeUnmount(clearCopiedState);
</script>

<template>
  <el-dialog
    :model-value="visible"
    width="min(920px, calc(100vw - 32px))"
    append-to-body
    destroy-on-close
    class="api-key-usage-dialog"
    @close="emit('close')"
  >
    <template #header>
      <div class="api-key-usage-dialog__title">
        <span>使用 API 密钥</span>
        <span v-if="apiKey" class="api-key-usage-dialog__key-name">· {{ apiKey.name }}</span>
      </div>
    </template>

    <div class="api-key-usage-dialog__content">
      <p class="api-key-usage-dialog__intro">{{ description }}</p>

      <DsSkeleton v-if="loading" :rows="5" />

      <template v-else-if="plaintextKey">
        <DsTabs
          :tabs="toolTabs"
          :model-value="activeTool"
          @update:model-value="selectTool"
        />

        <DsTabs
          class="api-key-usage-dialog__platform-tabs"
          :tabs="platformTabs"
          :model-value="activePlatform"
          @update:model-value="selectPlatform"
        />

        <div class="api-key-usage-dialog__notice">
          <Info :size="16" />
          <span>含 API 密钥的文件仅供个人使用，请勿提交到代码仓库。</span>
        </div>

        <section v-for="file in files" :key="file.id" class="api-key-config-file">
          <div class="api-key-config-file__bar">
            <div class="api-key-config-file__identity">
              <span class="api-key-config-file__label">{{ file.label }}</span>
              <code class="api-key-config-file__path">{{ file.path }}</code>
            </div>
            <div class="api-key-config-file__actions">
              <el-tooltip :content="copiedFileID === file.id ? '已复制' : '复制内容'" placement="top">
                <el-button
                  circle
                  text
                  :icon="Copy"
                  :aria-label="copiedFileID === file.id ? '已复制' : `复制 ${file.filename}`"
                  @click="copyFile(file.id, file.content)"
                />
              </el-tooltip>
              <el-tooltip :content="`下载 ${file.filename}`" placement="top">
                <el-button
                  circle
                  text
                  :icon="Download"
                  :aria-label="`下载 ${file.filename}`"
                  @click="downloadFile(file)"
                />
              </el-tooltip>
            </div>
          </div>
          <pre class="api-key-config-file__code">{{ file.content }}</pre>
        </section>
      </template>

      <div v-else class="api-key-usage-dialog__error">无法读取该密钥，请关闭后重试。</div>
    </div>

    <template #footer>
      <div class="api-key-usage-dialog__footer">
        <a href="/help/dev/tooling" target="_blank" class="api-key-usage-dialog__docs">查看完整 API 文档</a>
        <el-button @click="emit('close')">关闭</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped>
.api-key-usage-dialog__title {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 8px;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 650;
}

.api-key-usage-dialog__key-name {
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 14px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.api-key-usage-dialog__content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.api-key-usage-dialog__intro {
  margin: 0;
  color: var(--ds-ink-soft);
  line-height: 1.6;
}

.api-key-usage-dialog__platform-tabs {
  margin-top: 4px;
}

.api-key-usage-dialog__notice {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid color-mix(in srgb, var(--ds-warning) 28%, var(--ds-line));
  border-radius: var(--ds-radius-control);
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
  font-size: 13px;
  line-height: 1.5;
}

.api-key-usage-dialog__notice svg {
  flex: none;
  margin-top: 1px;
}

.api-key-config-file {
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
}

.api-key-config-file__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--ds-line);
  background: var(--ds-panel);
}

.api-key-config-file__identity {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
}

.api-key-config-file__label {
  flex: none;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 650;
}

.api-key-config-file__path {
  min-width: 0;
  overflow: hidden;
  color: var(--ds-muted);
  font-family: var(--ds-font-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.api-key-config-file__actions {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 2px;
}

.api-key-config-file__code {
  max-height: 280px;
  margin: 0;
  overflow: auto;
  padding: 16px;
  background: var(--ds-ink);
  color: var(--ds-accent-contrast);
  font-family: var(--ds-font-mono);
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.api-key-usage-dialog__error {
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--ds-danger) 28%, var(--ds-line));
  border-radius: var(--ds-radius-control);
  background: var(--ds-danger-soft);
  color: var(--ds-danger);
}

.api-key-usage-dialog__docs {
  min-width: 0;
  color: var(--ds-accent);
  font-size: 13px;
  text-decoration: none;
}

.api-key-usage-dialog__docs:hover {
  color: var(--ds-accent-hover);
}

.api-key-usage-dialog__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

@media (max-width: 640px) {
  .api-key-usage-dialog__footer {
    align-items: flex-end;
    flex-direction: column;
    gap: 10px;
  }

  .api-key-config-file__bar {
    align-items: flex-start;
  }

  .api-key-config-file__identity {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }

  .api-key-config-file__path {
    max-width: 210px;
  }
}
</style>
