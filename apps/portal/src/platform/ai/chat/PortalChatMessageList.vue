<script setup lang="ts">
import { nextTick, shallowRef, useTemplateRef } from "vue";
import { ArrowDown } from "@element-plus/icons-vue";

import PortalMarkdownRenderer from "./PortalMarkdownRenderer.vue";
import type { PortalChatUiMessage } from "./types";

const props = defineProps<{
  messages?: PortalChatUiMessage[];
  emptyIcon?: unknown;
  copyIcon?: unknown;
}>();

const emit = defineEmits<{
  copyMessage: [message: PortalChatUiMessage];
}>();

const listRef = useTemplateRef<HTMLElement>("listRef");

const autoScroll = shallowRef(true);
const showScrollButton = shallowRef(false);
let scrollTimer: number | undefined;

const SCROLL_LOCK_THRESHOLD = 80;
const SCROLL_THROTTLE_MS = 80;

function onScroll() {
  const el = listRef.value;
  if (!el) return;
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
  if (distanceFromBottom < SCROLL_LOCK_THRESHOLD) {
    autoScroll.value = true;
    showScrollButton.value = false;
  } else {
    autoScroll.value = false;
    showScrollButton.value = true;
  }
}

/**
 * 节流滚动到底部 —— 用于流式回复期间。
 * 遵循 autoScroll 锁定：用户上滑时不抢夺滚动。
 */
async function scrollToBottom() {
  if (scrollTimer !== undefined) return;
  scrollTimer = window.setTimeout(() => {
    scrollTimer = undefined;
    void doScrollToBottom();
  }, SCROLL_THROTTLE_MS);
}

async function doScrollToBottom() {
  if (!autoScroll.value) return;
  await nextTick();
  const el = listRef.value;
  if (el) {
    el.scrollTop = el.scrollHeight;
  }
}

/**
 * 强制滚动到底部 —— 用于加载会话、发送消息等需要立即滚动的场景。
 * 忽略 autoScroll 锁定、不做节流。
 */
async function forceScrollToBottom() {
  autoScroll.value = true;
  showScrollButton.value = false;
  if (scrollTimer !== undefined) {
    window.clearTimeout(scrollTimer);
    scrollTimer = undefined;
  }
  await nextTick();
  const el = listRef.value;
  if (el) {
    el.scrollTop = el.scrollHeight;
  }
}

async function jumpToBottom() {
  autoScroll.value = true;
  showScrollButton.value = false;
  await nextTick();
  const el = listRef.value;
  if (el) {
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  }
}

defineExpose({ scrollToBottom, forceScrollToBottom });
</script>

<template>
  <div class="message-list-wrap">
    <div ref="listRef" class="message-list" @scroll="onScroll">
      <div v-if="!messages || messages.length === 0" class="empty-state">
        <el-icon v-if="emptyIcon" :size="42"><component :is="emptyIcon" /></el-icon>
        <h2>开始一次自动协议对话</h2>
        <p>选择已授权模型后发送消息，后台会自动匹配最佳协议。</p>
      </div>

      <article v-for="message in messages" :key="message.id" class="message-row" :class="message.role">
        <div class="message-avatar">{{ message.role === "user" ? "我" : "AI" }}</div>
        <div class="message-bubble">
          <div
            v-if="message.content"
            class="message-toolbar"
            :class="{ 'message-toolbar--meta': message.role === 'assistant' && message.protocol }"
          >
            <div v-if="message.role === 'assistant' && message.protocol" class="message-meta">{{ message.protocol }}</div>
            <el-tooltip content="复制消息" placement="top">
              <el-button
                v-if="props.copyIcon"
                link
                class="copy-message-button"
                :icon="props.copyIcon"
                @click.stop="emit('copyMessage', message)"
              />
              <el-button v-else link class="copy-message-button copy-message-button--text" @click.stop="emit('copyMessage', message)">
                复制
              </el-button>
            </el-tooltip>
          </div>
          <p v-if="message.role !== 'assistant' && message.content" class="message-plain">{{ message.content }}</p>
          <PortalMarkdownRenderer v-else-if="message.content" :source="message.content" />
          <span v-else class="typing-dot">生成中...</span>
        </div>
      </article>
    </div>

    <transition name="fade-slide">
      <button
        v-if="showScrollButton"
        class="scroll-bottom-btn"
        type="button"
        aria-label="滚动到底部"
        @click="jumpToBottom"
      >
        <el-icon><ArrowDown /></el-icon>
      </button>
    </transition>
  </div>
</template>

<style scoped>
.message-list-wrap {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.message-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px;
  background: var(--ds-panel-muted);
}

.empty-state {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--ds-faint);
  text-align: center;
}

.empty-state h2 {
  margin: 14px 0 4px;
  color: var(--ds-ink-soft);
  font-size: 18px;
  font-weight: 900;
}

.empty-state p {
  margin: 0;
  font-size: 13px;
}

.message-row {
  display: flex;
  gap: 12px;
  margin-bottom: 18px;
  max-width: 860px;
  margin-left: auto;
  margin-right: auto;
}

.message-row.user {
  flex-direction: row-reverse;
}

.message-avatar {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--ds-white);
  font-size: 12px;
  font-weight: 900;
  border-radius: var(--ds-radius-panel);
  background: var(--ds-accent);
}

.message-row.assistant .message-avatar {
  background: var(--ds-positive);
}

.message-bubble {
  position: relative;
  max-width: min(760px, 76%);
  padding: 12px 14px;
  color: var(--ds-ink);
  background: var(--ds-white);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-shell);
  box-shadow: var(--ds-shadow-sm);
}

.message-toolbar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.message-toolbar--meta {
  justify-content: space-between;
}

.copy-message-button {
  min-height: auto;
  padding: 2px;
  color: inherit;
}

.copy-message-button--text {
  padding: 0;
  font-size: 12px;
}

.message-row.user .message-bubble {
  color: var(--ds-white);
  background: var(--ds-accent);
  border-color: var(--ds-accent);
}

.message-plain {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.7;
  font-size: 14px;
}

.message-meta {
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 800;
}

.typing-dot {
  color: var(--ds-muted);
  font-size: 13px;
  font-weight: 700;
}

/* —— 回到底部按钮 —— */
.scroll-bottom-btn {
  position: absolute;
  bottom: 12px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 10;
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-circle);
  background: var(--ds-panel);
  color: var(--ds-ink-soft);
  box-shadow: var(--ds-shadow-sm);
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease, color 0.15s ease;
}

.scroll-bottom-btn:hover {
  transform: translateX(-50%) translateY(-2px);
  color: var(--ds-accent);
  box-shadow: var(--ds-shadow-panel);
}

.scroll-bottom-btn .el-icon {
  font-size: 16px;
}

.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.fade-slide-enter-from,
.fade-slide-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(8px);
}
</style>
