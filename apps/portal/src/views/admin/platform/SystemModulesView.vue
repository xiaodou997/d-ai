<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Blocks, Plus, RefreshCw, Trash2 } from "lucide-vue-next";
import { ElMessage, ElMessageBox } from "element-plus";
import { PortalPagePanel, PortalContentCard } from "@/platform";
import { DsButton, DsEmpty, DsInput, DsSelect, DsSwitch, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { systemModulesApi, type ProxyNode, type SystemModuleStatus } from "@/api/systemModules";

const loading = ref(false);
const modules = ref<SystemModuleStatus[]>([]);
const nodes = ref<ProxyNode[]>([]);
const savingModule = ref<string | null>(null);
const editingID = ref("");
const nodeForm = ref({ name: "", proxyType: "http" as "http" | "socks5", endpoint: "", username: "", password: "", weight: "1", status: "active" as "active" | "disabled" });
const notificationForm = ref({ eventKey: "system.test", recipientUserId: "", title: "D-AI 通知测试", body: "这是一条来自统一通知服务的测试通知。" });

const moduleByName = computed(() => new Map(modules.value.map((item) => [item.name, item])));
const nodeColumns: DsTableColumn[] = [
  { key: "name", title: "节点名称" },
  { key: "proxyType", title: "类型", width: 100 },
  { key: "endpoint", title: "出口地址", mono: true },
  { key: "weight", title: "权重", width: 80, align: "right" },
  { key: "healthStatus", title: "健康", width: 100 },
  { key: "actions", title: "操作", width: 100 }
];

function tone(item: SystemModuleStatus) { return item.active ? "positive" : item.configValidated ? "neutral" : "warning"; }
function statusLabel(item: SystemModuleStatus) { return item.active ? "运行中" : item.enabled ? "待配置" : "已关闭"; }
function nodeTone(status: string) { return status === "healthy" ? "positive" : status === "unhealthy" ? "danger" : "neutral"; }

async function load() {
  loading.value = true;
  try {
    [modules.value, nodes.value] = await Promise.all([systemModulesApi.list(), systemModulesApi.listProxyNodes()]);
  } catch (error: any) { ElMessage.error(`加载系统模块失败：${error?.message || "未知错误"}`); }
  finally { loading.value = false; }
}

async function toggle(item: SystemModuleStatus, enabled: boolean) {
  savingModule.value = item.name;
  try {
    const updated = await systemModulesApi.setEnabled(item.name, enabled);
    const index = modules.value.findIndex((current) => current.name === updated.name);
    if (index >= 0) modules.value[index] = updated;
    ElMessage.success(enabled ? `${item.displayName}已启用` : `${item.displayName}已关闭`);
  } catch (error: any) {
    ElMessage.error(error?.message || "模块状态更新失败");
  } finally { savingModule.value = null; }
}

function resetNodeForm() {
  editingID.value = "";
  nodeForm.value = { name: "", proxyType: "http", endpoint: "", username: "", password: "", weight: "1", status: "active" };
}

function editNode(node: ProxyNode) {
  editingID.value = node.id;
  nodeForm.value = { name: node.name, proxyType: node.proxyType, endpoint: node.endpoint, username: node.username || "", password: "", weight: String(node.weight), status: node.status };
}

async function saveNode() {
  if (!nodeForm.value.name || !nodeForm.value.endpoint) { ElMessage.warning("请填写节点名称和出口地址"); return; }
  const payload = { ...nodeForm.value, weight: Number(nodeForm.value.weight) || 1, ...(nodeForm.value.password ? {} : { password: undefined }) };
  try {
    const saved = editingID.value ? await systemModulesApi.updateProxyNode(editingID.value, payload) : await systemModulesApi.createProxyNode(payload);
    const index = nodes.value.findIndex((node) => node.id === saved.id);
    if (index >= 0) nodes.value[index] = saved; else nodes.value.unshift(saved);
    resetNodeForm();
    ElMessage.success("代理节点已保存");
  } catch (error: any) { ElMessage.error(error?.message || "代理节点保存失败"); }
}

async function removeNode(node: ProxyNode) {
  try {
    await ElMessageBox.confirm(`确认删除代理节点“${node.name}”吗？`, "删除节点", { type: "warning" });
    await systemModulesApi.deleteProxyNode(node.id);
    nodes.value = nodes.value.filter((item) => item.id !== node.id);
    ElMessage.success("代理节点已删除");
  } catch (error: any) { if (error !== "cancel" && error !== "close") ElMessage.error(error?.message || "删除失败"); }
}

async function sendNotification() {
  if (!notificationForm.value.recipientUserId || !notificationForm.value.title) { ElMessage.warning("请填写收件人用户 ID 和通知标题"); return; }
  try {
    await systemModulesApi.sendNotification({ eventKey: notificationForm.value.eventKey, channel: "in_app", recipientUserId: notificationForm.value.recipientUserId, title: notificationForm.value.title, body: notificationForm.value.body });
    ElMessage.success("站内通知已发送");
  } catch (error: any) { ElMessage.error(error?.message || "通知发送失败"); }
}

onMounted(load);
</script>

<template>
  <div class="system-modules-page">
    <PortalPagePanel :icon="Blocks" :breadcrumbs="[{ label: '平台运营' }, { label: '系统模块' }]" description="只保留当前系统需要的三个能力：敏感信息保护、统一通知和代理出口。">
      <template #actions><DsButton :disabled="loading" @click="load"><template #icon><RefreshCw :size="14" /></template>刷新</DsButton></template>
      <div class="module-body">
        <div class="module-grid">
          <article v-for="item in modules" :key="item.name" class="module-card">
            <div class="module-card__head"><div><p class="module-card__title">{{ item.displayName }}</p><p class="module-card__name">{{ item.name }}</p></div><DsSwitch :model-value="item.enabled" :disabled="savingModule === item.name" @update:model-value="toggle(item, $event)" /></div>
            <p class="module-card__description">{{ item.description }}</p>
            <div class="module-card__foot"><DsTag :tone="tone(item)">{{ statusLabel(item) }}</DsTag><span v-if="item.configError" class="module-error">{{ item.configError }}</span></div>
          </article>
        </div>

        <PortalContentCard title="代理出口节点" description="AI 上游请求会按权重从健康节点中选择；关闭模块后自动恢复直连。">
          <template #actions><DsTag :tone="moduleByName.get('proxy_egress')?.active ? 'positive' : 'neutral'">{{ moduleByName.get('proxy_egress')?.active ? '已接管出口' : '未接管出口' }}</DsTag></template>
          <div class="node-layout">
            <DsTable v-if="nodes.length" :columns="nodeColumns" :rows="nodes" row-key="id" :frame="false">
              <template #cell-healthStatus="{ row }"><DsTag :tone="nodeTone(row.healthStatus)">{{ row.healthStatus }}</DsTag></template>
              <template #cell-status="{ row }"><DsTag :tone="row.status === 'active' ? 'positive' : 'neutral'">{{ row.status === 'active' ? '启用' : '停用' }}</DsTag></template>
              <template #cell-actions="{ row }"><div class="row-actions"><DsButton size="sm" variant="ghost" @click="editNode(row)">编辑</DsButton><DsButton size="sm" variant="ghost" @click="removeNode(row)"><template #icon><Trash2 :size="13" /></template></DsButton></div></template>
            </DsTable>
            <DsEmpty v-else title="还没有代理节点" description="先添加一个 HTTP 或 SOCKS5 节点，再启用代理出口模块。" />
            <form class="node-form" @submit.prevent="saveNode">
              <div class="form-title">{{ editingID ? '编辑代理节点' : '添加代理节点' }}</div>
              <DsInput v-model="nodeForm.name" placeholder="节点名称" />
              <DsSelect v-model="nodeForm.proxyType" :options="[{ label: 'HTTP', value: 'http' }, { label: 'SOCKS5', value: 'socks5' }]" />
              <DsInput v-model="nodeForm.endpoint" placeholder="例如 http://proxy.example.com:8080" />
              <div class="form-row"><DsInput v-model="nodeForm.username" placeholder="用户名（可选）" /><DsInput v-model="nodeForm.password" type="password" :placeholder="editingID ? '密码留空表示不修改' : '密码（可选）'" /></div>
              <div class="form-row"><DsInput v-model="nodeForm.weight" type="number" placeholder="权重" /><DsSelect v-model="nodeForm.status" :options="[{ label: '启用', value: 'active' }, { label: '停用', value: 'disabled' }]" /></div>
              <div class="form-actions"><DsButton v-if="editingID" type="button" @click="resetNodeForm">取消</DsButton><DsButton variant="primary" type="submit"><template #icon><Plus :size="14" /></template>{{ editingID ? '保存修改' : '添加节点' }}</DsButton></div>
            </form>
          </div>
        </PortalContentCard>

        <PortalContentCard title="统一通知服务" description="当前第一版提供站内通知投递记录，并保留 Webhook 通道 API；公告管理继续使用原有公告能力。">
          <form class="notification-form" @submit.prevent="sendNotification">
            <DsInput v-model="notificationForm.eventKey" placeholder="事件标识，例如 system.test" />
            <DsInput v-model="notificationForm.recipientUserId" placeholder="收件人用户 ID" />
            <DsInput v-model="notificationForm.title" placeholder="通知标题" />
            <DsInput v-model="notificationForm.body" placeholder="通知内容" />
            <DsButton variant="primary" type="submit">发送站内测试通知</DsButton>
          </form>
        </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.system-modules-page { display: flex; flex-direction: column; min-height: 100%; }
.module-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.module-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.module-card { display: flex; flex-direction: column; gap: 16px; padding: 18px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel); box-shadow: var(--ds-shadow-sm); }
.module-card__head, .module-card__foot, .form-row, .form-actions, .row-actions { display: flex; align-items: center; gap: 10px; }
.module-card__head { justify-content: space-between; align-items: flex-start; }
.module-card__title, .module-card__name, .module-card__description, .module-error, .form-title { margin: 0; }
.module-card__title { color: var(--ds-ink); font-weight: 700; }
.module-card__name { margin-top: 4px; color: var(--ds-muted); font: 12px var(--ds-font-mono); }
.module-card__description { min-height: 42px; color: var(--ds-ink-soft); font-size: 13px; line-height: 1.65; }
.module-card__foot { align-items: flex-start; flex-wrap: wrap; }
.module-error { color: var(--ds-warning); font-size: 12px; line-height: 1.5; }
.node-layout { display: grid; grid-template-columns: minmax(0, 1fr) 330px; gap: 20px; }
.node-form { display: flex; flex-direction: column; gap: 10px; padding: 16px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-card); background: var(--ds-panel-muted); }
.form-title { color: var(--ds-ink); font-size: 13px; font-weight: 700; }
.form-row > * { flex: 1; min-width: 0; }
.form-actions { justify-content: flex-end; margin-top: 4px; }
.notification-form { display: grid; grid-template-columns: 180px 220px minmax(180px, 1fr) minmax(220px, 1.5fr) auto; align-items: center; gap: 10px; }
@media (max-width: 1100px) { .module-grid { grid-template-columns: 1fr; } .node-layout { grid-template-columns: 1fr; } }
@media (max-width: 900px) { .notification-form { grid-template-columns: 1fr; } }
</style>
