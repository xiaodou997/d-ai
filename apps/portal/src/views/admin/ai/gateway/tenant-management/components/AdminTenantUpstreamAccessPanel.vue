<!--
  租户上游权限面板 — 嵌在 AccessView 一体面板右栏 Tab 内。
  重构:el-table/el-tag/el-empty 换为 DsTable/DsTag/DsEmpty;倍率编辑使用 DsNumberInput。
-->
<script setup lang="ts">
import { computed } from "vue";
import { RefreshLeft } from "@element-plus/icons-vue";
import { DsEmpty, DsNumberInput, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { formatMultiplier } from "@/platform/ai/utils";
import type { TenantListItem } from "@/api/types/admin";
import type { TenantUpstreamAccessDTO } from "@/api/types/ai";
import type { AdminTenantUpstreamPolicyDraft } from "../types";

const props = defineProps<{
  selectedTenant: TenantListItem | null;
  loading: boolean;
  resources: TenantUpstreamAccessDTO[];
  policies: Record<string, AdminTenantUpstreamPolicyDraft>;
}>();

defineEmits<{
  (e: "toggle", resource: TenantUpstreamAccessDTO, granted: boolean): void;
  (e: "update-multiplier", resource: TenantUpstreamAccessDTO, value: number | null): void;
}>();

const columns: DsTableColumn[] = [
  { key: "resource", title: "平台资源", width: 220 },
  { key: "kind", title: "类型", width: 90 },
  { key: "accessMode", title: "可见范围", width: 100 },
  { key: "status", title: "资源状态", width: 100 },
  { key: "allowed", title: "允许访问", width: 100, align: "center" },
  { key: "defaultMultiplier", title: "默认倍率", width: 100, align: "right" },
  { key: "tenantMultiplier", title: "租户倍率", width: 210 },
  { key: "effective", title: "生效倍率", width: 100, align: "right" }
];

function resourceKey(resource: TenantUpstreamAccessDTO) {
  return `${resource.resource_kind}:${resource.resource_id}`;
}

// DsTable 需要单一 rowKey,资源主键是 kind:id 组合,展开成行内字段
const tableRows = computed(() =>
  props.resources.map((resource) => ({ ...resource, _key: resourceKey(resource) }))
);

function kindLabel(kind: TenantUpstreamAccessDTO["resource_kind"]) {
  return kind === "oauth_pool" ? "账号池" : "上游账号";
}

function policyFor(resource: TenantUpstreamAccessDTO) {
  return props.policies[resourceKey(resource)] || {
    access_granted: false,
    tenant_multiplier_override: null
  };
}

function effectiveMultiplier(resource: TenantUpstreamAccessDTO) {
  return policyFor(resource).tenant_multiplier_override
    ?? resource.default_tenant_multiplier;
}

function multiplierDisabled(resource: TenantUpstreamAccessDTO) {
  return resource.access_mode === "restricted" && !policyFor(resource).access_granted;
}
</script>

<template>
  <section v-loading="loading" class="upstream-access-panel">
    <template v-if="selectedTenant">
      <el-alert
        :closable="false"
        type="info"
        title="公开资源固定允许访问；专属资源需要授权。租户倍率留空时继承账号或账号池的默认倍率。"
      />

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="tableRows"
        row-key="_key"
        class="resource-table"
      >
        <template #cell-resource="{ row }">
          <strong>{{ row.internal_name }}</strong>
          <div class="display-name">租户看到：{{ row.tenant_display_name }}</div>
        </template>
        <template #cell-kind="{ row }">{{ kindLabel(row.resource_kind) }}</template>
        <template #cell-accessMode="{ row }">
          <DsTag :tone="row.access_mode === 'public' ? 'positive' : 'warning'">
            {{ row.access_mode === "public" ? "公开" : "专属" }}
          </DsTag>
        </template>
        <template #cell-status="{ row }">
          <DsTag :tone="row.status === 'active' ? 'positive' : 'info'">{{ row.status }}</DsTag>
        </template>
        <template #cell-allowed="{ row }">
          <el-switch
            :model-value="row.access_mode === 'public' || policyFor(row).access_granted"
            :disabled="row.access_mode === 'public'"
            @change="$emit('toggle', row, Boolean($event))"
          />
        </template>
        <template #cell-defaultMultiplier="{ row }">{{ formatMultiplier(row.default_tenant_multiplier) }}</template>
        <template #cell-tenantMultiplier="{ row }">
          <div class="multiplier-editor">
            <DsNumberInput
              :model-value="policyFor(row).tenant_multiplier_override"
              :min="0"
              :precision="4"
              :step="0.1"
              allow-empty
              :disabled="multiplierDisabled(row)"
              placeholder="继承默认"
              size="sm"
              class="multiplier-input"
              @update:model-value="$emit('update-multiplier', row, $event ?? null)"
            />
            <el-tooltip content="恢复继承默认倍率" placement="top">
              <el-button
                text
                :icon="RefreshLeft"
                :disabled="multiplierDisabled(row) || policyFor(row).tenant_multiplier_override === null"
                aria-label="恢复继承默认倍率"
                @click="$emit('update-multiplier', row, null)"
              />
            </el-tooltip>
          </div>
        </template>
        <template #cell-effective="{ row }">
          <strong>{{ formatMultiplier(effectiveMultiplier(row)) }}</strong>
        </template>
      </DsTable>
    </template>

    <DsEmpty v-else title="从左侧选择一个租户进行策略配置" />
  </section>
</template>

<style scoped>
.upstream-access-panel { min-height: 260px; }
.resource-table { margin-top: 16px; }
.display-name { margin-top: 4px; color: var(--ds-muted); font-size: 12px; }
.multiplier-editor { display: flex; align-items: center; gap: 4px; }
.multiplier-input { width: 132px; }
</style>
