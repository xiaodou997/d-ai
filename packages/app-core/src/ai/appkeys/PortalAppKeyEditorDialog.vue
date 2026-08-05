<script setup lang="ts">
import { computed, reactive, watch } from "vue";

import type {
  PortalAppKeyRecord,
  PortalAppKeyWriteInput,
  PortalVisibleAgentRecord
} from "./types";

const props = withDefaults(
  defineProps<{
    visible: boolean;
    loading: boolean;
    appKey?: PortalAppKeyRecord | null;
    agents: PortalVisibleAgentRecord[];
    namePlaceholder?: string;
  }>(),
  {
    namePlaceholder: "例如：个人知识库入口"
  }
);

const emit = defineEmits<{
  close: [];
  submit: [payload: PortalAppKeyWriteInput];
}>();

const form = reactive({
  name: "",
  status: "active" as "active" | "disabled",
  agentId: "",
  expiresAtValue: ""
});

const filteredAgents = computed(() =>
  props.agents.filter((agent) => agent.capability === "chat" || agent.capability === "image_generation" || agent.capability === "image_edit")
);
const canSubmit = computed(() => Boolean(form.name.trim() && form.agentId));
const expiresShortcutButtons = [
  { label: "30 天", days: 30 },
  { label: "90 天", days: 90 },
  { label: "1 年", days: 365 }
];
const expiresPickerShortcuts = expiresShortcutButtons.map((item) => ({
  text: item.label,
  value: () => new Date(Date.now() + item.days * 24 * 60 * 60 * 1000)
}));

function agentPublisherLabel(label?: string) {
  return label || "应用";
}

function applyExpiryPreset(days: number) {
  form.expiresAtValue = String(Date.now() + days * 24 * 60 * 60 * 1000);
}

function clearExpiry() {
  form.expiresAtValue = "";
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return;
    form.name = props.appKey?.name || "";
    form.status = props.appKey?.status || "active";
    form.agentId = props.appKey?.agent_id || filteredAgents.value[0]?.id || "";
    form.expiresAtValue = props.appKey?.expires_at ? String(props.appKey.expires_at) : "";
  },
  { immediate: true }
);

watch(
  filteredAgents,
  (items) => {
    if (props.appKey) return;
    if (!items.some((item) => item.id === form.agentId)) {
      form.agentId = items[0]?.id || "";
    }
  },
  { immediate: true }
);

function handleSubmit() {
  emit("submit", {
    name: form.name.trim(),
    status: form.status,
    agent_id: form.agentId,
    expires_at: form.expiresAtValue ? Number(form.expiresAtValue) : null
  });
}
</script>

<template>
  <el-dialog :model-value="visible" :title="appKey ? '编辑应用运行密钥' : '新建应用运行密钥'" width="640px" @close="emit('close')">
    <el-form label-width="110px">
      <el-form-item label="名称">
        <el-input v-model="form.name" :placeholder="namePlaceholder" />
      </el-form-item>
      <el-form-item label="状态">
        <el-radio-group v-model="form.status">
          <el-radio value="active">启用</el-radio>
          <el-radio value="disabled">停用</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="应用">
        <el-select v-model="form.agentId" filterable placeholder="选择应用">
          <el-option
            v-for="agent in filteredAgents"
            :key="agent.id"
            :label="agent.name"
            :value="agent.id"
          >
            <div class="agent-option">
              <span>{{ agent.name }}</span>
              <small>{{ agentPublisherLabel(agent.publisher_label) }}</small>
            </div>
          </el-option>
          <template #empty>
            <div class="picker-empty">当前没有可绑定的应用</div>
          </template>
        </el-select>
      </el-form-item>
      <el-form-item label="过期时间">
        <div class="expiry-picker">
          <el-date-picker
            v-model="form.expiresAtValue"
            class="w-full"
            type="datetime"
            placeholder="留空表示不过期"
            value-format="x"
            format="YYYY-MM-DD HH:mm"
            :shortcuts="expiresPickerShortcuts"
          />
          <div class="expiry-shortcuts">
            <el-button
              v-for="item in expiresShortcutButtons"
              :key="item.label"
              size="small"
              plain
              @click="applyExpiryPreset(item.days)"
            >
              {{ item.label }}
            </el-button>
            <el-button size="small" text @click="clearExpiry">永不过期</el-button>
          </div>
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="emit('close')">取消</el-button>
      <el-button type="primary" :loading="loading" :disabled="!canSubmit" @click="handleSubmit">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.expiry-picker {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.agent-option {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.agent-option span {
  color: var(--ds-ink);
  font-weight: 700;
}

.agent-option small,
.picker-empty {
  color: var(--ds-muted);
}

.expiry-shortcuts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.picker-empty {
  padding: 12px;
  font-size: 13px;
  line-height: 1.5;
}
</style>
