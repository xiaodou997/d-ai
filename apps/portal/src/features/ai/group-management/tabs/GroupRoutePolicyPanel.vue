<script setup lang="ts">
import { computed, reactive, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { Save } from "lucide-vue-next";
import { DsButton } from "@/shared/ui";
import { HttpProblem, PortalContentCard } from "@/platform";
import { aiTenantApi } from "@/api/aiTenant";
import type { OperationBody } from "@/api";
import type { TenantAiVisibleGroup } from "@/api/types/aiTenant";

const props = defineProps<{ group: TenantAiVisibleGroup }>();
const emit = defineEmits<{ saved: []; reload: [] }>();
const saving = shallowRef(false);
const form = reactive<{
  strategy: OperationBody<"ai-update-group-route-policy">["route_strategy"];
  objective: OperationBody<"ai-update-group-route-policy">["route_objective"];
}>({
  strategy: (props.group.route_strategy || "adaptive") as OperationBody<"ai-update-group-route-policy">["route_strategy"],
  objective: (props.group.route_objective || "balanced") as OperationBody<"ai-update-group-route-policy">["route_objective"]
});
const isDirty = computed(() => form.strategy !== (props.group.route_strategy || "adaptive") || form.objective !== (props.group.route_objective || "balanced"));

watch(() => [props.group.id, props.group.route_strategy, props.group.route_objective], () => {
  form.strategy = (props.group.route_strategy || "adaptive") as typeof form.strategy;
  form.objective = (props.group.route_objective || "balanced") as typeof form.objective;
});

watch(() => form.strategy, (strategy) => {
  if (strategy !== "adaptive") form.objective = "balanced";
});

async function save() {
  if (!isDirty.value) return;
  saving.value = true;
  try {
    await aiTenantApi.updateGroupRoutePolicy(props.group.id, {
      route_strategy: form.strategy,
      route_objective: form.objective,
      route_policy_version: props.group.route_policy_version
    });
    ElMessage.success("路由配置已保存");
    emit("saved");
  } catch (error: unknown) {
    if (error instanceof HttpProblem && error.code === "group_route_policy_conflict") {
      ElMessage.warning("路由配置已被其他窗口修改，已重新加载最新版本");
      emit("reload");
    } else {
      ElMessage.error(error instanceof Error ? error.message : "保存路由配置失败");
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
        <h3>分组路由配置</h3>
        <p>先按分组顺序和目标故障层级过滤，再在同一层内执行这里的选择策略。</p>
      </div>
      <DsButton variant="primary" :loading="saving" :disabled="!isDirty" @click="save"><template #icon><Save :size="14" /></template>保存</DsButton>
    </template>
    <div class="route-policy-grid">
      <label>选择策略
        <select v-model="form.strategy">
          <option value="adaptive">自适应</option>
          <option value="weighted">按权重</option>
          <option value="failover">严格故障转移</option>
        </select>
      </label>
      <label v-if="form.strategy === 'adaptive'">优化目标
        <select v-model="form.objective">
          <option value="balanced">均衡</option>
          <option value="cost">成本</option>
          <option value="latency">速度</option>
          <option value="stability">稳定性</option>
        </select>
      </label>
    </div>
    <p class="route-policy-note">目标的优先级决定主备层级；同一层内的目标可在“关联上游目标”页签调整分流权重。</p>
  </PortalContentCard>
</template>

<style scoped>
.route-policy-panel { display: flex; flex-direction: column; gap: 16px; }
.route-policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
label { display: grid; gap: 8px; color: var(--ds-ink); font-weight: 600; }
select { width: 100%; min-height: 36px; border: 1px solid var(--ds-line); border-radius: var(--ds-radius-sm); padding: 0 10px; background: var(--ds-panel); color: var(--ds-ink); }
.route-policy-note { margin: 0; color: var(--ds-muted); font-size: 13px; }
@media (max-width: 640px) { .route-policy-grid { grid-template-columns: 1fr; } }
</style>
