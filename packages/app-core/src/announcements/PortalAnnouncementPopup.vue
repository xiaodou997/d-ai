<script setup lang="ts">
import { computed, shallowRef } from "vue";

import { renderAnnouncementMarkdown } from "./markdown";
import type { PortalAnnouncement } from "./types";

const props = defineProps<{ item: PortalAnnouncement | null }>();
const emit = defineEmits<{ acknowledge: [] }>();

const acknowledging = shallowRef(false);
const renderedContent = computed(() => renderAnnouncementMarkdown(props.item?.contentMarkdown ?? ""));

async function acknowledge() {
  if (acknowledging.value) return;
  acknowledging.value = true;
  try {
    emit("acknowledge");
  } finally {
    window.setTimeout(() => {
      acknowledging.value = false;
    }, 300);
  }
}
</script>

<template>
  <el-dialog
    :model-value="item !== null"
    width="min(640px, 94vw)"
    append-to-body
    :show-close="false"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
  >
    <template #header>
      <div v-if="item" class="announcement-popup__header">
        <span class="announcement-popup__label">重要公告</span>
        <h2 class="announcement-popup__title">{{ item.title }}</h2>
      </div>
    </template>
    <div v-if="item" class="announcement-popup__content" v-html="renderedContent" />
    <template #footer>
      <el-button type="primary" :loading="acknowledging" @click="acknowledge">我知道了</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.announcement-popup__header {
  border-left: 3px solid var(--ds-accent);
  padding-left: 12px;
}

.announcement-popup__label {
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
}

.announcement-popup__title {
  margin: 5px 0 0;
  color: var(--ds-ink);
  font-size: 20px;
  line-height: 1.35;
}

.announcement-popup__content {
  max-height: 50vh;
  overflow-y: auto;
  color: var(--ds-ink);
  font-size: 14px;
  line-height: 1.75;
  overflow-wrap: anywhere;
}
</style>
