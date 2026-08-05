<script setup lang="ts">
import { PanelTopClose, PanelTopOpen, Pencil, Plus, Power, Trash2 } from "lucide-vue-next";

import type { ServiceRegistryDetail, ServiceSourceItem } from "../../../types/admin";
import ServiceInstancesPanel from "./ServiceInstancesPanel.vue";

defineProps<{
  modelValue: boolean;
  service: ServiceRegistryDetail | null;
  loading: boolean;
  pendingAction?: "status" | "portal" | "delete" | null;
  portalModuleLabel?: string;
}>();
const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  edit: [];
  "toggle-status": [];
  "toggle-portal": [];
  delete: [];
  "add-source": [];
  "edit-source": [source: ServiceSourceItem];
  "delete-source": [source: ServiceSourceItem];
}>();

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "从未连接";
}

function portalStatus(service: ServiceRegistryDetail) {
  if (!service.portalEnabled) return "未开放";
  return service.status === "active" ? "已开放" : "已开放（服务停用期间隐藏）";
}
</script>

<template>
  <el-drawer :model-value="modelValue" size="min(760px, 96vw)" append-to-body @update:model-value="emit('update:modelValue', $event)">
    <template #header>
      <div v-if="service" class="drawer-title">
        <div><strong>{{ service.displayName }}</strong><code>{{ service.serviceId }}</code></div>
        <span :class="['drawer-title__status', `drawer-title__status--${service.status}`]">{{ service.status === "active" ? "已启用" : "已停用" }}</span>
      </div>
    </template>

    <div v-loading="loading" class="service-detail">
      <template v-if="service">
        <div class="detail-actions">
          <el-button :icon="Pencil" :disabled="Boolean(pendingAction)" @click="emit('edit')">编辑</el-button>
          <el-button
            :type="service.portalEnabled ? 'warning' : 'primary'"
            plain
            :icon="service.portalEnabled ? PanelTopClose : PanelTopOpen"
            :loading="pendingAction === 'portal'"
            :disabled="Boolean(pendingAction)"
            @click="emit('toggle-portal')"
          >
            {{ service.portalEnabled ? "关闭门户入口" : "开启门户入口" }}
          </el-button>
          <el-button
            :type="service.status === 'active' ? 'danger' : 'success'"
            plain
            :icon="Power"
            :loading="pendingAction === 'status'"
            :disabled="Boolean(pendingAction)"
            @click="emit('toggle-status')"
          >
            {{ service.status === "active" ? "停用注册续签" : "启用注册续签" }}
          </el-button>
          <el-button
            type="danger"
            plain
            :icon="Trash2"
            :loading="pendingAction === 'delete'"
            :disabled="Boolean(pendingAction)"
            @click="emit('delete')"
          >删除</el-button>
        </div>

        <dl class="detail-facts">
          <div><dt>在线实例</dt><dd>{{ service.onlineInstances }} 个</dd></div>
          <div><dt>最后活动</dt><dd>{{ formatTime(service.lastSeen) }}</dd></div>
          <div>
            <dt>门户入口</dt>
            <dd>
              <span>{{ portalStatus(service) }}</span>
              <small :class="{ 'module-support--enabled': portalModuleLabel }">
                {{ portalModuleLabel ? `前端已接入 · ${portalModuleLabel}` : "前端未接入" }}
              </small>
            </dd>
          </div>
          <div><dt>说明</dt><dd>{{ service.description || "未填写" }}</dd></div>
        </dl>

        <section class="detail-section">
          <header><div><h3>外部来源</h3><p>异机实例必须从下列 CIDR 发起注册与调用。</p></div><el-button :icon="Plus" @click="emit('add-source')">添加</el-button></header>
          <div v-if="service.sources.length === 0" class="empty-line">未配置外部来源，仅允许内网自动注册。</div>
          <div v-for="source in service.sources" v-else :key="source.id" class="source-row">
            <div><code>{{ source.sourceCidr }}</code><span>{{ source.description || "无备注" }}</span></div>
            <div><el-button text :icon="Pencil" title="编辑来源" @click="emit('edit-source', source)" /><el-button text type="danger" :icon="Trash2" title="删除来源" @click="emit('delete-source', source)" /></div>
          </div>
        </section>

        <section class="detail-section">
          <header><div><h3>在线实例</h3><p>最近 6 分钟有续签活动</p></div></header>
          <ServiceInstancesPanel :instances="service.instances" />
        </section>
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
.drawer-title { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.drawer-title > div { display: grid; gap: 4px; }
.drawer-title strong { font-size: 18px; }
.drawer-title code { color: var(--ds-muted); font-size: 12px; }
.drawer-title__status { padding: 4px 8px; border-radius: 4px; background: var(--ds-panel-muted); color: var(--ds-muted); font-size: 12px; }
.drawer-title__status--active { background: var(--ds-positive-soft); color: var(--ds-positive); }
.service-detail { min-height: 240px; }
.detail-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.detail-actions :deep(.el-button + .el-button) { margin-left: 0; }
.detail-facts { margin: 20px 0 28px; display: grid; grid-template-columns: 1fr 1.3fr 1.4fr 2fr; border: 1px solid var(--ds-line); border-radius: 6px; }
.detail-facts > div { padding: 14px; border-right: 1px solid var(--ds-line); }
.detail-facts > div:last-child { border-right: 0; }
.detail-facts dt { color: var(--ds-muted); font-size: 11px; }
.detail-facts dd { margin: 6px 0 0; display: grid; gap: 4px; color: var(--ds-ink-soft); font-size: 13px; }
.detail-facts small { color: var(--ds-muted); font-size: 11px; }
.detail-facts .module-support--enabled { color: var(--ds-positive); }
.detail-section { margin-top: 28px; }
.detail-section > header { margin-bottom: 10px; display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; }
.detail-section h3 { margin: 0; font-size: 15px; }
.detail-section p { margin: 4px 0 0; color: var(--ds-muted); font-size: 12px; }
.source-row { min-height: 58px; display: flex; align-items: center; justify-content: space-between; gap: 12px; border-top: 1px solid var(--ds-line); }
.source-row > div:first-child { display: grid; gap: 4px; }
.source-row span, .empty-line { color: var(--ds-muted); font-size: 12px; }
.empty-line { padding: 22px 0; border-top: 1px solid var(--ds-line); }
@media (max-width: 620px) { .detail-facts { grid-template-columns: 1fr; } .detail-facts > div { border-right: 0; border-bottom: 1px solid var(--ds-line); } }
</style>
