<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RefreshCw, Save } from "lucide-vue-next";
import { ElMessage } from "element-plus";
import { PortalContentCard } from "@/platform";
import { DsButton, DsInput, DsSelect, DsTag } from "@/shared/ui";
import {
  systemModulesApi,
  type RequestRecordingSettings
} from "@/api/systemModules";

const levelOptions = [
  { label: "BASIC — 仅基本信息，不保存请求头和正文", value: "basic" },
  { label: "HEADERS — 保存脱敏后的请求头和响应头", value: "headers" },
  { label: "FULL — 保存脱敏请求头及完整请求响应", value: "full" }
];

const loading = ref(true);
const saving = ref(false);
const loaded = ref(false);
const loadError = ref("");
const level = ref<RequestRecordingSettings["level"]>("basic");
const headers = ref("");
const savedValue = ref("");

function normalizeHeaders(value: string) {
  return [...new Set(value
    .split(",")
    .map((name) => name.trim().toLowerCase())
    .filter(Boolean))];
}

const value = computed<RequestRecordingSettings>(() => ({
  level: level.value,
  sensitive_headers: normalizeHeaders(headers.value)
}));
const changed = computed(() => loaded.value && JSON.stringify(value.value) !== savedValue.value);

function accept(settings: RequestRecordingSettings) {
  level.value = settings.level;
  headers.value = normalizeHeaders((settings.sensitive_headers ?? []).join(",")).join(", ");
  savedValue.value = JSON.stringify(value.value);
  loaded.value = true;
  loadError.value = "";
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function updateLevel(next: string | number) {
  if (next === "basic" || next === "headers" || next === "full") level.value = next;
}

async function load() {
  loading.value = true;
  loadError.value = "";
  try {
    accept(await systemModulesApi.getRequestRecordingSettings());
  } catch (error) {
    loaded.value = false;
    loadError.value = errorMessage(error, "服务暂时不可用");
    ElMessage.error(`读取请求记录设置失败：${loadError.value}`);
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!loaded.value || !changed.value) return;
  saving.value = true;
  try {
    accept(await systemModulesApi.updateRequestRecordingSettings(value.value));
    ElMessage.success("请求记录设置已保存，对新请求立即生效");
  } catch (error) {
    ElMessage.error(`保存请求记录设置失败：${errorMessage(error, "请稍后重试")}`);
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <PortalContentCard
    id="request-recording-settings"
    title="请求记录"
    description="控制请求与响应详情的入库范围；请求元数据、用量和消费记录始终保留。"
  >
    <template #actions>
      <DsTag :tone="loaded ? 'positive' : loadError ? 'danger' : 'neutral'">
        {{ loaded ? "配置已载入" : loadError ? "读取失败" : "正在读取" }}
      </DsTag>
      <DsButton size="sm" :disabled="loading || saving" @click="load">
        <template #icon><RefreshCw :size="13" /></template>
        {{ loadError ? "重新加载" : "刷新" }}
      </DsButton>
      <DsButton v-if="!loadError" variant="primary" size="sm" :loading="saving" :disabled="loading || !changed" @click="save">
        <template #icon><Save :size="13" /></template>
        保存
      </DsButton>
    </template>

    <div v-loading="loading" class="recording-settings" :aria-busy="loading">
      <div v-if="loadError" class="recording-error" role="alert">
        <strong>无法读取请求记录设置</strong>
        <span>{{ loadError }}</span>
      </div>

      <template v-else>
        <div class="recording-fields">
          <label class="recording-field" for="request-record-level">
            <span>记录详细程度</span>
            <DsSelect
              id="request-record-level"
              :model-value="level"
              :options="levelOptions"
              :disabled="!loaded || saving"
              @update:model-value="updateLevel"
            />
            <small>敏感请求头和响应头在任何档位下都不会以明文保存。</small>
          </label>

          <label class="recording-field" for="request-sensitive-headers">
            <span>敏感请求头</span>
            <DsInput
              id="request-sensitive-headers"
              v-model="headers"
              :disabled="!loaded || saving"
              placeholder="authorization, x-api-key, cookie"
            />
            <small>使用逗号分隔；系统内置的身份凭据名单不可移除，并同时应用于响应头。</small>
          </label>
        </div>

        <div v-if="level === 'full'" class="recording-warning" role="status">
          FULL 会保存请求和响应正文，可能包含敏感内容，并显著增加数据库空间占用。
        </div>

        <p class="recording-note">设置只影响保存后的新请求；历史记录不会自动改写，可在下方“数据生命周期”中清理。</p>
      </template>
    </div>
  </PortalContentCard>
</template>

<style scoped>
.recording-settings { min-height: 96px; }
.recording-fields { display: grid; grid-template-columns: minmax(260px, 0.9fr) minmax(320px, 1.1fr); gap: 18px; }
.recording-field { display: flex; flex-direction: column; gap: 7px; min-width: 0; color: var(--ds-ink-soft); font-size: 12px; }
.recording-field > span { color: var(--ds-ink); font-size: 13px; font-weight: 650; }
.recording-field small, .recording-note { color: var(--ds-muted); font-size: 12px; line-height: 1.6; }
.recording-note { margin: 16px 0 0; }
.recording-warning, .recording-error { margin-top: 16px; padding: 12px 14px; border: 1px solid var(--ds-line-strong); border-radius: var(--ds-radius-control); font-size: 12px; line-height: 1.6; }
.recording-warning { background: var(--ds-warning-soft); color: var(--ds-warning); }
.recording-error { display: flex; flex-direction: column; gap: 3px; margin-top: 0; background: var(--ds-danger-soft); color: var(--ds-danger); }
@media (max-width: 800px) { .recording-fields { grid-template-columns: 1fr; } }
</style>
