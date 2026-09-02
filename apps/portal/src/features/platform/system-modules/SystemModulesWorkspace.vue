<!-- Admin system-modules workspace. -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { Blocks, FileText, Play, RefreshCw, Save, Settings, Trash2 } from "lucide-vue-next";
import { ElMessage, ElMessageBox } from "element-plus";
import { PortalPagePanel, PortalContentCard } from "@/platform";
import { DsButton, DsDrawer, DsEmpty, DsInput, DsSwitch, DsTable, DsTag } from "@/shared/ui";
import {
  systemModulesApi,
  type DataCleanupPolicy,
  type DataCleanupPreview,
  type DataCleanupRun,
  type SystemModuleStatus
} from "@/api/systemModules";

const loading = ref(false);
const router = useRouter();
const modules = ref<SystemModuleStatus[]>([]);
const savingModule = ref<string | null>(null);
const notificationForm = ref({ eventKey: "system.test", recipientUserId: "", title: "D-AI 通知测试", body: "这是一条来自统一通知服务的测试通知。" });
const cleanupPolicy = ref<DataCleanupPolicy>({ enabled: true, requestBodyDays: 30, requestPayloadDays: 180, notificationDays: 90, moderationDays: 90, riskEventDays: 365, adminAuditDays: 365, auditBlobDays: 180, usageRollupDays: 730, batchSize: 1000 });
const cleanupPreview = ref<DataCleanupPreview | null>(null);
const cleanupRuns = ref<DataCleanupRun[]>([]);
const cleanupBusy = ref(false);
const selectedCleanupTargets = ref<string[]>([]);
const cleanupRunsDrawerOpen = ref(false);
const visibleCleanupRuns = computed(() => cleanupRuns.value.slice(0, 8));
const activeBodyPurgeRun = computed(() => cleanupRuns.value.find((run) => run.targets?.includes("request_body_purge") && (run.status === "queued" || run.status === "running")));
const cleanupRunColumns = [
  { key: "status", title: "状态", width: 90 },
  { key: "trigger", title: "触发方式", width: 90 },
  { key: "createdAt", title: "开始时间" },
  { key: "summary", title: "处理数量", width: 110, align: "right" as const }
];
let cleanupPollTimer: ReturnType<typeof setTimeout> | undefined;

function tone(item: SystemModuleStatus) { return item.active ? "positive" : item.configValidated ? "neutral" : "warning"; }
function statusLabel(item: SystemModuleStatus) { return item.active ? "运行中" : item.enabled ? "待配置" : "已关闭"; }
function cleanupRunTone(status: string) { return status === "completed" ? "positive" : status === "failed" ? "danger" : status === "running" ? "info" : "neutral"; }
function cleanupRunLabel(status: string) { return status === "completed" ? "已完成" : status === "failed" ? "失败" : status === "running" ? "执行中" : "排队中"; }
function cleanupRunTotal(summary: Record<string, number>) { return Object.values(summary).reduce((sum, value) => sum + value, 0).toLocaleString(); }
function cleanupRunProgress(run: DataCleanupRun) {
  const progress = run.progress;
  if (!progress || progress.totalRows <= 0) return progress?.phase === "preparing" ? "正在统计待处理数量…" : "—";
  const processed = Math.min(Math.max(progress.processedRows, 0), progress.totalRows);
  const percent = Math.floor((processed / progress.totalRows) * 100);
  return `${processed.toLocaleString()} / ${progress.totalRows.toLocaleString()}（${percent}%）`;
}
function cleanupRunPhase(run: DataCleanupRun) {
  switch (run.progress?.phase) {
    case "preparing": return "正在统计";
    case "clearing": return "正在清理";
    case "compacting": return "正在回收 TOAST 空间";
    case "cleared": return "正文已清理，空间回收完成";
    case "completed": return "已完成";
    case "failed": return "执行失败";
    default: return run.status === "queued" ? "等待启动" : "执行中";
  }
}
function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const unitIndex = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** unitIndex;
  return `${value.toLocaleString("zh-CN", { maximumFractionDigits: unitIndex === 0 ? 0 : 1 })} ${units[unitIndex]}`;
}
function updateCleanupNumber(key: keyof DataCleanupPolicy, value: string) {
  cleanupPolicy.value[key] = Number(value) as never;
}
function cleanupDate(value?: string) { return value ? new Date(value).toLocaleString() : "-"; }
function hasDetailPage(item: SystemModuleStatus) { return item.adminRoute !== "/admin/system-modules"; }

async function load() {
  loading.value = true;
  try {
    modules.value = await systemModulesApi.list();
    await loadCleanup();
  } catch (error: any) { ElMessage.error(`加载系统模块失败：${error?.message || "未知错误"}`); }
  finally { loading.value = false; }
}

async function loadCleanup() {
  const [policy, preview, runs] = await Promise.all([systemModulesApi.getCleanupPolicy(), systemModulesApi.previewCleanup(), systemModulesApi.listCleanupRuns()]);
  cleanupPolicy.value = policy;
  cleanupPreview.value = preview;
  cleanupRuns.value = runs;
  if (!selectedCleanupTargets.value.length) selectedCleanupTargets.value = preview.items.map((item) => item.target);
  const activeRun = runs.find((run) => run.status === "queued" || run.status === "running");
  if (activeRun) pollCleanupRun(activeRun.id);
}

async function saveCleanupPolicy() {
  cleanupBusy.value = true;
  try {
    cleanupPolicy.value = await systemModulesApi.updateCleanupPolicy(cleanupPolicy.value);
    cleanupPreview.value = await systemModulesApi.previewCleanup();
    ElMessage.success("数据清理策略已保存");
  } catch (error: any) { ElMessage.error(error?.message || "清理策略保存失败"); }
  finally { cleanupBusy.value = false; }
}

async function startCleanup() {
  if (!selectedCleanupTargets.value.length) { ElMessage.warning("至少选择一个清理范围"); return; }
  try {
    const result = await ElMessageBox.prompt("这是不可逆操作。请先确认预览数量，再输入 CLEANUP_DATA 开始异步清理。", "执行数据清理", {
      type: "warning",
      inputPlaceholder: "输入 CLEANUP_DATA",
      inputPattern: /^CLEANUP_DATA$/,
      inputErrorMessage: "确认短语不正确"
    });
    cleanupBusy.value = true;
    const run = await systemModulesApi.startCleanup({ targets: selectedCleanupTargets.value, confirmation: result.value });
    cleanupRuns.value = [run, ...cleanupRuns.value.filter((item) => item.id !== run.id)];
    ElMessage.success("清理任务已开始");
    pollCleanupRun(run.id);
  } catch (error: any) {
    if (error !== "cancel" && error !== "close") ElMessage.error(error?.message || "启动清理失败");
  } finally { cleanupBusy.value = false; }
}

async function purgeRequestBodies() {
  cleanupBusy.value = true;
  try {
    const preview = await systemModulesApi.previewCleanup();
    cleanupPreview.value = preview;
    const bodyPreview = preview.requestBodyPurge;
    const result = await ElMessageBox.prompt(
      `这是不可逆操作。系统会清空所有请求/响应正文、请求参数、媒体引用和底层错误详情，保留请求元数据、用量统计及账务数据；完成后会压缩审计表，期间可能短暂阻塞新的审计写入。

本次预览：${bodyPreview.eligibleRows.toLocaleString()} 条正文数据，占用约 ${formatBytes(bodyPreview.occupiedBytes)}。
预览时间：${cleanupDate(preview.generatedAt)}

请输入 CLEANUP_DATA 继续。`,
      "清空请求体",
      {
        type: "warning",
        inputPlaceholder: "输入 CLEANUP_DATA",
        inputPattern: /^CLEANUP_DATA$/,
        inputErrorMessage: "确认短语不正确"
      }
    );
    const run = await systemModulesApi.purgeRequestBodies({ confirmation: result.value });
    cleanupRuns.value = [run, ...cleanupRuns.value.filter((item) => item.id !== run.id)];
    ElMessage.success("请求体清理任务已开始");
    pollCleanupRun(run.id);
  } catch (error) {
    if (error !== "cancel" && error !== "close") {
      ElMessage.error(error instanceof Error ? error.message : "清空请求体失败");
    }
  } finally {
    cleanupBusy.value = false;
  }
}

function pollCleanupRun(runID: string) {
  if (cleanupPollTimer) clearTimeout(cleanupPollTimer);
  cleanupPollTimer = setTimeout(async () => {
    try {
      cleanupRuns.value = await systemModulesApi.listCleanupRuns();
      const run = cleanupRuns.value.find((item) => item.id === runID);
      if (run && (run.status === "queued" || run.status === "running")) pollCleanupRun(runID);
      else await loadCleanup();
    } catch { /* the next manual refresh can recover a transient polling error */ }
  }, 1500);
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

async function sendNotification() {
  if (!notificationForm.value.recipientUserId || !notificationForm.value.title) { ElMessage.warning("请填写收件人用户 ID 和通知标题"); return; }
  try {
    await systemModulesApi.sendNotification({ eventKey: notificationForm.value.eventKey, channel: "in_app", recipientUserId: notificationForm.value.recipientUserId, title: notificationForm.value.title, body: notificationForm.value.body });
    ElMessage.success("站内通知已发送");
  } catch (error: any) { ElMessage.error(error?.message || "通知发送失败"); }
}

onMounted(load);
onBeforeUnmount(() => { if (cleanupPollTimer) clearTimeout(cleanupPollTimer); });
</script>

<template>
  <div class="system-modules-page">
    <PortalPagePanel :icon="Blocks" :breadcrumbs="[{ label: '平台运营' }, { label: '系统模块' }]" description="集中查看可插拔系统能力的状态与基础维护配置">
      <template #actions><DsButton :disabled="loading" @click="load"><template #icon><RefreshCw :size="14" /></template>刷新</DsButton></template>
      <div class="module-body">
        <div class="module-grid">
          <article v-for="item in modules" :key="item.name" class="module-card">
            <div class="module-card__head"><div><p class="module-card__title">{{ item.displayName }}</p><p class="module-card__name">{{ item.name }}</p></div><DsSwitch :model-value="item.enabled" :disabled="savingModule === item.name" @update:model-value="toggle(item, $event)" /></div>
            <p class="module-card__description">{{ item.description }}</p>
            <div class="module-card__foot">
              <div class="module-card__status"><DsTag :tone="tone(item)">{{ statusLabel(item) }}</DsTag><span v-if="item.configError" class="module-error">{{ item.configError }}</span></div>
              <DsButton v-if="hasDetailPage(item)" size="sm" @click="router.push(item.adminRoute)"><template #icon><Settings :size="13" /></template>配置</DsButton>
            </div>
          </article>
        </div>

        <PortalContentCard title="数据生命周期" description="按保留策略清理历史内容和运营日志，不触碰计费、用户、配置与结算数据。">
          <template #actions>
            <DsTag :tone="cleanupPolicy.enabled ? 'positive' : 'neutral'">{{ cleanupPolicy.enabled ? '自动清理已开启' : '自动清理已关闭' }}</DsTag>
            <DsButton :disabled="cleanupBusy" @click="loadCleanup"><template #icon><RefreshCw :size="14" /></template>刷新</DsButton>
          </template>
          <div class="cleanup-policy">
            <label class="cleanup-switch"><input v-model="cleanupPolicy.enabled" type="checkbox" /> <span>每日自动清理</span></label>
            <label>请求正文保留天数 <DsInput :model-value="String(cleanupPolicy.requestBodyDays)" type="number" @update:model-value="updateCleanupNumber('requestBodyDays', $event)" /></label>
            <label>请求记录保留天数 <DsInput :model-value="String(cleanupPolicy.requestPayloadDays)" type="number" @update:model-value="updateCleanupNumber('requestPayloadDays', $event)" /></label>
            <label>通知记录保留天数 <DsInput :model-value="String(cleanupPolicy.notificationDays)" type="number" @update:model-value="updateCleanupNumber('notificationDays', $event)" /></label>
            <label>审核记录保留天数 <DsInput :model-value="String(cleanupPolicy.moderationDays)" type="number" @update:model-value="updateCleanupNumber('moderationDays', $event)" /></label>
            <label>风险事件保留天数 <DsInput :model-value="String(cleanupPolicy.riskEventDays)" type="number" @update:model-value="updateCleanupNumber('riskEventDays', $event)" /></label>
            <label>审计日志保留天数 <DsInput :model-value="String(cleanupPolicy.adminAuditDays)" type="number" @update:model-value="updateCleanupNumber('adminAuditDays', $event)" /></label>
            <label>媒体 Blob 保留天数 <DsInput :model-value="String(cleanupPolicy.auditBlobDays)" type="number" @update:model-value="updateCleanupNumber('auditBlobDays', $event)" /></label>
            <label>用量汇总保留天数 <DsInput :model-value="String(cleanupPolicy.usageRollupDays)" type="number" @update:model-value="updateCleanupNumber('usageRollupDays', $event)" /></label>
            <label>单批处理数量 <DsInput :model-value="String(cleanupPolicy.batchSize)" type="number" @update:model-value="updateCleanupNumber('batchSize', $event)" /></label>
            <div class="cleanup-policy__actions"><DsButton variant="primary" :disabled="cleanupBusy" @click="saveCleanupPolicy"><template #icon><Save :size="14" /></template>保存策略</DsButton></div>
          </div>
          <div v-if="cleanupPreview" class="cleanup-workspace">
            <div class="cleanup-preview">
              <div class="cleanup-section-head"><div><strong>清理预览</strong><span>预计处理 {{ cleanupPreview.items.reduce((sum, item) => sum + item.eligibleRows, 0).toLocaleString() }} 条</span></div><DsButton variant="danger" :disabled="cleanupBusy" @click="startCleanup"><template #icon><Play :size="14" /></template>执行选中范围</DsButton></div>
              <div class="cleanup-targets">
                <label v-for="item in cleanupPreview.items" :key="item.target" class="cleanup-target"><input v-model="selectedCleanupTargets" type="checkbox" :value="item.target" /><span><strong>{{ item.label }}</strong><small>超过 {{ item.retentionDays }} 天：{{ item.eligibleRows.toLocaleString() }} 条</small></span></label>
              </div>
            </div>
            <div class="cleanup-runs">
              <div class="cleanup-section-head">
                <div>
                  <strong>最近运行</strong>
                  <span>展示最近 {{ Math.min(cleanupRuns.length, 8) }} 次，共 {{ cleanupRuns.length }} 次</span>
                </div>
                <DsButton v-if="cleanupRuns.length > 8" size="sm" variant="ghost" @click="cleanupRunsDrawerOpen = true">查看全部</DsButton>
              </div>
              <div v-if="cleanupRuns.length" class="cleanup-runs__viewport">
                <DsTable :columns="cleanupRunColumns" :rows="visibleCleanupRuns" row-key="id" :frame="false">
                  <template #cell-status="{ row }"><DsTag :tone="cleanupRunTone(row.status)">{{ cleanupRunLabel(row.status) }}</DsTag></template>
                  <template #cell-trigger="{ row }">{{ row.trigger === 'automatic' ? '自动' : '手动' }}</template>
                  <template #cell-createdAt="{ row }">{{ cleanupDate(row.createdAt) }}</template>
                  <template #cell-summary="{ row }"><div class="cleanup-run-progress"><strong>{{ cleanupRunTotal(row.summary) }}</strong><small v-if="row.progress?.totalRows">{{ cleanupRunProgress(row) }} · 每批 {{ row.progress.batchSize.toLocaleString() }} 条</small><small v-else-if="row.status === 'running'">{{ cleanupRunPhase(row) }}</small></div></template>
                </DsTable>
              </div>
              <DsEmpty v-else title="还没有清理记录" description="保存策略后，系统会在每日后台任务中执行清理。" />
            </div>
          </div>
          <div class="cleanup-danger-zone">
            <div class="cleanup-danger-zone__copy">
              <div class="cleanup-danger-zone__title"><FileText :size="15" /><strong>立即清空请求体</strong></div>
              <p>清除所有请求/响应正文等大字段，保留请求记录、用量统计和对账数据；任务完成后自动回收 TOAST 空间。</p>
              <p v-if="cleanupPreview" class="cleanup-danger-zone__preview">当前待清理 {{ cleanupPreview.requestBodyPurge.eligibleRows.toLocaleString() }} 条正文数据，占用约 <strong>{{ formatBytes(cleanupPreview.requestBodyPurge.occupiedBytes) }}</strong>；预览于 {{ cleanupDate(cleanupPreview.generatedAt) }}</p>
              <p v-if="activeBodyPurgeRun" class="cleanup-danger-zone__progress">执行进度：{{ cleanupRunProgress(activeBodyPurgeRun) }}；每批 {{ activeBodyPurgeRun.progress?.batchSize?.toLocaleString() || cleanupPolicy.batchSize.toLocaleString() }} 条；{{ cleanupRunPhase(activeBodyPurgeRun) }}</p>
            </div>
            <DsButton variant="danger" :disabled="cleanupBusy || !!activeBodyPurgeRun" @click="purgeRequestBodies">
              <template #icon><Trash2 :size="14" /></template>
              {{ cleanupBusy || activeBodyPurgeRun ? "清理中…" : "清空请求体" }}
            </DsButton>
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

    <DsDrawer
      :open="cleanupRunsDrawerOpen"
      title="清理运行记录"
      :subtitle="`保留最近 ${cleanupRuns.length} 次运行记录`"
      width="min(760px, 100vw)"
      @close="cleanupRunsDrawerOpen = false"
    >
      <div class="cleanup-runs__drawer-table">
        <DsTable v-if="cleanupRuns.length" :columns="cleanupRunColumns" :rows="cleanupRuns" row-key="id" :frame="false">
          <template #cell-status="{ row }"><DsTag :tone="cleanupRunTone(row.status)">{{ cleanupRunLabel(row.status) }}</DsTag></template>
          <template #cell-trigger="{ row }">{{ row.trigger === 'automatic' ? '自动' : '手动' }}</template>
          <template #cell-createdAt="{ row }">{{ cleanupDate(row.createdAt) }}</template>
          <template #cell-summary="{ row }"><div class="cleanup-run-progress"><strong>{{ cleanupRunTotal(row.summary) }}</strong><small v-if="row.progress?.totalRows">{{ cleanupRunProgress(row) }} · 每批 {{ row.progress.batchSize.toLocaleString() }} 条</small><small v-else-if="row.status === 'running'">{{ cleanupRunPhase(row) }}</small></div></template>
        </DsTable>
        <DsEmpty v-else title="还没有清理记录" description="保存策略后，系统会在每日后台任务中执行清理。" />
      </div>
    </DsDrawer>
  </div>
</template>

<style scoped>
.system-modules-page { display: flex; flex-direction: column; min-height: 100%; }
.module-body { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.module-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.module-card { display: flex; flex-direction: column; gap: 16px; padding: 18px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel); box-shadow: var(--ds-shadow-sm); }
.module-card__head, .module-card__foot, .module-card__status { display: flex; align-items: center; gap: 10px; }
.module-card__head { justify-content: space-between; align-items: flex-start; }
.module-card__foot { justify-content: space-between; }
.module-card__title, .module-card__name, .module-card__description, .module-error { margin: 0; }
.module-card__title { color: var(--ds-ink); font-weight: 700; }
.module-card__name { margin-top: 4px; color: var(--ds-muted); font: 12px var(--ds-font-mono); }
.module-card__description { min-height: 42px; color: var(--ds-ink-soft); font-size: 13px; line-height: 1.65; }
.module-card__foot { align-items: flex-start; flex-wrap: wrap; }
.module-error { color: var(--ds-warning); font-size: 12px; line-height: 1.5; }
.cleanup-policy { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; align-items: end; }
.cleanup-policy > label { display: flex; flex-direction: column; gap: 6px; color: var(--ds-ink-soft); font-size: 12px; }
.cleanup-switch { flex-direction: row !important; align-items: center; min-height: 36px; }
.cleanup-switch input, .cleanup-target input { accent-color: var(--ds-accent); }
.cleanup-policy__actions { display: flex; justify-content: flex-end; }
.cleanup-workspace { display: grid; grid-template-columns: minmax(320px, 0.85fr) minmax(460px, 1.15fr); gap: 22px; margin-top: 22px; padding-top: 20px; border-top: 1px solid var(--ds-line); }
.cleanup-section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; color: var(--ds-ink); font-size: 13px; }
.cleanup-section-head div { display: flex; align-items: baseline; gap: 10px; }
.cleanup-section-head span { color: var(--ds-muted); font-size: 12px; font-weight: 400; }
.cleanup-targets { display: grid; gap: 8px; }
.cleanup-target { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); cursor: pointer; }
.cleanup-target span { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.cleanup-target strong { color: var(--ds-ink-soft); font-size: 13px; font-weight: 600; }
.cleanup-target small { color: var(--ds-muted); font-size: 11px; }
.cleanup-runs { min-width: 0; align-self: start; }
.cleanup-runs__viewport { max-height: 430px; overflow-y: auto; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); }
.cleanup-runs__viewport :deep(.ds-table) { border: 0; border-radius: var(--ds-radius-none); }
.cleanup-runs__viewport :deep(.ds-table__th), .cleanup-runs__drawer-table :deep(.ds-table__th) { padding: 9px 12px; }
.cleanup-runs__viewport :deep(.ds-table__td), .cleanup-runs__drawer-table :deep(.ds-table__td) { padding: 10px 12px; }
.cleanup-runs__drawer-table { min-width: 0; }
.cleanup-runs__drawer-table :deep(.ds-table) { border: 0; border-radius: var(--ds-radius-none); }
.cleanup-run-progress { display: flex; flex-direction: column; gap: 2px; align-items: flex-end; }
.cleanup-run-progress strong { color: var(--ds-ink); font-size: 12px; font-weight: 600; }
.cleanup-run-progress small { color: var(--ds-muted); font-size: 11px; white-space: nowrap; }
.cleanup-danger-zone { display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-top: 22px; padding: 16px; border: 1px solid var(--ds-line-strong); border-radius: var(--ds-radius-control); background: var(--ds-danger-soft); }
.cleanup-danger-zone__copy { min-width: 0; }
.cleanup-danger-zone__title { display: flex; align-items: center; gap: 7px; color: var(--ds-danger); font-size: 13px; }
.cleanup-danger-zone__copy p { margin: 6px 0 0; color: var(--ds-ink-soft); font-size: 12px; line-height: 1.6; }
.cleanup-danger-zone__preview strong { color: var(--ds-danger); font-weight: 700; }
.cleanup-danger-zone__progress { color: var(--ds-accent) !important; font-weight: 600; }
.notification-form { display: grid; grid-template-columns: 180px 220px minmax(180px, 1fr) minmax(220px, 1.5fr) auto; align-items: center; gap: 10px; }
@media (max-width: 1100px) { .module-grid { grid-template-columns: 1fr; } .cleanup-policy { grid-template-columns: repeat(2, minmax(0, 1fr)); } .cleanup-workspace { grid-template-columns: 1fr; } }
@media (max-width: 900px) { .notification-form { grid-template-columns: 1fr; } }
@media (max-width: 700px) { .cleanup-danger-zone { align-items: stretch; flex-direction: column; } .cleanup-danger-zone .ds-btn { width: 100%; } }
</style>
