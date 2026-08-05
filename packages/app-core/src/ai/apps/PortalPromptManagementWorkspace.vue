<!--
  提示词管理工作区(租户端/用户端共用)。
  重构:列表页由遗留 PortalPageHeader + PortalContentCard + el-table 迁移到 DsUI 一体面板
       (PortalPagePanel:图标徽章 + 面包屑标题 + 描述同行,筛选/表格/分页同卡),对齐分组管理页;
       补关键词搜索与前端分页(接口一次返回全量),空态 DsEmpty;详情视图同样收进一体面板。
       业务逻辑、请求与导入导出/编辑弹窗(element-plus)保持不变。
-->
<script setup lang="ts">
import { ArrowLeft, FileText } from "lucide-vue-next";
import { computed, onMounted, ref, shallowRef, watch } from "vue";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  type DsTableColumn
} from "@dai/ui";

import PortalContentCard from "../../page/PortalContentCard.vue";
import PortalPagePanel from "../../page/PortalPagePanel.vue";
import PortalPromptEditorDialog from "./PortalPromptEditorDialog.vue";
import type { PortalAppPromptApi, PortalAppPromptDetailRecord, PortalAppPromptRecord, PortalAppPromptWriteInput, PortalAppScope } from "./types";

const props = withDefaults(
  defineProps<{
    api: PortalAppPromptApi;
    scope: PortalAppScope;
    refreshIcon?: unknown;
    createIcon?: unknown;
    editIcon?: unknown;
    deleteIcon?: unknown;
    importIcon?: unknown;
    exportIcon?: unknown;
    notifySuccess?: (message: string) => void;
    notifyError?: (message: string) => void;
    confirmDelete?: (name: string) => Promise<boolean>;
  }>(),
  { scope: "tenant" }
);

interface PromptTransferItem {
  name: string;
  description: string;
  status: "active" | "disabled";
  template_text: string;
}

interface PromptImportPreviewItem extends PromptTransferItem {
  action: "create" | "update";
}

const prompts = shallowRef<PortalAppPromptRecord[]>([]);
const detail = shallowRef<PortalAppPromptDetailRecord | null>(null);
const editorDetail = shallowRef<PortalAppPromptDetailRecord | null>(null);
const loading = shallowRef(false);
const detailLoading = shallowRef(false);
const saving = shallowRef(false);
const updatingStatusIds = shallowRef(new Set<string>());
const dialogVisible = shallowRef(false);
const dialogMode = shallowRef<"create" | "edit">("create");
const importDialogVisible = shallowRef(false);
const importingPrompts = shallowRef(false);
const exportingPrompts = shallowRef(false);
const importFileInput = ref<HTMLInputElement | null>(null);
const importFileName = shallowRef("");
const importItems = shallowRef<PromptTransferItem[]>([]);

const keyword = shallowRef("");
const page = shallowRef(1);
const pageSize = shallowRef(20);

const selectedPrompt = computed(() => detail.value?.prompt || null);
const selectedPromptVariables = computed(() => selectedPrompt.value?.variables || []);
const promptNoun = computed(() => (props.scope === "tenant" ? "租户提示词" : "我的提示词"));
const headerDescription = computed(() => `管理${promptNoun.value}内容；更新会同步作用于绑定应用。`);
const breadcrumbs = computed(() => [
  { label: "智能服务" },
  { label: props.scope === "tenant" ? "应用与密钥" : "密钥与应用" },
  { label: props.scope === "tenant" ? "提示词管理" : "我的提示词" }
]);

const columns: DsTableColumn[] = [
  { key: "name", title: "名称" },
  { key: "variables", title: "变量", width: 240 },
  { key: "status", title: "状态", width: 108, align: "center" },
  { key: "updated", title: "更新时间", width: 180 },
  { key: "actions", title: "操作", width: 170, align: "right" }
];

// 接口一次返回全量:关键词过滤 + 前端切片分页(与分组管理页同款)
const visiblePrompts = computed(() => {
  const query = keyword.value.trim().toLowerCase();
  if (!query) return prompts.value;
  return prompts.value.filter((item) =>
    [item.name, item.description, ...(item.variables || [])]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(query))
  );
});

const pagedPrompts = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return visiblePrompts.value.slice(start, start + pageSize.value);
});

watch([keyword, pageSize], () => {
  page.value = 1;
});

function formatUpdatedAt(value?: number | null) {
  return value ? new Date(value).toLocaleString("zh-CN") : "-";
}
const importPreviewItems = computed<PromptImportPreviewItem[]>(() => {
  const existingNames = new Set(prompts.value.map((prompt) => normalizePromptName(prompt.name)));
  return importItems.value.map((item) => ({
    ...item,
    action: existingNames.has(normalizePromptName(item.name)) ? "update" : "create"
  }));
});

function normalizePromptName(value: string) {
  return value.trim().normalize("NFC");
}

function extractPromptVariables(templateText: string) {
  const variables = new Set<string>();
  for (const match of templateText.matchAll(/\{\{\s*([^{}]+?)\s*\}\}/gu)) {
    const name = match[1]?.trim().normalize("NFC");
    if (name) variables.add(name);
  }
  return [...variables].sort((a, b) => a.localeCompare(b, "zh-CN"));
}

async function fetchPrompts() {
  loading.value = true;
  try {
    const response = await props.api.listPrompts();
    prompts.value = response.items || [];
    if (selectedPrompt.value) {
      const stillExists = prompts.value.some((item) => item.id === selectedPrompt.value?.id);
      if (!stillExists) detail.value = null;
    }
  } catch (error) {
    props.notifyError?.((error as Error).message || "提示词加载失败");
  } finally {
    loading.value = false;
  }
}

async function openDetail(prompt: PortalAppPromptRecord) {
  detailLoading.value = true;
  try {
    detail.value = await props.api.getPrompt(prompt.id);
  } catch (error) {
    props.notifyError?.((error as Error).message || "提示词详情加载失败");
  } finally {
    detailLoading.value = false;
  }
}

function openCreateDialog() {
  editorDetail.value = null;
  dialogMode.value = "create";
  dialogVisible.value = true;
}

async function openEditDialog(prompt?: PortalAppPromptRecord) {
  if (!prompt) {
    if (!detail.value) return;
    editorDetail.value = detail.value;
    dialogMode.value = "edit";
    dialogVisible.value = true;
    return;
  }
  detailLoading.value = true;
  try {
    editorDetail.value = await props.api.getPrompt(prompt.id);
    dialogMode.value = "edit";
    dialogVisible.value = true;
  } catch (error) {
    props.notifyError?.((error as Error).message || "提示词详情加载失败");
  } finally {
    detailLoading.value = false;
  }
}

async function handleSubmit(payload: PortalAppPromptWriteInput) {
  saving.value = true;
  try {
    if (dialogMode.value === "create") {
      await props.api.createPrompt({
        ...payload,
        name: payload.name || "",
        template_text: payload.template_text || ""
      });
      props.notifySuccess?.(`${promptNoun.value}已创建`);
    } else if (editorDetail.value?.prompt) {
      const updatedDetail = await props.api.updatePrompt(editorDetail.value.prompt.id, payload);
      editorDetail.value = updatedDetail;
      if (selectedPrompt.value?.id === updatedDetail.prompt.id) detail.value = updatedDetail;
      props.notifySuccess?.(`${promptNoun.value}已更新`);
    }
    dialogVisible.value = false;
    await fetchPrompts();
  } catch (error) {
    props.notifyError?.((error as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function handleDelete(prompt: PortalAppPromptRecord) {
  const confirmed = props.confirmDelete
    ? await props.confirmDelete(prompt.name)
    : window.confirm(`确定删除${promptNoun.value}「${prompt.name}」吗？`);
  if (!confirmed) return;
  try {
    await props.api.deletePrompt(prompt.id);
    if (selectedPrompt.value?.id === prompt.id) detail.value = null;
    props.notifySuccess?.(`${promptNoun.value}已删除`);
    await fetchPrompts();
  } catch (error) {
    props.notifyError?.((error as Error).message || "删除失败");
  }
}

function setPromptStatusLoading(promptId: string, active: boolean) {
  const next = new Set(updatingStatusIds.value);
  if (active) next.add(promptId);
  else next.delete(promptId);
  updatingStatusIds.value = next;
}

async function togglePromptStatus(prompt: PortalAppPromptRecord, enabled: boolean) {
  if (updatingStatusIds.value.has(prompt.id)) return;
  const status = enabled ? "active" : "disabled";
  if (prompt.status === status) return;
  setPromptStatusLoading(prompt.id, true);
  try {
    const updatedDetail = await props.api.updatePrompt(prompt.id, { status });
    prompts.value = prompts.value.map((item) => (item.id === prompt.id ? updatedDetail.prompt : item));
    if (selectedPrompt.value?.id === prompt.id) detail.value = updatedDetail;
    if (editorDetail.value?.prompt.id === prompt.id) editorDetail.value = updatedDetail;
    props.notifySuccess?.(`${promptNoun.value}已${enabled ? "启用" : "停用"}`);
  } catch (error) {
    props.notifyError?.((error as Error).message || `${enabled ? "启用" : "停用"}失败`);
  } finally {
    setPromptStatusLoading(prompt.id, false);
  }
}

function downloadJSON(data: unknown, filename: string) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function handleExportPrompts() {
  if (!prompts.value.length) return;
  exportingPrompts.value = true;
  try {
    downloadJSON(
      {
        format: "unihub-app-prompts",
        version: 2,
        scope: props.scope,
        exported_at: new Date().toISOString(),
        items: prompts.value.map(({ name, description, status, template_text }) => ({ name, description, status, template_text }))
      },
      `unihub-${props.scope}-prompts-${new Date().toISOString().slice(0, 10)}.json`
    );
    props.notifySuccess?.("导出文件已生成");
  } finally {
    exportingPrompts.value = false;
  }
}

function openImportDialog() {
  importFileName.value = "";
  importItems.value = [];
  importDialogVisible.value = true;
}

async function handleImportFileSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  try {
    const parsed = JSON.parse(await file.text()) as { items?: unknown } | unknown[];
    const rawItems = Array.isArray(parsed) ? parsed : parsed.items;
    if (!Array.isArray(rawItems) || !rawItems.length) throw new Error("导入文件缺少 items 数组");
    const seen = new Set<string>();
    importItems.value = rawItems.map((raw, index) => {
      if (!raw || typeof raw !== "object") throw new Error(`第 ${index + 1} 条提示词格式不正确`);
      const item = raw as Partial<PromptTransferItem>;
      const name = String(item.name || "").trim();
      const templateText = String(item.template_text || "").trim();
      if (!name || !templateText) throw new Error(`第 ${index + 1} 条提示词缺少名称或内容`);
      const normalized = normalizePromptName(name);
      if (seen.has(normalized)) throw new Error(`导入文件中存在重复名称「${name}」`);
      seen.add(normalized);
      return {
        name,
        description: String(item.description || "").trim(),
        status: item.status === "disabled" ? "disabled" : "active",
        template_text: templateText
      };
    });
    importFileName.value = file.name;
  } catch (error) {
    importItems.value = [];
    props.notifyError?.((error as Error).message || "导入文件解析失败");
  } finally {
    input.value = "";
  }
}

async function confirmImportPrompts() {
  if (!importPreviewItems.value.length) return;
  importingPrompts.value = true;
  try {
    const promptByName = new Map(prompts.value.map((prompt) => [normalizePromptName(prompt.name), prompt]));
    for (const item of importPreviewItems.value) {
      const existing = promptByName.get(normalizePromptName(item.name));
      if (existing) {
        await props.api.updatePrompt(existing.id, item);
      } else {
        await props.api.createPrompt(item);
      }
    }
    props.notifySuccess?.(`导入完成，共处理 ${importPreviewItems.value.length} 个提示词`);
    importDialogVisible.value = false;
    await fetchPrompts();
  } catch (error) {
    props.notifyError?.((error as Error).message || "导入失败");
  } finally {
    importingPrompts.value = false;
  }
}

onMounted(fetchPrompts);
</script>

<template>
  <div class="page-container prompt-page">
    <PortalPagePanel
      v-if="!selectedPrompt"
      fill
      :icon="FileText"
      :breadcrumbs="breadcrumbs"
      :description="headerDescription"
    >
      <template #actions>
        <el-button :icon="refreshIcon" :loading="loading" @click="fetchPrompts">刷新</el-button>
        <el-button :icon="importIcon" @click="openImportDialog">导入</el-button>
        <el-button :icon="exportIcon" :loading="exportingPrompts" :disabled="!prompts.length" @click="handleExportPrompts">导出</el-button>
        <el-button type="primary" :icon="createIcon" @click="openCreateDialog">新建提示词</el-button>
      </template>

      <template #filters>
        <DsFilterBar>
          <DsFilterField label="关键词">
            <el-input v-model="keyword" clearable placeholder="搜索名称、描述或变量" class="search-input" />
          </DsFilterField>
          <template #actions>
            <span class="count-label">{{ visiblePrompts.length }} 个提示词</span>
          </template>
        </DsFilterBar>
      </template>

      <div class="prompt-table-wrap">
        <DsTable
          :frame="false"
          :columns="columns"
          :rows="pagedPrompts"
          row-key="id"
          :loading="loading || detailLoading"
        >
          <template #empty>
            <DsEmpty title="暂无提示词" :description="`还没有${promptNoun}，先新建一个吧`">
              <template #action>
                <el-button type="primary" :icon="createIcon" @click="openCreateDialog">新建提示词</el-button>
              </template>
            </DsEmpty>
          </template>
          <template #cell-name="{ row }">
            <div class="name-cell">
              <strong>{{ row.name }}</strong>
              <span>{{ row.description || "未填写描述" }}</span>
            </div>
          </template>
          <template #cell-variables="{ row }">{{ row.variables?.join("、") || "-" }}</template>
          <template #cell-status="{ row }">
            <el-switch
              :model-value="row.status === 'active'"
              :loading="updatingStatusIds.has(row.id)"
              inline-prompt
              active-text="启用"
              inactive-text="停用"
              @click.stop
              @change="togglePromptStatus(row, Boolean($event))"
            />
          </template>
          <template #cell-updated="{ row }">
            <span class="time-text">{{ formatUpdatedAt(row.updated_at) }}</span>
          </template>
          <template #cell-actions="{ row }">
            <el-button link type="primary" @click.stop="openDetail(row)">详情</el-button>
            <el-button link type="warning" @click.stop="openEditDialog(row)">编辑</el-button>
            <el-button link type="danger" @click.stop="handleDelete(row)">删除</el-button>
          </template>
        </DsTable>
      </div>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="visiblePrompts.length"
          @update:page="page = $event"
          @update:page-size="pageSize = $event"
        />
      </template>
    </PortalPagePanel>

    <PortalPagePanel
      v-else
      fill
      :icon="FileText"
      :breadcrumbs="[...breadcrumbs, { label: selectedPrompt.name }]"
      :description="selectedPrompt.description || '暂无描述'"
    >
      <template #actions>
        <el-button :icon="ArrowLeft" @click="detail = null">返回列表</el-button>
        <el-switch
          :model-value="selectedPrompt.status === 'active'"
          :loading="updatingStatusIds.has(selectedPrompt.id)"
          inline-prompt
          active-text="启用"
          inactive-text="停用"
          @change="togglePromptStatus(selectedPrompt, Boolean($event))"
        />
        <el-button :icon="editIcon" @click="openEditDialog()">编辑</el-button>
        <el-button type="danger" plain :icon="deleteIcon" @click="handleDelete(selectedPrompt)">删除</el-button>
      </template>

      <div class="detail-body">
        <PortalContentCard title="提示词内容">
          <template #actions>
            <div class="variable-tags">
              <el-tag v-for="variable in selectedPromptVariables" :key="variable" size="small">{{ variable }}</el-tag>
              <span v-if="!selectedPromptVariables.length" class="count-label">无变量</span>
            </div>
          </template>
          <pre class="prompt-body">{{ selectedPrompt.template_text }}</pre>
        </PortalContentCard>
      </div>
    </PortalPagePanel>

    <PortalPromptEditorDialog
      :visible="dialogVisible"
      :mode="dialogMode"
      :loading="saving"
      :scope="scope"
      :detail="editorDetail"
      @close="dialogVisible = false"
      @submit="handleSubmit"
    />

    <el-dialog v-model="importDialogVisible" title="导入提示词" width="min(760px, 92vw)">
      <div class="import-file-row">
        <el-button :icon="importIcon" @click="importFileInput?.click()">选择 JSON 文件</el-button>
        <span class="count-label">{{ importFileName || "未选择文件" }}</span>
        <input ref="importFileInput" type="file" accept="application/json,.json" class="hidden-file-input" @change="handleImportFileSelected" />
      </div>
      <el-table v-if="importPreviewItems.length" :data="importPreviewItems" max-height="360" size="small">
        <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
        <el-table-column label="动作" width="90">
          <template #default="{ row }"><el-tag :type="row.action === 'create' ? 'success' : 'warning'" size="small">{{ row.action === "create" ? "新建" : "更新" }}</el-tag></template>
        </el-table-column>
        <el-table-column label="变量" min-width="200" show-overflow-tooltip>
          <template #default="{ row }">{{ extractPromptVariables(row.template_text).join("、") || "-" }}</template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="importingPrompts" :disabled="!importPreviewItems.length" @click="confirmImportPrompts">确认导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.prompt-page {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.prompt-table-wrap {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.search-input {
  width: min(420px, 100%);
}

.name-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.name-cell strong {
  overflow-wrap: anywhere;
  color: var(--ds-ink);
  font-size: 13px;
}

.name-cell span {
  overflow: hidden;
  color: var(--ds-muted);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.time-text {
  color: var(--ds-muted);
  font-size: 12px;
}

/* 详情视图:面板 body 无内边距,内容用 24px 容器承载 */
.detail-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 18px 24px;
}

.count-label {
  color: var(--ds-muted);
  font-size: 13px;
}

.variable-tags,
.import-file-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.prompt-body {
  min-height: 260px;
  margin: 0;
  padding: 18px;
  overflow: auto;
  border: 1px solid var(--ds-border);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink);
  font: 14px/1.8 "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.hidden-file-input {
  display: none;
}

.import-file-row {
  margin-bottom: 18px;
}

@media (max-width: 720px) {
  .detail-body {
    padding-inline: 16px;
  }
}
</style>
