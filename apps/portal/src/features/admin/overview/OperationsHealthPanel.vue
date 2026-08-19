<script setup lang="ts">
import { computed } from "vue";

import type { SystemStatusDTO } from "@/api/types/ai";
import type { ProxyNode, SystemModuleStatus } from "@/api/systemModules";
import { PortalContentCard, PortalMetricGrid } from "@/platform";
import { DsEmpty, DsMetricCard, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";

import { formatNumber, statusLabel, statusTone, systemStatusLabel, systemStatusTone } from "./overviewUtils";

const props = defineProps<{
  system: SystemStatusDTO | null;
  modules: SystemModuleStatus[];
  proxyNodes: ProxyNode[];
}>();

const healthyModules = computed(() => props.modules.filter((item) => item.active && item.health !== "unhealthy").length);
const unhealthyProxyNodes = computed(() => props.proxyNodes.filter((node) => node.status !== "active" || node.healthStatus === "unhealthy").length);
const healthRecords = computed(() => props.system?.health.records ?? []);

const healthColumns: DsTableColumn[] = [
  { key: "target_id", title: "健康目标" },
  { key: "kind", title: "目标类型", width: 120 },
  { key: "state", title: "状态", width: 110 },
  { key: "consecutive_failures", title: "连续失败", width: 110, align: "right" }
];
const moduleColumns: DsTableColumn[] = [
  { key: "displayName", title: "系统模块" },
  { key: "category", title: "类型", width: 120 },
  { key: "active", title: "运行状态", width: 110 },
  { key: "configValidated", title: "配置校验", width: 110 }
];
const proxyColumns: DsTableColumn[] = [
  { key: "name", title: "代理节点" },
  { key: "endpoint", title: "出口地址" },
  { key: "status", title: "启用状态", width: 110 },
  { key: "healthStatus", title: "健康状态", width: 110 }
];

function healthKindLabel(value: string) {
  if (value === "account") return "上游账号 / 端点";
  if (value === "pool") return "OAuth 账号池";
  return value || "未知目标";
}

function moduleTone(row: { active: boolean; health: string }) {
  if (!row.active) return "neutral";
  return statusTone(row.health);
}

function proxyTone(row: { status: string; healthStatus: string }) {
  if (row.status !== "active") return "neutral";
  return statusTone(row.healthStatus);
}
</script>

<template>
  <div class="operations-health-panel">
    <PortalMetricGrid>
      <DsMetricCard label="系统总状态" :value="systemStatusLabel(system)" hint="PostgreSQL、Redis 与路由健康" />
      <DsMetricCard label="路由健康目标" :value="`${formatNumber(system?.health.total_tracked)} 个`" :hint="`${system?.health.open_count ?? 0} 个异常，${system?.health.half_open_count ?? 0} 个观察中`" />
      <DsMetricCard label="模块健康" :value="`${healthyModules}/${modules.length}`" hint="已启用且通过健康校验" />
      <DsMetricCard label="代理节点异常" :value="`${unhealthyProxyNodes} 个`" hint="停用或探测失败的节点" />
    </PortalMetricGrid>

    <PortalContentCard title="基础设施状态" description="基础设施不可用时，优先处理连接和运行环境问题。">
      <div class="infra-grid">
        <div class="infra-item">
          <span class="infra-item__label">系统总状态</span>
          <DsTag :tone="systemStatusTone(system)">{{ systemStatusLabel(system) }}</DsTag>
        </div>
        <div class="infra-item">
          <span class="infra-item__label">PostgreSQL</span>
          <DsTag :tone="statusTone(system?.db.status)">{{ statusLabel(system?.db.status) }}</DsTag>
        </div>
        <div class="infra-item">
          <span class="infra-item__label">Redis</span>
          <DsTag :tone="statusTone(system?.redis.status)">{{ statusLabel(system?.redis.status) }}</DsTag>
        </div>
      </div>
    </PortalContentCard>

    <div class="health-grid">
      <PortalContentCard title="路由目标健康" description="当前后端健康跟踪器记录的上游账号与 OAuth 账号池，直接反映端点/模型路由可用性。">
        <DsTable v-if="healthRecords.length" :columns="healthColumns" :rows="healthRecords" row-key="target_id" :frame="false">
          <template #cell-kind="{ row }">{{ healthKindLabel(row.kind) }}</template>
          <template #cell-state="{ row }"><DsTag :tone="statusTone(row.state)">{{ statusLabel(row.state) }}</DsTag></template>
          <template #cell-consecutive_failures="{ row }">{{ formatNumber(row.consecutive_failures) }}</template>
        </DsTable>
        <DsEmpty v-else title="暂无路由健康记录" description="请求产生路由目标后，健康跟踪器会在这里显示账号和账号池状态。" />
      </PortalContentCard>

      <PortalContentCard title="系统模块健康" description="敏感信息保护、统一通知和代理出口等可插拔模块的运行状态。">
        <DsTable v-if="modules.length" :columns="moduleColumns" :rows="modules" row-key="name" :frame="false">
          <template #cell-active="{ row }"><DsTag :tone="moduleTone(row)">{{ row.active ? '运行中' : '未运行' }}</DsTag></template>
          <template #cell-configValidated="{ row }"><DsTag :tone="row.configValidated ? 'positive' : 'warning'">{{ row.configValidated ? '已校验' : '待检查' }}</DsTag></template>
        </DsTable>
        <DsEmpty v-else title="暂无系统模块" description="系统模块状态暂不可用。" />
      </PortalContentCard>
    </div>

    <PortalContentCard title="代理出口健康" description="代理节点既是出口能力，也是上游访问链路的一部分。">
      <DsTable v-if="proxyNodes.length" :columns="proxyColumns" :rows="proxyNodes" row-key="id" :frame="false">
        <template #cell-status="{ row }"><DsTag :tone="row.status === 'active' ? 'positive' : 'neutral'">{{ row.status === 'active' ? '已启用' : '已停用' }}</DsTag></template>
        <template #cell-healthStatus="{ row }"><DsTag :tone="proxyTone(row)">{{ statusLabel(row.healthStatus) }}</DsTag></template>
      </DsTable>
      <DsEmpty v-else title="暂无代理出口节点" description="配置代理节点后，这里会展示节点健康状态。" />
    </PortalContentCard>
  </div>
</template>

<style scoped>
.operations-health-panel { display: flex; flex-direction: column; gap: 20px; }
.health-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
.infra-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.infra-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 52px; padding: 12px 14px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-control); background: var(--ds-panel-muted); }
.infra-item__label { color: var(--ds-ink-soft); font-size: 13px; font-weight: 650; }
@media (max-width: 980px) { .health-grid { grid-template-columns: 1fr; } }
@media (max-width: 680px) { .infra-grid { grid-template-columns: 1fr; } }
</style>
