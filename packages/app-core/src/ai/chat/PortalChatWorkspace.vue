<script setup lang="ts">
import PortalChatComposer from "./PortalChatComposer.vue";
import PortalChatMessageList from "./PortalChatMessageList.vue";
import PortalChatSessionList from "./PortalChatSessionList.vue";
import type { PortalChatApi } from "./types";
import { usePortalChatExperience } from "./usePortalChatExperience";

const props = withDefaults(
  defineProps<{
    api: PortalChatApi;
    usageMessage?: string;
    sourceLabel?: string;
    refreshIcon?: unknown;
    createIcon?: unknown;
    copyIcon?: unknown;
    sendIcon?: unknown;
    clearIcon?: unknown;
    deleteIcon?: unknown;
    collapseOpenIcon?: unknown;
    collapseClosedIcon?: unknown;
    settingsIcon?: unknown;
    modelNoteIcon?: unknown;
    emptyIcon?: unknown;
    notifySuccess?: (message: string) => void;
    notifyWarning?: (message: string) => void;
    notifyError?: (message: string) => void;
    confirmDelete?: (name: string) => Promise<boolean>;
  }>(),
  {
    usageMessage: "消耗计入当前用量。",
    sourceLabel: "网页对话"
  }
);

const pageTitle = "AI 对话";
const {
  canSend,
  clearConversation,
  copyMessage,
  fetchModels,
  input,
  loadSession,
  loadingModels,
  loadingSessions,
  messageListRef,
  messages,
  models,
  newConversation,
  removeSession,
  selectedModel,
  selectedModelInfo,
  selectedSessionId,
  sendMessage,
  sending,
  sessions,
  stopGeneration
} = usePortalChatExperience({
  api: props.api,
  notifySuccess: props.notifySuccess,
  notifyWarning: props.notifyWarning,
  notifyError: props.notifyError,
  confirmDelete: props.confirmDelete
});
</script>

<template>
  <div class="chat-page">
    <header class="chat-header">
      <div class="title-block">
        <p class="eyebrow">在线体验</p>
        <h1>{{ pageTitle }}</h1>
      </div>
      <div class="header-actions">
        <el-button :icon="refreshIcon" :loading="loadingModels" @click="fetchModels">
          刷新
        </el-button>
        <el-button :icon="createIcon" type="primary" @click="newConversation">新对话</el-button>
      </div>
    </header>

    <section class="chat-shell">
      <aside class="control-rail">
        <PortalChatSessionList
          :sessions="sessions"
          :loading="loadingSessions"
          :selected-session-id="selectedSessionId"
          :create-icon="createIcon"
          :delete-icon="deleteIcon"
          @new-session="newConversation"
          @select-session="loadSession"
          @remove-session="removeSession"
        />
      </aside>

      <main class="conversation">
        <PortalChatMessageList
          ref="messageListRef"
          :messages="messages"
          :empty-icon="emptyIcon"
          :copy-icon="copyIcon"
          @copy-message="copyMessage"
        />
        <PortalChatComposer
          v-model="input"
          :sending="sending"
          :can-send="canSend"
          :clear-icon="clearIcon"
          :send-icon="sendIcon"
          :has-active-session="Boolean(selectedSessionId)"
          :models="models"
          :loading-models="loadingModels"
          :selected-model="selectedModel"
          :selected-model-info="selectedModelInfo"
          :source-label="sourceLabel"
          @send="sendMessage"
          @stop="stopGeneration"
          @clear="clearConversation"
          @update:selected-model="selectedModel = $event"
        />
      </main>
    </section>
  </div>
</template>

<style scoped>
.chat-page {
  /* 壳层 DsAppShell 的 canvas 用 min-height（非定高），满高页必须自身定高，
     否则内容超视口时 flex:1 无法约束、会退化成整页滚动。
     56 = 顶栏高度，76 = 内容区上下内边距，均取自 DsAppShell 布局常量。 */
  height: calc(100dvh - 56px - 76px);
  min-height: 420px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  color: var(--ds-ink);
  overflow: hidden;
}

.chat-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 22px;
  background: var(--ds-panel);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
}

.title-block {
  min-width: 0;
}

.eyebrow {
  margin: 0 0 2px;
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.title-block h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 750;
  color: var(--ds-ink);
  letter-spacing: 0;
}

.header-actions {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.chat-shell {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: minmax(220px, 280px) minmax(0, 1fr);
  gap: 14px;
}

.control-rail {
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.conversation {
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--ds-panel);
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
}

@media (max-width: 1200px) {
  .chat-shell {
    grid-template-columns: 1fr;
  }

  .control-rail {
    max-height: 240px;
  }
}

@media (max-width: 960px) {
  .chat-page {
    height: auto;
    min-height: 0;
  }
}
</style>
