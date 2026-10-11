import { computed, onUnmounted, shallowRef } from "vue";
import type {
  PortalImageJobRecord, PortalImageModelRecord, PortalImageTaskPayload,
  PortalImageTaskCreateResponse, PortalImageWorkspaceProps
} from "./types";
import { PORTAL_IMAGE_ACTIVE_STATUSES, usePortalImageTaskQueue } from "./usePortalImageTaskQueue";
import { copyPromptText } from "./imageJobPresentation";

type ImageJobsOptions = Pick<
  PortalImageWorkspaceProps,
  "api" | "usageMessage" | "pollIntervalMs" | "confirmDelete" | "notifySuccess" | "notifyError"
>;

function acceptedJob(created: PortalImageTaskCreateResponse, payload: PortalImageTaskPayload): PortalImageJobRecord {
  const input = payload instanceof FormData ? {
    operation: "edit" as const,
    model: String(payload.get("model") || ""),
    group_id: String(payload.get("group_id") || ""),
    prompt: String(payload.get("prompt") || ""),
    n: Number(payload.get("n")) || 1,
    size: String(payload.get("size") || "auto")
  } : payload;
  return {
    id: created.task_id,
    status: created.status,
    operation: input.operation || "generation",
    model_code: input.model,
    group_id: input.group_id,
    prompt: input.prompt,
    requested_output_count: input.n || 1,
    size: input.size,
    storage_policy: "temporary",
    raw_image_retained: false,
    caller_charge_usd: 0,
    image_count: 0,
    inline_count: 0,
    url_count: 0,
    created_at: Date.now()
  };
}

export function usePortalImageJobs(props: ImageJobsOptions, onQueued: () => void) {
  const models = shallowRef<PortalImageModelRecord[]>([]);
  const jobs = shallowRef<PortalImageJobRecord[]>([]);
  const loading = shallowRef(false);
  const submitting = shallowRef(false);
  const cancellingJobId = shallowRef("");
  const deletingJobId = shallowRef("");
  const modelsError = shallowRef("");
  let refreshInFlight: Promise<void> | undefined;
  let modelsController: AbortController | undefined;
  let disposed = false;

  const queue = usePortalImageTaskQueue({ api: props.api, jobs, pollIntervalMs: () => props.pollIntervalMs });
  const errors = computed(() => [modelsError.value, queue.listError.value, queue.syncError.value].filter(Boolean));

  async function fetchModels() {
    const request = new AbortController();
    modelsController = request;
    const timeout = window.setTimeout(() => request.abort(), 15_000);
    try {
      const result = await props.api.listModels(request.signal);
      if (disposed) return;
      models.value = result;
      modelsError.value = "";
    } catch {
      if (!disposed) modelsError.value = "模型列表加载失败，请刷新重试。";
    } finally {
      window.clearTimeout(timeout);
    }
  }

  function refreshAll() {
    if (disposed) return Promise.resolve();
    if (refreshInFlight) return refreshInFlight;
    loading.value = true;
    refreshInFlight = Promise.all([fetchModels(), queue.refreshJobs()]).then(() => undefined).finally(() => {
      loading.value = false;
      refreshInFlight = undefined;
    });
    return refreshInFlight;
  }

  async function submitTask(payload: PortalImageTaskPayload, onCreated: () => void = () => undefined) {
    if (submitting.value || disposed) return;
    submitting.value = true;
    let created: PortalImageTaskCreateResponse;
    try {
      created = await props.api.createTask(payload);
    } catch (error) {
      if (!disposed) props.notifyError?.((error as Error).message || "创建生图任务失败");
      return;
    } finally {
      submitting.value = false;
    }
    if (disposed) return;
    queue.addAcceptedJob(acceptedJob(created, payload));
    onCreated();
    props.notifySuccess?.(`任务已加入队列，${props.usageMessage}`);
    onQueued();
    // Only reads are retried after acceptance. A GET failure must never become a creation failure.
    await queue.refreshActiveJobs();
  }

  async function retryJob(job: PortalImageJobRecord) {
    if (PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status) || job.status === "completed" || job.operation === "edit") return;
    await submitTask({
      operation: "generation",
      model: job.model_code,
      group_id: job.group_id,
      prompt: job.retry_prompt || job.prompt,
      n: job.requested_output_count || job.image_count || 1,
      size: job.size || undefined
    });
  }

  async function copyPrompt(job: PortalImageJobRecord) {
    const text = copyPromptText(job);
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      props.notifySuccess?.("提示词已复制");
    } catch {
      props.notifyError?.("复制失败，请手动选择文本复制");
    }
  }

  async function cancelJob(job: PortalImageJobRecord) {
    if (!PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status) || cancellingJobId.value || disposed) return;
    cancellingJobId.value = job.id;
    try {
      queue.mergeJob(await props.api.cancelTask(job.id));
      if (!disposed) props.notifySuccess?.("任务已取消");
    } catch (error) {
      if (!disposed) props.notifyError?.((error as Error).message || "取消任务失败");
    } finally {
      cancellingJobId.value = "";
    }
  }

  async function deleteJob(job: PortalImageJobRecord) {
    if (PORTAL_IMAGE_ACTIVE_STATUSES.has(job.status) || deletingJobId.value || disposed) return;
    // Lock before the asynchronous confirmation, so rapid clicks cannot open multiple deletions.
    deletingJobId.value = job.id;
    try {
      const confirmed = props.confirmDelete
        ? await props.confirmDelete(job)
        : window.confirm("确定删除该生图任务记录及已保留的图片吗？此操作不可恢复。");
      if (!confirmed || disposed) return;
      await props.api.deleteTask(job.id);
      if (disposed) return;
      queue.removeJob(job.id);
      props.notifySuccess?.("任务记录已删除");
    } catch (error) {
      if (!disposed) props.notifyError?.((error as Error).message || "删除任务记录失败");
    } finally {
      deletingJobId.value = "";
    }
  }

  onUnmounted(() => {
    disposed = true;
    modelsController?.abort();
  });

  return {
    models, jobs, loading, submitting, cancellingJobId, deletingJobId, errors,
    pendingDetails: queue.pendingDetails,
    refreshAll, submitTask, retryJob, copyPrompt, cancelJob, deleteJob
  };
}
