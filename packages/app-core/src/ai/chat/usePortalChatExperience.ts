import { computed, onMounted, onUnmounted, shallowRef, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import type {
  PortalChatApi,
  PortalChatMessageRecord,
  PortalChatModelRecord,
  PortalChatSessionRecord,
  PortalChatUiMessage
} from "./types";

interface PortalChatMessageListHandle {
  scrollToBottom: () => Promise<void>;
  forceScrollToBottom: () => Promise<void>;
}

export interface PortalChatExperienceOptions {
  api: PortalChatApi;
  notifySuccess?: (message: string) => void;
  notifyWarning?: (message: string) => void;
  notifyError?: (message: string) => void;
  confirmDelete?: (name: string) => Promise<boolean>;
}

export function usePortalChatExperience(options: PortalChatExperienceOptions) {
  const route = useRoute();
  const router = useRouter();
  const loadingModels = shallowRef(false);
  const loadingSessions = shallowRef(false);
  const sending = shallowRef(false);
  const models = shallowRef<PortalChatModelRecord[]>([]);
  const sessions = shallowRef<PortalChatSessionRecord[]>([]);
  const selectedSessionId = shallowRef("");
  const selectedModel = shallowRef("");
  const input = shallowRef("");
  const messages = shallowRef<PortalChatUiMessage[]>([]);
  const messageListRef = shallowRef<PortalChatMessageListHandle | null>(null);
  let abortController: AbortController | null = null;
  let incompleteSessionRefreshTimer: number | undefined;

  const selectedModelInfo = computed(() =>
    models.value.find((model) => modelOptionKey(model) === selectedModel.value) || null
  );
  const requestMessages = computed(() =>
    messages.value
      .filter((message) => message.role === "user" || message.role === "assistant")
      .filter((message) => message.content.trim())
      .map((message) => ({ role: message.role, content: message.content }))
  );
  const canSend = computed(() =>
    !sending.value && Boolean(input.value.trim()) && Boolean(selectedModelInfo.value)
  );

  function routeSessionId() {
    return typeof route.query.session === "string" ? route.query.session : "";
  }

  async function fetchModels() {
    loadingModels.value = true;
    try {
      models.value = await options.api.listModels();
      if (!selectedModel.value && models.value.length > 0) {
        selectedModel.value = modelOptionKey(models.value[0]);
      }
    } finally {
      loadingModels.value = false;
    }
  }

  async function fetchSessions() {
    loadingSessions.value = true;
    try {
      sessions.value = await options.api.listSessions();
    } finally {
      loadingSessions.value = false;
    }
  }

  async function createSession(): Promise<PortalChatSessionRecord | null> {
    const model = selectedModelInfo.value;
    if (!model) {
      options.notifyWarning?.("请先选择模型");
      return null;
    }
    const session = await options.api.createSession({
      model_code: model.model_code,
      group_id: model.group_id,
      title: "新对话"
    });
    sessions.value = [session, ...sessions.value.filter((item) => item.id !== session.id)];
    selectedSessionId.value = session.id;
    return session;
  }

  async function loadSession(sessionId: string) {
    if (!sessionId || sending.value) return;
    const detail = await options.api.getSession(sessionId);
    selectedSessionId.value = detail.session.id;
    selectedModel.value = sessionModelKey(detail.session) || selectedModel.value;
    messages.value = (detail.messages || []).map(normalizeMessage);
    scheduleIncompleteSessionRefresh(sessionId);
    await forceScrollToBottom();
    if (routeSessionId() !== sessionId) {
      void router.replace({ path: route.path, query: { session: sessionId } });
    }
  }

  function newConversation() {
    clearIncompleteSessionRefresh();
    abortController?.abort();
    abortController = null;
    sending.value = false;
    selectedSessionId.value = "";
    input.value = "";
    messages.value = [];
    if (route.query.session) void router.replace({ path: route.path });
  }

  async function removeSession(session: PortalChatSessionRecord) {
    const confirmed = options.confirmDelete
      ? await options.confirmDelete(session.title || "新对话")
      : window.confirm(`确定删除「${session.title || "新对话"}」吗？`);
    if (!confirmed) return;
    await options.api.deleteSession(session.id);
    sessions.value = sessions.value.filter((item) => item.id !== session.id);
    if (selectedSessionId.value !== session.id) return;
    clearIncompleteSessionRefresh();
    selectedSessionId.value = "";
    messages.value = [];
    if (routeSessionId() === session.id) void router.replace({ path: route.path });
  }

  function stopGeneration() {
    abortController?.abort();
  }

  function clearConversation() {
    messages.value = [];
  }

  async function copyMessage(message: PortalChatUiMessage) {
    if (!message.content.trim()) {
      options.notifyWarning?.("没有可复制的消息内容");
      return;
    }
    try {
      await navigator.clipboard.writeText(message.content);
      options.notifySuccess?.(message.role === "assistant" ? "AI 回复已复制到剪贴板" : "消息已复制到剪贴板");
    } catch {
      options.notifyWarning?.("复制失败，请手动复制");
    }
  }

  async function sendMessage() {
    if (!canSend.value) return;
    let sessionId = selectedSessionId.value;
    if (!sessionId) {
      const session = await createSession();
      if (!session) return;
      sessionId = session.id;
    }

    const content = input.value.trim();
    input.value = "";
    appendMessage({ id: newMessageId(), role: "user", content });
    appendMessage({ id: newMessageId(), role: "assistant", content: "", streamStatus: "streaming" });
    await forceScrollToBottom();

    abortController = new AbortController();
    sending.value = true;
    try {
      await options.api.streamMessage({
        sessionId,
        model: selectedModelInfo.value?.model_code,
        messages: requestMessages.value,
        signal: abortController.signal,
        onDelta: (delta) => {
          updateLastAssistant(delta);
          void scrollToBottom();
        }
      });
      await fetchSessions();
    } catch (error) {
      const err = error as Error;
      if (err.name !== "AbortError") {
        updateLastAssistant(`\n\n请求失败：${err.message}`);
        options.notifyError?.(err.message || "AI 对话请求失败");
      }
    } finally {
      sending.value = false;
      abortController = null;
      await forceScrollToBottom();
    }
  }

  function scheduleIncompleteSessionRefresh(sessionId: string) {
    clearIncompleteSessionRefresh();
    const lastMessage = messages.value[messages.value.length - 1];
    if (
      selectedSessionId.value !== sessionId ||
      !lastMessage ||
      lastMessage.role !== "assistant" ||
      lastMessage.streamStatus !== "streaming"
    ) return;

    incompleteSessionRefreshTimer = window.setTimeout(async () => {
      incompleteSessionRefreshTimer = undefined;
      if (selectedSessionId.value !== sessionId || sending.value) return;
      try {
        const detail = await options.api.getSession(sessionId);
        if (selectedSessionId.value !== sessionId) return;
        messages.value = (detail.messages || []).map(normalizeMessage);
        await forceScrollToBottom();
      } catch {
        // Retry until the persisted stream state is no longer incomplete.
      }
      scheduleIncompleteSessionRefresh(sessionId);
    }, 750);
  }

  function clearIncompleteSessionRefresh() {
    if (incompleteSessionRefreshTimer === undefined) return;
    window.clearTimeout(incompleteSessionRefreshTimer);
    incompleteSessionRefreshTimer = undefined;
  }

  function appendMessage(message: PortalChatUiMessage) {
    messages.value = [...messages.value, message];
  }

  function updateLastAssistant(delta: string) {
    const next = [...messages.value];
    const last = next[next.length - 1];
    if (!last || last.role !== "assistant") return;
    next[next.length - 1] = { ...last, content: last.content + delta };
    messages.value = next;
  }

  async function scrollToBottom() {
    await messageListRef.value?.scrollToBottom();
  }

  async function forceScrollToBottom() {
    await messageListRef.value?.forceScrollToBottom();
  }

  watch(
    () => route.query.session,
    async (sessionId) => {
      if (typeof sessionId !== "string" || !sessionId || loadingSessions.value) return;
      if (sessionId === selectedSessionId.value) return;
      await loadSession(sessionId);
    }
  );

  onMounted(async () => {
    await Promise.all([fetchModels(), fetchSessions()]);
    const initialSessionId = routeSessionId();
    if (initialSessionId) {
      await loadSession(initialSessionId);
    } else if (sessions.value.length > 0) {
      await loadSession(sessions.value[0].id);
    }
  });

  onUnmounted(() => {
    clearIncompleteSessionRefresh();
    abortController?.abort();
  });

  return {
    canSend,
    copyMessage,
    clearConversation,
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
  };
}

function newMessageId() {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return `message-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function normalizeMessage(message: Partial<PortalChatMessageRecord>): PortalChatUiMessage {
  const streamStatus = message.error?.stream_status;
  return {
    id: message.id || newMessageId(),
    role: message.role || "",
    content: message.content || "",
    protocol: message.protocol || "",
    streamStatus: typeof streamStatus === "string" ? streamStatus : ""
  };
}

function modelOptionKey(model: Pick<PortalChatModelRecord, "group_id" | "model_code">) {
  return `${model.group_id || ""}::${model.model_code}`;
}

function sessionModelKey(session: Pick<PortalChatSessionRecord, "group_id" | "model_code">) {
  if (!session.group_id || !session.model_code) return "";
  return `${session.group_id}::${session.model_code}`;
}
