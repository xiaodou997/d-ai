<!-- Admin proxy-egress system-module workspace. -->
<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Network, Pencil, Plus, RefreshCw, Save, Trash2 } from "lucide-vue-next";

import {
  systemModulesApi,
  type ProxyNode,
  type SystemModuleStatus
} from "@/api/systemModules";
import { PortalPagePanel } from "@/platform";
import {
  DsButton,
  DsDrawer,
  DsInput,
  DsSelect,
  DsSwitch,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";

type NodeForm = {
  name: string;
  proxyType: "http" | "socks5";
  endpoint: string;
  username: string;
  password: string;
  weight: string;
  status: "active" | "disabled";
};

const emptyNodeForm = (): NodeForm => ({
  name: "",
  proxyType: "http",
  endpoint: "",
  username: "",
  password: "",
  weight: "1",
  status: "active"
});

const loading = ref(false);
const savingModule = ref(false);
const savingNode = ref(false);
const modules = ref<SystemModuleStatus[]>([]);
const nodes = ref<ProxyNode[]>([]);
const drawerOpen = ref(false);
const editingID = ref("");
const nodeForm = ref<NodeForm>(emptyNodeForm());

const proxyModule = computed(() => modules.value.find((item) => item.name === "proxy_egress"));
const enabledNodes = computed(() => nodes.value.filter((node) => node.status === "active").length);
const schedulableNodes = computed(() => nodes.value.filter((node) => node.status === "active" && node.healthStatus !== "unhealthy").length);

const columns: DsTableColumn[] = [
  { key: "name", title: "节点名称", width: 180 },
  { key: "endpoint", title: "出口地址", mono: true },
  { key: "weight", title: "权重", width: 80, align: "right" },
  { key: "status", title: "状态", width: 90 },
  { key: "healthStatus", title: "健康", width: 180, wrap: true },
  { key: "lastCheckedAt", title: "最近检查", width: 170 },
  { key: "actions", title: "操作", width: 90, align: "right" }
];

function moduleLabel(item?: SystemModuleStatus) {
  if (!item?.enabled) return "未启用";
  return item.active ? "已接管出口" : "等待可用节点";
}

function moduleTone(item?: SystemModuleStatus): "positive" | "warning" | "neutral" {
  if (item?.active) return "positive";
  return item?.enabled ? "warning" : "neutral";
}

function healthLabel(status: string) {
  if (status === "healthy") return "健康";
  if (status === "unhealthy") return "异常";
  return "待检查";
}

function healthTone(status: string): "positive" | "danger" | "neutral" {
  if (status === "healthy") return "positive";
  if (status === "unhealthy") return "danger";
  return "neutral";
}

function formatTime(value?: string | null) {
  return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
}

async function load() {
  loading.value = true;
  try {
    const [moduleItems, nodeItems] = await Promise.all([
      systemModulesApi.list(),
      systemModulesApi.listProxyNodes()
    ]);
    modules.value = moduleItems;
    nodes.value = nodeItems;
  } catch (error: any) {
    ElMessage.error(error?.message || "代理出口加载失败");
  } finally {
    loading.value = false;
  }
}

async function toggleModule(enabled: boolean) {
  if (!proxyModule.value) return;
  savingModule.value = true;
  try {
    const updated = await systemModulesApi.setEnabled(proxyModule.value.name, enabled);
    const index = modules.value.findIndex((item) => item.name === updated.name);
    if (index >= 0) modules.value[index] = updated;
    ElMessage.success(enabled ? "代理出口已启用" : "代理出口已关闭，AI 请求将恢复直连");
  } catch (error: any) {
    ElMessage.error(error?.message || "代理出口状态更新失败");
  } finally {
    savingModule.value = false;
  }
}

function openCreate() {
  editingID.value = "";
  nodeForm.value = emptyNodeForm();
  drawerOpen.value = true;
}

function openEdit(node: ProxyNode) {
  editingID.value = node.id;
  nodeForm.value = {
    name: node.name,
    proxyType: node.proxyType,
    endpoint: node.endpoint,
    username: node.username || "",
    password: "",
    weight: String(node.weight),
    status: node.status
  };
  drawerOpen.value = true;
}

function closeDrawer() {
  if (savingNode.value) return;
  drawerOpen.value = false;
}

function validateNode() {
  if (!nodeForm.value.name.trim() || !nodeForm.value.endpoint.trim()) return "请填写节点名称和出口地址";
  const weight = Number(nodeForm.value.weight);
  if (!Number.isInteger(weight) || weight < 1 || weight > 1000) return "权重必须是 1-1000 之间的整数";
  return "";
}

async function saveNode() {
  const message = validateNode();
  if (message) {
    ElMessage.warning(message);
    return;
  }
  savingNode.value = true;
  const payload = {
    name: nodeForm.value.name.trim(),
    proxyType: nodeForm.value.proxyType,
    endpoint: nodeForm.value.endpoint.trim(),
    username: nodeForm.value.username.trim(),
    password: nodeForm.value.password || undefined,
    weight: Number(nodeForm.value.weight),
    status: nodeForm.value.status
  };
  try {
    if (editingID.value) await systemModulesApi.updateProxyNode(editingID.value, payload);
    else await systemModulesApi.createProxyNode(payload);
    drawerOpen.value = false;
    ElMessage.success(editingID.value ? "代理节点已更新" : "代理节点已添加");
    await load();
  } catch (error: any) {
    ElMessage.error(error?.message || "代理节点保存失败");
  } finally {
    savingNode.value = false;
  }
}

async function removeNode(node: ProxyNode) {
  try {
    await ElMessageBox.confirm(`确认删除代理节点“${node.name}”吗？`, "删除代理节点", { type: "warning" });
    await systemModulesApi.deleteProxyNode(node.id);
    ElMessage.success("代理节点已删除");
    await load();
  } catch (error: any) {
    if (error !== "cancel" && error !== "close") ElMessage.error(error?.message || "代理节点删除失败");
  }
}

onMounted(load);
</script>

<template>
  <div class="proxy-page">
    <PortalPagePanel
      :icon="Network"
      :breadcrumbs="[{ label: '平台运营' }, { label: '代理出口' }]"
      description="管理 AI 上游请求使用的 HTTP 与 SOCKS5 出口节点"
    >
      <template #actions>
        <DsButton :disabled="loading" @click="load">
          <template #icon><RefreshCw :size="14" /></template>
          刷新
        </DsButton>
        <DsButton variant="primary" @click="openCreate">
          <template #icon><Plus :size="14" /></template>
          添加节点
        </DsButton>
      </template>

      <div class="proxy-workspace">
        <section class="status-band">
          <div class="status-copy">
            <div class="section-title-row">
              <h2>出口状态</h2>
              <DsTag :tone="moduleTone(proxyModule)">{{ moduleLabel(proxyModule) }}</DsTag>
            </div>
            <dl class="status-metrics">
              <div><dt>节点总数</dt><dd>{{ nodes.length }}</dd></div>
              <div><dt>已启用</dt><dd>{{ enabledNodes }}</dd></div>
              <div><dt>可调度</dt><dd>{{ schedulableNodes }}</dd></div>
            </dl>
            <p v-if="proxyModule?.configError" class="module-error">{{ proxyModule.configError }}</p>
          </div>
          <div class="status-control">
            <span>{{ proxyModule?.enabled ? "停止代理出口" : "启用代理出口" }}</span>
            <DsSwitch
              :model-value="proxyModule?.enabled || false"
              :disabled="loading || savingModule || !proxyModule"
              @update:model-value="toggleModule"
            />
          </div>
        </section>

        <section class="workspace-section">
          <div class="section-head">
            <div>
              <h2>代理节点</h2>
              <p>启用节点按权重参与调度，健康异常的节点不会被选择。</p>
            </div>
          </div>

          <DsTable
            :columns="columns"
            :rows="nodes"
            row-key="id"
            :loading="loading"
            empty-title="暂无代理节点"
            empty-description="添加节点后即可配置代理出口。"
          >
            <template #cell-name="{ row }">
              <div class="node-name"><strong>{{ row.name }}</strong><span>{{ row.proxyType.toUpperCase() }}</span></div>
            </template>
            <template #cell-endpoint="{ row }"><span class="endpoint">{{ row.endpoint }}</span></template>
            <template #cell-status="{ row }"><DsTag :tone="row.status === 'active' ? 'positive' : 'neutral'">{{ row.status === 'active' ? '启用' : '停用' }}</DsTag></template>
            <template #cell-healthStatus="{ row }">
              <div class="health-cell">
                <DsTag :tone="healthTone(row.healthStatus)">{{ healthLabel(row.healthStatus) }}</DsTag>
                <span v-if="row.lastError" :title="row.lastError">{{ row.lastError }}</span>
              </div>
            </template>
            <template #cell-lastCheckedAt="{ row }">{{ formatTime(row.lastCheckedAt) }}</template>
            <template #cell-actions="{ row }">
              <div class="row-actions">
                <DsButton size="sm" variant="ghost" title="编辑代理节点" aria-label="编辑代理节点" @click="openEdit(row)">
                  <template #icon><Pencil :size="14" /></template>
                </DsButton>
                <DsButton size="sm" variant="ghost" title="删除代理节点" aria-label="删除代理节点" @click="removeNode(row)">
                  <template #icon><Trash2 :size="14" /></template>
                </DsButton>
              </div>
            </template>
          </DsTable>
        </section>
      </div>
    </PortalPagePanel>

    <DsDrawer
      :open="drawerOpen"
      :title="editingID ? '编辑代理节点' : '添加代理节点'"
      :subtitle="editingID ? '密码留空将保留当前密码' : '凭证会加密保存'"
      width="min(560px, 100vw)"
      @close="closeDrawer"
    >
      <form id="proxy-node-form" class="node-form" @submit.prevent="saveNode">
        <label class="form-field"><span>节点名称</span><DsInput v-model="nodeForm.name" placeholder="例如：新加坡出口" /></label>
        <div class="form-grid">
          <label class="form-field"><span>代理类型</span><DsSelect v-model="nodeForm.proxyType" :options="[{ label: 'HTTP', value: 'http' }, { label: 'SOCKS5', value: 'socks5' }]" /></label>
          <label class="form-field"><span>节点状态</span><DsSelect v-model="nodeForm.status" :options="[{ label: '启用', value: 'active' }, { label: '停用', value: 'disabled' }]" /></label>
        </div>
        <label class="form-field"><span>出口地址</span><DsInput v-model="nodeForm.endpoint" placeholder="例如 http://proxy.example.com:8080" /></label>
        <div class="form-grid">
          <label class="form-field"><span>用户名</span><DsInput v-model="nodeForm.username" placeholder="可选" /></label>
          <label class="form-field"><span>密码</span><DsInput v-model="nodeForm.password" type="password" :placeholder="editingID ? '留空不修改' : '可选'" /></label>
        </div>
        <label class="form-field"><span>调度权重</span><DsInput v-model="nodeForm.weight" type="number" placeholder="1-1000" /></label>
      </form>
      <template #footer>
        <DsButton :disabled="savingNode" @click="closeDrawer">取消</DsButton>
        <DsButton variant="primary" type="submit" form="proxy-node-form" :disabled="savingNode">
          <template #icon><Save :size="14" /></template>
          {{ savingNode ? "保存中" : "保存节点" }}
        </DsButton>
      </template>
    </DsDrawer>
  </div>
</template>

<style scoped>
.proxy-page { display: flex; min-height: 100%; flex-direction: column; }
.proxy-workspace { display: flex; flex-direction: column; gap: 24px; padding: 24px; }
.status-band { display: flex; align-items: center; justify-content: space-between; gap: 28px; padding: 18px 20px; border-left: 3px solid var(--ds-accent); background: var(--ds-accent-soft); }
.status-copy { min-width: 0; }
.section-title-row, .status-control, .section-head, .row-actions { display: flex; align-items: center; gap: 10px; }
.section-title-row h2, .section-head h2, .section-head p, .module-error { margin: 0; }
.section-title-row h2, .section-head h2 { color: var(--ds-ink); font-size: 15px; font-weight: 700; }
.status-metrics { display: flex; gap: 24px; margin: 14px 0 0; }
.status-metrics div { display: flex; align-items: baseline; gap: 7px; }
.status-metrics dt { color: var(--ds-muted); font-size: 12px; }
.status-metrics dd { margin: 0; color: var(--ds-ink); font-size: 18px; font-weight: 700; font-variant-numeric: tabular-nums; }
.module-error { margin-top: 10px; color: var(--ds-warning); font-size: 12px; }
.status-control { flex: 0 0 auto; padding-left: 24px; border-left: 1px solid var(--ds-line-strong); color: var(--ds-ink-soft); font-size: 13px; font-weight: 600; }
.workspace-section { min-width: 0; }
.section-head { justify-content: space-between; margin-bottom: 14px; }
.section-head p { margin-top: 4px; color: var(--ds-muted); font-size: 12px; line-height: 1.5; }
.node-name { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.node-name strong { overflow: hidden; color: var(--ds-ink); text-overflow: ellipsis; }
.node-name span { color: var(--ds-muted); font-size: 11px; }
.endpoint { color: var(--ds-ink-soft); }
.health-cell { display: flex; min-width: 0; flex-direction: column; align-items: flex-start; gap: 4px; }
.health-cell span { max-width: 160px; overflow: hidden; color: var(--ds-danger); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.row-actions { justify-content: flex-end; gap: 2px; }
.node-form { display: flex; flex-direction: column; gap: 18px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.form-field { display: flex; min-width: 0; flex-direction: column; gap: 7px; color: var(--ds-ink-soft); font-size: 12px; font-weight: 600; }
@media (max-width: 720px) {
  .proxy-workspace { padding: 16px; }
  .status-band { align-items: flex-start; flex-direction: column; }
  .status-control { width: 100%; justify-content: space-between; padding: 14px 0 0; border-top: 1px solid var(--ds-line-strong); border-left: 0; }
  .status-metrics { width: 100%; justify-content: space-between; gap: 12px; }
  .form-grid { grid-template-columns: 1fr; }
}
</style>
