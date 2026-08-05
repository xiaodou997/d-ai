<!--
  应用运行密钥工作区(用户端/租户端共用,embedded 时嵌在密钥管理面板内)。
  重构:迁移至 DsUI——非嵌入态用 PortalPagePanel 一体面板(图标徽章+面包屑+描述同行),
       列表统一 DsTable(嵌入面板 :frame="false")+ DsEmpty + DsPagination(接口一次返回全量,
       前端切片分页),状态 el-tag → DsTag;行点击进详情改为「管理」操作列(DsTable 无行事件),
       详情区收进同卡 body 的 24px 容器,代码块颜色全部走 var(--ds-*) token。
       业务逻辑/请求参数/复制与轮换交互不变,编辑弹窗仍为 element-plus(过渡期)。
-->
<script setup lang="ts">
import { ArrowLeft, KeyRound } from "lucide-vue-next";
import { computed, inject, onMounted, shallowRef, watch } from "vue";
import { DsEmpty, DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import PortalEmbeddedPanelFrame from "../../page/PortalEmbeddedPanelFrame.vue";
import PortalPagePanel from "../../page/PortalPagePanel.vue";
import { appCurlExample } from "../apps/contract";
import PortalAppKeyEditorDialog from "./PortalAppKeyEditorDialog.vue";
import type { PortalAppKeyApi, PortalAppKeyRecord, PortalAppKeyWriteInput, PortalVisibleAgentRecord } from "./types";

const props = withDefaults(
  defineProps<{
    api: PortalAppKeyApi;
    publicBaseUrl: string;
    chatExampleInput?: string;
    exampleBrandName?: string;
    namePlaceholder?: string;
    embedded?: boolean;
    refreshIcon?: unknown;
    createIcon?: unknown;
    copyIcon?: unknown;
    editIcon?: unknown;
    deleteIcon?: unknown;
    notifySuccess?: (message: string) => void;
    notifyError?: (message: string) => void;
    notifyWarning?: (message: string) => void;
    confirmDelete?: (name: string) => Promise<boolean>;
    confirmRotate?: () => Promise<boolean>;
  }>(),
  {
    chatExampleInput: "请帮我总结这段内容",
    exampleBrandName: "Demo Tenant",
    namePlaceholder: "例如：个人知识库入口"
  }
);

// 嵌入 PortalKeyManagementWorkspace 时向上注册操作方法,由后者在页头统一渲染按钮
const tabActions = inject<Record<string, { loading: boolean; refresh: () => void; create: () => void }> | null>(
  "keyManagementTabActions",
  null
);

const description = "为应用创建独立调用凭证，统一通过 /v1/run 执行。";
const breadcrumbs = [{ label: "应用" }, { label: "应用运行密钥" }];

const columns: DsTableColumn[] = [
  { key: "name", title: "名称" },
  { key: "target", title: "绑定应用" },
  { key: "lastFour", title: "后四位", width: 110, mono: true },
  { key: "status", title: "状态", width: 90 },
  { key: "expires", title: "过期时间", width: 180 },
  { key: "actions", title: "操作", width: 90 }
];

const appKeys = shallowRef<PortalAppKeyRecord[]>([]);
const agents = shallowRef<PortalVisibleAgentRecord[]>([]);
const loading = shallowRef(false);
const saving = shallowRef(false);
const dialogVisible = shallowRef(false);
const selectedAppKeyId = shallowRef("");
const revealingKeyId = shallowRef("");
const generatedKey = shallowRef("");
const showKeyDialog = shallowRef(false);

// 接口一次返回全量,前端切片分页
const page = shallowRef(1);
const pageSize = shallowRef(20);
const pagedAppKeys = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return appKeys.value.slice(start, start + pageSize.value);
});

function handlePageChange(value: number) {
  page.value = value;
}

function handlePageSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
}

const selectedAppKey = computed(() => appKeys.value.find((item) => item.id === selectedAppKeyId.value) || null);
const agentNameMap = computed(() => Object.fromEntries(agents.value.map((agent) => [agent.id, agent.name])));
const agentMap = computed(() => Object.fromEntries(agents.value.map((agent) => [agent.id, agent])));

function requestExample(appKeyValue = "rk_xxx") {
  const appKey = selectedAppKey.value;
  const app = appKey?.agent_id ? agentMap.value[appKey.agent_id] : null;
  if (app) {
    return appCurlExample(
      app.capability,
      app.variables || [],
			app.prompt_strategy,
      props.publicBaseUrl,
      appKeyValue,
      false,
      app.prompt_names
    );
  }
  return `curl ${props.publicBaseUrl}/v1/run \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${appKeyValue}" \\
  -d '${JSON.stringify({ input: props.chatExampleInput }, null, 2)}'`;
}

function formatTimestamp(value?: number) {
  return value ? new Date(value).toLocaleString("zh-CN") : "-";
}

function describeAppKeyTarget(appKey: PortalAppKeyRecord) {
  return appKey.agent_name || (appKey.agent_id ? agentNameMap.value[appKey.agent_id] : "") || appKey.agent_id || "-";
}

async function copyPlaintextKey(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    props.notifySuccess?.("已复制到剪贴板");
  } catch {
    props.notifyWarning?.("复制失败，请手动复制");
    generatedKey.value = text;
    showKeyDialog.value = true;
  }
}

async function fetchAll() {
  loading.value = true;
  try {
    const [appKeyRes, agentRes] = await Promise.all([props.api.listAppKeys(), props.api.listVisibleAgents()]);
    appKeys.value = appKeyRes.items || [];
    agents.value = agentRes.items || [];
    page.value = 1;
    if (selectedAppKeyId.value && !appKeys.value.some((item) => item.id === selectedAppKeyId.value)) selectedAppKeyId.value = "";
  } catch (error) {
    props.notifyError?.((error as Error).message || "应用运行密钥加载失败");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  selectedAppKeyId.value = "";
  dialogVisible.value = true;
}

function openEdit(appKey: PortalAppKeyRecord) {
  selectedAppKeyId.value = appKey.id;
  dialogVisible.value = true;
}

async function handleSubmit(payload: PortalAppKeyWriteInput) {
  saving.value = true;
  try {
    if (selectedAppKey.value) {
      await props.api.updateAppKey(selectedAppKey.value.id, payload);
      props.notifySuccess?.("应用运行密钥已更新");
    } else {
      const created = await props.api.createAppKey(payload);
      generatedKey.value = created.plaintext_key;
      selectedAppKeyId.value = created.key.id;
      showKeyDialog.value = true;
      props.notifySuccess?.("应用运行密钥已创建");
    }
    dialogVisible.value = false;
    await fetchAll();
  } catch (error) {
    props.notifyError?.((error as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function handleDelete(appKey: PortalAppKeyRecord) {
  const confirmed = props.confirmDelete ? await props.confirmDelete(appKey.name) : window.confirm(`确定删除应用运行密钥「${appKey.name}」吗？`);
  if (!confirmed) return;
  try {
    await props.api.deleteAppKey(appKey.id);
    selectedAppKeyId.value = "";
    props.notifySuccess?.("应用运行密钥已删除");
    await fetchAll();
  } catch (error) {
    props.notifyError?.((error as Error).message || "删除失败");
  }
}

async function revealKey(appKey: PortalAppKeyRecord) {
  revealingKeyId.value = appKey.id;
  try {
    const response = await props.api.revealAppKey(appKey.id);
    await copyPlaintextKey(response.plaintext_key);
  } catch (error) {
    props.notifyError?.((error as Error).message || "查看明文失败");
  } finally {
    revealingKeyId.value = "";
  }
}

async function rotateKey(appKey: PortalAppKeyRecord) {
  const confirmed = props.confirmRotate ? await props.confirmRotate() : window.confirm("轮换后旧密钥立即失效，确定继续吗？");
  if (!confirmed) return;
  try {
    const response = await props.api.rotateAppKey(appKey.id);
    generatedKey.value = response.plaintext_key;
    showKeyDialog.value = true;
    props.notifySuccess?.("应用运行密钥已轮换");
    await fetchAll();
  } catch (error) {
    props.notifyError?.((error as Error).message || "轮换失败");
  }
}

onMounted(fetchAll);

// 向 PortalKeyManagementWorkspace 注册操作方法 + loading 同步
if (tabActions) {
  tabActions.application.refresh = fetchAll;
  tabActions.application.create = openCreate;
  watch(loading, (value) => { tabActions.application.loading = value; }, { immediate: true });
}
</script>

<template>
  <div class="appkey-page">
    <component
      :is="embedded ? PortalEmbeddedPanelFrame : PortalPagePanel"
      v-bind="embedded ? { showActions: !tabActions } : { icon: KeyRound, breadcrumbs, description, fill: true }"
    >
      <template #actions>
        <el-button :icon="refreshIcon" :loading="loading" @click="fetchAll">刷新</el-button>
        <el-button v-if="!selectedAppKey" type="primary" :icon="createIcon" @click="openCreate">新建应用运行密钥</el-button>
      </template>

      <DsTable
        v-if="!selectedAppKey"
        :frame="false"
        :columns="columns"
        :rows="pagedAppKeys"
        row-key="id"
        :loading="loading"
      >
        <template #empty>
          <DsEmpty title="暂无应用运行密钥" description="还没有应用运行密钥，先新建一个吧">
            <template #action>
              <el-button type="primary" :icon="createIcon" @click="openCreate">新建应用运行密钥</el-button>
            </template>
          </DsEmpty>
        </template>
        <template #cell-target="{ row }">{{ describeAppKeyTarget(row) }}</template>
        <template #cell-lastFour="{ row }"><code class="key-prefix">••••{{ row.last_four }}</code></template>
        <template #cell-status="{ row }">
          <DsTag :tone="row.status === 'active' ? 'positive' : 'neutral'">
            {{ row.status === "active" ? "启用" : "停用" }}
          </DsTag>
        </template>
        <template #cell-expires="{ row }">
          <span class="time-text">{{ formatTimestamp(row.expires_at) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <el-button link type="primary" @click="selectedAppKeyId = row.id">管理</el-button>
        </template>
      </DsTable>

      <div v-else class="appkey-detail">
        <div class="detail-toolbar">
          <el-button :icon="ArrowLeft" text @click="selectedAppKeyId = ''">返回列表</el-button>
          <div class="detail-actions">
            <el-button :icon="copyIcon" :loading="revealingKeyId === selectedAppKey.id" @click="revealKey(selectedAppKey)">查看明文</el-button>
            <el-button @click="rotateKey(selectedAppKey)">轮换</el-button>
            <el-button :icon="editIcon" @click="openEdit(selectedAppKey)">编辑</el-button>
            <el-button type="danger" plain :icon="deleteIcon" @click="handleDelete(selectedAppKey)">删除</el-button>
          </div>
        </div>

        <section class="detail-summary">
          <div><h2>{{ selectedAppKey.name }}</h2><p>{{ describeAppKeyTarget(selectedAppKey) }}</p></div>
          <DsTag :tone="selectedAppKey.status === 'active' ? 'positive' : 'neutral'">
            {{ selectedAppKey.status === "active" ? "启用" : "停用" }}
          </DsTag>
        </section>

        <section class="detail-section">
          <h3 class="detail-section__title">密钥信息</h3>
          <dl class="meta-grid">
            <dt>后四位</dt><dd><code class="key-prefix">••••{{ selectedAppKey.last_four }}</code></dd>
            <dt>绑定应用</dt><dd>{{ describeAppKeyTarget(selectedAppKey) }}</dd>
            <dt>过期时间</dt><dd>{{ formatTimestamp(selectedAppKey.expires_at) }}</dd>
          </dl>
        </section>

        <section class="detail-section">
          <div class="detail-section__head">
            <h3 class="detail-section__title">调用示例</h3>
            <el-button :icon="copyIcon" @click="copyPlaintextKey(requestExample())">复制示例</el-button>
          </div>
          <pre class="code-block">{{ requestExample() }}</pre>
        </section>
      </div>

      <template v-if="!selectedAppKey" #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="appKeys.length"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </component>

    <PortalAppKeyEditorDialog
      :visible="dialogVisible"
      :loading="saving"
      :app-key="selectedAppKey"
      :agents="agents"
      :name-placeholder="namePlaceholder"
      @close="dialogVisible = false"
      @submit="handleSubmit"
    />

    <el-dialog v-model="showKeyDialog" title="应用运行密钥明文" width="min(560px, 92vw)" append-to-body>
      <div class="key-display">
        <code class="full-key">{{ generatedKey }}</code>
        <el-button :icon="copyIcon" type="primary" @click="copyPlaintextKey(generatedKey)">复制密钥</el-button>
      </div>
      <template #footer><el-button type="primary" @click="showKeyDialog = false">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<style scoped>
.appkey-page {
  display: flex;
  min-width: 0;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}

/* 嵌入态布局(操作条带/body/分页脚)见 PortalEmbeddedPanelFrame */

/* 详情视图:面板 body 无内边距,内容用 24px 容器承载 */
.appkey-detail {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
}

.detail-toolbar,
.detail-summary,
.detail-actions,
.detail-section__head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.detail-toolbar,
.detail-summary,
.detail-section__head {
  justify-content: space-between;
}

.detail-summary {
  padding-bottom: 18px;
  border-bottom: 1px solid var(--ds-line);
}

.detail-summary h2 {
  margin: 0 0 6px;
  color: var(--ds-ink);
  font-size: 20px;
  letter-spacing: 0;
}

.detail-summary p {
  margin: 0;
  color: var(--ds-muted);
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-section__title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 650;
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
  margin: 0;
  color: var(--ds-ink);
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

.time-text {
  font-size: 12px;
  color: var(--ds-muted);
}

.code-block {
  margin: 0;
  padding: 16px;
  overflow: auto;
  border-radius: var(--ds-radius-control);
  background: var(--ds-ink);
  color: var(--ds-line);
  font: 13px/1.65 var(--ds-font-mono);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.key-display {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.full-key {
  padding: 16px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-ink);
  color: var(--ds-line);
  overflow-wrap: anywhere;
  font-family: var(--ds-font-mono);
  font-size: 13px;
}

@media (max-width: 720px) {
  .detail-toolbar,
  .detail-summary {
    align-items: flex-start;
    flex-direction: column;
  }

  .detail-actions {
    flex-wrap: wrap;
  }

  .meta-grid {
    grid-template-columns: minmax(0, 1fr);
    gap: 5px;
  }
}
</style>
