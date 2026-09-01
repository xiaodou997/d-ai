<script setup lang="ts">
import { reactive, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { CircleDollarSign, Gauge, Save, ShieldCheck, Sparkles, Zap } from "lucide-vue-next";
import { DsButton } from "@/shared/ui";
import { HttpProblem, PortalContentCard } from "@/platform";
import { aiTenantApi } from "@/api/aiTenant";
import type { OperationBody } from "@/api";
import type { TenantAiVisibleGroup } from "@/api/types/aiTenant";

type RoutePolicy = OperationBody<"ai-update-group-route-policy">["route_policy"];

const props = defineProps<{ group: TenantAiVisibleGroup }>();
const emit = defineEmits<{ saved: []; reload: [] }>();
const saving = shallowRef(false);

const policyOptions: Array<{
  value: RoutePolicy;
  label: string;
  description: string;
  detail: string;
  icon: typeof Sparkles;
  tone: string;
}> = [
  {
    value: "balanced",
    label: "智能均衡",
    description: "综合成本、速度、负载和稳定性自动选择。",
    detail: "推荐大多数分组使用",
    icon: Sparkles,
    tone: "mint"
  },
  {
    value: "cost",
    label: "成本优先",
    description: "在健康可用的前提下优先选择成本较低的上游。",
    detail: "适合预算敏感或批量任务",
    icon: CircleDollarSign,
    tone: "green"
  },
  {
    value: "latency",
    label: "速度优先",
    description: "优先选择近期响应更快的上游。",
    detail: "适合对响应速度敏感的请求",
    icon: Zap,
    tone: "orange"
  },
  {
    value: "stability",
    label: "稳定优先",
    description: "优先选择成功率和健康状态更好的上游。",
    detail: "适合关键业务和连续对话",
    icon: ShieldCheck,
    tone: "blue"
  }
];

const form = reactive<{ policy: RoutePolicy }>({
  policy: (props.group.route_policy || "balanced") as RoutePolicy
});

const currentPolicy = () => (props.group.route_policy || "balanced") as RoutePolicy;

watch(() => [props.group.id, props.group.route_policy], () => {
  form.policy = currentPolicy();
});

async function save() {
  if (form.policy === currentPolicy()) return;
  saving.value = true;
  try {
    await aiTenantApi.updateGroupRoutePolicy(props.group.id, {
      route_policy: form.policy,
      route_policy_version: props.group.route_policy_version
    });
    ElMessage.success("路由策略已保存");
    emit("saved");
  } catch (error: unknown) {
    if (error instanceof HttpProblem && error.code === "group_route_policy_conflict") {
      ElMessage.warning("路由策略已被其他窗口修改，已重新加载最新版本");
      emit("reload");
    } else {
      ElMessage.error(error instanceof Error ? error.message : "保存路由策略失败");
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <PortalContentCard class="route-policy-panel">
    <template #header>
      <div>
        <h3>路由策略</h3>
        <p>系统会在分组关联的健康上游中自动选择，并在请求失败时自动切换。</p>
      </div>
      <DsButton variant="primary" :loading="saving" :disabled="form.policy === currentPolicy()" @click="save">
        <template #icon><Save :size="14" /></template>
        保存
      </DsButton>
    </template>

    <div class="policy-grid" role="radiogroup" aria-label="路由策略">
      <button
        v-for="item in policyOptions"
        :key="item.value"
        type="button"
        class="policy-card"
        :class="[`tone-${item.tone}`, { selected: form.policy === item.value }]"
        role="radio"
        :aria-checked="form.policy === item.value"
        @click="form.policy = item.value"
      >
        <span class="policy-icon"><component :is="item.icon" :size="19" /></span>
        <span class="policy-copy">
          <strong>{{ item.label }}</strong>
          <span>{{ item.description }}</span>
          <small>{{ item.detail }}</small>
        </span>
      </button>
    </div>

    <div class="policy-note">
      <Gauge :size="17" />
      <span>成本、响应速度、负载和健康状态由系统自动采集；租户无需为每个上游账号填写权重或优先级。</span>
    </div>
  </PortalContentCard>
</template>

<style scoped>
.route-policy-panel { display: flex; flex-direction: column; gap: 16px; }
.policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.policy-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-height: 118px;
  padding: 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  color: var(--ds-ink);
  text-align: left;
  cursor: pointer;
  transition: border-color .16s ease, box-shadow .16s ease, background .16s ease;
}
.policy-card:hover { border-color: var(--ds-accent); }
.policy-card.selected { border: 2px solid var(--ds-accent); padding: 15px; box-shadow: var(--ds-shadow-sm); background: var(--ds-panel-muted); }
.policy-icon { display: inline-flex; flex: 0 0 auto; padding: 10px; border-radius: var(--ds-radius-control); }
.tone-mint .policy-icon { color: var(--ds-accent); background: var(--ds-accent-soft); }
.tone-green .policy-icon { color: var(--ds-positive); background: var(--ds-positive-soft); }
.tone-orange .policy-icon { color: var(--ds-warning); background: var(--ds-warning-soft); }
.tone-blue .policy-icon { color: var(--ds-info); background: var(--ds-info-soft); }
.policy-copy { display: grid; gap: 6px; min-width: 0; }
.policy-copy strong { font-size: 15px; }
.policy-copy span { color: var(--ds-muted); font-size: 13px; line-height: 1.55; }
.policy-copy small { color: var(--ds-muted); font-size: 12px; }
.policy-note { display: flex; align-items: flex-start; gap: 8px; padding: 12px 14px; border: 1px solid var(--ds-accent); border-radius: var(--ds-radius-sm); background: var(--ds-accent-soft); color: var(--ds-accent-hover); font-size: 13px; line-height: 1.6; }
@media (max-width: 680px) { .policy-grid { grid-template-columns: 1fr; } }
</style>
