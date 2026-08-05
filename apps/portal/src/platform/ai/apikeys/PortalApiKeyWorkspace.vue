<!--
  模型 API 密钥工作区(用户端/租户端共用,embedded 时嵌在密钥管理面板内)。
  重构:迁移至 DsUI——非嵌入态用 PortalPagePanel 一体面板(图标徽章+面包屑+描述同行),
       列表统一 DsTable(嵌入面板 :frame="false"),空态 DsEmpty,分页 DsPagination
       (接口一次返回全量,前端切片分页);颜色全部走 var(--ds-*) token。
       业务逻辑/请求参数/复制与轮换交互不变,编辑弹窗仍为 element-plus(过渡期)。
-->
<script setup lang="ts">
import { computed, inject, onMounted, shallowRef, watch } from "vue";
import { KeyRound } from "lucide-vue-next";
import { DsEmpty, DsPagination, DsTable, type DsTableColumn } from "@/shared/ui";

import PortalEmbeddedPanelFrame from "../../page/PortalEmbeddedPanelFrame.vue";
import PortalPagePanel from "../../page/PortalPagePanel.vue";
import PortalApiKeyEditorDialog from "./PortalApiKeyEditorDialog.vue";
import PortalApiKeyUsageDialog from "./PortalApiKeyUsageDialog.vue";
import { formatMultiplier } from "../utils";
import type {
  PortalApiKeyApi,
  PortalApiKeyGroupRecord,
  PortalApiKeyLimitPolicy,
  PortalApiKeyRecord,
  PortalApiKeyWriteInput
} from "./types";

const props = defineProps<{
  api: PortalApiKeyApi;
  title: string;
  description: string;
  eyebrow: string;
  publicBaseUrl: string;
  embedded?: boolean;
  refreshIcon?: unknown;
  createIcon?: unknown;
  editIcon?: unknown;
  deleteIcon?: unknown;
  copyIcon?: unknown;
  statusOptions: Array<{ label: string; value: string }>;
  formatCredits: (value: number | null | undefined) => string;
  formatWholeCredits: (value: number | null | undefined) => string;
  notifySuccess?: (message: string) => void;
  notifyError?: (message: string) => void;
  notifyWarning?: (message: string) => void;
  confirmDelete?: (name: string) => Promise<boolean>;
  confirmRotate?: () => Promise<boolean>;
}>();

// 嵌入 PortalKeyManagementWorkspace 时向上注册操作方法,由后者在页头统一渲染按钮
const tabActions = inject<Record<string, { loading: boolean; refresh: () => void; create: () => void }> | null>(
  "keyManagementTabActions",
  null
);

const columns: DsTableColumn[] = [
  { key: "name", title: "名称" },
  { key: "key", title: "模型 API 密钥", width: 200 },
  { key: "quota", title: "配额限制", width: 130, align: "right" },
  { key: "used", title: "已使用", width: 130, align: "right" },
  { key: "group", title: "绑定分组", width: 200 },
  { key: "limit", title: "限流策略", width: 240 },
  { key: "status", title: "状态", width: 90 },
  { key: "lastUsed", title: "最后使用时间", width: 170 },
  { key: "created", title: "创建时间", width: 160 },
  { key: "actions", title: "操作", width: 250 }
];

// 面包屑 = eyebrow(按 "/" 拆分的菜单分组) + 页面标题(末级)
const breadcrumbs = computed(() => [
  ...props.eyebrow.split("/").map((item) => ({ label: item.trim() })),
  { label: props.title }
]);

const loading = shallowRef(false);
const apiKeys = shallowRef<PortalApiKeyRecord[]>([]);
const groups = shallowRef<PortalApiKeyGroupRecord[]>([]);
const dialogVisible = shallowRef(false);
const editingKeyId = shallowRef("");
const saving = shallowRef(false);
const generatedKey = shallowRef("");
const showKeyDialog = shallowRef(false);
const revealingKeyId = shallowRef("");
const updatingStatusId = shallowRef("");
const usageDialogVisible = shallowRef(false);
const usageDialogLoading = shallowRef(false);
const usageApiKey = shallowRef<PortalApiKeyRecord | null>(null);
const usagePlaintextKey = shallowRef("");

// 接口一次返回全量,前端切片分页
const page = shallowRef(1);
const pageSize = shallowRef(20);
const pagedApiKeys = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return apiKeys.value.slice(start, start + pageSize.value);
});

function handlePageChange(value: number) {
  page.value = value;
}

function handlePageSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
}

const selectedApiKey = computed(() => apiKeys.value.find((item) => item.id === editingKeyId.value) || null);
const isEditing = computed(() => Boolean(editingKeyId.value));

async function fetchAPIKeys() {
  loading.value = true;
  try {
    const res = await props.api.listApiKeys();
    apiKeys.value = res.items || [];
    page.value = 1;
  } finally {
    loading.value = false;
  }
}

async function fetchGroups() {
  if (!props.api.listGroups) return;
  try {
    const res = await props.api.listGroups();
    groups.value = res.items || [];
  } catch {
    // Group options are auxiliary for older deployments.
  }
}

function resetForm() {
  editingKeyId.value = "";
}

function openCreateDialog() {
  resetForm();
  dialogVisible.value = true;
}

function openEditDialog(row: PortalApiKeyRecord) {
  editingKeyId.value = row.id;
  dialogVisible.value = true;
}

function hasLimitValues(limitPolicy?: PortalApiKeyLimitPolicy | null) {
  return Boolean(limitPolicy && limitPolicy.concurrency_limit != null);
}

function normalizePayload(payload: PortalApiKeyWriteInput) {
  const limitPolicy = payload.limit_policy || null;
  if (!limitPolicy) return payload;
  const hasValues = hasLimitValues(limitPolicy);
  const current = selectedApiKey.value?.limit_policy || null;
  const hadExisting = Boolean(current);
  if (!hasValues && (limitPolicy.status || "disabled") === "disabled" && !hadExisting) {
    return {
      ...payload,
      limit_policy: undefined
    };
  }
  return payload;
}

async function handleSubmit(payload: PortalApiKeyWriteInput) {
  if (!payload.name) {
    props.notifyWarning?.("请输入名称");
    return;
  }
  if (!payload.group_id) {
    props.notifyWarning?.("请选择一个可用分组");
    return;
  }
  saving.value = true;
  try {
    const normalized = normalizePayload(payload);
    if (isEditing.value) {
      await props.api.updateApiKey(editingKeyId.value, normalized);
      props.notifySuccess?.("已更新");
      dialogVisible.value = false;
    } else {
      const res = await props.api.createApiKey(normalized);
      if (res?.plaintext_key) {
        generatedKey.value = res.plaintext_key;
        showKeyDialog.value = true;
        dialogVisible.value = false;
      }
      props.notifySuccess?.("已创建");
    }
    await fetchAPIKeys();
  } catch (error) {
    props.notifyError?.((error as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function updateStatus(row: PortalApiKeyRecord, nextStatus: string) {
  try {
    updatingStatusId.value = row.id;
    await props.api.updateApiKeyStatus(row.id, nextStatus);
    props.notifySuccess?.(nextStatus === "active" ? "已启用" : "已停用");
    await fetchAPIKeys();
  } catch (error) {
    props.notifyError?.((error as Error).message || "更新状态失败");
  } finally {
    updatingStatusId.value = "";
  }
}

async function copyPlaintextKey(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    props.notifySuccess?.("已复制到剪贴板");
    return true;
  } catch {
    props.notifyWarning?.("复制失败，请手动复制");
    generatedKey.value = text;
    showKeyDialog.value = true;
    return false;
  }
}

async function copyPublicBaseUrl() {
  try {
    await navigator.clipboard.writeText(props.publicBaseUrl);
    props.notifySuccess?.("API 接入地址已复制");
  } catch {
    props.notifyWarning?.("复制失败，请手动复制地址");
  }
}

async function rotateKey(row: PortalApiKeyRecord) {
  const confirmed = props.confirmRotate ? await props.confirmRotate() : window.confirm("确定继续轮换 API 密钥吗？");
  if (!confirmed) return;
  try {
    const res = await props.api.rotateApiKey(row.id);
    generatedKey.value = res.plaintext_key;
    showKeyDialog.value = true;
    await fetchAPIKeys();
  } catch (error) {
    props.notifyError?.((error as Error).message || "轮换失败");
  }
}

async function revealKey(row: PortalApiKeyRecord) {
  revealingKeyId.value = row.id;
  try {
    const res = await props.api.revealApiKey(row.id);
    await copyPlaintextKey(res.plaintext_key);
  } catch (error) {
    props.notifyError?.((error as Error).message || "复制密钥失败");
  } finally {
    revealingKeyId.value = "";
  }
}

async function openUsageDialog(row: PortalApiKeyRecord) {
  usageApiKey.value = row;
  usagePlaintextKey.value = "";
  usageDialogVisible.value = true;
  usageDialogLoading.value = true;
  try {
    const res = await props.api.revealApiKey(row.id);
    usagePlaintextKey.value = res.plaintext_key;
  } catch (error) {
    props.notifyError?.((error as Error).message || "读取密钥失败");
  } finally {
    usageDialogLoading.value = false;
  }
}

function closeUsageDialog() {
  usageDialogVisible.value = false;
  usageDialogLoading.value = false;
  usagePlaintextKey.value = "";
  usageApiKey.value = null;
}

function notifyUsageCopied() {
  props.notifySuccess?.("配置已复制到剪贴板");
}

function notifyUsageDownloaded(filename: string) {
  props.notifySuccess?.(`${filename} 已下载`);
}

async function deleteKey(row: PortalApiKeyRecord) {
  const confirmed = props.confirmDelete ? await props.confirmDelete(row.name) : window.confirm(`确定删除 API 密钥「${row.name}」吗？`);
  if (!confirmed) return;
  try {
    await props.api.deleteApiKey(row.id);
    props.notifySuccess?.("已删除");
    await fetchAPIKeys();
  } catch (error) {
    props.notifyError?.((error as Error).message || "删除失败");
  }
}

function groupName(groupId: string) {
  return groups.value.find((group) => group.id === groupId)?.name || groupId;
}

function groupByID(groupId: string) {
  return groups.value.find((group) => group.id === groupId) || null;
}

function groupMultiplier(group: PortalApiKeyGroupRecord) {
  const value = group.effective_user_multiplier;
  if (value == null) return "";
  return `${formatMultiplier(value)}x`;
}

function boundGroupMultiplier(groupId: string) {
  const group = groupByID(groupId);
  return group ? groupMultiplier(group) : "";
}

async function copyKey(text?: string) {
  await copyPlaintextKey(text ?? generatedKey.value);
}

function formatDate(value?: number | null) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function formatLastUsed(value?: number | null) {
  if (!value) return "从未使用";
  return new Date(value).toLocaleString();
}

function formatLimitValue(value: number | null | undefined, label: string) {
  return value != null ? `${Number(value).toLocaleString()} ${label}` : `${label} 不限`;
}

onMounted(() => {
  void fetchAPIKeys();
  void fetchGroups();
});

// 向 PortalKeyManagementWorkspace 注册操作方法 + loading 同步
if (tabActions) {
  tabActions.api.refresh = fetchAPIKeys;
  tabActions.api.create = openCreateDialog;
  watch(loading, (value) => { tabActions.api.loading = value; }, { immediate: true });
}
</script>

<template>
  <div class="apikey-page">
    <component
      :is="embedded ? PortalEmbeddedPanelFrame : PortalPagePanel"
      v-bind="embedded ? { showActions: !tabActions } : { icon: KeyRound, breadcrumbs, description, fill: true }"
    >
      <template #actions>
        <slot name="header-actions" />
        <el-button :icon="refreshIcon" :loading="loading" @click="fetchAPIKeys">刷新</el-button>
        <el-button type="primary" :icon="createIcon" @click="openCreateDialog">创建模型 API 密钥</el-button>
      </template>

      <div class="api-access-address">
        <div class="api-access-address__content">
          <span class="api-access-address__label">API 接入地址</span>
          <code class="api-access-address__value">{{ publicBaseUrl }}</code>
        </div>
        <el-button link type="primary" @click="copyPublicBaseUrl">复制地址</el-button>
      </div>

      <DsTable :frame="false" :columns="columns" :rows="pagedApiKeys" row-key="id" :loading="loading">
        <template #empty>
          <DsEmpty title="暂无 API 密钥" description="还没有模型 API 密钥，先创建一个吧">
            <template #action>
              <el-button type="primary" :icon="createIcon" @click="openCreateDialog">创建模型 API 密钥</el-button>
            </template>
          </DsEmpty>
        </template>
        <template #cell-key="{ row }">
          <div class="key-cell">
            <code class="key-prefix">····{{ row.last_four || "????" }}</code>
            <el-tooltip content="复制密钥" placement="top">
              <el-button
                v-if="copyIcon"
                link
                class="copy-key-button"
                :icon="copyIcon"
                :loading="revealingKeyId === row.id"
                @click.stop="revealKey(row)"
              />
              <el-button
                v-else
                link
                class="copy-key-button copy-key-button--text"
                :loading="revealingKeyId === row.id"
                @click.stop="revealKey(row)"
              >
                复制
              </el-button>
            </el-tooltip>
          </div>
        </template>
        <template #cell-quota="{ row }">
          {{
            row.quota_limit_credits !== null && row.quota_limit_credits !== undefined
              ? formatWholeCredits(row.quota_limit_credits) + " 积分"
              : "无限制"
          }}
        </template>
        <template #cell-used="{ row }">{{ formatCredits(row.quota_used_credits) }} 积分</template>
        <template #cell-group="{ row }">
          <span v-if="row.group_id" class="group-tag">
            <span class="group-tag__name">{{ groupName(row.group_id) }}</span>
            <span v-if="boundGroupMultiplier(row.group_id)" class="group-tag__multiplier">
              {{ boundGroupMultiplier(row.group_id) }}
            </span>
          </span>
          <span v-else class="group-empty">未绑定分组</span>
        </template>
        <template #cell-limit="{ row }">
          <div class="limit-summary" :class="{ 'is-disabled': row.limit_policy?.status === 'disabled' }">
            <span class="limit-pill limit-pill--concurrency">
              {{ formatLimitValue(row.limit_policy?.concurrency_limit, "并发") }}
            </span>
            <span v-if="row.limit_policy?.status === 'disabled'" class="limit-state">已停用</span>
          </div>
        </template>
        <template #cell-status="{ row }">
          <el-switch
            :model-value="row.status"
            active-value="active"
            inactive-value="disabled"
            inline-prompt
            active-text="开"
            inactive-text="关"
            :loading="updatingStatusId === row.id"
            @change="updateStatus(row, String($event))"
          />
        </template>
        <template #cell-lastUsed="{ row }">
          <span class="time-text" :class="{ 'muted-text': !row.last_used_at }">{{ formatLastUsed(row.last_used_at) }}</span>
        </template>
        <template #cell-created="{ row }">
          <span class="time-text">{{ formatDate(row.created_at) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button link type="primary" @click="openUsageDialog(row)">接入配置</el-button>
          <el-button link type="primary" @click="openEditDialog(row)">编辑</el-button>
          <el-button link type="warning" @click="rotateKey(row)">轮换</el-button>
          <el-button link type="danger" @click="deleteKey(row)">删除</el-button>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="apiKeys.length"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </component>

    <PortalApiKeyEditorDialog
      :visible="dialogVisible"
      :loading="saving"
      :api-key="selectedApiKey"
      :groups="groups"
      :status-options="statusOptions"
      @close="dialogVisible = false"
      @submit="handleSubmit"
    />

    <PortalApiKeyUsageDialog
      :visible="usageDialogVisible"
      :loading="usageDialogLoading"
      :api-key="usageApiKey"
      :plaintext-key="usagePlaintextKey"
      :base-url="publicBaseUrl"
      @close="closeUsageDialog"
      @copied="notifyUsageCopied"
      @downloaded="notifyUsageDownloaded"
    />

    <el-dialog v-model="showKeyDialog" title="模型 API 密钥明文" width="520px" append-to-body>
      <div class="key-display">
        <code class="full-key">{{ generatedKey }}</code>
        <p class="key-hint">这个密钥后续也可以在列表中再次复制。</p>
        <el-button :icon="copyIcon" type="primary" @click="copyKey()">复制 Key</el-button>
      </div>
      <template #footer>
        <el-button type="primary" @click="showKeyDialog = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.apikey-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  min-width: 0;
}

/* 嵌入态布局(操作条带/body/分页脚)见 PortalEmbeddedPanelFrame */

.api-access-address {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 24px;
  border-bottom: 1px solid var(--ds-line);
  background: var(--ds-panel);
}

.api-access-address__content {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 12px;
}

.api-access-address__label {
  flex: none;
  color: var(--ds-ink-soft);
  font-size: 13px;
  font-weight: 650;
}

.api-access-address__value {
  min-width: 0;
  overflow-x: auto;
  color: var(--ds-accent-hover);
  font-family: var(--ds-font-mono);
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
}

.key-cell {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.copy-key-button {
  min-height: auto;
  padding: 0;
  color: var(--ds-muted);
}

.copy-key-button:hover {
  color: var(--ds-accent);
}

.copy-key-button--text {
  font-size: 12px;
}

@media (max-width: 640px) {
  .api-access-address {
    align-items: flex-start;
    padding-inline: 16px;
  }

  .api-access-address__content {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }

  .api-access-address__value {
    max-width: min(58vw, 360px);
  }
}

.limit-summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.limit-summary.is-disabled {
  opacity: 0.72;
}

.limit-pill {
  font-weight: 700;
  white-space: nowrap;
}

.limit-pill--concurrency {
  color: var(--ds-positive);
}

.limit-state {
  color: var(--ds-warning);
  font-weight: 700;
}

.group-tag {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  max-width: 150px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 30%, transparent);
  border-radius: var(--ds-radius-pill);
  overflow: hidden;
  background: var(--ds-accent-soft);
}

.group-tag__name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 3px 8px;
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
}

.group-tag__multiplier {
  flex: 0 0 auto;
  padding: 3px 7px;
  border-left: 1px solid color-mix(in srgb, var(--ds-accent) 30%, transparent);
  background: color-mix(in srgb, var(--ds-accent) 16%, var(--ds-accent-soft));
  color: var(--ds-accent-hover);
  font-size: 11px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.group-empty {
  color: var(--ds-warning);
  font-size: 12px;
  font-weight: 700;
}

.time-text {
  font-size: 12px;
  color: var(--ds-muted);
}

.muted-text {
  color: var(--ds-faint);
}

.key-prefix {
  font-family: var(--ds-font-mono);
  font-size: 12.5px;
  color: var(--ds-ink-soft);
  background: var(--ds-panel-muted);
  padding: 2px 8px;
  border-radius: var(--ds-radius-control);
  word-break: break-all;
}

.key-display {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.key-hint {
  margin: 0;
  color: var(--ds-muted);
  font-size: 12px;
}

.full-key {
  font-family: var(--ds-font-mono);
  font-size: 13px;
  word-break: break-all;
  background: var(--ds-ink);
  color: var(--ds-line);
  padding: 16px;
  border-radius: var(--ds-radius-control);
  line-height: 1.6;
}
</style>
