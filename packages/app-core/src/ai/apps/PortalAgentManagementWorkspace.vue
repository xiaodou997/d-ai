<!--
  应用管理工作区(租户端/用户端共用)。
  重构:遗留 PortalPageHeader(大标题) + 游离 el-segmented + PortalContentCard/el-table
       迁移到 DsUI 一体面板 —— PortalPagePanel(图标徽章 + 面包屑标题 + 描述同行),
       模式切换换 DsTabs(同密钥管理页),创建态只留模板网格(不再重复一层大标题),
       管理态补关键词搜索 + 前端分页(DsFilterBar/DsTable/DsEmpty/DsPagination,同分组管理页),
       详情态收进同款面板(面包屑追加应用名,操作提到页头)。业务逻辑与请求保持不变。
-->
<script setup lang="ts">
import { ArrowLeft, Blocks, Braces, ChevronRight, Images, KeyRound, Layers3, MessageSquare } from "lucide-vue-next";
import { computed, onMounted, shallowRef, watch } from "vue";
import {
  DsEmpty,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsTable,
  DsTabs,
  type DsTableColumn
} from "@dai/ui";

import PortalContentCard from "../../page/PortalContentCard.vue";
import PortalPagePanel from "../../page/PortalPagePanel.vue";
import {
  agentTypeLabel,
  appCurlExample,
  appRuntimeConfig,
  creativityLabel,
  PORTAL_DEFAULT_RESOLUTION,
  resolutionLabel
} from "./contract";
import PortalAgentEditorDialog from "./PortalAgentEditorDialog.vue";
import PortalAppPreviewPanel from "./PortalAppPreviewPanel.vue";
import type {
  PortalAppApi,
  PortalAppModelRecord,
  PortalAppPromptRecord,
  PortalAppRecord,
  PortalAppScope,
  PortalAppTemplate,
  PortalAppTemplateRecord,
  PortalAppTemplateId,
  PortalAppWriteInput
} from "./types";

const props = withDefaults(
  defineProps<{
    api: PortalAppApi;
    scope: PortalAppScope;
    refreshIcon?: unknown;
    createIcon?: unknown;
    editIcon?: unknown;
    deleteIcon?: unknown;
    notifySuccess?: (message: string) => void;
    notifyError?: (message: string) => void;
    confirmDelete?: (name: string) => Promise<boolean>;
  }>(),
  { scope: "tenant" }
);

const templateIcons = {
  standard_chat: MessageSquare,
  keyword_selector: KeyRound,
  dynamic_prompt_composition: Braces,
  text_to_image: Layers3,
  image_to_image: Images
};

const templates = shallowRef<PortalAppTemplate[]>([]);
const apps = shallowRef<PortalAppRecord[]>([]);
const prompts = shallowRef<PortalAppPromptRecord[]>([]);
const models = shallowRef<PortalAppModelRecord[]>([]);
const loading = shallowRef(false);
const saving = shallowRef(false);
const loadingModels = shallowRef(false);
const dialogVisible = shallowRef(false);
const mode = shallowRef<"create" | "manage">("create");
const selectedAppId = shallowRef("");
const editingApp = shallowRef<PortalAppRecord | null>(null);
const selectedTemplate = shallowRef<PortalAppTemplate | null>(null);
const publishing = shallowRef<Record<string, boolean>>({});

const keyword = shallowRef("");
const page = shallowRef(1);
const pageSize = shallowRef(20);

const activeApps = computed(() => apps.value);
const selectedApp = computed(() => activeApps.value.find((item) => item.id === selectedAppId.value) || null);
const appNoun = computed(() => (props.scope === "tenant" ? "租户应用" : "我的应用"));
const breadcrumbs = computed(() => [
  { label: "智能服务" },
  { label: props.scope === "tenant" ? "应用与密钥" : "密钥与应用" },
  { label: props.scope === "tenant" ? "应用管理" : "我的应用" }
]);
const modeTabs = [
  { key: "create", label: "创建应用" },
  { key: "manage", label: "管理应用" }
];

const columns = computed<DsTableColumn[]>(() => [
  { key: "name", title: "名称" },
  { key: "template", title: "模板", width: 180 },
  { key: "capability", title: "能力", width: 100, align: "center" },
  { key: "model", title: "模型", width: 200 },
  { key: "prompts", title: "提示词", width: 220 },
  ...(props.api.setPublication
    ? [{ key: "publication", title: publicationLabel, width: 120, align: "center" } as DsTableColumn]
    : []),
  { key: "status", title: "状态", width: 100, align: "center" },
  { key: "actions", title: "操作", width: 170, align: "right" }
]);

// 接口一次返回全量:关键词过滤 + 前端切片分页(同分组管理页)
const visibleApps = computed(() => {
  const query = keyword.value.trim().toLowerCase();
  if (!query) return activeApps.value;
  return activeApps.value.filter((app) =>
    [
      app.name,
      app.description,
      app.model_code,
      promptStrategyLabel(app),
      ...(app.prompt_bindings || []).map((binding) => binding.prompt_name)
    ]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(query))
  );
});

const pagedApps = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return visibleApps.value.slice(start, start + pageSize.value);
});

watch([keyword, pageSize, mode], () => {
  page.value = 1;
});

function appPromptNames(app: PortalAppRecord) {
  return (app.prompt_bindings || []).map((binding) => binding.prompt_name).join("、") || "-";
}

function handleModeChange(key: string) {
  mode.value = key as "create" | "manage";
  selectedAppId.value = "";
}
const publicationLabel = "开放给用户";
const selectedVariables = computed(() => {
  if (!selectedApp.value || selectedApp.value.prompt_strategy === "none") return [];
  return [...new Set(selectedApp.value.prompt_bindings.flatMap((binding) => binding.variables || []))];
});
const selectedRuntimeConfig = computed(() => (selectedApp.value ? appRuntimeConfig(selectedApp.value) : {}));
const runtimeSettingRows = computed(() => {
  if (!selectedApp.value) return [];
  const config = selectedRuntimeConfig.value;
  if (selectedApp.value.capability === "chat") {
    return [
      ["创造性", creativityLabel(config.chat?.creativity ?? "balanced")],
      ["附件", config.chat?.allow_attachments ? "允许图片和文件" : "不允许"]
    ];
  }
  return [
    ["输出尺寸", resolutionLabel(config.image?.resolution || PORTAL_DEFAULT_RESOLUTION)],
    ...(config.image?.resolution && config.image.resolution !== "auto" ? [["输出比例", config.image.aspect_ratio]] : []),
    ["图片数量", config.image?.allow_output_count_override ? `默认 ${config.image.default_output_count}，最多 ${config.image.max_output_count}` : `固定 ${config.image?.default_output_count || 1}`]
  ];
});
const selectedCurlExample = computed(() =>
  selectedApp.value
    ? appCurlExample(selectedApp.value.capability, selectedVariables.value, selectedApp.value.prompt_strategy, "{PUBLIC_BASE_URL}", "rk_xxx", Boolean(selectedRuntimeConfig.value.chat?.allow_attachments), selectedApp.value.prompt_bindings.map((item) => item.prompt_name))
    : ""
);

function templateById(id: PortalAppTemplateId) {
  return templates.value.find((template) => template.id === id) || null;
}

function templateForApp(app: PortalAppRecord) {
  if (app.prompt_strategy === "caller_variables") return templateById("keyword_selector");
  if (app.prompt_strategy === "bound_prompt_exact") return templateById("dynamic_prompt_composition");
  if (app.capability === "image_generation") return templateById("text_to_image");
  if (app.capability === "image_edit") return templateById("image_to_image");
  return templateById("standard_chat");
}

function promptStrategyLabel(app: PortalAppRecord) {
  return templateForApp(app)?.name || "未知模板";
}

function normalizeTemplate(record: PortalAppTemplateRecord): PortalAppTemplate {
  return {
    id: record.id,
    name: record.name,
    description: record.description,
    defaultCapability: record.default_capability,
    allowedCapabilities: record.allowed_capabilities,
    promptStrategy: record.prompt_strategy,
    minPromptBindings: record.min_prompt_bindings,
    maxPromptBindings: record.max_prompt_bindings
  };
}

function appPublished(app: PortalAppRecord) {
  return Boolean(app.published_by_tenant);
}

async function fetchAll() {
  loading.value = true;
  loadingModels.value = true;
  try {
    const [templateRes, appRes, promptRes, modelItems] = await Promise.all([
      props.api.listTemplates(),
      props.api.listApps(),
      props.api.listPrompts(),
      props.api.listModels?.().catch((error) => {
        props.notifyError?.((error as Error).message || "模型列表加载失败");
        return [];
      }) || Promise.resolve([])
    ]);
    templates.value = (templateRes.items || []).map(normalizeTemplate);
    if (!selectedTemplate.value) selectedTemplate.value = templates.value[0] || null;
    apps.value = appRes.items || [];
    prompts.value = promptRes.items || [];
    models.value = modelItems || [];
  } catch (error) {
    props.notifyError?.((error as Error).message || "应用数据加载失败");
  } finally {
    loading.value = false;
    loadingModels.value = false;
  }
}

function chooseTemplate(template: PortalAppTemplate) {
  selectedTemplate.value = template;
  editingApp.value = null;
  dialogVisible.value = true;
}

function openDetail(app: PortalAppRecord) {
  selectedAppId.value = app.id;
}

function openEdit(app: PortalAppRecord) {
  const template = templateForApp(app);
  if (!template) {
    props.notifyError?.("应用模板不可用，请刷新后重试");
    return;
  }
  editingApp.value = app;
  selectedTemplate.value = template;
  dialogVisible.value = true;
}

async function handleSubmit(payload: PortalAppWriteInput) {
  saving.value = true;
  try {
    if (editingApp.value) {
      const updated = await props.api.updateApp(editingApp.value.id, payload);
      selectedAppId.value = updated.id;
      props.notifySuccess?.(`${appNoun.value}已更新`);
    } else {
      const created = await props.api.createApp(payload);
      mode.value = "manage";
      selectedAppId.value = created.id;
      props.notifySuccess?.(`${appNoun.value}已创建`);
    }
    dialogVisible.value = false;
    editingApp.value = null;
    await fetchAll();
  } catch (error) {
    props.notifyError?.((error as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function togglePublication(app: PortalAppRecord, published: boolean) {
  if (!props.api.setPublication) return;
  publishing.value = { ...publishing.value, [app.id]: true };
  try {
    await props.api.setPublication(app.id, published);
    props.notifySuccess?.(published ? `「${app.name}」已发布` : `「${app.name}」已撤回`);
    await fetchAll();
  } catch (error) {
    props.notifyError?.((error as Error).message || "发布状态更新失败");
  } finally {
    publishing.value = { ...publishing.value, [app.id]: false };
  }
}

async function handleDelete(app: PortalAppRecord) {
  const confirmed = props.confirmDelete ? await props.confirmDelete(app.name) : window.confirm(`确定删除${appNoun.value}「${app.name}」吗？`);
  if (!confirmed) return;
  try {
    await props.api.deleteApp(app.id);
    selectedAppId.value = "";
    props.notifySuccess?.(`${appNoun.value}已删除`);
    await fetchAll();
  } catch (error) {
    props.notifyError?.((error as Error).message || "删除失败");
  }
}

onMounted(fetchAll);
</script>

<template>
  <div class="page-container app-page">
    <!-- 列表/创建:一体面板 + DsTabs 模式切换(同密钥管理页) -->
    <PortalPagePanel
      v-if="!selectedApp"
      fill
      :icon="Blocks"
      :breadcrumbs="breadcrumbs"
      description="从固定业务模板创建应用，统一绑定能力、提示词策略、模型与运行设置。"
    >
      <template #actions>
        <el-button :icon="refreshIcon" :loading="loading" @click="fetchAll">刷新</el-button>
        <el-button v-if="mode === 'manage'" type="primary" :icon="createIcon" @click="handleModeChange('create')">
          新建应用
        </el-button>
      </template>

      <div class="app-workspace">
        <div class="app-workspace__tabs">
          <DsTabs :tabs="modeTabs" :model-value="mode" @update:model-value="handleModeChange" />
        </div>

        <!-- 创建态:模板网格(标题层级由面板页头承担,此处只留一行说明) -->
        <section v-if="mode === 'create'" class="template-section" aria-label="选择应用模板">
          <div class="template-section__hint">
            <span>选择模板后配置业务参数，模板已预设执行逻辑，只需补充模型、提示词与运行设置。</span>
            <span class="template-count">{{ templates.length }} 种模板</span>
          </div>
          <div class="template-scroll">
            <div class="template-grid">
              <button
                v-for="template in templates"
                :key="template.id"
                type="button"
                class="template-card"
                @click="chooseTemplate(template)"
              >
                <span class="template-card__content">
                  <span class="template-icon"><component :is="templateIcons[template.id]" :size="21" /></span>
                  <span class="template-copy">
                    <strong>{{ template.name }}</strong>
                    <small>{{ template.description }}</small>
                  </span>
                </span>
                <span class="template-card__footer">
                  <span class="capability-list">{{ template.allowedCapabilities.map(agentTypeLabel).join(" / ") }}</span>
                  <ChevronRight class="template-arrow" :size="18" />
                </span>
              </button>
            </div>
          </div>
        </section>

        <!-- 管理态:筛选 + 表格 + 分页(同分组管理页) -->
        <template v-else>
          <div class="app-workspace__filters">
            <DsFilterBar>
              <DsFilterField label="关键词">
                <el-input v-model="keyword" clearable placeholder="搜索名称、模型或提示词" class="search-input" />
              </DsFilterField>
              <template #actions>
                <span class="muted-text">{{ visibleApps.length }} 个应用</span>
              </template>
            </DsFilterBar>
          </div>

          <div class="app-table-wrap">
            <DsTable :frame="false" :columns="columns" :rows="pagedApps" row-key="id" :loading="loading">
              <template #empty>
                <DsEmpty title="暂无应用" :description="`还没有${appNoun}，先从模板创建一个吧`">
                  <template #action>
                    <el-button type="primary" :icon="createIcon" @click="handleModeChange('create')">去选择模板</el-button>
                  </template>
                </DsEmpty>
              </template>
              <template #cell-name="{ row }">
                <div class="name-cell">
                  <strong>{{ row.name }}</strong>
                  <span>{{ row.description || "未填写描述" }}</span>
                </div>
              </template>
              <template #cell-template="{ row }">{{ promptStrategyLabel(row) }}</template>
              <template #cell-capability="{ row }">{{ agentTypeLabel(row.capability) }}</template>
              <template #cell-model="{ row }"><span class="mono-text">{{ row.model_code }}</span></template>
              <template #cell-prompts="{ row }">{{ appPromptNames(row) }}</template>
              <template #cell-publication="{ row }">
                <el-switch
                  :model-value="appPublished(row)"
                  :loading="publishing[row.id]"
                  @click.stop
                  @change="(value: boolean) => togglePublication(row, value)"
                />
              </template>
              <template #cell-status="{ row }">
                <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">
                  {{ row.status === "active" ? "启用" : "停用" }}
                </el-tag>
              </template>
              <template #cell-actions="{ row }">
                <el-button link type="primary" @click.stop="openDetail(row)">详情</el-button>
                <el-button link type="warning" @click.stop="openEdit(row)">编辑</el-button>
                <el-button link type="danger" @click.stop="handleDelete(row)">删除</el-button>
              </template>
            </DsTable>
          </div>
        </template>
      </div>

      <template v-if="mode === 'manage'" #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="visibleApps.length"
          @update:page="page = $event"
          @update:page-size="pageSize = $event"
        />
      </template>
    </PortalPagePanel>

    <!-- 详情:同款一体面板,面包屑追加应用名,操作提到页头 -->
    <PortalPagePanel
      v-else
      fill
      :icon="Blocks"
      :breadcrumbs="[...breadcrumbs, { label: selectedApp.name }]"
      :description="selectedApp.description || '暂无描述'"
    >
      <template #actions>
        <el-tag :type="selectedApp.status === 'active' ? 'success' : 'info'" size="small">
          {{ selectedApp.status === "active" ? "启用" : "停用" }}
        </el-tag>
        <el-button :icon="ArrowLeft" @click="selectedAppId = ''">返回列表</el-button>
        <el-button :icon="editIcon" @click="openEdit(selectedApp)">编辑</el-button>
        <el-button type="danger" plain :icon="deleteIcon" @click="handleDelete(selectedApp)">删除</el-button>
      </template>

      <div class="detail-body">
        <PortalContentCard title="应用配置">
          <dl class="meta-grid">
            <dt>模板</dt><dd>{{ promptStrategyLabel(selectedApp) }}</dd>
            <dt>执行能力</dt><dd>{{ agentTypeLabel(selectedApp.capability) }}</dd>
            <dt>模型</dt><dd>{{ selectedApp.model_code }}</dd>
            <dt>提示词</dt><dd>{{ selectedApp.prompt_bindings.map((binding) => binding.prompt_name).join("、") || "无" }}</dd>
            <template v-for="row in runtimeSettingRows" :key="row[0]"><dt>{{ row[0] }}</dt><dd>{{ row[1] }}</dd></template>
          </dl>
        </PortalContentCard>

        <PortalContentCard title="调用示例">
          <pre class="code-block">{{ selectedCurlExample }}</pre>
        </PortalContentCard>

        <PortalAppPreviewPanel
          :app="selectedApp"
          :variables="selectedVariables"
          :can-preview="Boolean(api.previewApp)"
          :preview-app="api.previewApp"
          :notify-error="notifyError"
        />
      </div>
    </PortalPagePanel>

    <PortalAgentEditorDialog
      v-if="selectedTemplate"
      :visible="dialogVisible"
      :loading="saving"
      :scope="scope"
      :template="selectedTemplate"
      :app="editingApp"
      :prompts="prompts"
      :models="models"
      :loading-models="loadingModels"
      :model-selector-enabled="Boolean(api.listModels)"
      @close="dialogVisible = false"
      @submit="handleSubmit"
    />
  </div>
</template>

<style scoped>
.app-page {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

/* 面板 body 无内边距:Tab 条与筛选条各自带 24px 容器,表格通栏 */
.app-workspace {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

/* 分隔线画在容器上才通栏(DsTabs 自身不带线) */
.app-workspace__tabs {
  padding: 16px 24px 14px;
  border-bottom: 1px solid var(--ds-line);
}

.app-workspace__filters {
  padding: 14px 24px;
  border-bottom: 1px solid var(--ds-line);
}

.app-table-wrap {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.app-table-wrap :deep(.ds-table) {
  display: flex;
  min-height: 100%;
  flex-direction: column;
}

.app-table-wrap :deep(.ds-table__empty) {
  flex: 1;
  justify-content: center;
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

.mono-text {
  color: var(--ds-ink-soft);
  font-family: var(--ds-font-mono);
  font-size: 12.5px;
}

.template-section {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

.template-section__hint {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 24px;
  border-bottom: 1px solid var(--ds-line);
  color: var(--ds-muted);
  font-size: 12.5px;
  line-height: 1.6;
}

.template-count {
  flex: 0 0 auto;
  color: var(--ds-faint);
  font-size: 12px;
  white-space: nowrap;
}

.template-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 18px 24px 22px;
}

.template-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(288px, 1fr));
  gap: 12px;
}

.template-card {
  display: flex;
  min-height: 142px;
  flex-direction: column;
  justify-content: space-between;
  gap: 18px;
  padding: 17px 18px 15px;
  /* 静息态就要看得出是卡片:实边框 + 轻阴影,不再用几乎融进背景的淡边 */
  border: 1px solid var(--ds-line-strong);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
  color: inherit;
  cursor: pointer;
  text-align: left;
  transition: border-color 160ms ease, box-shadow 160ms ease, transform 160ms ease, background-color 160ms ease;
}

.template-card:hover {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
  box-shadow: var(--ds-shadow-pop);
  transform: translateY(-1px);
}

.template-card:focus-visible {
  outline: 2px solid var(--ds-accent);
  outline-offset: 2px;
}

.template-card__content,
.template-card__footer {
  display: flex;
  min-width: 0;
}

.template-card__content {
  align-items: flex-start;
  gap: 12px;
}

.template-card__footer {
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.template-icon {
  display: grid;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 8px;
  background: color-mix(in srgb, var(--ds-accent) 10%, var(--ds-panel));
  color: var(--ds-accent);
  transition: background-color 160ms ease;
}

/* hover 时整卡换成 accent-soft 底,徽章改白底才不糊在一起 */
.template-card:hover .template-icon {
  background: var(--ds-panel);
}

.template-copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.template-copy strong {
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 680;
  line-height: 1.5;
}

.template-copy small,
.capability-list,
.muted-text {
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.capability-list {
  color: var(--ds-accent-hover);
  font-weight: 600;
}

.template-arrow {
  flex: 0 0 auto;
  color: var(--ds-faint);
  transition: color 160ms ease, transform 160ms ease;
}

.template-card:hover .template-arrow {
  color: var(--ds-accent);
  transform: translateX(2px);
}

/* 详情视图:面板 body 无内边距,内容用 24px 容器承载 */
.detail-body {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: 18px;
  overflow: auto;
  padding: 18px 24px 22px;
}

.meta-grid {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 13px 20px;
  margin: 0;
}

.meta-grid dt {
  color: var(--ds-muted);
}

.meta-grid dd {
  min-width: 0;
  margin: 0;
  color: var(--ds-ink);
  overflow-wrap: anywhere;
}

.code-block {
  margin: 0;
  padding: 16px;
  overflow: auto;
  border-radius: var(--ds-radius-control);
  background: #17202a;
  color: #f4f7fa;
  font: 13px/1.65 "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

@media (max-width: 768px) {
  .app-workspace__tabs,
  .app-workspace__filters,
  .template-section__hint,
  .template-scroll,
  .detail-body {
    padding-inline: 16px;
  }
}

@media (max-width: 680px) {
  .template-section__hint {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }

  .template-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .meta-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 5px;
  }

  .meta-grid dd {
    margin-bottom: 10px;
  }
}
</style>
