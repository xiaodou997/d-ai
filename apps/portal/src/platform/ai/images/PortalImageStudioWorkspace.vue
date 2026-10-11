<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, shallowRef, watch } from "vue";
import PortalImageComposer from "./PortalImageComposer.vue";
import PortalImageTaskCard from "./PortalImageTaskCard.vue";
import { usePortalImageJobs } from "./usePortalImageJobs";
import { PORTAL_IMAGE_ACTIVE_STATUSES } from "./usePortalImageTaskQueue";
import { buildImageJobCard, type PortalImagePreviewAsset } from "./imageJobPresentation";
import type { PortalImageWorkspaceProps, PortalImageJobRecord } from "./types";

const props = withDefaults(defineProps<PortalImageWorkspaceProps>(), {
  usageMessage: "消耗会计入当前用量",
  pollIntervalMs: 20_000
});

const {
  models,
  jobs,
  loading,
  submitting,
  cancellingJobId,
  deletingJobId,
  errors,
  pendingDetails,
  refreshAll,
  submitTask,
  retryJob,
  copyPrompt,
  cancelJob,
  deleteJob
} = usePortalImageJobs(props, () => void scrollToBottom());
let scrollTimer: number | undefined;

const activeFilter = shallowRef<"all" | "active" | "completed" | "failed">("all");

const previewImage = shallowRef("");

const resultStreamRef = shallowRef<HTMLElement | null>(null);

const expandedPromptState = shallowRef<Record<string, boolean>>({});

const activeStatuses = PORTAL_IMAGE_ACTIVE_STATUSES;

const queueCounts = computed(() => ({
  all: jobs.value.length,
  active: jobs.value.filter((job) => activeStatuses.has(job.status)).length,
  completed: jobs.value.filter((job) => job.status === "completed").length,
  failed: jobs.value.filter((job) => job.status === "failed").length
}));

const filterOptions = computed(() => [
  { label: `全部 ${queueCounts.value.all}`, value: "all" },
  { label: `进行中 ${queueCounts.value.active}`, value: "active" },
  { label: `完成 ${queueCounts.value.completed}`, value: "completed" },
  { label: `失败 ${queueCounts.value.failed}`, value: "failed" }
]);

const visibleJobs = computed(() => {
  const filtered = jobs.value.filter((job) => {
    if (activeFilter.value === "active") return activeStatuses.has(job.status);
    if (activeFilter.value === "completed") return job.status === "completed";
    if (activeFilter.value === "failed") return job.status === "failed";
    return true;
  });
  return [...filtered].sort((a, b) => (a.created_at || 0) - (b.created_at || 0));
});

const previewVisible = computed({
  get: () => Boolean(previewImage.value),
  set: (value: boolean) => {
    if (!value) previewImage.value = "";
  }
});

function togglePrompt(job: PortalImageJobRecord) {
  expandedPromptState.value = {
    ...expandedPromptState.value,
    [job.id]: !expandedPromptState.value[job.id]
  };
}

async function scrollToBottom() {
  await nextTick();
  const el = resultStreamRef.value;
  if (!el) return;
  el.scrollTop = el.scrollHeight;
  // 缩略图异步加载后高度会变，稍后再兜一次，确保停在最新任务
  window.clearTimeout(scrollTimer);
  scrollTimer = window.setTimeout(() => {
    const target = resultStreamRef.value;
    if (target) target.scrollTop = target.scrollHeight;
  }, 150);
}

function openPreview(src: string) {
  previewImage.value = src;
}

function downloadImage(src: string) {
  window.open(src, "_blank", "noopener,noreferrer");
}

function downloadAll(images: PortalImagePreviewAsset[]) {
  for (const image of images) {
    if (image.original) downloadImage(image.original);
  }
}

const visibleJobCards = computed(() => visibleJobs.value.map((job) => buildImageJobCard(job, {
  assetBaseUrl: props.assetBaseUrl,
  cancellingJobId: cancellingJobId.value,
  deletingJobId: deletingJobId.value,
  awaitingDetails: pendingDetails.value.has(job.id),
  promptExpanded: Boolean(expandedPromptState.value[job.id])
})));

watch(
  visibleJobCards,
  (items) => {
    const keep = new Set(items.map((item) => item.job.id));
    const next = Object.fromEntries(
      Object.entries(expandedPromptState.value).filter(([jobId, expanded]) => keep.has(jobId) && expanded)
    );
    if (Object.keys(next).length !== Object.keys(expandedPromptState.value).length) {
      expandedPromptState.value = next;
    }
  },
  { immediate: true }
);

onMounted(async () => {
  await refreshAll();
  void scrollToBottom();
});
onUnmounted(() => window.clearTimeout(scrollTimer));
</script>

<template>
  <div class="image-page">
    <header class="studio-bar">
      <div class="studio-bar__title">
        <span class="studio-bar__eyebrow">在线体验 · AI GATEWAY</span>
        <h1>AI 生图</h1>
      </div>
      <div class="studio-bar__actions">
        <el-segmented v-model="activeFilter" size="small" :options="filterOptions" />
        <el-button class="studio-bar__refresh" :icon="refreshIcon" :loading="loading" text @click="refreshAll">刷新</el-button>
      </div>
    </header>

    <div v-if="errors.length" class="studio-errors" role="alert" aria-label="图片工作区更新失败">
      <p v-for="error in errors" :key="error">{{ error }}</p>
      <el-button size="small" :loading="loading" @click="refreshAll">重新加载</el-button>
    </div>

    <div ref="resultStreamRef" class="result-stream">
      <div class="queue-notice">
        <span class="queue-notice__icon">⏳</span>
        <span>生成的图片仅临时保留，过期后图片和任务记录都会被自动删除，请及时下载需要的原图。</span>
      </div>

      <div v-if="visibleJobCards.length" class="result-list">
        <PortalImageTaskCard
          v-for="card in visibleJobCards"
          :key="card.job.id"
          :card="card"
          :formatUSD="formatUSD"
          :cancelling="cancellingJobId === card.job.id"
          :deleting="deletingJobId === card.job.id"
          @preview="openPreview"
          @download="downloadImage"
          @download-all="downloadAll"
          @copy="copyPrompt"
          @retry="retryJob"
          @cancel="cancelJob"
          @delete="deleteJob"
          @toggle-prompt="togglePrompt"
        />
      </div>

      <div v-else-if="!loading && !errors.length" class="stream-empty">
        <span class="stream-empty__icon">🎨</span>
        <h2>想生成什么？</h2>
        <p>在下方输入提示词，选择模型与参数，点击发送即可加入生成队列。</p>
      </div>
    </div>

    <PortalImageComposer
      :models="models"
      :submitting="submitting"
      :submit-icon="submitIcon"
      :notify-error="notifyError"
      :submit-task="submitTask"
      @preview="openPreview"
    />

    <el-dialog v-model="previewVisible" width="min(920px, 92vw)" append-to-body>
      <img v-if="previewImage" class="preview-image" :src="previewImage" alt="" />
    </el-dialog>

  </div>
</template>

<style scoped>
.image-page {
  --studio-content-width: 960px;
  height: calc(100dvh - 56px - 76px);
  min-height: 420px;
  display: flex;
  flex-direction: column;
  color: var(--ds-ink);
  background: var(--ds-paper);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
  overflow: hidden;
}
.studio-bar {
  flex-shrink: 0;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  padding: 15px 22px 13px;
  border-bottom: 1px solid var(--ds-line);
  background: linear-gradient(180deg, var(--ds-panel), color-mix(in srgb, var(--ds-panel) 55%, var(--ds-paper)));
}

.studio-bar__title {
  min-width: 0;
}

.studio-bar__eyebrow {
  display: block;
  margin-bottom: 3px;
  color: var(--ds-faint);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.studio-bar__title h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 750;
  letter-spacing: 0;
  color: var(--ds-ink);
}

.studio-bar__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.studio-bar :deep(.el-segmented) {
  --el-segmented-item-selected-bg-color: var(--ds-accent);
  --el-segmented-item-selected-color: var(--ds-white);
  border-radius: var(--ds-radius-pill);
  padding: 3px;
}

.studio-bar__refresh {
  color: var(--ds-muted);
}
.result-stream {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 20px 20px 12px;
  scroll-behavior: smooth;
}

.queue-notice {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 8px;
  box-sizing: border-box;
  width: min(100%, var(--studio-content-width));
  margin: 0 auto 16px;
  padding: 8px 14px;
  border-radius: var(--ds-radius-pill);
  border: 1px solid color-mix(in srgb, var(--ds-warning) 24%, var(--ds-line));
  background: color-mix(in srgb, var(--ds-warning) 10%, var(--ds-panel));
  color: color-mix(in srgb, var(--ds-warning) 42%, var(--ds-ink));
  font-size: 12px;
  line-height: 1.5;
  box-shadow: var(--ds-shadow-sm);
}

.queue-notice__icon {
  flex-shrink: 0;
  font-size: 13px;
}

.result-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-sizing: border-box;
  width: min(100%, var(--studio-content-width));
  margin: 0 auto;
}

.studio-errors {
  flex-shrink: 0;
  box-sizing: border-box;
  width: min(calc(100% - 40px), var(--studio-content-width));
  margin: 14px auto 0;
  padding: 14px;
  border: 1px solid var(--ds-warning);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-warning-soft);
  color: var(--ds-ink);
  font-size: 13px;
}

.studio-errors p {
  margin: 0 0 8px;
}

.stream-empty {
  height: 100%;
  min-height: 260px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: var(--ds-muted);
  text-align: center;
}

.stream-empty__icon {
  font-size: 44px;
  opacity: 0.9;
}

.stream-empty h2 {
  margin: 10px 0 0;
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 750;
  letter-spacing: 0;
}

.stream-empty p {
  margin: 0;
  font-size: 13px;
}

.action-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
}

.preview-image {
  display: block;
  max-width: 100%;
  max-height: 78vh;
  margin: 0 auto;
  border-radius: var(--ds-radius-panel);
}

@media (max-width: 960px) {
  .image-page {
    height: auto;
    min-height: 0;
  }
}

@media (max-width: 560px) {
  .studio-bar {
    align-items: stretch;
    flex-direction: column;
    padding: 12px;
  }

  .studio-bar__actions {
    min-width: 0;
    justify-content: space-between;
  }

  .studio-bar__actions :deep(.el-segmented) {
    min-width: 0;
  }

  .studio-bar__title h1 {
    white-space: nowrap;
  }
}
</style>
