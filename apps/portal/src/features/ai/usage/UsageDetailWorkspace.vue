<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { ScrollText } from "lucide-vue-next";
import { useAuthStore } from "@/stores/auth";
import { PortalPagePanel } from "@/platform";
import { DsEmpty, DsSkeleton } from "@/shared/ui";
import { recordsApi, type RequestRecord } from "./recordsApi";
import { copyRecordText, object, recordPath, recordRole, recordTitle } from "./recordPresentation";
import RecordDetailContent from "./components/RecordDetailContent.vue";
const route = useRoute(), router = useRouter(), auth = useAuthStore();
const role = computed(() => recordRole(auth.userInfo?.userType));
const record = ref<RequestRecord | null>(null), busy = ref(false), error = ref("");
const refundReason = ref(""), refundBusy = ref(false), debugBusy = ref(false), debugError = ref("");
const debug = ref<unknown>(null);
const debugParts = computed(() => Object.entries(object(debug.value)).map(([key, value]) => ({ key, title: ({ request_params: "请求参数", request_messages: "请求消息", request_headers: "请求头", response_message: "响应内容", response_headers: "响应头", media_refs: "媒体引用", request: "请求", response: "响应" } as Record<string, string>)[key] || key, text: JSON.stringify(value, null, 2) })));
const returnTo = computed(() => {
  const value = String(route.query.returnTo || "");
  // Return only to this role's local area. Route guards still enforce capabilities.
  return value.startsWith(`/${role.value}/`) && !value.includes("\\") ? value : recordPath(role.value);
});
let controller: AbortController | undefined, debugController: AbortController | undefined;
async function load(id: string) {
  controller?.abort(); debugController?.abort(); debugBusy.value = false;
  const active = new AbortController(); controller = active;
  record.value = null; busy.value = true; error.value = ""; debug.value = null; debugError.value = ""; refundReason.value = "";
  try { const result = await recordsApi.detail(id, active.signal); if (!active.signal.aborted) record.value = result; }
  catch (e) { if (!active.signal.aborted) error.value = e instanceof Error ? e.message : "读取详情失败"; }
  finally { if (!active.signal.aborted) busy.value = false; }
}
async function readDebug() {
  if (!record.value) return;
  debugController?.abort(); const active = new AbortController(); debugController = active;
  debugBusy.value = true; debugError.value = "";
  try { const payload = await recordsApi.debug(record.value.request_id, active.signal); if (!active.signal.aborted) debug.value = payload; }
  catch (e) { if (!active.signal.aborted) debugError.value = e instanceof Error ? e.message : "读取失败"; }
  finally { if (!active.signal.aborted) debugBusy.value = false; }
}
async function refund() {
  if (!record.value || refundBusy.value || !refundReason.value.trim()) return;
  const id = record.value.request_id;
  refundBusy.value = true;
  try {
    await recordsApi.refund(id, refundReason.value.trim());
    ElMessage.success("费用与相应额度已退回");
    if (String(route.params.requestId) === id) await load(id);
  } catch (e) { ElMessage.error(e instanceof Error ? e.message : "退款失败"); }
  finally { refundBusy.value = false; }
}
watch(() => String(route.params.requestId || ""), id => { void load(id); }, { immediate: true });
onBeforeUnmount(() => { controller?.abort(); debugController?.abort(); });
</script>
<template>
  <div class="record-detail-root">
  <PortalPagePanel fill :icon="ScrollText" :breadcrumbs="[{ label: '智能服务' }, { label: recordTitle(role), to: returnTo }, { label: '请求详情' }]" description="调用主体、实际费用与请求证据">
    <template #actions><el-button @click="router.push(returnTo)">返回列表</el-button><el-button v-if="record" @click="copyRecordText(record.request_id)">复制请求 ID</el-button></template>
    <div class="record-detail-page">
      <DsSkeleton v-if="busy" />
      <DsEmpty v-else-if="error || !record" title="无法读取请求详情" :description="error || '请求不存在或不可访问'"><template #action><el-button @click="load(String(route.params.requestId))">重试</el-button></template></DsEmpty>
      <template v-else>
        <p v-if="!record.execution_available" class="record-detail-note">执行详情已过保留期。以下仍展示可查询的费用、用量及请求时快照。</p>
        <RecordDetailContent :record="record" :role="role" />
        <section v-if="role === 'admin'" class="record-detail-section">
          <h2>限时调试内容</h2><p>仅展示现有采集策略保存的内容；未开启采集或内容过期时不可读取。</p>
          <el-button :loading="debugBusy" @click="readDebug">读取调试内容</el-button>
          <p v-if="debugError" role="alert">{{ debugError }}。若提示不可用，该请求可能未采集或已过保留期；其他读取错误可重试。</p>
          <details v-for="part in debugParts" :key="part.key"><summary>{{ part.title }}</summary><el-button link @click="copyRecordText(part.text)">复制</el-button><pre>{{ part.text }}</pre></details>
          <pre v-if="debug !== null && !debugParts.length">{{ JSON.stringify(debug, null, 2) }}</pre>
        </section>
        <section v-if="role === 'admin' && record.charge.state === 'posted'" class="record-detail-section">
          <h2>全额退款</h2><p>退回本次实际费用与相应计费额度，保留原扣款及退款记录。</p>
          <el-input v-model="refundReason" aria-label="退款原因" placeholder="填写退款原因" maxlength="500" />
          <el-button type="primary" :loading="refundBusy" :disabled="!refundReason.trim()" @click="refund">退回费用与计费额度</el-button>
        </section>
      </template>
    </div>
  </PortalPagePanel>
  </div>
</template>
<style scoped>
.record-detail-root { display: flex; flex: 1; flex-direction: column; min-width: 0; min-height: 0; }
.record-detail-page { display: grid; gap: 18px; padding: 24px; min-width: 0; }
.record-detail-note { color: var(--ds-muted); font-size: 13px; margin: 0; }
.record-detail-section { padding: 20px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-panel); background: var(--ds-panel); min-width: 0; }
.record-detail-section h2 { font-size: 15px; margin: 0 0 12px; }
.record-detail-section p { color: var(--ds-muted); font-size: 12px; }
.record-detail-section > .el-input { max-width: 520px; margin-right: 12px; margin-bottom: 12px; }
.record-detail-section details { margin-top: 12px; }
.record-detail-section summary { cursor: pointer; font-size: 13px; }
.record-detail-section pre { white-space: pre-wrap; overflow-wrap: anywhere; max-height: 420px; overflow: auto; font-size: 12px; }
</style>
